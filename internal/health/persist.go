// Package health 承担渠道健康管理后台任务（任务书 4.5⑦）。
// 当前实现 cooldown 状态机的持久化与恢复（验收 #8：重启后冷却不丢）；
// 主动探测任务在 P1 接入。
package health

import (
	"log/slog"
	"time"

	"gorm.io/gorm"

	"evergate/internal/pool"
	"evergate/internal/store"
)

// RestoreCooldowns 启动时从 channel_keys 恢复状态机快照。
func RestoreCooldowns(db *gorm.DB, m *pool.Manager) error {
	var keys []*store.ChannelKey
	if err := db.Where("state <> ?", store.KeyStateActive).Find(&keys).Error; err != nil {
		return err
	}
	snaps := make([]pool.KeySnapshot, 0, len(keys))
	for _, k := range keys {
		var until time.Time
		if k.CooldownUntil != nil {
			until = *k.CooldownUntil
		}
		snaps = append(snaps, pool.KeySnapshot{
			KeyID:            k.ID,
			State:            k.State,
			ConsecutiveFails: k.ConsecutiveFails,
			CooldownUntil:    until,
			Level:            k.Level,
		})
	}
	m.StateMachine().Restore(snaps)
	return nil
}

// StartCooldownPersister 定期把状态机快照写回 channel_keys，返回停止函数。
func StartCooldownPersister(db *gorm.DB, m *pool.Manager, logger *slog.Logger, interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				persistOnce(db, m, logger)
			case <-done:
				persistOnce(db, m, logger) // 停机前刷一次，优雅退出不丢状态
				return
			}
		}
	}()
	return func() { close(done) }
}

func persistOnce(db *gorm.DB, m *pool.Manager, logger *slog.Logger) {
	for _, s := range m.StateMachine().Snapshot() {
		updates := map[string]any{
			"state":             s.State,
			"consecutive_fails": s.ConsecutiveFails,
			"cooldown_until":    s.CooldownUntil,
			"level":             s.Level,
		}
		if err := db.Model(&store.ChannelKey{}).Where("id = ?", s.KeyID).Updates(updates).Error; err != nil {
			logger.Error("persist key state failed", slog.Int64("key_id", s.KeyID), slog.Any("error", err))
		}
	}
}
