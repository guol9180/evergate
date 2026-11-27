package admin

// 请求日志：列表与单条详情（筛选、统计与导出见 logs_extra.go）。

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

func (a *API) listLogs(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q, filterErr := applyLogFilters(c, a.opt.DB.Model(&store.RequestLog{}))
	if filterErr != nil {
		failLogFilter(c, filterErr)
		return
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	var logs []*store.RequestLog
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	ok(c, gin.H{"items": logs, "total": total, "page": page, "page_size": pageSize})
}

func (a *API) getLog(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return
	}
	var log store.RequestLog
	if err := a.opt.DB.First(&log, id).Error; err != nil {
		fail(c, http.StatusNotFound, "log_not_found", "日志不存在")
		return
	}
	ok(c, gin.H{"log": log})
}

// ---------- 工具 ----------
