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
	calc := func(channelID, in, out int64) (int64, bool) {
		micros, source := mgr.CalculateWithSource(channelID, "m", Tokens{Input: in, Output: out})
		return micros, source != ""
	}
	if _, ok := calc(1, 1, 1); ok {
		t.Fatal("empty pricing should be unpriced")
	}
	if err := mgr.SetDefault(context.Background(), PriceInput{Input: 0, Output: 0}); err != nil {
		t.Fatalf("set zero default: %v", err)
	}
	if micros, ok := calc(1, 3, 2); !ok || micros != 0 {
		t.Fatalf("explicit zero price = %d %v, want 0 true", micros, ok)
	}
	if err := mgr.SetDefault(context.Background(), PriceInput{Input: 1_000_000, Output: 2_000_000}); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if micros, ok := calc(1, 3, 2); !ok || micros != 7 {
		t.Fatalf("default calc = %d %v, want 7 true", micros, ok)
	}
	if _, err := mgr.UpsertOverride(context.Background(), 1, "m", PriceInput{Input: 10_000_000, Output: 20_000_000}); err != nil {
		t.Fatalf("override: %v", err)
	}
	if micros, ok := calc(1, 3, 2); !ok || micros != 70 {
		t.Fatalf("override calc = %d %v, want 70 true", micros, ok)
	}
	if micros, ok := calc(2, 3, 2); !ok || micros != 7 {
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
	// 小时聚合：日志落在 00 / 01 / 02 / 03 四个小时，渠道 a 占三个；全量重建两次验证幂等
	for i := 0; i < 2; i++ {
		if err := RecomputeAll(context.Background(), db, slog.Default()); err != nil {
			t.Fatalf("recompute %d: %v", i, err)
		}
	}
	var daily []store.UsageDaily
	if err := db.Order("channel_id asc, date asc").Find(&daily).Error; err != nil {
		t.Fatal(err)
	}
	if len(daily) != 4 {
		t.Fatalf("hourly rows = %d, want 4 (channel-a × 3 hours + channel-b × 1)", len(daily))
	}
	var sum store.UsageDaily
	for _, row := range daily {
		if row.ChannelID == ch1.ID {
			sum.Requests += row.Requests
			sum.Success += row.Success
			sum.TokensIn += row.TokensIn
			sum.CostMicros += row.CostMicros
			sum.PricedRequests += row.PricedRequests
			sum.UnpricedRequests += row.UnpricedRequests
			sum.NoUsageRequests += row.NoUsageRequests
			sum.UnknownCostRequests += row.UnknownCostRequests
		}
	}
	if sum.Requests != 4 || sum.Success != 4 || sum.TokensIn != 6 || sum.CostMicros != 7 ||
		sum.PricedRequests != 1 || sum.UnpricedRequests != 1 ||
		sum.NoUsageRequests != 1 || sum.UnknownCostRequests != 1 {
		t.Fatalf("channel-a aggregate = %#v", sum)
	}

	report, err := BuildReport(db, ReportQuery{StartDate: "2026-11-10", EndDate: "2026-11-10"})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.Totals.Requests != 5 || report.Totals.Success != 5 || report.Totals.TokensIn != 7 ||
		report.Totals.TokensOut != 3 || report.Totals.CostMicros != 10 ||
		report.Totals.PricedRequests != 2 || report.Totals.UnpricedRequests != 1 ||
		report.Totals.NoUsageRequests != 1 || report.Totals.UnknownCostRequests != 1 {
		t.Fatalf("report totals = %#v", report.Totals)
	}
	// 按小时返回，起点为 UTC 整点；总量与小时数都与日志一致
	if len(report.Hourly) != 4 || report.Hourly[0].Date != "2026-11-10T00:00:00Z" || report.Hourly[3].Date != "2026-11-10T03:00:00Z" {
		t.Fatalf("hourly report = %#v", report.Hourly)
	}
	// 「今天」按用户本地日：东八区的 11-10 覆盖 UTC 11-09 16:00 → 11-10 16:00，包含全部； UTC-8 的 11-10 则一条都没有
	if east, err := BuildReport(db, ReportQuery{StartDate: "2026-11-10", EndDate: "2026-11-10", TZOffset: 480}); err != nil || east.Totals.Requests != 5 {
		t.Fatalf("UTC+8 local day should include every request: %v %#v", err, east.Totals)
	}
	if west, err := BuildReport(db, ReportQuery{StartDate: "2026-11-10", EndDate: "2026-11-10", TZOffset: -480}); err != nil || west.Totals.Requests != 0 {
		t.Fatalf("UTC-8 local day should exclude them: %v %#v", err, west.Totals)
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
	named, err := BuildReport(db, ReportQuery{StartDate: "2026-11-10", EndDate: "2026-11-10"})
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

	filtered, err := BuildReport(db, ReportQuery{StartDate: "2026-11-10", EndDate: "2026-11-10", ChannelID: ch1.ID, Model: "m", TokenID: tok1.ID})
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
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o", Tokens{Input: 1_000_000, Output: 0}); cost != 2_500_000 || source != PriceSourceCatalog {
		t.Fatalf("catalog price = %d %q, want 2500000 catalog", cost, source)
	}
	if _, source := mgr.CalculateWithSource(preset.ID, "unknown-model", Tokens{Input: 10, Output: 10}); source != "" {
		t.Fatalf("model missing from catalog must stay unpriced, got %q", source)
	}
	if _, source := mgr.CalculateWithSource(custom.ID, "gpt-4o", Tokens{Input: 10, Output: 10}); source != "" {
		t.Fatalf("custom channel must not use the preset catalog, got %q", source)
	}
	// 全局默认优先于目录，覆盖优先于全部
	if err := mgr.SetDefault(context.Background(), PriceInput{Input: 1_000_000, Output: 1_000_000}); err != nil {
		t.Fatal(err)
	}
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o", Tokens{Input: 1_000_000, Output: 0}); cost != 1_000_000 || source != PriceSourceDefault {
		t.Fatalf("default should win over catalog: %d %q", cost, source)
	}
	if _, err := mgr.UpsertOverride(context.Background(), preset.ID, "gpt-4o", PriceInput{Input: 3_000_000, Output: 0}); err != nil {
		t.Fatal(err)
	}
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o", Tokens{Input: 1_000_000, Output: 0}); cost != 3_000_000 || source != PriceSourceOverride {
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
	if cost, source := mgr.CalculateWithSource(preset.ID, "gpt-4o-mini", Tokens{Input: 1_000_000, Output: 1_000_000}); cost != 750_000 || source != PriceSourceOverride {
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

func ptrOf(v int64) *int64 { return &v }

// 缓存计价规则：缓存价未设置时按输入价全价计；设置后普通输入 / 缓存读 / 缓存写分别计价；
// 更新时未提供缓存价（nil）保留原值，显式 0 才回到输入价。
func TestCachePricingRules(t *testing.T) {
	db := setupDB(t)
	mgr, err := Load(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ctx := context.Background()
	if err := mgr.SetDefault(ctx, PriceInput{Input: 1_000_000, Output: 2_000_000}); err != nil {
		t.Fatal(err)
	}
	tokens := Tokens{Input: 104, CacheRead: 80, CacheWrite: 4, Output: 10}
	assertCost := func(step string, channelID int64, model string, want int64, wantSource string) {
		t.Helper()
		cost, source := mgr.CalculateWithSource(channelID, model, tokens)
		if cost != want || source != wantSource {
			t.Fatalf("%s: cost = %d %q, want %d %q", step, cost, source, want, wantSource)
		}
	}
	// 未设置缓存价：104×$1 + 10×$2 = 124（缓存按全价，宁可高估）
	assertCost("unset cache price", 7, "m", 124, PriceSourceDefault)
	if _, err := mgr.UpsertOverride(ctx, 7, "m", PriceInput{
		Input: 1_000_000, Output: 2_000_000, CacheRead: ptrOf(100_000), CacheWrite: ptrOf(1_250_000),
	}); err != nil {
		t.Fatal(err)
	}
	// 普通 20×$1 + 缓存读 80×$0.1 + 缓存写 4×$1.25 + 输出 10×$2 = 53
	assertCost("cache prices set", 7, "m", 53, PriceSourceOverride)
	if _, err := mgr.UpsertOverride(ctx, 7, "m", PriceInput{Input: 2_000_000, Output: 2_000_000}); err != nil {
		t.Fatal(err)
	}
	// 未提供缓存价：保留原值，仅输入价变为 $2 → 40 + 8 + 5 + 20 = 73
	assertCost("nil keeps cache prices", 7, "m", 73, PriceSourceOverride)
	if _, err := mgr.UpsertOverride(ctx, 7, "m", PriceInput{
		Input: 2_000_000, Output: 2_000_000, CacheRead: ptrOf(0),
	}); err != nil {
		t.Fatal(err)
	}
	// 显式 0：缓存读回到输入价 80×$2，缓存写不变 → 40 + 160 + 5 + 20 = 225
	assertCost("explicit zero resets", 7, "m", 225, PriceSourceOverride)
	if _, err := mgr.UpsertOverride(ctx, 7, "m", PriceInput{Input: 1, Output: 1, CacheRead: ptrOf(-1)}); err == nil {
		t.Fatal("negative cache price must be rejected")
	}

	// 全局默认的缓存价同样生效：60×$1 + 40×$0.05 = 62
	if err := mgr.SetDefault(ctx, PriceInput{Input: 1_000_000, Output: 2_000_000, CacheRead: ptrOf(50_000)}); err != nil {
		t.Fatal(err)
	}
	if cost, source := mgr.CalculateWithSource(99, "other", Tokens{Input: 100, CacheRead: 40}); cost != 62 || source != PriceSourceDefault {
		t.Fatalf("default cache price = %d %q, want 62 default", cost, source)
	}
}

// 目录只在「没有全局默认价」时兜底；带缓存价的条目直接计价，一键写入时把缓存价一并落为覆盖。
func TestCatalogCachePrices(t *testing.T) {
	db := setupDB(t)
	mgr, err := Load(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	sonnet := store.Channel{Name: "sonnet", Type: "anthropic", PresetID: "anthropic", BaseURL: "https://api.anthropic.com", Models: []string{"claude-sonnet-4-5"}}
	flash := store.Channel{Name: "flash", Type: "gemini", PresetID: "gemini", BaseURL: "https://generativelanguage.googleapis.com", Models: []string{"gemini-2.5-flash"}}
	if err := db.Create(&sonnet).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&flash).Error; err != nil {
		t.Fatal(err)
	}
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}
	// 10 万×$3 + 90 万×$0.3 + 1000×$15 = 585000
	if cost, source := mgr.CalculateWithSource(sonnet.ID, "claude-sonnet-4-5", Tokens{Input: 1_000_000, CacheRead: 900_000, Output: 1000}); cost != 585_000 || source != PriceSourceCatalog {
		t.Fatalf("catalog cache cost = %d %q, want 585000 catalog", cost, source)
	}
	// 目录未给缓存价的条目（Gemini）仍按输入价全价计，不静默打折。
	if cost, source := mgr.CalculateWithSource(flash.ID, "gemini-2.5-flash", Tokens{Input: 1_000_000, CacheRead: 500_000}); cost != 300_000 || source != PriceSourceCatalog {
		t.Fatalf("catalog without cache price = %d %q, want 300000 at full input price", cost, source)
	}
	created, err := mgr.ApplyPreset(context.Background(), sonnet.ID, "anthropic", []string{"claude-sonnet-4-5"})
	if err != nil || created != 1 {
		t.Fatalf("apply preset = %d %v, want 1", created, err)
	}
	if cost, source := mgr.CalculateWithSource(sonnet.ID, "claude-sonnet-4-5", Tokens{Input: 1_000_000, CacheRead: 900_000, Output: 1000}); cost != 585_000 || source != PriceSourceOverride {
		t.Fatalf("applied preset cost = %d %q, want 585000 override", cost, source)
	}
	effective := mgr.Effective(sonnet.ID, []string{"claude-sonnet-4-5"})
	if len(effective) != 1 || effective[0].CacheReadMicrosPerMillion != 300_000 || effective[0].CacheWriteMicrosPerMillion != 3_750_000 {
		t.Fatalf("effective prices = %#v", effective)
	}
}

func TestBuildReportFiltersModelsAndMultiDay(t *testing.T) {
	db := setupDB(t)
	day1 := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	ch1 := &store.Channel{Name: "ch-1", Type: "openai-compatible", BaseURL: "http://a", Models: []string{"m1"}, Enabled: true}
	ch2 := &store.Channel{Name: "ch-2", Type: "openai-compatible", BaseURL: "http://b", Models: []string{"m2"}, Enabled: true}
	if err := db.Create(ch1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(ch2).Error; err != nil {
		t.Fatal(err)
	}
	tok1 := &store.Token{Name: "tok-1", KeyHash: "h1", Enabled: true}
	tok2 := &store.Token{Name: "tok-2", KeyHash: "h2", Enabled: true}
	if err := db.Create(tok1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(tok2).Error; err != nil {
		t.Fatal(err)
	}
	logs := []store.RequestLog{
		{CreatedAt: day1.Add(time.Hour), ChannelID: ch1.ID, Model: "m1", TokenID: tok1.ID, StatusCode: 200, PromptTokens: 10, CompletionTokens: 5, CostMicros: 100, CostStatus: CostStatusPriced},
		{CreatedAt: day1.Add(2 * time.Hour), ChannelID: ch2.ID, Model: "m2", TokenID: tok2.ID, StatusCode: 500, PromptTokens: 20, CostMicros: 200, CostStatus: CostStatusPriced},
		{CreatedAt: day2.Add(time.Hour), ChannelID: ch1.ID, Model: "m1", TokenID: tok1.ID, StatusCode: 200, PromptTokens: 30, CompletionTokens: 15, CostMicros: 300, CostStatus: CostStatusPriced},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if err := RecomputeAll(context.Background(), db, slog.Default()); err != nil {
		t.Fatal(err)
	}

	full, err := BuildReport(db, ReportQuery{StartDate: "2026-11-09", EndDate: "2026-11-10"})
	if err != nil {
		t.Fatal(err)
	}
	// 三个小时点；按日期前缀求和仍是每天 300
	if len(full.Hourly) != 3 {
		t.Fatalf("hourly buckets = %#v", full.Hourly)
	}
	perDay := map[string]int64{}
	for _, point := range full.Hourly {
		perDay[point.Date[:10]] += point.CostMicros
	}
	if perDay["2026-11-09"] != 300 || perDay["2026-11-10"] != 300 {
		t.Fatalf("daily cost = %#v", perDay)
	}
	if len(full.TopModels) != 2 || full.TopModels[0].Name != "m1" || full.TopModels[0].CostMicros != 400 {
		t.Fatalf("top models = %#v", full.TopModels)
	}

	byChannel, err := BuildReport(db, ReportQuery{StartDate: "2026-11-09", EndDate: "2026-11-10", ChannelID: ch2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if byChannel.Totals.Requests != 1 || byChannel.Totals.Success != 0 || byChannel.Totals.CostMicros != 200 {
		t.Fatalf("channel filter totals = %#v", byChannel.Totals)
	}
	if len(byChannel.TopChannels) != 1 || byChannel.TopChannels[0].Name != "ch-2" {
		t.Fatalf("channel filter rows = %#v", byChannel.TopChannels)
	}

	byModel, err := BuildReport(db, ReportQuery{StartDate: "2026-11-09", EndDate: "2026-11-10", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if byModel.Totals.Requests != 2 || len(byModel.TopModels) != 1 || byModel.TopModels[0].Name != "m1" {
		t.Fatalf("model filter = %#v %#v", byModel.Totals, byModel.TopModels)
	}

	byToken, err := BuildReport(db, ReportQuery{StartDate: "2026-11-09", EndDate: "2026-11-10", TokenID: tok2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if byToken.Totals.Requests != 1 || len(byToken.TopTokens) != 1 || byToken.TopTokens[0].Name != "tok-2" {
		t.Fatalf("token filter = %#v %#v", byToken.Totals, byToken.TopTokens)
	}
}
