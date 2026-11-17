//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmCommand         = 0x0111
	wmNull            = 0x0000
	wmLButtonUp       = 0x0202
	wmRButtonUp       = 0x0205
	wmApp             = 0x8000
	trayCallbackMsg   = wmApp + 1
	ninSelect         = 0x0400
	ninKeySelect      = 0x0401
	nimAdd            = 0x00000000
	nimDelete         = 0x00000002
	nimSetVersion     = 0x00000004
	nifMessage        = 0x00000001
	nifIcon           = 0x00000002
	nifTip            = 0x00000004
	notifyIconVer4    = 4
	tpmRightButton    = 0x0002
	tpmReturnCmd      = 0x0100
	menuOpen          = 1001
	menuLogs          = 1002
	menuRestart       = 1003
	menuExit          = 1004
	errorAlreadyExist = 183
	idiApplication    = 32512
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	procCreateMutexW        = kernel32.NewProc("CreateMutexW")
	trayWindowProcCallback  = windows.NewCallback(trayWindowProc)
)

type point struct {
	X int32
	Y int32
}

type message struct {
	HWnd     windows.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type notifyIconData struct {
	CbSize          uint32
	HWnd            windows.Handle
	UID             uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            windows.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        windows.GUID
	BalloonIcon     windows.Handle
}

var trayActions TrayActions
var trayStopMu sync.Mutex
var trayStop func()

var startDetachedCommand = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// AcquireSingleInstance 获取当前用户会话内的 EverGate 命名互斥。
func AcquireSingleInstance() (func(), bool, error) {
	return acquireSingleInstance(`Local\EverGate`)
}

func acquireSingleInstance(name string) (func(), bool, error) {
	namePtr, _ := windows.UTF16PtrFromString(name)
	handle, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if handle == 0 {
		return nil, false, fmt.Errorf("CreateMutexW: %w", err)
	}
	if errno, ok := err.(syscall.Errno); ok && errno == errorAlreadyExist {
		_ = windows.CloseHandle(windows.Handle(handle))
		return func() {}, false, nil
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = windows.CloseHandle(windows.Handle(handle))
		})
	}, true, nil
}

// ShowError 在 GUI 产物中显示启动失败提示；console 产物继续使用 stderr。
func ShowError(title, text string) {
	if !IsGUI() {
		return
	}
	titlePtr, _ := windows.UTF16PtrFromString(title)
	textPtr, _ := windows.UTF16PtrFromString(text)
	_, _, _ = procMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), 0x10)
}

// RevealPath 在资源管理器中打开目录并选中目标文件。
func RevealPath(path string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("log file is unavailable: %w", err)
	}
	if info.IsDir() {
		return errors.New("log path is a directory")
	}
	return startDetachedCommand("explorer.exe", "/select,"+path)
}

// StartTray 启动消息循环和系统托盘；仅 GUI 产物生效。
func StartTray(actions TrayActions) (func(), error) {
	if !IsGUI() {
		return func() {}, nil
	}
	trayActions = actions
	ready := make(chan error, 1)
	done := make(chan struct{})
	var hwnd windows.Handle

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		created, err := createTrayWindow()
		if err != nil {
			ready <- err
			return
		}
		hwnd = created
		if err := addTrayIcon(hwnd); err != nil {
			_ = destroyTrayWindow(hwnd)
			ready <- err
			return
		}
		ready <- nil

		var msg message
		for {
			ret, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) <= 0 {
				if int32(ret) < 0 {
					_ = callErr
				}
				break
			}
			_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
		removeTrayIcon(hwnd)
	}()

	if err := <-ready; err != nil {
		return nil, err
	}
	stop := func() {
		if hwnd != 0 {
			_, _, _ = procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
		}
		<-done
	}
	trayStopMu.Lock()
	trayStop = stop
	trayStopMu.Unlock()
	return func() {
		stop()
		trayStopMu.Lock()
		trayStop = nil
		trayStopMu.Unlock()
	}, nil
}

// StopTray 唤醒桌面消息循环，供进程信号或 HTTP 服务错误触发优雅停机。
func StopTray() {
	trayStopMu.Lock()
	stop := trayStop
	trayStopMu.Unlock()
	if stop != nil {
		stop()
	}
}

func createTrayWindow() (windows.Handle, error) {
	instance, _, _ := procGetModuleHandleW.Call(0)
	className, _ := windows.UTF16PtrFromString("EverGateTrayWindow")
	class := wndClassEx{
		CbSize: uint32(unsafe.Sizeof(wndClassEx{})), LpfnWndProc: trayWindowProcCallback,
		HInstance: windows.Handle(instance), LpszClassName: className,
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return 0, fmt.Errorf("RegisterClassExW: %w", err)
	}
	title, _ := windows.UTF16PtrFromString("EverGate")
	hwnd, _, createErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, instance, 0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %w", createErr)
	}
	return windows.Handle(hwnd), nil
}

func destroyTrayWindow(hwnd windows.Handle) error {
	ret, _, err := procDestroyWindow.Call(uintptr(hwnd))
	if ret == 0 {
		return fmt.Errorf("DestroyWindow: %w", err)
	}
	return nil
}

func addTrayIcon(hwnd windows.Handle) error {
	instance, _, _ := procGetModuleHandleW.Call(0)
	icon, _, _ := procLoadIconW.Call(instance, 1)
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, idiApplication)
	}
	data := notifyIconData{
		HWnd: hwnd, UID: 1, Flags: nifMessage | nifIcon | nifTip,
		CallbackMessage: trayCallbackMsg, Icon: windows.Handle(icon), Version: notifyIconVer4,
	}
	data.CbSize = uint32(unsafe.Sizeof(data))
	copyUTF16(data.Tip[:], "EverGate")
	ret, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	if ret == 0 {
		return fmt.Errorf("Shell_NotifyIcon NIM_ADD: %w", err)
	}
	_, _, _ = procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&data)))
	return nil
}

func removeTrayIcon(hwnd windows.Handle) {
	data := notifyIconData{
		HWnd: hwnd, UID: 1, Flags: nifMessage | nifIcon | nifTip,
		CallbackMessage: trayCallbackMsg,
	}
	data.CbSize = uint32(unsafe.Sizeof(data))
	_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
}

func trayWindowProc(hwnd windows.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case trayCallbackMsg:
		event := uint32(lParam) & 0xffff
		if event == wmLButtonUp || event == wmRButtonUp || event == ninSelect || event == ninKeySelect {
			showTrayMenu(hwnd)
		}
		return 0
	case wmCommand:
		runTrayCommand(uint32(wParam) & 0xffff)
		return 0
	case wmClose:
		_ = destroyTrayWindow(hwnd)
		return 0
	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	default:
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return ret
	}
}

func showTrayMenu(hwnd windows.Handle) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendMenu(menu, menuOpen, "打开管理台")
	appendMenu(menu, menuLogs, "打开日志目录")
	appendMenu(menu, menuRestart, "重启")
	appendMenu(menu, menuExit, "退出")

	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	_, _, _ = procSetForegroundWindow.Call(uintptr(hwnd))
	command, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
	_, _, _ = procPostMessageW.Call(uintptr(hwnd), wmNull, 0, 0)
	if command != 0 {
		runTrayCommand(uint32(command))
	}
}

func appendMenu(menu uintptr, id uint32, label string) {
	text, _ := windows.UTF16PtrFromString(label)
	_, _, _ = procAppendMenuW.Call(menu, 0, uintptr(id), uintptr(unsafe.Pointer(text)))
}

func runTrayCommand(command uint32) {
	switch command {
	case menuOpen:
		if trayActions.OpenAdmin != nil {
			go trayActions.OpenAdmin()
		}
	case menuLogs:
		if trayActions.OpenLogs != nil {
			go trayActions.OpenLogs()
		}
	case menuRestart:
		if trayActions.Restart != nil {
			go trayActions.Restart()
		}
	case menuExit:
		if trayActions.Exit != nil {
			go trayActions.Exit()
		}
	}
}

func copyUTF16(dst []uint16, value string) {
	encoded, _ := windows.UTF16FromString(value)
	copy(dst, encoded)
}
