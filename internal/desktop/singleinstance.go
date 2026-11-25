package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"
)

// singleInstanceKey 按数据目录派生互斥名后缀：同一数据目录仍保持单实例，不同数据目录允许并行
// （开发实例与已安装实例共存）。路径先取绝对值；Windows 大小写不敏感，统一小写避免大小写差异绕过互斥。
func singleInstanceKey(dataDir string) string {
	clean := filepath.Clean(dataDir)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	if runtime.GOOS == "windows" {
		clean = strings.ToLower(clean)
	}
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:8])
}
