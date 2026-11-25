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
	channelKinds := map[string]string{}
	for _, row := range named.TopChannels {
		channelKinds[row.NameKind+":"+row.Name] = row.NameKind
	}
	tokenKinds := map[string]string{}
	for _, row := range named.TopTokens {
		tokenKinds[row.NameKind+":"+row.Name] = row.NameKind
	}
	for _, want := range []string{"unattributed:", "deleted:#999"} {
		if _, ok := channelKinds[want]; !ok {
			t.Fatalf("top channels missing %q: %#v", want, named.TopChannels)
		}
	}
	for _, want := range []string{"playground:", "deleted:#888"} {
		if _, ok := tokenKinds[want]; !ok {
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

func TestPriceCatalogFallbackAndApplyPreset(t *testing.T) {
	db := setupDB(t)
	mgr, err := Load(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	preset := store.Channel{Name: "openai-main", Type: "openai", PresetID: "openai", BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-4o", "gpt-4o-mini", "unknown-model"}}
	custom := store.Channel{Name: "custom", Type: "openai-compatible", BaseURL: "https://example.com/v1", Models: []string{"gpt-4o"}}
	if err := db.Create(&preset).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatal(err)
	}
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}
	// 目录命中：1M 输入 gpt-4o = $2.5
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o", 1_000_000, 0); cost != 2_500_000 || source != PriceSourceCatalog {
		t.Fatalf("catalog price = %d %q, want 2500000 catalog", cost, source)
	}
	if _, source := mgr.CalculateWithSource(preset.ID, "unknown-model", 10, 10); source != "" {
		t.Fatalf("model missing from catalog must stay unpriced, got %q", source)
	}
	if _, source := mgr.CalculateWithSource(custom.ID, "gpt-4o", 10, 10); source != "" {
		t.Fatalf("custom channel must not use the preset catalog, got %q", source)
	}
	// 全局默认优先于目录，覆盖优先于全部
	if err := mgr.SetDefault(context.Background(), 1_000_000, 1_000_000); err != nil {
		t.Fatal(err)
	}
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o", 1_000_000, 0); cost != 1_000_000 || source != PriceSourceDefault {
		t.Fatalf("default should win over catalog: %d %q", cost, source)
	}
	if _, err := mgr.UpsertOverride(context.Background(), preset.ID, "gpt-4o", 3_000_000, 0); err != nil {
		t.Fatal(err)
	}
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o", 1_000_000, 0); cost != 3_000_000 || source != PriceSourceOverride {
		t.Fatalf("override should win: %d %q", cost, source)
	}
	prices := mgr.Effective(preset.ID, []string{"gpt-4o", "unknown-model", ""})
	if len(prices) != 2 || prices[0].Source != PriceSourceOverride || prices[1].Source != PriceSourceDefault {
		t.Fatalf("effective prices = %#v", prices)
	}
	// 一键写入：已覆盖的 gpt-4o 跳过，目录里的 gpt-4o-mini 新建，unknown-model 目录无价格跳过
	created, err := mgr.ApplyPreset(context.Background(), preset.ID, "openai", []string{"gpt-4o", "gpt-4o-mini", "unknown-model"})
	if err != nil || created != 1 {
		t.Fatalf("apply preset = %d %v, want 1", created, err)
	}
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o-mini", 1_000_000, 1_000_000); cost != 750_000 || source != PriceSourceOverride {
		t.Fatalf("applied gpt-4o-mini = %d %q, want 750000 override", cost, source)
	}
	if created, err := mgr.ApplyPreset(context.Background(), preset.ID, "openai", []string{"gpt-4o", "gpt-4o-mini"}); err != nil || created != 0 {
		t.Fatalf("second apply must be idempotent: %d %v", created, err)
	}
	asOf, _, entries, err := Catalog()
	if err != nil || asOf == "" || len(entries) == 0 {
		t.Fatalf("catalog = %q %d %v", asOf, len(entries), err)
	}
}
