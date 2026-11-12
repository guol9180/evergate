package settings

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/store"
)

func setup(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoadSeedsOnceFromYAML(t *testing.T) {
	db := setup(t)
	cfg := config.Default()
	cfg.Storage.LogRetentionDays = 17
	now := time.Date(2026, 11, 11, 7, 0, 0, 0, time.UTC)
	mgr, err := Load(db, cfg, func() time.Time { return now })
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	first := mgr.Snapshot()
	if first.Source != SourceYAML || first.LogRetentionDays != 17 || !first.InitializedAt.Equal(now) {
		t.Fatalf("首次种子不正确: %#v", first)
	}

	changed := config.Default()
	changed.Storage.LogRetentionDays = 99
	mgr2, err := Load(db, changed, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	second := mgr2.Snapshot()
	if second.LogRetentionDays != 17 || second.Source != SourceYAML {
		t.Fatalf("SQLite 已存在时不应被 YAML 覆盖: %#v", second)
	}
}

func TestUpdateValidatesBeforePersistenceAndPublish(t *testing.T) {
	db := setup(t)
	mgr, err := Load(db, config.Default(), func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	current := mgr.Snapshot()

	invalid := current
	invalid.ConnectTimeoutMS = 0
	if _, err := mgr.Update(invalid); err == nil {
		t.Fatal("非法连接超时应返回错误")
	}
	var row store.RuntimeSetting
	if err := db.First(&row, "id = ?", 1).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.ConnectTimeoutMS != current.ConnectTimeoutMS || row.Source != SourceYAML {
		t.Fatalf("非法更新不应落库: %#v", row)
	}
	if got := mgr.Snapshot(); got.ConnectTimeoutMS != current.ConnectTimeoutMS {
		t.Fatalf("非法更新不应发布快照: %#v", got)
	}

	next := current
	next.FirstTokenTimeoutMS = 45_000
	next.CooldownFailThreshold = 4
	updated, err := mgr.Update(next)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.FirstTokenTimeoutMS != 45_000 || updated.CooldownFailThreshold != 4 || updated.Source != SourceDatabase {
		t.Fatalf("更新结果不正确: %#v", updated)
	}
	if err := db.First(&row, "id = ?", 1).Error; err != nil {
		t.Fatalf("read updated row: %v", err)
	}
	if row.FirstTokenTimeoutMS != 45_000 || row.Source != SourceDatabase {
		t.Fatalf("更新未持久化: %#v", row)
	}
}

func TestUpdateNotifiesSubscribers(t *testing.T) {
	db := setup(t)
	mgr, err := Load(db, config.Default(), func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	changes, stop := mgr.Subscribe()
	defer stop()

	next := mgr.Snapshot()
	next.LogRetentionDays = 21
	if _, err := mgr.Update(next); err != nil {
		t.Fatalf("update: %v", err)
	}
	select {
	case <-changes:
	default:
		t.Fatal("设置更新后应通知后台任务")
	}
	stop()
	mgr.notify()
	select {
	case <-changes:
		t.Fatal("取消订阅后不应继续收到通知")
	default:
	}
}

func TestCleanupUsesLatestRetentionSetting(t *testing.T) {
	db := setup(t)
	mgr, err := Load(db, config.Default(), func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	now := time.Now().UTC()
	old := &store.RequestLog{CreatedAt: now.AddDate(0, 0, -3), Model: "old"}
	fresh := &store.RequestLog{CreatedAt: now.Add(-time.Minute), Model: "fresh"}
	if err := db.Create([]*store.RequestLog{old, fresh}).Error; err != nil {
		t.Fatalf("seed logs: %v", err)
	}

	next := mgr.Snapshot()
	next.LogRetentionDays = 1
	if _, err := mgr.Update(next); err != nil {
		t.Fatalf("update retention: %v", err)
	}
	cleanupOnce(db, mgr, discardLogger())
	var count int64
	if err := db.Model(&store.RequestLog{}).Count(&count).Error; err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("保留清理后的日志数 = %d, want 1", count)
	}
	var kept store.RequestLog
	if err := db.First(&kept).Error; err != nil || kept.Model != "fresh" {
		t.Fatalf("保留日志不正确: model=%q err=%v", kept.Model, err)
	}
}
