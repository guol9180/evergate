// Package selfupdate 实现标准发行包更新：Windows/macOS 下载版本化安装器，
// Linux 下载版本化 tar.gz 后安全提取并原位置换。
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
	StageInstalling  = "installing"
	StageRestarting  = "restarting"
	StageFailed      = "failed"

	jobTimeout = 10 * time.Minute
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
	Cancelable    bool      `json:"cancelable"`
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
	cancel  context.CancelFunc
}

// New 创建 Manager。
func New(opt Options) *Manager {
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
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
		cap.Reason = "开发版本（dev/dirty/git describe）不支持自动更新，请使用正式 Release 版本"
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
	if err := validateInstallation(m.opt.GOOS, m.opt.Executable); err != nil {
		cap.Reason = err.Error()
		return cap
	}
	cap.Available = true
	return cap
}

// Start 创建并异步执行更新任务。version 为空时使用 Latest 返回的最新版本。
// source 仅接受 github / auto；npm 包由 npm 独立更新。
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
	case "github":
	default:
		return nil, fmt.Errorf("标准安装包更新不支持通道 %q", source)
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
		Message:       "准备下载平台安装包…",
		Cancelable:    true,
		StartedAt:     m.opt.Now(),
		UpdatedAt:     m.opt.Now(),
	}
	m.job = job
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	m.cancel = cancel
	out := job.clone()
	m.mu.Unlock()

	go m.run(ctx, cancel, job.ID, target, source)
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

func (m *Manager) run(ctx context.Context, cancel context.CancelFunc, id, target, source string) {
	defer cancel()
	defer m.clearCancel()

	m.setStage(id, StageDownloading, 15, "正在下载平台安装包…")
	artifact, usedSource, err := m.download(ctx, target, source)
	if err != nil {
		m.fail(id, "下载或校验失败："+err.Error())
		return
	}
	m.setStageWithSource(id, StageVerifying, 55, usedSource, "下载完成，正在校验 SHA256 与平台签名…")
	if err := m.verifyArtifact(ctx, artifact); err != nil {
		m.fail(id, "平台安装包校验失败："+err.Error())
		return
	}

	m.setStageWithSource(id, StageInstalling, 75, usedSource, installMessage(m.opt.GOOS))
	spec, backup, err := m.installArtifact(ctx, target, artifact)
	if err != nil {
		m.fail(id, "准备安装失败："+err.Error())
		return
	}

	m.mu.Lock()
	if m.job != nil && m.job.ID == id {
		m.job.Source = usedSource
		m.job.BackupPath = backup
		m.job.Stage = StageRestarting
		m.job.Progress = 95
		m.job.Message = "正在优雅退出并进入安装流程…"
		m.job.Cancelable = false
		m.job.UpdatedAt = m.opt.Now()
		m.job.launchPath = spec.Path
		m.job.launchArgs = append([]string(nil), spec.Args...)
		m.job.launchDir = spec.Dir
	}
	m.pending = &spec
	m.mu.Unlock()

	m.opt.Logger.Info("self-update prepared platform installer and restart",
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
	if m.job.Stage != StageDownloading && m.job.Stage != StageVerifying {
		m.job.Cancelable = false
	}
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
	m.job.Cancelable = false
	m.job.UpdatedAt = m.opt.Now()
	m.opt.Logger.Warn("self-update failed", slog.String("error", message))
}

// Cancel 中止尚未进入安装器阶段的更新任务。
func (m *Manager) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil || (m.job.Stage != StageDownloading && m.job.Stage != StageVerifying) {
		return errors.New("当前更新阶段不可取消")
	}
	if m.cancel == nil {
		return errors.New("更新任务缺少取消句柄")
	}
	m.cancel()
	return nil
}

func (m *Manager) clearCancel() {
	m.mu.Lock()
	m.cancel = nil
	m.mu.Unlock()
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
	return update.IsDevelopmentVersion(m.opt.CurrentVersion)
}

// CurrentVersion 返回当前进程版本，供重启后的进度轮询确认目标版本是否已生效。
func (m *Manager) CurrentVersion() string { return m.opt.CurrentVersion }

// Executable 返回当前可执行文件路径（启动清理备份用）。
func (m *Manager) Executable() string { return m.opt.Executable }
