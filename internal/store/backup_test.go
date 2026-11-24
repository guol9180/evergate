package store

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func listBackups(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read backups dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestMigrateWithBackupSkipsFreshAndDevDatabases(t *testing.T) {
	t.Run("fresh database is not backed up", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "evergate.db")
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer closeDB(t, db)
		if err := MigrateWithBackup(db, path, "v1.0.0", nil); err != nil {
			t.Fatalf("MigrateWithBackup: %v", err)
		}
		if got := listBackups(t, filepath.Join(dir, "backups")); len(got) != 0 {
			t.Fatalf("fresh database produced backups: %v", got)
		}
		if v, _ := GetSchemaMeta(db, SchemaMetaKeyAppVersion); v != "v1.0.0" {
			t.Fatalf("recorded version = %q, want v1.0.0", v)
		}
	})

	t.Run("development version never backs up", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "evergate.db")
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer closeDB(t, db)
		if err := MigrateWithBackup(db, path, "v1.0.0", nil); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := MigrateWithBackup(db, path, "v1.0.1-3-gabcdef0-dirty", nil); err != nil {
			t.Fatalf("dev migrate: %v", err)
		}
		if got := listBackups(t, filepath.Join(dir, "backups")); len(got) != 0 {
			t.Fatalf("dev version produced backups: %v", got)
		}
	})

	t.Run("same version is not backed up twice", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "evergate.db")
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer closeDB(t, db)
		for i := 0; i < 3; i++ {
			if err := MigrateWithBackup(db, path, "v1.0.0", nil); err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
		}
		if got := listBackups(t, filepath.Join(dir, "backups")); len(got) != 0 {
			t.Fatalf("same version produced backups: %v", got)
		}
	})
}

func TestMigrateWithBackupOnVersionChangeAndPrunes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evergate.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeDB(t, db)
	if err := MigrateWithBackup(db, path, "v1.0.0", nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 写入一行数据，确保备份文件确实包含数据库内容而非空壳。
	if err := db.Create(&Channel{Name: "backup-check", Type: "openai", BaseURL: "https://example.com", Models: []string{"m"}}).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	versions := []string{"v1.0.1", "v1.0.2", "v1.0.3", "v1.0.4", "v1.0.5", "v1.1.0", "v1.2.0"}
	for _, v := range versions {
		if err := MigrateWithBackup(db, path, v, nil); err != nil {
			t.Fatalf("migrate %s: %v", v, err)
		}
	}
	backups := listBackups(t, filepath.Join(dir, "backups"))
	if len(backups) != BackupKeepCount {
		t.Fatalf("kept %d backups, want %d: %v", len(backups), BackupKeepCount, backups)
	}
	for _, name := range backups {
		if len(name) < len("evergate-20060102T150405Z.db") {
			t.Fatalf("unexpected backup name %q", name)
		}
	}
	// 备份应能独立打开并包含种子数据。
	restored, err := Open(filepath.Join(dir, "backups", backups[0]))
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer closeDB(t, restored)
	var count int64
	if err := restored.Model(&Channel{}).Count(&count).Error; err != nil {
		t.Fatalf("count in backup: %v", err)
	}
	if count != 1 {
		t.Fatalf("backup contains %d channels, want 1", count)
	}
}

func TestMigrateWithBackupLegacyDatabaseWithoutSchemaMeta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evergate.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeDB(t, db)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// 模拟旧版本：schema_meta 表不存在且已有数据。
	if err := db.Migrator().DropTable(&SchemaMeta{}); err != nil {
		t.Fatalf("drop schema_meta: %v", err)
	}
	if err := db.Create(&Channel{Name: "legacy", Type: "openai", BaseURL: "https://example.com", Models: []string{"m"}}).Error; err != nil {
		t.Fatalf("legacy channel: %v", err)
	}
	if err := MigrateWithBackup(db, path, "v1.0.0", nil); err != nil {
		t.Fatalf("MigrateWithBackup: %v", err)
	}
	if got := listBackups(t, filepath.Join(dir, "backups")); len(got) != 1 {
		t.Fatalf("legacy database should be backed up once, got %v", got)
	}
	if !db.Migrator().HasTable(&SchemaMeta{}) {
		t.Fatal("schema_meta should be recreated by migration")
	}
	if v, _ := GetSchemaMeta(db, SchemaMetaKeyAppVersion); v != "v1.0.0" {
		t.Fatalf("recorded version = %q", v)
	}
}

func TestSetSchemaMetaIsUpsert(t *testing.T) {
	db := newTestDB(t)
	if err := SetSchemaMeta(db, "k", "a"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := SetSchemaMeta(db, "k", "b"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var count int64
	db.Model(&SchemaMeta{}).Where("`key` = ?", "k").Count(&count)
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}
	if v, _ := GetSchemaMeta(db, "k"); v != "b" {
		t.Fatalf("value = %q, want b", v)
	}
	if v, err := GetSchemaMeta(db, "missing"); v != "" || err != nil {
		t.Fatalf("missing key should return empty without error, got %q / %v", v, err)
	}
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
