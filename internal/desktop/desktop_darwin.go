//go:build darwin && cgo

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

extern void evergateTrayCommand(int command);
extern void evergateShowError(const char *title, const char *text);

@interface EverGateMenuTarget : NSObject
@end

@implementation EverGateMenuTarget
- (void)openAdmin:(id)sender { evergateTrayCommand(1); }
- (void)openLogs:(id)sender { evergateTrayCommand(2); }
- (void)restart:(id)sender { evergateTrayCommand(3); }
- (void)quit:(id)sender { evergateTrayCommand(4); }
@end

static EverGateMenuTarget *evergateMenuTarget = nil;
static NSStatusItem *evergateStatusItem = nil;

static void EverGateStopApplication(void) {
	[NSApp stop:nil];
	NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
		location:NSZeroPoint modifierFlags:0 timestamp:0 windowNumber:0
		context:nil subtype:0 data1:0 data2:0];
	[NSApp postEvent:event atStart:YES];
}

static int EverGateRunTray(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		evergateMenuTarget = [[EverGateMenuTarget alloc] init];
		evergateStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];

		NSButton *button = evergateStatusItem.button;
		button.title = @"EG";
		button.toolTip = @"EverGate";
		button.font = [NSFont systemFontOfSize:12 weight:NSFontWeightSemibold];

		NSMenu *menu = [[NSMenu alloc] initWithTitle:@"EverGate"];
		NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:@"打开管理台" action:@selector(openAdmin:) keyEquivalent:@""];
		item.target = evergateMenuTarget;
		[menu addItem:item];
		item = [[NSMenuItem alloc] initWithTitle:@"打开日志目录" action:@selector(openLogs:) keyEquivalent:@""];
		item.target = evergateMenuTarget;
		[menu addItem:item];
		[menu addItem:[NSMenuItem separatorItem]];
		item = [[NSMenuItem alloc] initWithTitle:@"重启" action:@selector(restart:) keyEquivalent:@""];
		item.target = evergateMenuTarget;
		[menu addItem:item];
		item = [[NSMenuItem alloc] initWithTitle:@"退出" action:@selector(quit:) keyEquivalent:@""];
		item.target = evergateMenuTarget;
		[menu addItem:item];
		evergateStatusItem.menu = menu;

		[NSApp run];
		[[NSStatusBar systemStatusBar] removeStatusItem:evergateStatusItem];
		evergateStatusItem = nil;
		evergateMenuTarget = nil;
		return 0;
	}
}

static void EverGateShowError(const char *title, const char *text) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = [NSString stringWithUTF8String:title];
		alert.informativeText = [NSString stringWithUTF8String:text];
		alert.alertStyle = NSAlertStyleCritical;
		[alert runModal];
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

var trayActions TrayActions

var startDetachedCommand = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// AcquireSingleInstance 使用当前用户缓存目录中的文件锁限制为单实例。
func AcquireSingleInstance() (func(), bool, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "EverGate")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return func() {}, false, nil
		}
		return nil, false, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			_ = file.Close()
		})
	}, true, nil
}

func ShowError(title, text string) {
	titleC := C.CString(title)
	textC := C.CString(text)
	defer C.free(unsafe.Pointer(titleC))
	defer C.free(unsafe.Pointer(textC))
	C.EverGateShowError(titleC, textC)
}

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
	return startDetachedCommand("open", "-R", path)
}

// StartTray runs the AppKit event loop on the main goroutine and returns after
// restart or exit has been requested from the menu.
func StartTray(actions TrayActions) (func(), error) {
	trayActions = actions
	C.EverGateRunTray()
	return func() {}, nil
}

// StopTray 从信号或 HTTP 服务错误路径停止 AppKit 事件循环。
func StopTray() {
	C.EverGateStopApplication()
}

//export evergateTrayCommand
func evergateTrayCommand(command C.int) {
	switch int(command) {
	case 1:
		if trayActions.OpenAdmin != nil {
			go trayActions.OpenAdmin()
		}
	case 2:
		if trayActions.OpenLogs != nil {
			go trayActions.OpenLogs()
		}
	case 3:
		if trayActions.Restart != nil {
			go trayActions.Restart()
		}
		C.EverGateStopApplication()
	case 4:
		if trayActions.Exit != nil {
			go trayActions.Exit()
		}
		C.EverGateStopApplication()
	}
}
