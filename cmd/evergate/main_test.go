package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"evergate/internal/appdir"
	"evergate/internal/config"
)

// 停止来源由托盘 / 自更新 / 退出三处并发触发，重复请求必须只关闭一次通道。
func TestStopSignalsRequestsAreIdempotent(t *testing.T) {
	signals := newStopSignals()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			signals.requestUpdate()
			signals.requestRestart()
			signals.requestExit()
		}()
	}
	wg.Wait()
	for name, ch := range map[string]<-chan struct{}{
		"update": signals.update, "restart": signals.restart, "exit": signals.exit,
	} {
		select {
		case <-ch:
		default:
			t.Fatalf("%s channel was not closed", name)
		}
	}
}

func TestBootstrapConfigCreatesConfigAtResolvedPath(t *testing.T) {
	root := t.TempDir()
	paths := appdir.Paths{
		ConfigPath: filepath.Join(root, "config", "evergate.yml"),
		DataDir:    filepath.Join(root, "data"),
		Source:     appdir.SourceDefault,
	}
	cfg, sqlitePath, err := resolveBootstrapConfig(paths)
	if err != nil {
		t.Fatalf("load bootstrap: %v", err)
	}
	cfg, generated, err := ensureBootstrapSecrets(paths, cfg)
	if err != nil {
		t.Fatalf("load bootstrap: %v", err)
	}
	if !generated || cfg.Security.MasterKey == "" || cfg.Security.SessionSecret == "" {
		t.Fatalf("secrets were not generated: %#v", cfg.Security)
	}
	if sqlitePath != filepath.Join(paths.DataDir, "evergate.db") || cfg.Storage.SQLitePath != sqlitePath {
		t.Fatalf("sqlite path = %q cfg=%q", sqlitePath, cfg.Storage.SQLitePath)
	}
	if _, err := os.Stat(paths.ConfigPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	reloaded, err := config.Load(paths.ConfigPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Storage.SQLitePath != sqlitePath ||
		reloaded.Security.MasterKey != cfg.Security.MasterKey ||
		reloaded.Security.SessionSecret != cfg.Security.SessionSecret {
		t.Fatalf("persisted config mismatch: %#v", reloaded)
	}
}

func TestBootstrapConfigKeepsPortableRelativeSQLitePath(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "portable", "evergate.yml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`
storage:
  sqlite_path: ./data/evergate.db
security:
  master_key: "master"
  session_secret: "session"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := appdir.Paths{
		ConfigPath: configPath, ConfigDir: filepath.Dir(configPath),
		DataDir: filepath.Join(root, "platform-data"),
		Source:  appdir.SourcePortable, ConfigExists: true,
	}
	cfg, sqlitePath, err := resolveBootstrapConfig(paths)
	if err != nil {
		t.Fatalf("load bootstrap: %v", err)
	}
	_, generated, err := ensureBootstrapSecrets(paths, cfg)
	if err != nil {
		t.Fatalf("load bootstrap: %v", err)
	}
	if generated {
		t.Fatal("existing portable secrets should not be regenerated")
	}
	if want := filepath.Join(filepath.Dir(configPath), "data", "evergate.db"); sqlitePath != want {
		t.Fatalf("sqlite path = %q, want %q", sqlitePath, want)
	}
}

func TestShouldOpenBrowserHonorsAutostart(t *testing.T) {
	server := config.Default().Server
	if !shouldOpenBrowser(server, false) {
		t.Fatal("manual launch should honor open_browser=true")
	}
	if shouldOpenBrowser(server, true) {
		t.Fatal("autostart must not open the browser")
	}
	disabled := false
	server.OpenBrowser = &disabled
	if shouldOpenBrowser(server, false) {
		t.Fatal("manual launch should honor open_browser=false")
	}
}
