//go:build windows

package winres

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// The DPI manifest occupies resource ID 1; rsrc emits the icon group as ID 2.
const trayIconResourceID = 2

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procLoadImageW      = user32.NewProc("LoadImageW")
	procGetSystemMetric = user32.NewProc("GetSystemMetrics")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

// LoadTrayIcon loads the small notification-area icon from the GUI resource bundle.
func LoadTrayIcon() (windows.Handle, error) {
	instance, _, _ := procGetModuleHandle.Call(0)
	width, _, _ := procGetSystemMetric.Call(49) // SM_CXSMICON
	height, _, _ := procGetSystemMetric.Call(50)
	icon, _, err := procLoadImageW.Call(
		instance,
		trayIconResourceID,
		1, // IMAGE_ICON
		width,
		height,
		uintptr(0x8000), // LR_SHARED
	)
	if icon == 0 {
		return 0, fmt.Errorf("LoadImageW icon resource %d: %w", trayIconResourceID, err)
	}
	return windows.Handle(icon), nil
}
