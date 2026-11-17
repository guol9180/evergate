//go:build windows && windowsgui

package desktop

const isGUI = true

// IsGUI 返回当前产物是否为 Windows GUI 子系统。
func IsGUI() bool { return isGUI }
