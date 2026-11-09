package store

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

// newTestDB 每个用例独立的临时 SQLite。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestMigrateCreatesAllTables(t *testing.T) {
	db := newTestDB(t)
	for _, table := range []string{"channels", "channel_keys", "tokens", "request_logs", "usage_daily", "admins"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("table %s not created", table)
		}
	}
}

func TestEnsureAdminCreatesOnceAndIdempotent(t *testing.T) {
	db := newTestDB(t)
	created, err := EnsureAdmin(db, DefaultAdminUsername, DefaultAdminPassword)
	if err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}
	if !created {
		t.Fatal("first EnsureAdmin should create")
	}
	created, err = EnsureAdmin(db, DefaultAdminUsername, DefaultAdminPassword)
	if err != nil {
		t.Fatalf("second EnsureAdmin: %v", err)
	}
	if created {
		t.Fatal("second EnsureAdmin should be no-op")
	}
	var count int64
	db.Model(&Admin{}).Count(&count)
	if count != 1 {
		t.Fatalf("admin count = %d, want 1", count)
	}
}

func TestDefaultAdminAuthenticates(t *testing.T) {
	db := newTestDB(t)
	if _, err := EnsureAdmin(db, DefaultAdminUsername, DefaultAdminPassword); err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}
	admin, err := GetAdminByUsername(db, DefaultAdminUsername)
	if err != nil {
		t.Fatalf("GetAdminByUsername: %v", err)
	}
	if !CheckAdminPassword(admin, DefaultAdminPassword) {
		t.Fatal("default password should verify")
	}
	if CheckAdminPassword(admin, "wrong") {
		t.Fatal("wrong password should not verify")
	}
	if CheckAdminPassword(nil, "x") {
		t.Fatal("nil admin should not verify")
	}
	if !admin.MustChangePassword {
		t.Fatal("default admin must be flagged MustChangePassword")
	}
}

func TestGormTimestampsStoredAsUTC(t *testing.T) {
	db := newTestDB(t)
	sec := 60
	ch := Channel{Name: "utc-check", Type: "openai", BaseURL: "https://example.com", Models: []string{"gpt-x"}, Timeouts: ChannelTimeouts{ConnectSec: &sec}}
	if err := db.Create(&ch).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	var stored string
	if err := db.Raw("SELECT created_at FROM channels WHERE id = ?", ch.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("raw select: %v", err)
	}
	ts, err := time.Parse(time.RFC3339Nano, stored)
	if err != nil {
		t.Fatalf("created_at %q is not RFC3339: %v", stored, err)
	}
	if _, offset := ts.Zone(); offset != 0 {
		t.Fatalf("created_at stored with offset %d, want UTC(0): %s", offset, stored)
	}
}
