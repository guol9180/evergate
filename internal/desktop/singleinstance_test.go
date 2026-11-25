package desktop

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestSingleInstanceKeyScopesByDataDir(t *testing.T) {
	a := t.TempDir()
	if got, want := singleInstanceKey(filepath.Join(a, "sub", "..")), singleInstanceKey(a); got != want {
		t.Fatalf("same data dir must derive same key: %s != %s", got, want)
	}
	if singleInstanceKey(a) == singleInstanceKey(t.TempDir()) {
		t.Fatal("different data dirs must derive different keys")
	}
	if runtime.GOOS == "windows" {
		if singleInstanceKey(`C:\Data\EverGate`) != singleInstanceKey(`c:\data\evergate`) {
			t.Fatal("windows paths are case-insensitive and must share one key")
		}
	}
}
