package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// dirWritable 尝试在目标目录创建临时文件，用于提前拒绝只读安装目录。
func dirWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".evergate-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// replaceExecutable 使用同目录改名交换完成替换，失败时恢复原文件。
// Windows 允许重命名运行中的 exe，但不能覆盖；因此先 rename 到 .bak 再就位。
func replaceExecutable(exe, targetVersion string, data []byte) (string, error) {
	if exe == "" {
		return "", fmt.Errorf("无法定位当前可执行文件")
	}
	dir := filepath.Dir(exe)
	if err := dirWritable(dir); err != nil {
		return "", fmt.Errorf("安装目录不可写: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(exe)+".new-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("写入新版本失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("同步新版本失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("关闭新版本文件失败: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		cleanup()
		return "", fmt.Errorf("设置执行权限失败: %w", err)
	}

	backup := exe + ".bak-" + targetVersion + "-" + time.Now().UTC().Format("20060102T150405")
	for i := 1; ; i++ {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			break
		}
		backup = fmt.Sprintf("%s.bak-%s-%s-%d", exe, targetVersion, time.Now().UTC().Format("20060102T150405"), i)
	}
	if err := os.Rename(exe, backup); err != nil {
		cleanup()
		return "", fmt.Errorf("备份当前版本失败: %w", err)
	}
	if err := os.Rename(tmpName, exe); err != nil {
		_ = os.Rename(backup, exe)
		cleanup()
		return "", fmt.Errorf("新版本就位失败（已恢复旧版本）: %w", err)
	}
	return backup, nil
}

// CleanupBackups 在启动时清理旧的替换残留：保留最近 keep 份 .bak，
// 删除崩溃遗留的 .new-* 临时文件。
func CleanupBackups(exe string, keep int) {
	if exe == "" {
		return
	}
	dir := filepath.Dir(exe)
	base := filepath.Base(exe)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type backup struct {
		name    string
		modTime time.Time
	}
	var backups []backup
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		switch {
		case strings.HasPrefix(name, base+".new-"):
			_ = os.Remove(path)
		case strings.HasPrefix(name, base+".bak-"):
			info, err := entry.Info()
			if err == nil {
				backups = append(backups, backup{name: path, modTime: info.ModTime()})
			}
		}
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].modTime.After(backups[j].modTime) })
	if keep < 0 {
		keep = 0
	}
	for i := keep; i < len(backups); i++ {
		_ = os.Remove(backups[i].name)
	}
}
