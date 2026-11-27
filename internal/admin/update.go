package admin

// 系统更新：检查 / 应用 / 进度 / 取消。

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/selfupdate"
)

func (a *API) systemUpdate(c *gin.Context) {
	if a.opt.Update == nil {
		fail(c, http.StatusServiceUnavailable, "update_not_configured", "更新检查未配置")
		return
	}
	// 检查顺序为 npm 双源 → GitHub 兜底，最坏情况需覆盖各源超时预算。
	ctx, cancel := context.WithTimeout(c.Request.Context(), 14*time.Second)
	defer cancel()
	status, err := a.opt.Update.Check(ctx)
	if err != nil {
		fail(c, http.StatusBadGateway, "update_check_failed", "更新检查失败："+err.Error())
		return
	}
	ok(c, status)
}

// applySystemUpdate 启动一键更新；任务异步执行，前端轮询 progress。
func (a *API) applySystemUpdate(c *gin.Context) {
	if a.opt.SelfUpdate == nil {
		fail(c, http.StatusServiceUnavailable, "update_not_configured", "自动更新未配置")
		return
	}
	var req struct {
		Version string `json:"version"`
		Source  string `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	job, err := a.opt.SelfUpdate.Start(req.Version, req.Source)
	if err != nil {
		if errors.Is(err, selfupdate.ErrBusy) {
			fail(c, http.StatusConflict, "conflict", err.Error())
			return
		}
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	ok(c, job)
}

func (a *API) systemUpdateProgress(c *gin.Context) {
	if a.opt.SelfUpdate == nil {
		fail(c, http.StatusServiceUnavailable, "update_not_configured", "自动更新未配置")
		return
	}
	ok(c, gin.H{
		"capability":      a.opt.SelfUpdate.Capability(),
		"job":             a.opt.SelfUpdate.Snapshot(),
		"current_version": a.opt.SelfUpdate.CurrentVersion(),
	})
}

func (a *API) cancelSystemUpdate(c *gin.Context) {
	if a.opt.SelfUpdate == nil {
		fail(c, http.StatusServiceUnavailable, "update_not_configured", "自动更新未配置")
		return
	}
	if err := a.opt.SelfUpdate.Cancel(); err != nil {
		fail(c, http.StatusConflict, "conflict", err.Error())
		return
	}
	ok(c, gin.H{"cancelled": true})
}
