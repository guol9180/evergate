package admin

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/store"
)

// logExportMaxRows 单次 CSV 导出行数上限（防止内存失控；更大范围请缩小筛选）。
const logExportMaxRows = 10000

// applyLogFilters 把 /logs、/logs/stats、/logs/export 共用的查询参数映射到 WHERE 条件。
func applyLogFilters(c *gin.Context, q *gorm.DB) *gorm.DB {
	if v := c.Query("channel_id"); v != "" {
		q = q.Where("channel_id = ?", queryInt64(c, "channel_id"))
	}
	if v := c.Query("token_id"); v != "" {
		q = q.Where("token_id = ?", queryInt64(c, "token_id"))
	}
	if v := c.Query("model"); v != "" {
		q = q.Where("model = ?", v)
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status_code = ?", queryInt(c, "status", 0))
	}
	if v := c.Query("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("created_at >= ?", t.UTC())
		}
	}
	if v := c.Query("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("created_at <= ?", t.UTC())
		}
	}
	return q
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
	q := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
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
	q := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
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
		"id", "created_at", "model", "upstream_model", "original_model", "channel", "token", "status_code",
		"first_token_ms", "total_ms", "prompt_tokens", "completion_tokens", "total_tokens",
		"cost_micros", "cost_status", "error_type", "error_message",
	})
	for _, l := range logs {
		_ = w.Write([]string{
			strconv.FormatInt(l.ID, 10), l.CreatedAt.UTC().Format(time.RFC3339), l.Model, l.UpstreamModel, l.OriginalModel,
			l.ChannelName, l.TokenName, strconv.Itoa(l.StatusCode),
			strconv.FormatInt(l.FirstTokenMS, 10), strconv.FormatInt(l.TotalMS, 10),
			strconv.FormatInt(l.PromptTokens, 10), strconv.FormatInt(l.CompletionTokens, 10), strconv.FormatInt(l.TotalTokens, 10),
			strconv.FormatInt(l.CostMicros, 10), l.CostStatus, l.ErrorType, truncateRunes(l.ErrorMessage, 500),
		})
	}
	w.Flush()
	filename := fmt.Sprintf("evergate-logs-%s.csv", a.now().Format("20060102T150405Z"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// truncateRunes 按字符数截断，避免切断多字节字符。
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
