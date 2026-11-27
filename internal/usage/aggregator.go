package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

// StartAggregator 启动 UTC 小时聚合任务。启动时清空派生表并按小时重建全部现存请求日志
// （也是旧日级数据迁移：总量来自 request_logs，不会变），运行中每分钟重建当前/上一小时；
// stop 会等待 goroutine 并再重建一次当前小时。展示层按本地日分桶，见 BuildReport。
func StartAggregator(db *gorm.DB, logger *slog.Logger, interval time.Duration) (stop func()) {
	if db == nil {
		return func() {}
	}
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = time.Minute
	}
	stopCh := make(chan struct{})
	done := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		defer close(done)
		ctx := context.Background()
		if err := RecomputeAll(ctx, db, logger); err != nil {
			logger.Error("usage aggregate startup rebuild failed", slog.Any("error", err))
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				recomputeRecent(ctx, db, logger)
			case <-stopCh:
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := RecomputeHour(shutdownCtx, db, time.Now()); err != nil {
					logger.Error("usage aggregate shutdown rebuild failed", slog.Any("error", err))
				}
				cancel()
				return
			}
		}
	}()
	return func() {
		stopOnce.Do(func() {
			close(stopCh)
			<-done
		})
	}
}

func recomputeRecent(ctx context.Context, db *gorm.DB, logger *slog.Logger) {
	now := time.Now()
	for _, hour := range []time.Time{now.Add(-time.Hour), now} {
		if err := RecomputeHour(ctx, db, hour); err != nil {
			logger.Error("usage aggregate hour rebuild failed", slog.Time("hour", utcHour(hour)), slog.Any("error", err))
		}
	}
}

func RecomputeAll(ctx context.Context, db *gorm.DB, logger *slog.Logger) error {
	var rows []struct {
		Hour string `gorm:"column:hour"`
	}
	if err := db.WithContext(ctx).
		Raw("SELECT DISTINCT strftime('%Y-%m-%dT%H', created_at) AS hour FROM request_logs").
		Scan(&rows).Error; err != nil {
		return err
	}
	// 派生表整体重建：旧版本按 UTC 日落库的行在这里被清掉，避免与新小时行叠加重复计数
	if err := db.WithContext(ctx).Where("1 = 1").Delete(&store.UsageDaily{}).Error; err != nil {
		return err
	}
	for _, row := range rows {
		hour, err := time.ParseInLocation("2006-01-02T15", row.Hour, time.UTC)
		if err != nil {
			logger.Warn("skip invalid usage aggregate hour", slog.String("hour", row.Hour))
			continue
		}
		if err := RecomputeHour(ctx, db, hour); err != nil {
			return err
		}
	}
	return nil
}

type aggregateRow struct {
	ChannelID           int64  `gorm:"column:channel_id"`
	Model               string `gorm:"column:model"`
	TokenID             int64  `gorm:"column:token_id"`
	Requests            int64  `gorm:"column:requests"`
	Success             int64  `gorm:"column:success"`
	TokensIn            int64  `gorm:"column:tokens_in"`
	TokensOut           int64  `gorm:"column:tokens_out"`
	CacheReadTokens     int64  `gorm:"column:cache_read_tokens"`
	CacheWriteTokens    int64  `gorm:"column:cache_write_tokens"`
	CostMicros          int64  `gorm:"column:cost_micros"`
	PricedRequests      int64  `gorm:"column:priced_requests"`
	UnpricedRequests    int64  `gorm:"column:unpriced_requests"`
	NoUsageRequests     int64  `gorm:"column:no_usage_requests"`
	UnknownCostRequests int64  `gorm:"column:unknown_cost_requests"`
}

// RecomputeHour 从 request_logs 全量重建指定 UTC 小时（按渠道 / 模型 / 令牌），事务内先删后插，保证幂等。
func RecomputeHour(ctx context.Context, db *gorm.DB, hour time.Time) error {
	hour = utcHour(hour)
	next := hour.Add(time.Hour)
	var rows []aggregateRow
	if err := db.WithContext(ctx).
		Model(&store.RequestLog{}).
		Select(`
			channel_id, model, token_id,
			COUNT(*) AS requests,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END), 0) AS success,
			COALESCE(SUM(prompt_tokens), 0) AS tokens_in,
			COALESCE(SUM(completion_tokens), 0) AS tokens_out,
			COALESCE(SUM(cached_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_write_tokens), 0) AS cache_write_tokens,
			COALESCE(SUM(cost_micros), 0) AS cost_micros,
			COALESCE(SUM(CASE WHEN cost_status = 'priced' THEN 1 ELSE 0 END), 0) AS priced_requests,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 AND cost_status = 'unpriced' THEN 1 ELSE 0 END), 0) AS unpriced_requests,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 AND cost_status = 'no_usage' THEN 1 ELSE 0 END), 0) AS no_usage_requests,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 AND cost_status NOT IN ('priced', 'unpriced', 'no_usage') THEN 1 ELSE 0 END), 0) AS unknown_cost_requests
		`).
		Where("created_at >= ? AND created_at < ?", hour, next).
		Group("channel_id, model, token_id").
		Scan(&rows).Error; err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("date = ?", hour).Delete(&store.UsageDaily{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		items := make([]store.UsageDaily, 0, len(rows))
		for _, row := range rows {
			items = append(items, store.UsageDaily{
				Date: hour, ChannelID: row.ChannelID, Model: row.Model, TokenID: row.TokenID,
				Requests: row.Requests, Success: row.Success,
				TokensIn: row.TokensIn, TokensOut: row.TokensOut,
				CacheReadTokens: row.CacheReadTokens, CacheWriteTokens: row.CacheWriteTokens,
				CostMicros: row.CostMicros, Cost: float64(row.CostMicros) / 1_000_000,
				PricedRequests: row.PricedRequests, UnpricedRequests: row.UnpricedRequests,
				NoUsageRequests: row.NoUsageRequests, UnknownCostRequests: row.UnknownCostRequests,
			})
		}
		return tx.CreateInBatches(&items, 200).Error
	})
}

func utcHour(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
}
