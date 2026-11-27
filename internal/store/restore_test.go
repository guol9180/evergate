package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func openMigrated(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return db
}

func channelNames(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var names []string
	if err := db.Model(&Channel{}).Order("name").Pluck("name", &names).Error; err != nil {
		t.Fatalf("pluck channels: %v", err)
	}
	return names
}

// 集成：备份 → 改数据 → 暂存恢复 → 启动应用 → 数据回到备份时刻；恢复前自动留下安全备份。
func TestBackupStageAndApplyRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evergate.db")
	db := openMigrated(t, path)
	if err := db.Create(&Channel{Name: "before", Type: "openai", BaseURL: "https://a", Models: []string{"m"}}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SetSchemaMeta(db, SchemaMetaKeyAppVersion, "v1.0.0"); err != nil {
		t.Fatalf("set version: %v", err)
	}

	name, err := BackupDatabase(db, path)
	if err != nil || !ValidBackupName(name) {
		t.Fatalf("backup = %q err=%v", name, err)
	}
	backups, err := ListBackups(path)
	if err != nil || len(backups) != 1 || backups[0].Name != name || backups[0].Version != "v1.0.0" || backups[0].Size == 0 {
		t.Fatalf("list = %+v err=%v", backups, err)
	}

	// 备份后修改数据
	if err := db.Create(&Channel{Name: "after", Type: "openai", BaseURL: "https://b", Models: []string{"m"}}).Error; err != nil {
		t.Fatalf("mutate: %v", err)
	}
	if err := db.Where("name = ?", "before").Delete(&Channel{}).Error; err != nil {
		t.Fatalf("mutate: %v", err)
	}
	if got := channelNames(t, db); strings.Join(got, ",") != "after" {
		t.Fatalf("mutated state = %v", got)
	}

	// 恢复：上传文件就是刚才那份备份
	safety, err := StageRestore(db, path, filepath.Join(BackupDir(path), name))
	if err != nil {
		t.Fatalf("stage restore: %v", err)
	}
	if !ValidBackupName(safety) {
		t.Fatalf("safety backup = %q", safety)
	}
	if _, err := os.Stat(path + restorePendingSuffix); err != nil {
		t.Fatalf("pending restore missing: %v", err)
	}
	if sqlDB, _ := db.DB(); sqlDB != nil {
		_ = sqlDB.Close()
	}

	applied, err := ApplyPendingRestore(path)
	if err != nil || !applied {
		t.Fatalf("apply restore: applied=%v err=%v", applied, err)
	}
	if _, err := os.Stat(path + ".pre-restore"); err != nil {
		t.Fatalf("pre-restore copy missing: %v", err)
	}

	restored := openMigrated(t, path)
	if got := channelNames(t, restored); strings.Join(got, ",") != "before" {
		t.Fatalf("restored state = %v, want [before]", got)
	}
	if v, _ := GetSchemaMeta(restored, SchemaMetaKeyAppVersion); v != "v1.0.0" {
		t.Fatalf("restored version = %q", v)
	}

	// 再次启动不应重复替换
	if applied, err := ApplyPendingRestore(path); err != nil || applied {
		t.Fatalf("second apply: applied=%v err=%v", applied, err)
	}
}

func TestBackupNameRejectsTraversalAndProbeRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"../evergate.db", "evergate.db", "evergate-x.db", "evergate-20260101T000000Z.db/..", ""} {
		if ValidBackupName(bad) {
			t.Fatalf("%q should be rejected", bad)
		}
		if _, err := BackupPath(filepath.Join("d", "x.db"), bad); err == nil {
			t.Fatalf("BackupPath(%q) should fail", bad)
		}
	}
	if !ValidBackupName("evergate-20260101T000000Z.db") || !ValidBackupName("evergate-20260101T000000Z-3.db") {
		t.Fatal("valid backup names rejected")
	}

	garbage := filepath.Join(t.TempDir(), "garbage.db")
	if err := os.WriteFile(garbage, []byte("not a sqlite file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeBackup(garbage); err == nil {
		t.Fatal("garbage file should fail probe")
	}
}

func TestDeleteBackupRemovesOnlyTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evergate.db")
	db := openMigrated(t, path)
	keep, err := BackupDatabase(db, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(BackupDir(path), "unrelated.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBackup(path, keep); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(BackupDir(path), keep)); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(BackupDir(path), "unrelated.txt")); err != nil {
		t.Fatalf("unrelated file was touched: %v", err)
	}
	if err := DeleteBackup(path, "../evergate.db"); err == nil {
		t.Fatal("traversal name must be rejected")
	}
}
