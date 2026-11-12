package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

// StartAggregator 启动 UTC 日聚合任务。启动时重建全部现存请求日志日期，
// 运行中每分钟重建今天/昨天；stop 会等待 goroutine 并再重建一次今天。
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
				if err := RecomputeDay(shutdownCtx, db, utcDay(time.Now())); err != nil {
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
	today := utcDay(time.Now())
	for _, day := range []time.Time{today.AddDate(0, 0, -1), today} {
		if err := RecomputeDay(ctx, db, day); err != nil {
			logger.Error("usage aggregate day rebuild failed", slog.Time("date", day), slog.Any("error", err))
		}
	}
}

func RecomputeAll(ctx context.Context, db *gorm.DB, logger *slog.Logger) error {
	var rows []struct {
		Day string `gorm:"column:day"`
	}
	if err := db.WithContext(ctx).
		Raw("SELECT DISTINCT date(created_at) AS day FROM request_logs").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		day, err := time.ParseInLocation("2006-01-02", row.Day, time.UTC)
		if err != nil {
			logger.Warn("skip invalid usage aggregate date", slog.String("date", row.Day))
			continue
		}
		if err := RecomputeDay(ctx, db, day); err != nil {
			return err
		}
	}
	return nil
}

type aggregateRow struct {
	ChannelID        int64  `gorm:"column:channel_id"`
	Model            string `gorm:"column:model"`
	TokenID          int64  `gorm:"column:token_id"`
	Requests         int64  `gorm:"column:requests"`
	Success          int64  `gorm:"column:success"`
	TokensIn         int64  `gorm:"column:tokens_in"`
	TokensOut        int64  `gorm:"column:tokens_out"`
	CostMicros       int64  `gorm:"column:cost_micros"`
	PricedRequests   int64  `gorm:"column:priced_requests"`
	UnpricedRequests int64  `gorm:"column:unpriced_requests"`
}

// RecomputeDay 从 request_logs 全量重建指定 UTC 日，事务内先删后插，保证幂等。
func RecomputeDay(ctx context.Context, db *gorm.DB, day time.Time) error {
	day = utcDay(day)
	next := day.AddDate(0, 0, 1)
	var rows []aggregateRow
	if err := db.WithContext(ctx).
		Model(&store.RequestLog{}).
		Select(`
			channel_id, model, token_id,
			COUNT(*) AS requests,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END), 0) AS success,
			COALESCE(SUM(prompt_tokens), 0) AS tokens_in,
			COALESCE(SUM(completion_tokens), 0) AS tokens_out,
			COALESCE(SUM(cost_micros), 0) AS cost_micros,
			COALESCE(SUM(CASE WHEN cost_status = 'priced' THEN 1 ELSE 0 END), 0) AS priced_requests,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 AND cost_status <> 'priced' THEN 1 ELSE 0 END), 0) AS unpriced_requests
		`).
		Where("created_at >= ? AND created_at < ?", day, next).
		Group("channel_id, model, token_id").
		Scan(&rows).Error; err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("date = ?", day).Delete(&store.UsageDaily{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		items := make([]store.UsageDaily, 0, len(rows))
		for _, row := range rows {
			items = append(items, store.UsageDaily{
				Date: day, ChannelID: row.ChannelID, Model: row.Model, TokenID: row.TokenID,
				Requests: row.Requests, Success: row.Success,
				TokensIn: row.TokensIn, TokensOut: row.TokensOut,
				CostMicros: row.CostMicros, Cost: float64(row.CostMicros) / 1_000_000,
				PricedRequests: row.PricedRequests, UnpricedRequests: row.UnpricedRequests,
			})
		}
		return tx.CreateInBatches(&items, 200).Error
	})
}

func utcDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
