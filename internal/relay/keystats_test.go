package relay

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

func newKeyStatsFixture(tb testing.TB) (*gorm.DB, int64) {
	tb.Helper()
	db, err := store.Open(filepath.Join(tb.TempDir(), "keystats.db"))
	if err != nil {
		tb.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		tb.Fatalf("migrate: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		tb.Cleanup(func() { _ = sqlDB.Close() })
	}
	ch := &store.Channel{Name: "stats", Type: "openai-compatible", BaseURL: "http://127.0.0.1:1/v1", Models: []string{"m"}, Enabled: true}
	if err := db.Create(ch).Error; err != nil {
		tb.Fatalf("create channel: %v", err)
	}
	key := &store.ChannelKey{ChannelID: ch.ID, KeyCipher: "cipher", Enabled: true, Weight: 1, State: store.KeyStateActive}
	if err := db.Create(key).Error; err != nil {
		tb.Fatalf("create key: %v", err)
	}
	return db, key.ID
}

func TestKeyStatsWriterMergesAndFlushes(t *testing.T) {
	db, keyID := newKeyStatsFixture(t)
	w := newKeyStatsWriter(db, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)

	w.Add(keyID, true, "")
	w.Add(keyID, true, "")
	w.Add(keyID, false, "boom")
	w.Flush()

	var row store.ChannelKey
	if err := db.First(&row, keyID).Error; err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if row.SuccessCount != 2 || row.FailCount != 1 || row.LastError != "boom" || row.LastUsedAt == nil {
		t.Fatalf("merged stats = %#v", row)
	}

	// 成功会把 last_error 清空（与逐条写入的最终状态一致）。
	w.Add(keyID, true, "")
	w.Flush()
	if err := db.First(&row, keyID).Error; err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if row.SuccessCount != 3 || row.FailCount != 1 || row.LastError != "" {
		t.Fatalf("stats after success = %#v", row)
	}
	w.Close()
}

func TestKeyStatsWriterCloseFlushesPending(t *testing.T) {
	db, keyID := newKeyStatsFixture(t)
	w := newKeyStatsWriter(db, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	w.Add(keyID, false, "offline")
	w.Close() // 停机会停协程并刷掉最后一批

	var row store.ChannelKey
	if err := db.First(&row, keyID).Error; err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if row.FailCount != 1 || row.LastError != "offline" {
		t.Fatalf("close did not flush pending stats: %#v", row)
	}
}

// 对比旧路径（每次尝试同步 UPDATE）与现在（只入队）的单次成本。
func BenchmarkKeyStatsPersistence(b *testing.B) {
	db, keyID := newKeyStatsFixture(b)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	b.Run("sync-update", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			err := db.Model(&store.ChannelKey{}).Where("id = ?", keyID).Updates(map[string]any{
				"success_count": gorm.Expr("success_count + 1"),
				"last_used_at":  time.Now().UTC(),
				"last_error":    "",
			}).Error
			if err != nil {
				b.Fatalf("update: %v", err)
			}
		}
	})

	b.Run("async-enqueue", func(b *testing.B) {
		w := newKeyStatsWriter(db, logger, time.Now)
		defer w.Close()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			w.Add(keyID, true, "")
		}
		b.StopTimer()
		w.Flush()
	})
}
