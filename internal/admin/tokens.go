package admin

// 下游令牌：CRUD 与治理规则（模型 / IP / 额度 / 过期）归一化。

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

func toTokenDTO(t *store.Token) tokenDTO {
	remaining := int64(-1)
	status := "active"
	switch {
	case !t.Enabled:
		status = "disabled"
	case t.ExpiredAt != nil && time.Now().UTC().After(*t.ExpiredAt):
		status = "expired"
	case t.Quota > 0:
		remaining = t.Quota - t.UsedQuota
		if remaining < 0 {
			remaining = 0
		}
		if remaining == 0 {
			status = "exhausted"
		}
	}
	return tokenDTO{
		ID: t.ID, Name: t.Name, Enabled: t.Enabled, Quota: t.Quota, UsedQuota: t.UsedQuota,
		RemainingQuota: remaining, Status: status,
		ExpiredAt: t.ExpiredAt, AllowedModels: t.AllowedModels, AllowedIPs: t.AllowedIPs,
		DisableFallback: t.DisableFallback, CreatedAt: t.CreatedAt,
	}
}

func (a *API) listTokens(c *gin.Context) {
	var tokens []*store.Token
	if err := a.opt.DB.Order("id desc").Find(&tokens).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	items := make([]tokenDTO, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, toTokenDTO(t))
	}
	ok(c, gin.H{"items": items})
}

func normalizeTokenRules(models, ips []string) ([]string, []string, error) {
	normalizeModels := make([]string, 0, len(models))
	seenModels := make(map[string]bool, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if seenModels[model] {
			continue
		}
		seenModels[model] = true
		normalizeModels = append(normalizeModels, model)
	}

	normalizedIPs := make([]string, 0, len(ips))
	seenIPs := make(map[string]bool, len(ips))
	for _, rule := range ips {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if ip := net.ParseIP(rule); ip != nil {
			rule = ip.String()
		} else if _, network, err := net.ParseCIDR(rule); err == nil {
			rule = network.String()
		} else {
			return nil, nil, errors.New("IP 白名单仅支持精确 IP 或 CIDR")
		}
		if seenIPs[rule] {
			continue
		}
		seenIPs[rule] = true
		normalizedIPs = append(normalizedIPs, rule)
	}
	return normalizeModels, normalizedIPs, nil
}

func (a *API) createToken(c *gin.Context) {
	var req struct {
		Name            string     `json:"name" binding:"required"`
		Quota           *int64     `json:"quota"`
		ExpiredAt       *time.Time `json:"expired_at" time_format:"2006-01-02T15:04:05Z07:00"`
		AllowedModels   []string   `json:"allowed_models"`
		AllowedIPs      []string   `json:"allowed_ips"`
		DisableFallback bool       `json:"disable_fallback"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	// 生成 24 字节随机 key；SHA-256 入库；原文仅本次响应返回一次（任务书 4.3）
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		fail(c, http.StatusInternalServerError, "token_create_failed", "生成失败")
		return
	}
	plain := "eg-" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	quota := derefInt64(req.Quota, 0)
	if quota < 0 {
		fail(c, http.StatusBadRequest, "negative_quota", "额度不能为负数")
		return
	}
	allowedModels, allowedIPs, err := normalizeTokenRules(req.AllowedModels, req.AllowedIPs)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	t := &store.Token{
		Name: req.Name, KeyHash: hex.EncodeToString(sum[:]), Enabled: true,
		Quota: quota, ExpiredAt: req.ExpiredAt,
		AllowedModels: allowedModels, AllowedIPs: allowedIPs,
		DisableFallback: req.DisableFallback,
	}
	if err := a.opt.DB.Create(t).Error; err != nil {
		fail(c, http.StatusInternalServerError, "create_failed", "创建失败")
		return
	}
	dto := toTokenDTO(t)
	ok(c, gin.H{"token": dto, "key": plain}) // 唯一一次明文返回
}

func (a *API) updateToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return
	}
	var t store.Token
	if err := a.opt.DB.First(&t, id).Error; err != nil {
		fail(c, http.StatusNotFound, "token_not_found", "令牌不存在")
		return
	}
	var req struct {
		Name            *string    `json:"name"`
		Enabled         *bool      `json:"enabled"`
		Quota           *int64     `json:"quota"`
		ExpiredAt       *time.Time `json:"expired_at"`
		ClearExpiredAt  bool       `json:"clear_expired_at"`
		AllowedModels   *[]string  `json:"allowed_models"`
		AllowedIPs      *[]string  `json:"allowed_ips"`
		DisableFallback *bool      `json:"disable_fallback"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Quota != nil {
		if *req.Quota < 0 {
			fail(c, http.StatusBadRequest, "negative_quota", "额度不能为负数")
			return
		}
		updates["quota"] = *req.Quota
	}
	if req.ExpiredAt != nil {
		updates["expired_at"] = *req.ExpiredAt
	}
	if req.ClearExpiredAt {
		updates["expired_at"] = nil
	}
	if req.AllowedModels != nil {
		updates["allowed_models"] = *req.AllowedModels
	}
	if req.AllowedIPs != nil {
		updates["allowed_ips"] = *req.AllowedIPs
	}
	if req.DisableFallback != nil {
		updates["disable_fallback"] = *req.DisableFallback
	}
	if req.AllowedModels != nil || req.AllowedIPs != nil {
		models := t.AllowedModels
		if req.AllowedModels != nil {
			models = *req.AllowedModels
		}
		ips := t.AllowedIPs
		if req.AllowedIPs != nil {
			ips = *req.AllowedIPs
		}
		normalizedModels, normalizedIPs, err := normalizeTokenRules(models, ips)
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid_param", err.Error())
			return
		}
		updates["allowed_models"] = normalizedModels
		updates["allowed_ips"] = normalizedIPs
	}
	if len(updates) > 0 {
		if err := a.opt.DB.Model(&t).Updates(updates).Error; err != nil {
			fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
			return
		}
	}
	ok(c, gin.H{"updated": true})
}

func (a *API) deleteToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return
	}
	if err := a.opt.DB.Delete(&store.Token{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, "delete_failed", "删除失败")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ---------- 请求日志 ----------
