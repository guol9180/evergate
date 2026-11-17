// Package desktop 封装桌面平台单实例、托盘、错误提示与文件定位。
package desktop

// TrayActions 是托盘菜单触发的应用动作。
type TrayActions struct {
	OpenAdmin func()
	OpenLogs  func()
	Restart   func()
	Exit      func()
}
