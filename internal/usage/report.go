package usage

import (
	"strconv"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

type ReportQuery struct {
	Start     time.Time
	End       time.Time
	ChannelID int64
	Model     string
	TokenID   int64
}

type ReportTotals struct {
	Requests         int64   `json:"requests"`
	Success          int64   `json:"success"`
	TokensIn         int64   `json:"tokens_in"`
	TokensOut        int64   `json:"tokens_out"`
	CostMicros       int64   `json:"cost_micros"`
	Cost             float64 `json:"cost"`
	PricedRequests   int64   `json:"priced_requests"`
	UnpricedRequests int64   `json:"unpriced_requests"`
}

type DailyPoint struct {
	Date       string  `json:"date"`
	Requests   int64   `json:"requests"`
	Success    int64   `json:"success"`
	TokensIn   int64   `json:"tokens_in"`
	TokensOut  int64   `json:"tokens_out"`
	CostMicros int64   `json:"cost_micros"`
	Cost       float64 `json:"cost"`
}

type DimensionRow struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Requests   int64   `json:"requests"`
	Success    int64   `json:"success"`
	TokensIn   int64   `json:"tokens_in"`
	TokensOut  int64   `json:"tokens_out"`
	CostMicros int64   `json:"cost_micros"`
	Cost       float64 `json:"cost"`
}

type Report struct {
	Start       string         `json:"start"`
	End         string         `json:"end"`
	Totals      ReportTotals   `json:"totals"`
	Daily       []DailyPoint   `json:"daily"`
	TopChannels []DimensionRow `json:"top_channels"`
	TopModels   []DimensionRow `json:"top_models"`
	TopTokens   []DimensionRow `json:"top_tokens"`
}

type reportAggregate struct {
	ChannelID  int64  `gorm:"column:channel_id"`
	Model      string `gorm:"column:model"`
	TokenID    int64  `gorm:"column:token_id"`
	Date       string `gorm:"column:day"`
	Requests   int64  `gorm:"column:requests"`
	Success    int64  `gorm:"column:success"`
	TokensIn   int64  `gorm:"column:tokens_in"`
	TokensOut  int64  `gorm:"column:tokens_out"`
	CostMicros int64  `gorm:"column:cost_micros"`
}

func BuildReport(db *gorm.DB, q ReportQuery) (Report, error) {
	start := utcDay(q.Start)
	end := utcDay(q.End)
	endExclusive := end.AddDate(0, 0, 1)
	report := Report{Start: start.Format("2006-01-02"), End: end.Format("2006-01-02")}
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
			COALESCE(SUM(cost_micros), 0) AS cost_micros
		`).Scan(&totals).Error; err != nil {
		return report, err
	}
	var counts struct {
		Priced   int64 `gorm:"column:priced_requests"`
		Unpriced int64 `gorm:"column:unpriced_requests"`
	}
	if err := filter(db.Model(&store.UsageDaily{})).
		Select(`
			COALESCE(SUM(priced_requests), 0) AS priced_requests,
			COALESCE(SUM(unpriced_requests), 0) AS unpriced_requests
		`).Scan(&counts).Error; err != nil {
		return report, err
	}
	report.Totals = ReportTotals{
		Requests: totals.Requests, Success: totals.Success,
		TokensIn: totals.TokensIn, TokensOut: totals.TokensOut,
		CostMicros: totals.CostMicros, Cost: float64(totals.CostMicros) / 1_000_000,
		PricedRequests: counts.Priced, UnpricedRequests: counts.Unpriced,
	}

	var daily []reportAggregate
	if err := filter(db.Model(&store.UsageDaily{})).
		Select(`
			strftime('%Y-%m-%d', date) AS day,
			COALESCE(SUM(requests), 0) AS requests,
			COALESCE(SUM(success), 0) AS success,
			COALESCE(SUM(tokens_in), 0) AS tokens_in,
			COALESCE(SUM(tokens_out), 0) AS tokens_out,
			COALESCE(SUM(cost_micros), 0) AS cost_micros
		`).Group("day").Order("day asc").Scan(&daily).Error; err != nil {
		return report, err
	}
	report.Daily = make([]DailyPoint, 0, len(daily))
	for _, row := range daily {
		report.Daily = append(report.Daily, DailyPoint{
			Date: row.Date, Requests: row.Requests, Success: row.Success,
			TokensIn: row.TokensIn, TokensOut: row.TokensOut,
			CostMicros: row.CostMicros, Cost: float64(row.CostMicros) / 1_000_000,
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
			COALESCE(SUM(cost_micros), 0) AS cost_micros`).
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
		switch column {
		case "channel_id":
			id = row.ChannelID
		case "token_id":
			id = row.TokenID
		case "model":
			name = row.Model
		}
		if name == "" {
			name = "#" + strconv.FormatInt(id, 10)
		}
		rows = append(rows, DimensionRow{
			ID: id, Name: name, Requests: row.Requests, Success: row.Success,
			TokensIn: row.TokensIn, TokensOut: row.TokensOut,
			CostMicros: row.CostMicros, Cost: float64(row.CostMicros) / 1_000_000,
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
		}
	}
}
