package admin

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"sort"
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
	if v := c.Query("start"); v != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("start 必须是 RFC3339 时间（如 2026-01-02T15:04:05Z）")
		}
		q = q.Where("created_at >= ?", t.UTC())
	}
	if v := c.Query("end"); v != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("end 必须是 RFC3339 时间（如 2026-01-02T15:04:05Z）")
		}
		q = q.Where("created_at <= ?", t.UTC())
	}
	return q, nil
}

// logModelFacets 返回当前筛选下日志里出现过的对外模型及请求数，供筛选栏做候选：
// 用户不必记得精确模型名。只 GROUP BY model（走已有索引），最多 500 个。
func (a *API) logModelFacets(c *gin.Context) {
	q, err := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var rows []struct {
		Model    string `gorm:"column:model"`
		Requests int64  `gorm:"column:requests"`
	}
	if err := q.Select("model, COUNT(*) AS requests").Group("model").Order("requests DESC, model ASC").Limit(500).Scan(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
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
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var rows []struct {
		StatusCode int   `gorm:"column:status_code"`
		TotalMS    int64 `gorm:"column:total_ms"`
	}
	if err := q.Select("status_code, total_ms").Find(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, gin.H{"stats": computeLogStats(rows)})
}

// computeLogStats 纯函数便于表驱动测试；success = 2xx/3xx。
func computeLogStats(rows []struct {
	StatusCode int   `gorm:"column:status_code"`
	TotalMS    int64 `gorm:"column:total_ms"`
}) logStats {
	stats := logStats{Total: int64(len(rows))}
	latencies := make([]int64, 0, len(rows))
	var sum int64
	for _, r := range rows {
		if r.StatusCode >= 200 && r.StatusCode < 400 {
			stats.Success++
		}
		if r.TotalMS > 0 {
			latencies = append(latencies, r.TotalMS)
			sum += r.TotalMS
		}
	}
	if len(rows) > 0 {
		rate := float64(stats.Success) / float64(len(rows)) * 100
		stats.SuccessRate = &rate
	}
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		p50 := percentile(latencies, 50)
		p95 := percentile(latencies, 95)
		avg := sum / int64(len(latencies))
		stats.P50MS, stats.P95MS, stats.AvgTotalMS = &p50, &p95, &avg
	}
	return stats
}

// percentile 最近秩法：rank = ceil(p% × N)，取第 rank-1 个（sorted 升序且非空）。
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

func (a *API) exportLogs(c *gin.Context) {
	q, err := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var logs []*store.RequestLog
	if err := q.Order("id desc").Limit(logExportMaxRows).Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
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
