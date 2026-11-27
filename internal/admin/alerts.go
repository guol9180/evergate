package admin

// 告警配置：读取 / 保存 Webhook 预设与订阅事件、发送测试、查看最近记录。

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"evergate/internal/alert"
)

func (a *API) getAlerts(c *gin.Context) {
	if a.opt.Alerts == nil {
		fail(c, http.StatusServiceUnavailable, "alerts_disabled", "告警未启用")
		return
	}
	setting, err := a.opt.Alerts.Setting()
	if err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	records, err := a.opt.Alerts.Recent(20)
	if err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	ok(c, gin.H{"setting": setting, "records": records, "events": alert.Events()})
}

func (a *API) updateAlerts(c *gin.Context) {
	if a.opt.Alerts == nil {
		fail(c, http.StatusServiceUnavailable, "alerts_disabled", "告警未启用")
		return
	}
	var req struct {
		Enabled *bool     `json:"enabled"`
		Preset  *string   `json:"preset"`
		URL     *string   `json:"url"`
		Events  *[]string `json:"events"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	setting, err := a.opt.Alerts.Setting()
	if err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	if req.Enabled != nil {
		setting.Enabled = *req.Enabled
	}
	if req.Preset != nil {
		setting.Preset = *req.Preset
	}
	if req.URL != nil {
		setting.URL = *req.URL
	}
	if req.Events != nil {
		setting.Events = *req.Events
	}
	saved, err := a.opt.Alerts.Save(setting)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	ok(c, gin.H{"setting": saved})
}

func (a *API) testAlert(c *gin.Context) {
	if a.opt.Alerts == nil {
		fail(c, http.StatusServiceUnavailable, "alerts_disabled", "告警未启用")
		return
	}
	record, err := a.opt.Alerts.SendTest()
	if err != nil {
		fail(c, http.StatusBadGateway, "alert_send_failed", err.Error())
		return
	}
	ok(c, gin.H{"record": record})
}
