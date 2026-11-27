package store

import (
	"path/filepath"
	"testing"
	"time"
)

// PRAGMA synchronous 取值：1 = NORMAL，2 = FULL。
func TestOpenAppliesSynchronousMode(t *testing.T) {
	cases := []struct {
		setting string
		want    int
	}{
		{"", 2},
		{"full", 2},
		{"NORMAL", 1},
	}
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "sync.db")
		db, err := OpenWithOptions(path, Options{Synchronous: tc.setting})
		if err != nil {
			t.Fatalf("open with %q: %v", tc.setting, err)
		}
		var value int
		if err := db.Raw("PRAGMA synchronous").Scan(&value).Error; err != nil {
			t.Fatalf("read pragma: %v", err)
		}
		if sqlDB, err := db.DB(); err == nil {
			t.Cleanup(func() { _ = sqlDB.Close() })
		}
		if value != tc.want {
			t.Fatalf("synchronous %q = %d, want %d", tc.setting, value, tc.want)
		}
	}
}

func TestOpenRejectsUnknownSynchronousMode(t *testing.T) {
	if _, err := OpenWithOptions(filepath.Join(t.TempDir(), "x.db"), Options{Synchronous: "off"}); err == nil {
		t.Fatal("unknown synchronous mode should be rejected")
	}
}

// 对比 FULL 与 NORMAL 下单行提交（每次写入自动提交）的延迟。
func BenchmarkInsertCommitSyncModes(b *testing.B) {
	for _, mode := range []string{"full", "normal"} {
		b.Run(mode, func(b *testing.B) {
			db, err := OpenWithOptions(filepath.Join(b.TempDir(), "bench.db"), Options{Synchronous: mode})
			if err != nil {
				b.Fatalf("open: %v", err)
			}
			if err := Migrate(db); err != nil {
				b.Fatalf("migrate: %v", err)
			}
			if sqlDB, err := db.DB(); err == nil {
				b.Cleanup(func() { _ = sqlDB.Close() })
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				row := &RequestLog{
					Model: "bench", StatusCode: 200, IP: "127.0.0.1",
					CreatedAt: time.Unix(int64(i), 0).UTC(),
				}
				if err := db.Create(row).Error; err != nil {
					b.Fatalf("insert: %v", err)
				}
			}
		})
	}
}
