package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerShouldOpenBrowser(t *testing.T) {
	cfg := Default()
	if !cfg.Server.ShouldOpenBrowser() {
		t.Fatal("default should open browser for desktop launcher experience")
	}
	disabled := false
	cfg.Server.OpenBrowser = &disabled
	if cfg.Server.ShouldOpenBrowser() {
		t.Fatal("explicit false should disable browser opening")
	}
	enabled := true
	cfg.Server.OpenBrowser = &enabled
	if !cfg.Server.ShouldOpenBrowser() {
		t.Fatal("explicit true should enable browser opening")
	}
}

func TestLoadOpenBrowserOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evergate.yml")
	if err := writeTestFile(path, "server:\n  port: 18787\n  open_browser: false\n"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ShouldOpenBrowser() {
		t.Fatal("YAML false should disable browser opening")
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
