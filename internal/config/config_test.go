package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerShouldOpenBrowser(t *testing.T) {
	cfg := Default()
	if cfg.Storage.SQLitePath != "" {
		t.Fatalf("default sqlite path should be resolved by appdir, got %q", cfg.Storage.SQLitePath)
	}
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

func TestAdminLoopbackOnlyDefaultsToTrue(t *testing.T) {
	cfg := Default()
	if !cfg.Server.ShouldRestrictAdminToLoopback() {
		t.Fatal("admin API should be loopback-only by default")
	}
	restrict := true
	cfg.Server.AdminLoopbackOnly = &restrict
	if !cfg.Server.ShouldRestrictAdminToLoopback() {
		t.Fatal("explicit true should restrict admin API")
	}
	allowRemote := false
	cfg.Server.AdminLoopbackOnly = &allowRemote
	if cfg.Server.ShouldRestrictAdminToLoopback() {
		t.Fatal("explicit false should allow remote admin access")
	}
}

func TestBasePathRejectsUnsafeCharacters(t *testing.T) {
	cfg := Default()
	cfg.Server.BasePath = `/bad"path`
	if err := cfg.Validate(); err == nil {
		t.Fatal("unsafe base path should be rejected")
	}
	cfg.Server.BasePath = "/eg-gateway_1"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("safe base path rejected: %v", err)
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
