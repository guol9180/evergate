package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"evergate/internal/httpx"
	"evergate/internal/store"
)

// auditSummaryKey 由 ok() 写入响应摘要，供 sessionMiddleware 在请求结束后落审计。
const auditSummaryKey = "evergate.audit.summary"

// auditMaxSummary 审计摘要长度上限（与 store.AuditLog.Summary 约束一致）。
const auditMaxSummary = 512

// auditActions "METHOD 路由模式" → 审计动作名；未列入的写操作不审计（探测/测试类只读语义）。
var auditActions = map[string]string{
	"POST /admin/api/channels":                         "channel.create",
	"PUT /admin/api/channels/:id":                      "channel.update",
	"DELETE /admin/api/channels/:id":                   "channel.delete",
	"POST /admin/api/channels/:id/toggle":              "channel.toggle",
	"POST /admin/api/channels/:id/keys":                "key.batch_add",
	"PUT /admin/api/keys/:id":                          "key.update",
	"DELETE /admin/api/keys/:id":                       "key.delete",
	"POST /admin/api/tokens":                           "token.create",
	"PUT /admin/api/tokens/:id":                        "token.update",
	"DELETE /admin/api/tokens/:id":                     "token.delete",
	"PUT /admin/api/system/settings":                   "settings.update",
	"PUT /admin/api/model-fallbacks":                   "fallback.replace",
	"PUT /admin/api/usage/prices/default":              "price.default_set",
	"POST /admin/api/usage/prices/overrides":           "price.upsert",
	"DELETE /admin/api/usage/prices/overrides/:id":     "price.delete",
	"POST /admin/api/channels/:id/prices/apply-preset": "price.apply_preset",
	"PUT /admin/api/auth/password":                     "admin.change_password",
	"POST /admin/api/system/update/apply":              "system.update_apply",
	"POST /admin/api/system/update/cancel":             "system.update_cancel",
	"GET /admin/api/config/export":                     "config.export",
	"POST /admin/api/config/import":                    "config.import",
}

// auditSummaryKeys 白名单：只有这些字段允许进入审计摘要，防止 key/token 等明文被写入。
var auditSummaryKeys = []string{"name", "id", "channel_id", "token_id", "enabled", "added", "created", "updated", "skipped"}

// stashAuditSummary 由 ok() 调用，从响应数据中按白名单提取可读摘要。
func stashAuditSummary(c *gin.Context, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	var parts []string
	for _, key := range auditSummaryKeys {
		value, exists := payload[key]
		if !exists || value == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	if len(parts) == 0 {
		return
	}
	c.Set(auditSummaryKey, strings.Join(parts, " "))
}

// auditWrite 在请求处理完成后记录写操作审计；只记录 2xx 响应，失败响应不产生审计噪音。
func (a *API) auditWrite(c *gin.Context) {
	if c.Writer.Status() < 200 || c.Writer.Status() >= 300 {
		return
	}
	action, audited := auditActions[c.Request.Method+" "+c.FullPath()]
	if !audited {
		return
	}
	summary := c.GetString(auditSummaryKey)
	target := c.Param("id")
	if target != "" {
		target = "#" + target
	}
	a.recordAudit(c, action, target, summary)
}

// recordAudit 写入一条审计记录；失败只记日志，绝不影响已成功的业务响应。
func (a *API) recordAudit(c *gin.Context, action, target, summary string) {
	summary = strings.TrimSpace(summary)
	summary = httpx.TruncateRunes(summary, auditMaxSummary)
	entry := store.AuditLog{
		Username: c.GetString("admin"), Action: action, Target: target, Summary: summary, IP: c.ClientIP(),
	}
	if err := a.opt.DB.Create(&entry).Error; err != nil && a.opt.Logger != nil {
		a.opt.Logger.Warn("write audit log failed", slog.String("action", action), slog.Any("error", err))
	}
}

func (a *API) listAuditLogs(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := a.opt.DB.Model(&store.AuditLog{})
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		q = q.Where("action = ?", action)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	var items []*store.AuditLog
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	ok(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}
