package usage

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"evergate/internal/store"
)

// 首 token 延迟：按模型 / 渠道给均值与最近秩 P50，按小时给均值；失败请求与 0 延迟不参与。
func TestReportFirstTokenLatencyByModelChannelAndHour(t *testing.T) {
	db := setupDB(t)
	hour := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	ch1 := &store.Channel{Name: "fast", Type: "openai-compatible", BaseURL: "http://a", Models: []string{"m1"}, Enabled: true}
	ch2 := &store.Channel{Name: "slow", Type: "openai-compatible", BaseURL: "http://b", Models: []string{"m2"}, Enabled: true}
	if err := db.Create(ch1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(ch2).Error; err != nil {
		t.Fatal(err)
	}
	log := func(channel int64, model string, offset time.Duration, status, firstToken int) store.RequestLog {
		return store.RequestLog{
			CreatedAt: hour.Add(offset), ChannelID: channel, Model: model, StatusCode: status,
			FirstTokenMS: int64(firstToken), PromptTokens: 1, CompletionTokens: 1, CostStatus: CostStatusPriced,
		}
	}
	logs := []store.RequestLog{
		// m1: 100 / 300 / 500 → avg 300, P50 (最近秩, n=3 → rank 2) = 300
		log(ch1.ID, "m1", 0, 200, 100),
		log(ch1.ID, "m1", 10*time.Minute, 200, 300),
		log(ch1.ID, "m1", 20*time.Minute, 200, 500),
		// m2: 900 / 1100 → avg 1000, P50 (n=2 → rank 1) = 900
		log(ch2.ID, "m2", 30*time.Minute, 200, 900),
		log(ch2.ID, "m2", 40*time.Minute, 200, 1100),
		// 失败请求与无首 token 样本的请求不计入延迟
		log(ch1.ID, "m1", 50*time.Minute, 500, 9999),
		log(ch1.ID, "m1", 55*time.Minute, 200, 0),
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if err := RecomputeAll(context.Background(), db, slog.Default()); err != nil {
		t.Fatal(err)
	}

	report, err := BuildReport(db, ReportQuery{StartDate: "2026-11-20", EndDate: "2026-11-20"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]DimensionRow{}
	for _, row := range report.TopModels {
		byName[row.Name] = row
	}
	m1, m2 := byName["m1"], byName["m2"]
	if m1.FirstTokenSamples != 3 || avg(m1) != 300 || p50(m1) != 300 {
		t.Fatalf("m1 latency = samples %d avg %v p50 %v", m1.FirstTokenSamples, m1.FirstTokenAvgMS, m1.FirstTokenP50MS)
	}
	if m2.FirstTokenSamples != 2 || avg(m2) != 1000 || p50(m2) != 900 {
		t.Fatalf("m2 latency = samples %d avg %v p50 %v", m2.FirstTokenSamples, m2.FirstTokenAvgMS, m2.FirstTokenP50MS)
	}

	chByName := map[string]DimensionRow{}
	for _, row := range report.TopChannels {
		chByName[row.Name] = row
	}
	if chByName["fast"].FirstTokenSamples != 3 || chByName["slow"].FirstTokenSamples != 2 {
		t.Fatalf("channel latency = %#v", report.TopChannels)
	}

	// 两个小时点：10:00 有 5 个样本 (100+300+500+900+1100)/5 = 580；11:00 的两条都不计入 → nil
	if len(report.Hourly) != 1 {
		t.Fatalf("hourly buckets = %#v", report.Hourly)
	}
	// RecomputeAll 把 10:xx 全部聚合到 10:00；50/55 分钟的请求也在此小时但无样本，不拉低均值
	if report.Hourly[0].FirstTokenAvgMS == nil || *report.Hourly[0].FirstTokenAvgMS != 580 {
		t.Fatalf("hourly avg = %v", report.Hourly[0].FirstTokenAvgMS)
	}

	// 筛选只看 m2 时，延迟统计也只看 m2
	onlyM2, err := BuildReport(db, ReportQuery{StartDate: "2026-11-20", EndDate: "2026-11-20", Model: "m2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyM2.TopModels) != 1 || avg(onlyM2.TopModels[0]) != 1000 {
		t.Fatalf("filtered latency = %#v", onlyM2.TopModels)
	}
}

func avg(row DimensionRow) float64 {
	if row.FirstTokenAvgMS == nil {
		return -1
	}
	return *row.FirstTokenAvgMS
}

func p50(row DimensionRow) int64 {
	if row.FirstTokenP50MS == nil {
		return -1
	}
	return *row.FirstTokenP50MS
}
