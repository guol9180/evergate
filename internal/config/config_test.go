package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestServerListenAddr(t *testing.T) {
	cases := []struct {
		host string
		port int
		want string
	}{
		{"", 8787, ":8787"},
		{"127.0.0.1", 18787, "127.0.0.1:18787"},
		{"::1", 8787, "[::1]:8787"},
		{"localhost", 8787, "localhost:8787"},
	}
	for _, tc := range cases {
		s := Default().Server
		s.Host, s.Port = tc.host, tc.port
		if got := s.ListenAddr(); got != tc.want {
			t.Fatalf("host %q port %d: got %q, want %q", tc.host, tc.port, got, tc.want)
		}
		if err := (Config{Server: s, Storage: Default().Storage, Defaults: Default().Defaults}).Validate(); err != nil {
			t.Fatalf("host %q should be valid: %v", tc.host, err)
		}
	}
}

func TestServerHostRejectsSchemeAndPort(t *testing.T) {
	for _, host := range []string{"http://127.0.0.1", "127.0.0.1:8787", "0.0.0.0/24", "bad host"} {
		cfg := Default()
		cfg.Server.Host = host
		if err := cfg.Validate(); err == nil {
			t.Fatalf("host %q should be rejected", host)
		}
	}
}

func TestStorageSQLiteSynchronousValidation(t *testing.T) {
	for _, mode := range []string{"", "full", "NORMAL", " normal "} {
		cfg := Default()
		cfg.Storage.SQLiteSynchronous = mode
		if err := cfg.Validate(); err != nil {
			t.Fatalf("mode %q should be valid: %v", mode, err)
		}
	}
	for _, mode := range []string{"off", "extra", "2"} {
		cfg := Default()
		cfg.Storage.SQLiteSynchronous = mode
		if err := cfg.Validate(); err == nil {
			t.Fatalf("mode %q should be rejected", mode)
		}
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evergate.yml")
	if err := writeTestFile(path, "server:\n  port: 18787\n  prot: 8787\n"); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "prot") {
		t.Fatalf("unknown field should be rejected with field name, got %v", err)
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
