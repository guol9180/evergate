//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNamedSingleInstance(t *testing.T) {
	name := fmt.Sprintf(`Local\EverGate-Test-%d`, time.Now().UnixNano())
	release, acquired, err := acquireSingleInstance(name)
	if err != nil || !acquired {
		t.Fatalf("first acquire: acquired=%v err=%v", acquired, err)
	}
	secondRelease, second, err := acquireSingleInstance(name)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if second {
		t.Fatal("second acquire should be rejected")
	}
	secondRelease()
	release()

	reacquiredRelease, reacquired, err := acquireSingleInstance(name)
	if err != nil || !reacquired {
		t.Fatalf("reacquire: acquired=%v err=%v", reacquired, err)
	}
	reacquiredRelease()
}

func TestAcquireSingleInstanceScopedByDataDir(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	releaseA, acquiredA, err := AcquireSingleInstance(dirA)
	if err != nil || !acquiredA {
		t.Fatalf("first data dir: acquired=%v err=%v", acquiredA, err)
	}
	defer releaseA()
	secondRelease, second, err := AcquireSingleInstance(dirA)
	if err != nil {
		t.Fatalf("same data dir second acquire: %v", err)
	}
	if second {
		secondRelease()
		t.Fatal("same data dir must be rejected while first instance holds the mutex")
	}
	releaseB, acquiredB, err := AcquireSingleInstance(dirB)
	if err != nil || !acquiredB {
		t.Fatalf("different data dir: acquired=%v err=%v", acquiredB, err)
	}
	releaseB()
}

func TestTrayCommandsInvokeActions(t *testing.T) {
	var opens, logs, restarts, exits atomic.Int32
	previous := trayActions
	trayActions = TrayActions{
		OpenAdmin: func() { opens.Add(1) },
		OpenLogs:  func() { logs.Add(1) },
		Restart:   func() { restarts.Add(1) },
		Exit:      func() { exits.Add(1) },
	}
	t.Cleanup(func() { trayActions = previous })

	runTrayCommand(menuOpen)
	runTrayCommand(menuLogs)
	runTrayCommand(menuRestart)
	runTrayCommand(menuExit)
	time.Sleep(10 * time.Millisecond)
	if opens.Load() != 1 || logs.Load() != 1 || restarts.Load() != 1 || exits.Load() != 1 {
		t.Fatalf("actions opens=%d logs=%d restarts=%d exits=%d", opens.Load(), logs.Load(), restarts.Load(), exits.Load())
	}
}

func TestRevealPathSelectsExistingLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "evergate.log")
	if err := os.WriteFile(logPath, []byte("log"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := startDetachedCommand
	var gotName string
	var gotArgs []string
	startDetachedCommand = func(name string, args ...string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() { startDetachedCommand = previous })

	if err := RevealPath(logPath); err != nil {
		t.Fatal(err)
	}
	if gotName != "explorer.exe" || !reflect.DeepEqual(gotArgs, []string{"/select," + logPath}) {
		t.Fatalf("command = %q %#v", gotName, gotArgs)
	}
}

func TestRevealPathRejectsMissingFileAndReportsCommandFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.log")
	if err := RevealPath(missing); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing file error = %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "evergate.log")
	if err := os.WriteFile(logPath, []byte("log"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := startDetachedCommand
	startDetachedCommand = func(string, ...string) error { return errors.New("explorer failed") }
	t.Cleanup(func() { startDetachedCommand = previous })
	if err := RevealPath(logPath); err == nil || !strings.Contains(err.Error(), "explorer failed") {
		t.Fatalf("command error = %v", err)
	}
}
