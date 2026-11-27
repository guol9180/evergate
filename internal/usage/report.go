package usage

import (
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

type ReportQuery struct {
	// StartDate / EndDate 为用户所选本地日（YYYY-MM-DD），TZOffset 为该本地时区相对 UTC 的分钟数（东八区 480）；
	// 聚合表按 UTC 小时落库，这里把本地日区间换算成 UTC 瞬时区间，保证「今天」口径与概览页一致。
	StartDate string
	EndDate   string
	TZOffset  int
	ChannelID int64
	Model     string
	TokenID   int64
}

// DayBounds 把本地日区间换算为 [start, endExclusive) 的 UTC 瞬时。
func (q ReportQuery) DayBounds() (time.Time, time.Time, error) {
	loc := time.FixedZone("local", q.TZOffset*60)
	start, err := time.ParseInLocation("2006-01-02", q.StartDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.ParseInLocation("2006-01-02", q.EndDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start.UTC(), end.AddDate(0, 0, 1).UTC(), nil
}

type ReportTotals struct {
	Requests            int64   `json:"requests"`
	Success             int64   `json:"success"`
	TokensIn            int64   `json:"tokens_in"`
	TokensOut           int64   `json:"tokens_out"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CostMicros          int64   `json:"cost_micros"`
	Cost                float64 `json:"cost"`
	PricedRequests      int64   `json:"priced_requests"`
	UnpricedRequests    int64   `json:"unpriced_requests"`
	NoUsageRequests     int64   `json:"no_usage_requests"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

type DailyPoint struct {
	Date            string  `json:"hour"` // UTC 小时起点（RFC3339），前端按本地日分桶
	Requests        int64   `json:"requests"`
	Success         int64   `json:"success"`
	TokensIn        int64   `json:"tokens_in"`
	TokensOut       int64   `json:"tokens_out"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	CostMicros      int64   `json:"cost_micros"`
	Cost            float64 `json:"cost"`
	Priced          int64   `json:"priced_requests"`
	Unpriced        int64   `json:"unpriced_requests"`
	NoUsage         int64   `json:"no_usage_requests"`
	Unknown         int64   `json:"unknown_cost_requests"`
}

type DimensionRow struct {
	ID                  int64   `json:"id"`
	Name                string  `json:"name"`
	NameKind            string  `json:"name_kind,omitempty"` // synthetic: unattributed（未命中渠道）/ playground（无令牌）/ deleted（已删除）
	Requests            int64   `json:"requests"`
	Success             int64   `json:"success"`
	TokensIn            int64   `json:"tokens_in"`
	TokensOut           int64   `json:"tokens_out"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CostMicros          int64   `json:"cost_micros"`
	Cost                float64 `json:"cost"`
	PricedRequests      int64   `json:"priced_requests"`
	UnpricedRequests    int64   `json:"unpriced_requests"`
	NoUsageRequests     int64   `json:"no_usage_requests"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

type Report struct {
	Start       string         `json:"start"`
	End         string         `json:"end"`
	Totals      ReportTotals   `json:"totals"`
	Hourly      []DailyPoint   `json:"hourly"`
	TopChannels []DimensionRow `json:"top_channels"`
	TopModels   []DimensionRow `json:"top_models"`
	TopTokens   []DimensionRow `json:"top_tokens"`
}

type reportAggregate struct {
	ChannelID           int64  `gorm:"column:channel_id"`
	Model               string `gorm:"column:model"`
	TokenID             int64  `gorm:"column:token_id"`
	Date                string `gorm:"column:day"`
	Requests            int64  `gorm:"column:requests"`
	Success             int64  `gorm:"column:success"`
	TokensIn            int64  `gorm:"column:tokens_in"`
	TokensOut           int64  `gorm:"column:tokens_out"`
	CacheReadTokens     int64  `gorm:"column:cache_read_tokens"`
	CostMicros          int64  `gorm:"column:cost_micros"`
	PricedRequests      int64  `gorm:"column:priced_requests"`
	UnpricedRequests    int64  `gorm:"column:unpriced_requests"`
	NoUsageRequests     int64  `gorm:"column:no_usage_requests"`
	UnknownCostRequests int64  `gorm:"column:unknown_cost_requests"`
}

func BuildReport(db *gorm.DB, q ReportQuery) (Report, error) {
	start, endExclusive, err := q.DayBounds()
	if err != nil {
		return Report{}, err
	}
	report := Report{Start: q.StartDate, End: q.EndDate}
	filter := func(db *gorm.DB) *gorm.DB {
		db = db.Where("date >= ? AND date < ?", start, endExclusive)
		if q.ChannelID > 0 {
			db = db.Where("channel_id = ?", q.ChannelID)
		}
		if q.Model != "" {
			db = db.Where("model = ?", q.Model)
		}
		if q.TokenID > 0 {
			db = db.Where("token_id = ?", q.TokenID)
		}
		return db
	}

	var totals reportAggregate
	if err := filter(db.Model(&store.UsageDaily{})).
		Select(`
			COALESCE(SUM(requests), 0) AS requests,
			COALESCE(SUM(success), 0) AS success,
			COALESCE(SUM(tokens_in), 0) AS tokens_in,
			COALESCE(SUM(tokens_out), 0) AS tokens_out,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cost_micros), 0) AS cost_micros
		`).Scan(&totals).Error; err != nil {
		return report, err
	}
	var counts struct {
		Priced   int64 `gorm:"column:priced_requests"`
		Unpriced int64 `gorm:"column:unpriced_requests"`
		NoUsage  int64 `gorm:"column:no_usage_requests"`
		Unknown  int64 `gorm:"column:unknown_cost_requests"`
	}
	if err := filter(db.Model(&store.UsageDaily{})).
		Select(`
			COALESCE(SUM(priced_requests), 0) AS priced_requests,
			COALESCE(SUM(unpriced_requests), 0) AS unpriced_requests,
			COALESCE(SUM(no_usage_requests), 0) AS no_usage_requests,
			COALESCE(SUM(unknown_cost_requests), 0) AS unknown_cost_requests
		`).Scan(&counts).Error; err != nil {
		return report, err
	}
	report.Totals = ReportTotals{
		Requests: totals.Requests, Success: totals.Success,
		TokensIn: totals.TokensIn, TokensOut: totals.TokensOut,
		CacheReadTokens: totals.CacheReadTokens,
		CostMicros:      totals.CostMicros, Cost: float64(totals.CostMicros) / 1_000_000,
		PricedRequests: counts.Priced, UnpricedRequests: counts.Unpriced,
		NoUsageRequests: counts.NoUsage, UnknownCostRequests: counts.Unknown,
	}

	// 按 UTC 小时分组返回，由前端按本地日分桶（每日一条），避免服务端猜测用户时区
	var daily []reportAggregate
	if err := filter(db.Model(&store.UsageDaily{})).
		Select(`
			strftime('%Y-%m-%dT%H:00:00Z', date) AS day,
			COALESCE(SUM(requests), 0) AS requests,
			COALESCE(SUM(success), 0) AS success,
			COALESCE(SUM(tokens_in), 0) AS tokens_in,
			COALESCE(SUM(tokens_out), 0) AS tokens_out,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cost_micros), 0) AS cost_micros,
			COALESCE(SUM(priced_requests), 0) AS priced_requests,
			COALESCE(SUM(unpriced_requests), 0) AS unpriced_requests,
			COALESCE(SUM(no_usage_requests), 0) AS no_usage_requests,
			COALESCE(SUM(unknown_cost_requests), 0) AS unknown_cost_requests
		`).Group("day").Order("day asc").Scan(&daily).Error; err != nil {
		return report, err
	}
	report.Hourly = make([]DailyPoint, 0, len(daily))
	for _, row := range daily {
		report.Hourly = append(report.Hourly, DailyPoint{
			Date: row.Date, Requests: row.Requests, Success: row.Success,
			TokensIn: row.TokensIn, TokensOut: row.TokensOut,
			CacheReadTokens: row.CacheReadTokens,
			CostMicros:      row.CostMicros, Cost: float64(row.CostMicros) / 1_000_000,
			Priced: row.PricedRequests, Unpriced: row.UnpricedRequests,
			NoUsage: row.NoUsageRequests, Unknown: row.UnknownCostRequests,
		})
	}

	channelRows, err := dimensionRows(filter, db, "channel_id")
	if err != nil {
		return report, err
	}
	tokenRows, err := dimensionRows(filter, db, "token_id")
	if err != nil {
		return report, err
	}
	modelRows, err := dimensionRows(filter, db, "model")
	if err != nil {
		return report, err
	}
	report.TopChannels = channelRows
	report.TopTokens = tokenRows
	report.TopModels = modelRows
	fillDimensionNames(db, report.TopChannels, &store.Channel{})
	fillDimensionNames(db, report.TopTokens, &store.Token{})
	return report, nil
}

func dimensionRows(filter func(*gorm.DB) *gorm.DB, db *gorm.DB, column string) ([]DimensionRow, error) {
	var aggregates []reportAggregate
	if err := filter(db.Model(&store.UsageDaily{})).
		Select(column + ` AS ` + column + `,
			COALESCE(SUM(requests), 0) AS requests,
			COALESCE(SUM(success), 0) AS success,
			COALESCE(SUM(tokens_in), 0) AS tokens_in,
			COALESCE(SUM(tokens_out), 0) AS tokens_out,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cost_micros), 0) AS cost_micros,
			COALESCE(SUM(priced_requests), 0) AS priced_requests,
			COALESCE(SUM(unpriced_requests), 0) AS unpriced_requests,
			COALESCE(SUM(no_usage_requests), 0) AS no_usage_requests,
			COALESCE(SUM(unknown_cost_requests), 0) AS unknown_cost_requests`).
		Group(column).
		Order("requests DESC, cost_micros DESC").
		Limit(10).
		Scan(&aggregates).Error; err != nil {
		return nil, err
	}
	rows := make([]DimensionRow, 0, len(aggregates))
	for _, row := range aggregates {
		var id int64
		var name string
		var nameKind string
		switch column {
		case "channel_id":
			id = row.ChannelID
		case "token_id":
			id = row.TokenID
		case "model":
			name = row.Model
		}
		// id 为 0 表示请求未命中任何渠道 / 未携带令牌（如调试台）：名称留空并打标，由前端按语言翻译。
		if name == "" {
			if id == 0 {
				nameKind = "unattributed"
				if column == "token_id" {
					nameKind = "playground"
				}
			} else {
				name = "#" + strconv.FormatInt(id, 10)
			}
		}
		rows = append(rows, DimensionRow{
			ID: id, Name: name, Requests: row.Requests, Success: row.Success,
			NameKind: nameKind,
			TokensIn: row.TokensIn, TokensOut: row.TokensOut,
			CacheReadTokens: row.CacheReadTokens,
			CostMicros:      row.CostMicros, Cost: float64(row.CostMicros) / 1_000_000,
			PricedRequests: row.PricedRequests, UnpricedRequests: row.UnpricedRequests,
			NoUsageRequests: row.NoUsageRequests, UnknownCostRequests: row.UnknownCostRequests,
		})
	}
	return rows, nil
}

func fillDimensionNames(db *gorm.DB, rows []DimensionRow, model any) {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ID > 0 {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var names []struct {
		ID   int64  `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	db.Model(model).Select("id, name").Where("id IN ?", ids).Scan(&names)
	byID := make(map[int64]string, len(names))
	for _, item := range names {
		byID[item.ID] = item.Name
	}
	for i := range rows {
		if name := byID[rows[i].ID]; name != "" {
			rows[i].Name = name
		} else if rows[i].ID > 0 && strings.HasPrefix(rows[i].Name, "#") {
			rows[i].NameKind = "deleted"
		}
	}
}
