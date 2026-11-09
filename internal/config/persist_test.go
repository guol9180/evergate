package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistReplacesEmptySecretsAndKeepsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evergate.yml")
	original := "server:\n  port: 8787 # 端口注释\nsecurity:\n  master_key: \"\"\n  session_secret: \"\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := Default()
	cfg.Security.MasterKey = "bWFzdGVyLXRlc3Qta2V5"
	cfg.Security.SessionSecret = "c2Vzc2lvbi10ZXN0LWtleQ=="
	if err := cfg.Persist(path); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# 端口注释") {
		t.Fatalf("comments lost:\n%s", text)
	}
	if !strings.Contains(text, `master_key: "bWFzdGVyLXRlc3Qta2V5"`) {
		t.Fatalf("master_key not replaced:\n%s", text)
	}
	if !strings.Contains(text, `session_secret: "c2Vzc2lvbi10ZXN0LWtleQ=="`) {
		t.Fatalf("session_secret not replaced:\n%s", text)
	}
	// 写回后应能被 Load 读出
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after persist: %v", err)
	}
	if reloaded.Security.MasterKey != cfg.Security.MasterKey {
		t.Fatalf("reloaded master_key = %q", reloaded.Security.MasterKey)
	}
}

func TestPersistCreatesFileWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evergate.yml")
	cfg := Default()
	cfg.Security.MasterKey = "abc"
	if err := cfg.Persist(path); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reloaded.Security.MasterKey != "abc" {
		t.Fatalf("master_key = %q, want abc", reloaded.Security.MasterKey)
	}
}
