package admin

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/httpx"
	"evergate/internal/store"
)

// logExportMaxRows 单次 CSV 导出行数上限（防止内存失控；更大范围请缩小筛选）。
const logExportMaxRows = 10000

// errLogEndBeforeStart 与用量接口同口径：start 晚于 end 时拒绝，而不是返回空结果让用户以为没有日志。
var errLogEndBeforeStart = errors.New("结束时间不能早于开始时间")

// applyLogFilters 把 /logs、/logs/stats、/logs/export 共用的查询参数映射到 WHERE 条件。
// 非法筛选值返回带字段名的错误，由调用方转成 400，避免静默忽略导致筛选结果与预期不符。
func applyLogFilters(c *gin.Context, q *gorm.DB) (*gorm.DB, error) {
	if v := c.Query("channel_id"); v != "" {
		id, err := parseQueryInt64(c, "channel_id")
		if err != nil {
			return nil, err
		}
		q = q.Where("channel_id = ?", id)
	}
	if v := c.Query("token_id"); v != "" {
		id, err := parseQueryInt64(c, "token_id")
		if err != nil {
			return nil, err
		}
		q = q.Where("token_id = ?", id)
	}
	if v := c.Query("model"); v != "" {
		q = q.Where("model = ?", v)
	}
	if v := c.Query("status"); v != "" {
		// success / error 是分组筛选（与 computeLogStats 的 2xx/3xx 成功口径一致），其余按具体状态码
		switch v {
		case "success":
			q = q.Where("status_code < ?", http.StatusBadRequest)
		case "error":
			q = q.Where("status_code >= ?", http.StatusBadRequest)
		default:
			code, err := parseQueryInt64(c, "status")
			if err != nil {
				return nil, err
			}
			q = q.Where("status_code = ?", code)
		}
	}
	start, hasStart, err := parseRFC3339Query(c, "start")
	if err != nil {
		return nil, err
	}
	if hasStart {
		q = q.Where("created_at >= ?", start)
	}
	end, hasEnd, err := parseRFC3339Query(c, "end")
	if err != nil {
		return nil, err
	}
	if hasEnd {
		q = q.Where("created_at <= ?", end)
	}
	if hasStart && hasEnd && start.After(end) {
		return nil, errLogEndBeforeStart
	}
	return q, nil
}

// parseRFC3339Query 解析可选的 RFC3339 查询参数；缺省时 has=false。
func parseRFC3339Query(c *gin.Context, name string) (time.Time, bool, error) {
	v := c.Query(name)
	if v == "" {
		return time.Time{}, false, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%s 必须是 RFC3339 时间（如 2026-01-02T15:04:05Z）", name)
	}
	return t.UTC(), true, nil
}

// failLogFilter 把筛选校验错误转成 400：区间倒置用稳定码 end_before_start，其余参数错误沿用 invalid_param。
func failLogFilter(c *gin.Context, err error) {
	if errors.Is(err, errLogEndBeforeStart) {
		fail(c, http.StatusBadRequest, "end_before_start", err.Error())
		return
	}
	fail(c, http.StatusBadRequest, "invalid_param", err.Error())
}

// logModelFacets 返回当前筛选下日志里出现过的对外模型及请求数，供筛选栏做候选：
// 用户不必记得精确模型名。只 GROUP BY model（走已有索引），最多 500 个。
func (a *API) logModelFacets(c *gin.Context) {
	q, err := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
	if err != nil {
		failLogFilter(c, err)
		return
	}
	var rows []struct {
		Model    string `gorm:"column:model"`
		Requests int64  `gorm:"column:requests"`
	}
	if err := q.Select("model, COUNT(*) AS requests").Group("model").Order("requests DESC, model ASC").Limit(500).Scan(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"model": row.Model, "requests": row.Requests})
	}
	ok(c, gin.H{"items": items})
}

// logStats 聚合结果：基于全部匹配行计算，不受分页限制（修复 Dashboard 只看最新 100 条的口径偏差）。
type logStats struct {
	Total       int64    `json:"total"`
	Success     int64    `json:"success"`
	SuccessRate *float64 `json:"success_rate"`
	P50MS       *int64   `json:"p50_ms"`
	P95MS       *int64   `json:"p95_ms"`
	AvgTotalMS  *int64   `json:"avg_total_ms"`
}

func (a *API) logStatsHandler(c *gin.Context) {
	q, err := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
	if err != nil {
		failLogFilter(c, err)
		return
	}
	var agg struct {
		Total      int64 `gorm:"column:total"`
		Success    int64 `gorm:"column:success"`
		LatencyN   int64 `gorm:"column:latency_n"`
		LatencySum int64 `gorm:"column:latency_sum"`
	}
	// 计数、成功率与均值一次聚合完成；2xx/3xx 计为成功，与筛选栏的 success 分组同口径。
	if err := q.Select(`COUNT(*) AS total,
		SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END) AS success,
		SUM(CASE WHEN total_ms > 0 THEN 1 ELSE 0 END) AS latency_n,
		COALESCE(SUM(CASE WHEN total_ms > 0 THEN total_ms END), 0) AS latency_sum`).Scan(&agg).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	stats := logStats{Total: agg.Total, Success: agg.Success}
	if agg.Total > 0 {
		rate := float64(agg.Success) / float64(agg.Total) * 100
		stats.SuccessRate = &rate
	}
	if agg.LatencyN > 0 {
		avg := agg.LatencySum / agg.LatencyN
		stats.AvgTotalMS = &avg
		p50, err := a.logPercentile(c, percentileRank(agg.LatencyN, 50))
		if err != nil {
			fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
			return
		}
		p95, err := a.logPercentile(c, percentileRank(agg.LatencyN, 95))
		if err != nil {
			fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
			return
		}
		stats.P50MS, stats.P95MS = &p50, &p95
	}
	ok(c, gin.H{"stats": stats})
}

// logPercentile 用 ORDER BY + LIMIT 1 OFFSET 取第 rank 小值，
// 让分位数在 SQL 内完成，不再把整表延迟读进内存排序。
func (a *API) logPercentile(c *gin.Context, rank int) (int64, error) {
	q, err := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
	if err != nil {
		return 0, err
	}
	var ms int64
	if err := q.Where("total_ms > 0").Order("total_ms ASC").Limit(1).Offset(rank-1).Pluck("total_ms", &ms).Error; err != nil {
		return 0, err
	}
	return ms, nil
}

// percentileRank 最近秩法：rank = ceil(p% × N)，对应升序第 rank 个（1-based）。
func percentileRank(n int64, p float64) int {
	rank := int(math.Ceil(p / 100 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	return rank
}

func (a *API) exportLogs(c *gin.Context) {
	q, err := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
	if err != nil {
		failLogFilter(c, err)
		return
	}
	var logs []*store.RequestLog
	if err := q.Order("id desc").Limit(logExportMaxRows).Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	buf := &bytes.Buffer{}
	// UTF-8 BOM 让 Excel 直接识别中文，避免乱码。
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(buf)
	_ = w.Write([]string{
		"id", "created_at", "source", "model", "upstream_model", "original_model", "channel", "token", "status_code",
		"first_token_ms", "total_ms", "prompt_tokens", "cached_tokens", "cache_write_tokens", "completion_tokens",
		"reasoning_tokens", "total_tokens", "retry_count", "cost_micros", "cost_status", "price_source",
		"error_type", "error_message",
	})
	for _, l := range logs {
		_ = w.Write([]string{
			strconv.FormatInt(l.ID, 10), l.CreatedAt.UTC().Format(time.RFC3339), csvCell(l.Source),
			csvCell(l.Model), csvCell(l.UpstreamModel), csvCell(l.OriginalModel),
			csvCell(l.ChannelName), csvCell(l.TokenName), strconv.Itoa(l.StatusCode),
			strconv.FormatInt(l.FirstTokenMS, 10), strconv.FormatInt(l.TotalMS, 10),
			strconv.FormatInt(l.PromptTokens, 10), strconv.FormatInt(l.CachedTokens, 10),
			strconv.FormatInt(l.CacheWriteTokens, 10), strconv.FormatInt(l.CompletionTokens, 10),
			strconv.FormatInt(l.ReasoningTokens, 10), strconv.FormatInt(l.TotalTokens, 10),
			strconv.Itoa(len(l.RetryTrace)), strconv.FormatInt(l.CostMicros, 10),
			csvCell(l.CostStatus), csvCell(l.PriceSource), csvCell(l.ErrorType),
			csvCell(httpx.TruncateRunes(l.ErrorMessage, 500)),
		})
	}
	w.Flush()
	filename := fmt.Sprintf("evergate-logs-%s.csv", a.now().Format("20060102T150405Z"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// csvCell 防公式注入：以 = + - @ 或制表/回车开头的单元格前置单引号，Excel 按文本处理。
// 只用于字符串列；数字列由 strconv 生成，不经过这里。
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
