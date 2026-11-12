package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
	"evergate/internal/usage"
)

func (a *API) getUsageReport(c *gin.Context) {
	if a.opt.Usage == nil {
		fail(c, http.StatusServiceUnavailable, "用量统计未配置")
		return
	}
	today := utcToday()
	start, err := parseDateParam(c.Query("start"), today.AddDate(0, 0, -6))
	if err != nil {
		fail(c, http.StatusBadRequest, "start 必须是 YYYY-MM-DD")
		return
	}
	end, err := parseDateParam(c.Query("end"), today)
	if err != nil {
		fail(c, http.StatusBadRequest, "end 必须是 YYYY-MM-DD")
		return
	}
	if end.Before(start) {
		fail(c, http.StatusBadRequest, "结束日期不能早于开始日期")
		return
	}
	if end.After(today) {
		fail(c, http.StatusBadRequest, "结束日期不能晚于今天")
		return
	}
	retention := 14
	if a.opt.Settings != nil {
		retention = a.opt.Settings.Snapshot().LogRetentionDays
	}
	earliest := today.AddDate(0, 0, -(retention - 1))
	if start.Before(earliest) {
		fail(c, http.StatusBadRequest, "查询范围超过请求日志保留窗口")
		return
	}
	report, err := usage.BuildReport(a.opt.DB, usage.ReportQuery{
		Start: start, End: end,
		ChannelID: queryInt64(c, "channel_id"),
		Model:     strings.TrimSpace(c.Query("model")),
		TokenID:   queryInt64(c, "token_id"),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "统计查询失败")
		return
	}
	ok(c, report)
}

func (a *API) listUsagePrices(c *gin.Context) {
	if a.opt.Usage == nil {
		fail(c, http.StatusServiceUnavailable, "计价配置未初始化")
		return
	}
	global, overrides, err := a.opt.Usage.List(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "价格查询失败")
		return
	}
	ok(c, gin.H{"default": global, "overrides": overrides})
}

func (a *API) setDefaultUsagePrice(c *gin.Context) {
	if a.opt.Usage == nil {
		fail(c, http.StatusServiceUnavailable, "计价配置未初始化")
		return
	}
	var req struct {
		InputMicrosPerMillion  int64 `json:"input_micros_per_million"`
		OutputMicrosPerMillion int64 `json:"output_micros_per_million"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	if err := a.opt.Usage.SetDefault(c.Request.Context(), req.InputMicrosPerMillion, req.OutputMicrosPerMillion); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	a.respondUsagePrices(c)
}

func (a *API) upsertUsagePriceOverride(c *gin.Context) {
	if a.opt.Usage == nil {
		fail(c, http.StatusServiceUnavailable, "计价配置未初始化")
		return
	}
	var req struct {
		ChannelID              int64  `json:"channel_id" binding:"required"`
		Model                  string `json:"model" binding:"required"`
		InputMicrosPerMillion  int64  `json:"input_micros_per_million"`
		OutputMicrosPerMillion int64  `json:"output_micros_per_million"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	var channel store.Channel
	if err := a.opt.DB.First(&channel, req.ChannelID).Error; err != nil {
		fail(c, http.StatusBadRequest, "渠道不存在")
		return
	}
	if _, err := a.opt.Usage.UpsertOverride(c.Request.Context(), req.ChannelID, req.Model,
		req.InputMicrosPerMillion, req.OutputMicrosPerMillion); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	a.respondUsagePrices(c)
}

func (a *API) deleteUsagePriceOverride(c *gin.Context) {
	if a.opt.Usage == nil {
		fail(c, http.StatusServiceUnavailable, "计价配置未初始化")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "非法 id")
		return
	}
	if err := a.opt.Usage.DeleteOverride(c.Request.Context(), id); err != nil {
		if errors.Is(err, usage.ErrPriceOverrideNotFound) {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	a.respondUsagePrices(c)
}

func (a *API) respondUsagePrices(c *gin.Context) {
	global, overrides, err := a.opt.Usage.List(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "价格查询失败")
		return
	}
	ok(c, gin.H{"default": global, "overrides": overrides})
}

func parseDateParam(value string, fallback time.Time) (time.Time, error) {
	if value == "" {
		return utcTodayFrom(fallback), nil
	}
	day, err := time.ParseInLocation("2006-01-02", value, time.UTC)
	if err != nil {
		return time.Time{}, err
	}
	return day, nil
}

func utcToday() time.Time {
	return utcTodayFrom(time.Now())
}

func utcTodayFrom(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
