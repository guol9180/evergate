// Package desktop 封装 Windows GUI 单实例、托盘与错误提示；其他平台保持无操作。
package desktop

// TrayActions 是托盘菜单触发的应用动作。
type TrayActions struct {
	OpenAdmin func()
	OpenLogs  func()
	Restart   func()
	Exit      func()
}
