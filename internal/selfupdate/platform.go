package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var runPlatformCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func validateInstallation(goos, executable string) error {
	if executable == "" {
		return errors.New("无法定位当前可执行文件")
	}
	clean := filepath.Clean(executable)
	lower := strings.ToLower(clean)
	switch goos {
	case "windows":
		suffix := strings.ToLower(filepath.Join("Programs", "EverGate", "evergate.exe"))
		if !strings.HasSuffix(lower, suffix) {
			return errors.New("当前程序不是标准 EverGate 安装器安装版本；npm 或便携版请通过原安装渠道更新")
		}
	case "darwin":
		if !strings.HasSuffix(clean, filepath.Join("Applications", "EverGate.app", "Contents", "MacOS", "evergate")) {
			return errors.New("当前程序不在 ~/Applications/EverGate.app 中；请通过原安装渠道更新")
		}
	case "linux":
		if runtime.GOOS == "linux" {
			if _, err := os.Stat("/.dockerenv"); err == nil || os.Getenv("EVERGATE_CONTAINER") != "" {
				return errors.New("容器环境请更新 Docker 镜像，不支持应用内自更新")
			}
		}
		if strings.Contains(strings.ToLower(clean), "node_modules") {
			return errors.New("npm 安装版本请运行 npm i -g @heguoliang/evergate@latest 更新")
		}
		if err := dirWritable(filepath.Dir(clean)); err != nil {
			return fmt.Errorf("安装目录不可写：%w", err)
		}
	default:
		return fmt.Errorf("当前平台 %s 不支持应用内更新", goos)
	}
	return nil
}

func installMessage(goos string) string {
	switch goos {
	case "windows", "darwin":
		return "安装包已就绪，将退出当前实例并打开安装器…"
	default:
		return "归档已就绪，正在安全替换 Linux 可执行文件并保留备份…"
	}
}

func (m *Manager) verifyArtifact(ctx context.Context, artifact []byte) error {
	switch m.opt.GOOS {
	case "darwin":
		file, err := writeTemporaryArtifact(artifact, "evergate-update-*.dmg")
		if err != nil {
			return err
		}
		defer os.Remove(file)
		if output, err := runPlatformCommand(ctx, "/usr/bin/xcrun", "stapler", "validate", file); err != nil {
			return fmt.Errorf("notarization 校验失败: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		if output, err := runPlatformCommand(ctx, "/usr/sbin/spctl", "-a", "-vv", "-t", "open",
			"--context", "context:primary-signature", file); err != nil {
			return fmt.Errorf("developer ID 签名校验失败: %w (%s)", err, strings.TrimSpace(string(output)))
		}
	case "windows":
		if os.Getenv("EVERGATE_REQUIRE_AUTHENTICODE") != "true" {
			return nil
		}
		file, err := writeTemporaryArtifact(artifact, "evergate-update-*.exe")
		if err != nil {
			return err
		}
		defer os.Remove(file)
		script := `$s=Get-AuthenticodeSignature -LiteralPath $args[0]; if ($s.Status -ne 'Valid') { Write-Error "$($s.Status): $($s.StatusMessage)"; exit 1 }`
		if output, err := runPlatformCommand(ctx, "powershell.exe", "-NoProfile", "-NonInteractive",
			"-Command", script, file); err != nil {
			return fmt.Errorf("authenticode 校验失败: %w (%s)", err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func (m *Manager) installArtifact(ctx context.Context, target string, artifact []byte) (LaunchSpec, string, error) {
	switch m.opt.GOOS {
	case "windows":
		file, err := writeTemporaryArtifact(artifact, "evergate-update-*.exe")
		if err != nil {
			return LaunchSpec{}, "", err
		}
		return LaunchSpec{
			Path: file,
			Args: []string{"/SP-", "/NORESTART"},
			Dir:  filepath.Dir(file),
		}, "", nil
	case "darwin":
		file, err := writeTemporaryArtifact(artifact, "evergate-update-*.dmg")
		if err != nil {
			return LaunchSpec{}, "", err
		}
		return LaunchSpec{Path: "/usr/bin/open", Args: []string{file}, Dir: filepath.Dir(file)}, "", nil
	case "linux":
		if err := ctx.Err(); err != nil {
			return LaunchSpec{}, "", err
		}
		binary, err := extractLinuxBinary(artifact)
		if err != nil {
			return LaunchSpec{}, "", err
		}
		backup, err := replaceExecutable(m.opt.Executable, target, binary)
		if err != nil {
			return LaunchSpec{}, "", err
		}
		return LaunchSpec{
			Path: m.opt.Executable,
			Args: append([]string(nil), m.opt.Args...),
			Dir:  m.opt.WorkDir,
		}, backup, nil
	default:
		return LaunchSpec{}, "", fmt.Errorf("平台 %s 不支持安装包", m.opt.GOOS)
	}
}

func writeTemporaryArtifact(data []byte, pattern string) (string, error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	name := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}
