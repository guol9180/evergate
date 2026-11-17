//go:build windows && windowsgui

package desktop

import "evergate/internal/winres"

const isGUI = true

func init() {
	loadTrayIcon = winres.LoadTrayIcon
}

// IsGUI 返回当前产物是否为 Windows GUI 子系统。
func IsGUI() bool { return isGUI }
