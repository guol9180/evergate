// Package selfupdate 实现管理台一键更新：下载、校验、替换与重启编排。
//
// 下载通道优先 npm registry（平台子包 tarball + dist.integrity），失败时回退
// GitHub Releases + checksums.txt。替换引擎只做同目录改名交换，不依赖外部工具。
package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"evergate/internal/update"
)

const (
	StageDownloading = "downloading"
	StageVerifying   = "verifying"
	StageReplacing   = "replacing"
	StageRestarting  = "restarting"
	StageFailed      = "failed"

	defaultRegistry = "https://registry.npmjs.org"
	jobTimeout      = 10 * time.Minute
)

var versionPattern = regexp.MustCompile(`^v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$`)

// ErrBusy 表示已有更新任务在执行。
var ErrBusy = errors.New("已有更新任务在进行中")

// Options 是 selfupdate 装配依赖。
type Options struct {
	CurrentVersion string
	Executable     string // 为空时取 os.Executable()
	Args           []string
	WorkDir        string
	Registry       string // npm registry，可配 npmmirror
	Latest         func(context.Context) (update.Status, error)
	Client         *http.Client
	Logger         *slog.Logger
	Now            func() time.Time
	RequestRestart func()
	GOOS           string
	GOARCH         string
}

// Capability 描述当前安装是否允许自更新，用于管理台提前提示。
type Capability struct {
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
	Executable string `json:"executable,omitempty"`
	Platform   string `json:"platform"`
	Dev        bool   `json:"dev"`
}

// Job 是一次更新任务的可轮询状态。
type Job struct {
	ID            string    `json:"id"`
	Stage         string    `json:"stage"`
	Progress      int       `json:"progress"`
	Source        string    `json:"source,omitempty"`
	TargetVersion string    `json:"target_version"`
	Message       string    `json:"message,omitempty"`
	Error         string    `json:"error,omitempty"`
	FailedStage   string    `json:"failed_stage,omitempty"`
	BackupPath    string    `json:"backup_path,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	launchPath string
	launchArgs []string
	launchDir  string
}

func (j Job) clone() Job {
	out := j
	out.launchArgs = append([]string(nil), j.launchArgs...)
	return out
}

// LaunchSpec 是新进程启动参数（重启前由 main 消费）。
type LaunchSpec struct {
	Path string
	Args []string
	Dir  string
}

// Manager 串行执行更新任务；同一时刻只允许一个活动任务。
type Manager struct {
	opt Options

	mu      sync.Mutex
	job     *Job
	pending *LaunchSpec
}

// New 创建 Manager。
func New(opt Options) *Manager {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Registry == "" {
		opt.Registry = defaultRegistry
	}
	opt.Registry = strings.TrimSuffix(opt.Registry, "/")
	if opt.Client == nil {
		opt.Client = &http.Client{Timeout: downloadTimeout}
	}
	if opt.GOOS == "" {
		opt.GOOS = runtime.GOOS
	}
	if opt.GOARCH == "" {
		opt.GOARCH = runtime.GOARCH
	}
	if opt.Executable == "" {
		if exe, err := os.Executable(); err == nil {
			opt.Executable = exe
		}
	}
	if opt.WorkDir == "" {
		if wd, err := os.Getwd(); err == nil {
			opt.WorkDir = wd
		}
	}
	opt.Args = append([]string(nil), opt.Args...)
	return &Manager{opt: opt}
}

// Capability 返回当前安装的可更新能力；不可用时 Reason 给出可操作提示。
func (m *Manager) Capability() Capability {
	cap := Capability{
		Executable: m.opt.Executable,
		Platform:   m.opt.GOOS + "/" + m.opt.GOARCH,
	}
	if m.devVersion() {
		cap.Dev = true
		cap.Reason = "开发版本（dev/dirty）不支持自动更新，请使用正式 Release 版本"
		return cap
	}
	if m.opt.Executable == "" {
		cap.Reason = "无法定位当前可执行文件"
		return cap
	}
	lower := strings.ToLower(filepath.Clean(m.opt.Executable))
	if strings.Contains(lower, "go-build") || strings.Contains(lower, "\\temp\\go-build") {
		cap.Reason = "go run 临时可执行文件不支持自动更新，请使用正式安装的二进制"
		return cap
	}
	if err := dirWritable(filepath.Dir(m.opt.Executable)); err != nil {
		cap.Reason = "安装目录不可写：" + err.Error()
		return cap
	}
	if _, _, err := platformPackage(m.opt.GOOS, m.opt.GOARCH); err != nil {
		cap.Reason = err.Error()
		return cap
	}
	cap.Available = true
	return cap
}

// Start 创建并异步执行更新任务。version 为空时使用 Latest 返回的最新版本。
// source 取 npm / github / 空（先 npm 后 github 自动回退）。
func (m *Manager) Start(version, source string) (*Job, error) {
	if m.opt.RequestRestart == nil {
		return nil, errors.New("重启编排未配置")
	}
	cap := m.Capability()
	if !cap.Available {
		return nil, errors.New(cap.Reason)
	}
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "", "auto":
		source = "auto"
	case "npm", "github":
	default:
		return nil, fmt.Errorf("未知更新通道 %q", source)
	}

	target, err := m.resolveTarget(version)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if m.job != nil && m.job.Stage != StageFailed {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	job := &Job{
		ID:            fmt.Sprintf("%d", m.opt.Now().UnixNano()),
		Stage:         StageDownloading,
		Progress:      5,
		TargetVersion: target,
		Message:       "准备下载新版本…",
		StartedAt:     m.opt.Now(),
		UpdatedAt:     m.opt.Now(),
	}
	m.job = job
	out := job.clone()
	m.mu.Unlock()

	go m.run(job.ID, target, source)
	return &out, nil
}

func (m *Manager) resolveTarget(version string) (string, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		if m.opt.Latest == nil {
			return "", errors.New("无法获取最新版本")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		status, err := m.opt.Latest(ctx)
		if err != nil {
			return "", fmt.Errorf("查询最新版本失败: %w", err)
		}
		version = status.LatestVersion
	}
	match := versionPattern.FindStringSubmatch(version)
	if match == nil {
		return "", fmt.Errorf("非法版本号 %q", version)
	}
	target := match[1]
	if update.CompareVersions(m.opt.CurrentVersion, target) >= 0 {
		return "", fmt.Errorf("当前版本 %s 已不低于目标版本 %s", m.opt.CurrentVersion, target)
	}
	return target, nil
}

func (m *Manager) run(id, target, source string) {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()

	m.setStage(id, StageDownloading, 15, "正在下载新版本…")
	binary, usedSource, err := m.download(ctx, target, source)
	if err != nil {
		m.fail(id, "下载或校验失败："+err.Error())
		return
	}
	m.setStageWithSource(id, StageVerifying, 65, usedSource, "下载完成，正在校验…")

	m.setStageWithSource(id, StageReplacing, 80, usedSource, "正在替换二进制（保留旧版本备份）…")
	backup, err := replaceExecutable(m.opt.Executable, target, binary)
	if err != nil {
		m.fail(id, "替换失败："+err.Error())
		return
	}

	m.mu.Lock()
	if m.job != nil && m.job.ID == id {
		m.job.Source = usedSource
		m.job.BackupPath = backup
		m.job.Stage = StageRestarting
		m.job.Progress = 95
		m.job.Message = "新版本已就位，正在优雅重启…"
		m.job.UpdatedAt = m.opt.Now()
		m.job.launchPath = m.opt.Executable
		m.job.launchArgs = append([]string(nil), m.opt.Args...)
		m.job.launchDir = m.opt.WorkDir
	}
	m.pending = &LaunchSpec{Path: m.opt.Executable, Args: append([]string(nil), m.opt.Args...), Dir: m.opt.WorkDir}
	m.mu.Unlock()

	m.opt.Logger.Info("self-update replaced binary, restarting",
		slog.String("target", target), slog.String("source", usedSource), slog.String("backup", backup))
	time.Sleep(600 * time.Millisecond)
	m.opt.RequestRestart()
}

func (m *Manager) setStage(id, stage string, progress int, message string) {
	m.setStageWithSource(id, stage, progress, "", message)
}

func (m *Manager) setStageWithSource(id, stage string, progress int, source, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil || m.job.ID != id || m.job.Stage == StageFailed {
		return
	}
	m.job.Stage = stage
	m.job.Progress = progress
	m.job.Message = message
	if source != "" {
		m.job.Source = source
	}
	m.job.UpdatedAt = m.opt.Now()
}

func (m *Manager) fail(id, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil || m.job.ID != id {
		return
	}
	m.job.FailedStage = m.job.Stage
	m.job.Stage = StageFailed
	m.job.Error = message
	m.job.Message = "更新已中止，当前版本仍可继续使用"
	m.job.UpdatedAt = m.opt.Now()
	m.opt.Logger.Warn("self-update failed", slog.String("error", message))
}

// Snapshot 返回当前任务副本；无任务时返回 nil。
func (m *Manager) Snapshot() *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil {
		return nil
	}
	out := m.job.clone()
	return &out
}

// PendingLaunch 返回替换成功后的新进程启动参数。
func (m *Manager) PendingLaunch() (LaunchSpec, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending == nil {
		return LaunchSpec{}, false
	}
	spec := *m.pending
	spec.Args = append([]string(nil), m.pending.Args...)
	return spec, true
}

func (m *Manager) devVersion() bool {
	v := m.opt.CurrentVersion
	return v == "" || v == "dev" || strings.Contains(v, "-dirty")
}

// CurrentVersion 返回当前进程版本，供重启后的进度轮询确认目标版本是否已生效。
func (m *Manager) CurrentVersion() string { return m.opt.CurrentVersion }

// Executable 返回当前可执行文件路径（启动清理备份用）。
func (m *Manager) Executable() string { return m.opt.Executable }
