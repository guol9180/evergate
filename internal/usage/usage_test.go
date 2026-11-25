package usage

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "usage.db"))
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

func TestPriceManagerDefaultAndOverride(t *testing.T) {
	db := setupDB(t)
	mgr, err := Load(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := mgr.Calculate(1, "m", 1, 1); ok {
		t.Fatal("empty pricing should be unpriced")
	}
	if err := mgr.SetDefault(context.Background(), 0, 0); err != nil {
		t.Fatalf("set zero default: %v", err)
	}
	if micros, ok := mgr.Calculate(1, "m", 3, 2); !ok || micros != 0 {
		t.Fatalf("explicit zero price = %d %v, want 0 true", micros, ok)
	}
	if err := mgr.SetDefault(context.Background(), 1_000_000, 2_000_000); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if micros, ok := mgr.Calculate(1, "m", 3, 2); !ok || micros != 7 {
		t.Fatalf("default calc = %d %v, want 7 true", micros, ok)
	}
	if _, err := mgr.UpsertOverride(context.Background(), 1, "m", 10_000_000, 20_000_000); err != nil {
		t.Fatalf("override: %v", err)
	}
	if micros, ok := mgr.Calculate(1, "m", 3, 2); !ok || micros != 70 {
		t.Fatalf("override calc = %d %v, want 70 true", micros, ok)
	}
	if micros, ok := mgr.Calculate(2, "m", 3, 2); !ok || micros != 7 {
		t.Fatalf("other channel should use default: %d %v", micros, ok)
	}
	global, overrides, err := mgr.List(context.Background())
	if err != nil || global == nil || len(overrides) != 1 {
		t.Fatalf("list = %#v %#v %v", global, overrides, err)
	}
}

func TestRecomputeDayIdempotentAndReportReconciles(t *testing.T) {
	db := setupDB(t)
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	ch1 := &store.Channel{Name: "channel-a", Type: "openai-compatible", BaseURL: "http://a", Models: []string{"m"}, Enabled: true}
	ch2 := &store.Channel{Name: "channel-b", Type: "openai-compatible", BaseURL: "http://b", Models: []string{"m2"}, Enabled: true}
	if err := db.Create(ch1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(ch2).Error; err != nil {
		t.Fatal(err)
	}
	tok1 := &store.Token{Name: "token-a", KeyHash: "h1", Enabled: true}
	tok2 := &store.Token{Name: "token-b", KeyHash: "h2", Enabled: true}
	if err := db.Create(tok1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(tok2).Error; err != nil {
		t.Fatal(err)
	}
	logs := []store.RequestLog{
		{CreatedAt: day.Add(1 * time.Hour), ChannelID: ch1.ID, Model: "m", TokenID: tok1.ID, StatusCode: 200, PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5, CostMicros: 7, CostStatus: CostStatusPriced},
		{CreatedAt: day.Add(2 * time.Hour), ChannelID: ch1.ID, Model: "m", TokenID: tok1.ID, StatusCode: 200, CostStatus: CostStatusNoUsage},
		{CreatedAt: day.Add(30 * time.Minute), ChannelID: ch1.ID, Model: "m", TokenID: tok1.ID, StatusCode: 200, PromptTokens: 2, CostStatus: CostStatusUnpriced},
		{CreatedAt: day.Add(45 * time.Minute), ChannelID: ch1.ID, Model: "m", TokenID: tok1.ID, StatusCode: 200, PromptTokens: 1},
		{CreatedAt: day.Add(3 * time.Hour), ChannelID: ch2.ID, Model: "m2", TokenID: tok2.ID, StatusCode: 200, PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2, CostMicros: 3, CostStatus: CostStatusPriced},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := RecomputeDay(context.Background(), db, day); err != nil {
			t.Fatalf("recompute %d: %v", i, err)
		}
	}
	var daily []store.UsageDaily
	if err := db.Order("channel_id asc").Find(&daily).Error; err != nil {
		t.Fatal(err)
	}
	if len(daily) != 2 {
		t.Fatalf("daily rows = %d, want 2", len(daily))
	}
	if daily[0].Requests != 4 || daily[0].Success != 4 || daily[0].TokensIn != 6 || daily[0].CostMicros != 7 ||
		daily[0].PricedRequests != 1 || daily[0].UnpricedRequests != 1 ||
		daily[0].NoUsageRequests != 1 || daily[0].UnknownCostRequests != 1 {
		t.Fatalf("channel-a aggregate = %#v", daily[0])
	}

	report, err := BuildReport(db, ReportQuery{Start: day, End: day})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.Totals.Requests != 5 || report.Totals.Success != 5 || report.Totals.TokensIn != 7 ||
		report.Totals.TokensOut != 3 || report.Totals.CostMicros != 10 ||
		report.Totals.PricedRequests != 2 || report.Totals.UnpricedRequests != 1 ||
		report.Totals.NoUsageRequests != 1 || report.Totals.UnknownCostRequests != 1 {
		t.Fatalf("report totals = %#v", report.Totals)
	}
	if len(report.Daily) != 1 || report.Daily[0].Date != "2026-11-10" {
		t.Fatalf("daily report = %#v", report.Daily)
	}
	if len(report.TopChannels) != 2 || report.TopChannels[0].Name != "channel-a" {
		t.Fatalf("top channels = %#v", report.TopChannels)
	}
	if row := report.TopChannels[0]; row.PricedRequests+row.UnpricedRequests+row.NoUsageRequests+row.UnknownCostRequests != 4 {
		t.Fatalf("top channel cost status counts = %#v", row)
	}
	if len(report.TopTokens) != 2 || report.TopTokens[0].Name != "token-a" {
		t.Fatalf("top tokens = %#v", report.TopTokens)
	}

	for _, extra := range []struct {
		channelID int64
		tokenID   int64
	}{
		{0, 0},     // 未命中渠道 / 调试台无令牌
		{999, 888}, // 已删除的渠道 / 令牌
	} {
		if err := db.Create(&store.UsageDaily{Date: day, ChannelID: extra.channelID, Model: "m", TokenID: extra.tokenID, Requests: 1, Success: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	named, err := BuildReport(db, ReportQuery{Start: day, End: day})
	if err != nil {
		t.Fatal(err)
	}
	channelNames := map[string]bool{}
	for _, row := range named.TopChannels {
		channelNames[row.Name] = true
	}
	tokenNames := map[string]bool{}
	for _, row := range named.TopTokens {
		tokenNames[row.Name] = true
	}
	for _, want := range []string{"未命中渠道", "已删除 #999"} {
		if !channelNames[want] {
			t.Fatalf("top channels missing %q: %#v", want, named.TopChannels)
		}
	}
	for _, want := range []string{"调试台（无令牌）", "已删除 #888"} {
		if !tokenNames[want] {
			t.Fatalf("top tokens missing %q: %#v", want, named.TopTokens)
		}
	}

	filtered, err := BuildReport(db, ReportQuery{Start: day, End: day, ChannelID: ch1.ID, Model: "m", TokenID: tok1.ID})
	if err != nil {
		t.Fatalf("filtered report: %v", err)
	}
	if filtered.Totals.Requests != 4 || filtered.Totals.CostMicros != 7 {
		t.Fatalf("filtered totals = %#v", filtered.Totals)
	}
}

func TestRecomputeAllAndAggregatorStop(t *testing.T) {
	db := setupDB(t)
	day := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	if err := db.Create(&store.RequestLog{CreatedAt: day, Model: "m", StatusCode: 200, PromptTokens: 1}).Error; err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := RecomputeAll(context.Background(), db, logger); err != nil {
		t.Fatalf("recompute all: %v", err)
	}
	var count int64
	db.Model(&store.UsageDaily{}).Count(&count)
	if count != 1 {
		t.Fatalf("usage daily count = %d, want 1", count)
	}
	stop := StartAggregator(db, logger, time.Hour)
	stop()
	stop()
}
