//go:build windows

package winres

import "testing"

func TestLoadTrayIcon(t *testing.T) {
	icon, err := LoadTrayIcon()
	if err != nil {
		t.Fatal(err)
	}
	if icon == 0 {
		t.Fatal("LoadTrayIcon returned a null handle")
	}
}
