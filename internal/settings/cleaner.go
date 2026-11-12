package settings

import (
	"log/slog"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

// StartLogRetentionCleaner 定期按最新快照清理过期请求日志。
// 返回 stop 函数会先执行一次清理，优雅停机尽量不留下刚过期的数据。
func StartLogRetentionCleaner(db *gorm.DB, m *Manager, logger *slog.Logger, interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	if m == nil {
		return func() {}
	}
	done := make(chan struct{})
	changes, unsubscribe := m.Subscribe()
	go func() {
		cleanupOnce(db, m, logger)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cleanupOnce(db, m, logger)
			case <-changes:
				cleanupOnce(db, m, logger)
			case <-done:
				cleanupOnce(db, m, logger)
				return
			}
		}
	}()
	return func() {
		close(done)
		unsubscribe()
	}
}

func cleanupOnce(db *gorm.DB, m *Manager, logger *slog.Logger) {
	if db == nil || m == nil {
		return
	}
	snapshot := m.Snapshot()
	cutoff := time.Now().UTC().AddDate(0, 0, -snapshot.LogRetentionDays)
	result := db.Where("created_at < ?", cutoff).Delete(&store.RequestLog{})
	if result.Error != nil {
		logger.Error("clean expired request logs failed", slog.Any("error", result.Error))
		return
	}
	if result.RowsAffected > 0 {
		logger.Info("cleaned expired request logs",
			slog.Int64("count", result.RowsAffected),
			slog.Time("cutoff", cutoff),
		)
	}
}
