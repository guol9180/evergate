package admin

// 系统设置：SQLite 运行时配置的读取与保存。

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"evergate/internal/pool"
	"evergate/internal/settings"
)

func (a *API) getSystemSettings(c *gin.Context) {
	if a.opt.Settings == nil {
		fail(c, http.StatusServiceUnavailable, "settings_not_ready", "系统设置未配置")
		return
	}
	snapshot := a.opt.Settings.Snapshot()
	ok(c, gin.H{
		"settings": snapshot,
		"paths": gin.H{
			"config_path":   a.opt.ConfigPath,
			"data_dir":      a.opt.DataDir,
			"config_source": a.opt.ConfigSource,
		},
		"scope": gin.H{
			"runtime_source":     "sqlite_runtime_settings (seeded from evergate.yml on first start)",
			"hot_update_enabled": true,
		},
	})
}

func (a *API) updateSystemSettings(c *gin.Context) {
	if a.opt.Settings == nil {
		fail(c, http.StatusServiceUnavailable, "settings_not_ready", "系统设置未配置")
		return
	}
	var req settings.Values
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	next, err := a.opt.Settings.UpdateValues(req)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	if a.opt.Pool != nil {
		defaults := next.ConfigDefaults()
		a.opt.Pool.UpdateCooldownConfig(
			defaults.Cooldown.FailThreshold,
			pool.ScheduleFrom(defaults.Cooldown.Base.Duration, defaults.Cooldown.Max.Duration),
		)
	}
	ok(c, gin.H{"settings": next})
}
