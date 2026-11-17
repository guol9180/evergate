//go:build windows

package desktop

import (
	"fmt"
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
