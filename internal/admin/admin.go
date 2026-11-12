// Package admin 实现管理台 REST API（任务书 4.4 /admin/api/*）：
// 会话鉴权、渠道与凭据管理、下游令牌管理、请求日志查询。
package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"evergate/internal/pool"
	"evergate/internal/settings"
	"evergate/internal/store"
	"evergate/internal/update"
)

// Options 管理 API 装配依赖。
type Options struct {
	DB        *gorm.DB
	MasterKey string        // 解密上游 key 用于连通测试
	Pool      *pool.Manager // 渠道健康推导 / key 重置
	Session   *SessionManager
	Update    *update.Checker
	Settings  *settings.Manager
}

// API 管理台接口集合。
type API struct {
	opt Options
}

// New 创建管理 API。
func New(opt Options) *API {
	return &API{opt: opt}
}

// RegisterRoutes 注册 /admin/api 全部路由（由 server.SetupRoutes 挂载）。
func (a *API) RegisterRoutes(r gin.IRoutes) {
	r.POST("/admin/api/auth/login", a.login)
	r.POST("/admin/api/auth/logout", a.logout)
	// 会话保护区（sessionMiddleware 放行 me/password 以强制首登改密）
	r.GET("/admin/api/auth/me", a.sessionMiddleware(), a.me)
	r.PUT("/admin/api/auth/password", a.sessionMiddleware(), a.changePassword)
	r.GET("/admin/api/system/update", a.sessionMiddleware(), a.systemUpdate)
	r.GET("/admin/api/system/settings", a.sessionMiddleware(), a.getSystemSettings)
	r.PUT("/admin/api/system/settings", a.sessionMiddleware(), a.updateSystemSettings)
	r.GET("/admin/api/channels", a.sessionMiddleware(), a.listChannels)
	r.POST("/admin/api/channels", a.sessionMiddleware(), a.createChannel)
	r.GET("/admin/api/channels/:id", a.sessionMiddleware(), a.getChannel)
	r.PUT("/admin/api/channels/:id", a.sessionMiddleware(), a.updateChannel)
	r.DELETE("/admin/api/channels/:id", a.sessionMiddleware(), a.deleteChannel)
	r.POST("/admin/api/channels/:id/toggle", a.sessionMiddleware(), a.toggleChannel)
	r.POST("/admin/api/channels/:id/test", a.sessionMiddleware(), a.testChannel)
	r.POST("/admin/api/channels/:id/keys", a.sessionMiddleware(), a.batchAddKeys)
	r.PUT("/admin/api/keys/:id", a.sessionMiddleware(), a.updateKey)
	r.DELETE("/admin/api/keys/:id", a.sessionMiddleware(), a.deleteKey)
	r.GET("/admin/api/tokens", a.sessionMiddleware(), a.listTokens)
	r.POST("/admin/api/tokens", a.sessionMiddleware(), a.createToken)
	r.PUT("/admin/api/tokens/:id", a.sessionMiddleware(), a.updateToken)
	r.DELETE("/admin/api/tokens/:id", a.sessionMiddleware(), a.deleteToken)
	r.GET("/admin/api/logs", a.sessionMiddleware(), a.listLogs)
	r.GET("/admin/api/logs/:id", a.sessionMiddleware(), a.getLog)
}

// ---------- 会话 ----------

func (a *API) login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	admin, err := store.GetAdminByUsername(a.opt.DB, req.Username)
	if err != nil || !store.CheckAdminPassword(admin, req.Password) {
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	now := time.Now()
	c.SetCookie(CookieName, a.opt.Session.Issue(req.Username, now), int(a.opt.Session.ttl.Seconds()), "/", "", false, true)
	ok(c, gin.H{"username": admin.Username, "must_change_password": admin.MustChangePassword})
}

func (a *API) logout(c *gin.Context) {
	c.SetCookie(CookieName, "", -1, "/", "", false, true)
	ok(c, gin.H{"logout": true})
}

func (a *API) me(c *gin.Context) {
	admin, err := store.GetAdminByUsername(a.opt.DB, c.GetString("admin"))
	if err != nil {
		fail(c, http.StatusUnauthorized, "会话无效")
		return
	}
	ok(c, gin.H{"username": admin.Username, "must_change_password": admin.MustChangePassword})
}

func (a *API) changePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误（新密码至少 8 位）")
		return
	}
	admin, err := store.GetAdminByUsername(a.opt.DB, c.GetString("admin"))
	if err != nil || !store.CheckAdminPassword(admin, req.OldPassword) {
		fail(c, http.StatusUnauthorized, "旧密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "加密失败")
		return
	}
	if err := a.opt.DB.Model(&store.Admin{}).Where("id = ?", admin.ID).
		Updates(map[string]any{"password_hash": string(hash), "must_change_password": false}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	ok(c, gin.H{"changed": true})
}

func (a *API) systemUpdate(c *gin.Context) {
	if a.opt.Update == nil {
		fail(c, http.StatusServiceUnavailable, "更新检查未配置")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	status, err := a.opt.Update.Check(ctx)
	if err != nil {
		fail(c, http.StatusBadGateway, "更新检查失败："+err.Error())
		return
	}
	ok(c, status)
}

func (a *API) getSystemSettings(c *gin.Context) {
	if a.opt.Settings == nil {
		fail(c, http.StatusServiceUnavailable, "系统设置未配置")
		return
	}
	snapshot := a.opt.Settings.Snapshot()
	ok(c, gin.H{
		"settings": snapshot,
		"scope": gin.H{
			"runtime_source":     "SQLite runtime_settings（首次初始化来自 evergate.yml）",
			"hot_update_enabled": true,
		},
	})
}

func (a *API) updateSystemSettings(c *gin.Context) {
	if a.opt.Settings == nil {
		fail(c, http.StatusServiceUnavailable, "系统设置未配置")
		return
	}
	var req settings.Values
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	next, err := a.opt.Settings.UpdateValues(req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
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

// sessionMiddleware 校验会话 Cookie；首登未改密前仅放行 me/password（强制改密）。
func (a *API) sessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(CookieName)
		if err != nil || token == "" {
			fail(c, http.StatusUnauthorized, "未登录")
			c.Abort()
			return
		}
		username, verr := a.opt.Session.Verify(token, time.Now())
		if verr != nil {
			fail(c, http.StatusUnauthorized, "会话无效或已过期")
			c.Abort()
			return
		}
		c.Set("admin", username)
		// 强制首登改密：me/password 放行，其余一律 403
		path := c.FullPath()
		if strings.HasSuffix(path, "/auth/me") || strings.HasSuffix(path, "/auth/password") {
			c.Next()
			return
		}
		admin, aerr := store.GetAdminByUsername(a.opt.DB, username)
		if aerr == nil && admin.MustChangePassword {
			c.JSON(http.StatusForbidden, gin.H{"code": "must_change_password", "message": "首次登录必须修改密码"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// ---------- 渠道 ----------

type channelDTO struct {
	ID                int64                 `json:"id"`
	Name              string                `json:"name"`
	Type              string                `json:"type"`
	BaseURL           string                `json:"base_url"`
	Priority          int                   `json:"priority"`
	Weight            int                   `json:"weight"`
	Models            []string              `json:"models"`
	ModelMapping      map[string]string     `json:"model_mapping"`
	Enabled           bool                  `json:"enabled"`
	Status            string                `json:"status"`
	SupportsResponses bool                  `json:"supports_responses"`
	Timeouts          store.ChannelTimeouts `json:"timeout_config"`
	KeyCount          int64                 `json:"key_count"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
}

func (a *API) toChannelDTO(ch *store.Channel, keyCount int64) channelDTO {
	return channelDTO{
		ID: ch.ID, Name: ch.Name, Type: ch.Type, BaseURL: ch.BaseURL,
		Priority: ch.Priority, Weight: ch.Weight, Models: ch.Models, ModelMapping: ch.ModelMapping,
		Enabled: ch.Enabled, Status: ch.Status, SupportsResponses: ch.SupportsResponses,
		Timeouts: ch.Timeouts, KeyCount: keyCount,
		CreatedAt: ch.CreatedAt, UpdatedAt: ch.UpdatedAt,
	}
}

func (a *API) listChannels(c *gin.Context) {
	var channels []*store.Channel
	if err := a.opt.DB.Order("priority asc, id asc").Find(&channels).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	out := make([]channelDTO, 0, len(channels))
	for _, ch := range channels {
		var keys []*store.ChannelKey
		a.opt.DB.Where("channel_id = ?", ch.ID).Find(&keys)
		status := pool.DeriveChannelStatus(keys, a.opt.Pool.StateMachine().State)
		dto := a.toChannelDTO(ch, int64(len(keys)))
		dto.Status = status
		out = append(out, dto)
	}
	ok(c, gin.H{"items": out})
}

type channelReq struct {
	Name              string                `json:"name" binding:"required"`
	Type              string                `json:"type" binding:"required"`
	BaseURL           string                `json:"base_url" binding:"required,url"`
	Priority          *int                  `json:"priority"`
	Weight            *int                  `json:"weight"`
	Models            []string              `json:"models" binding:"required,min=1"`
	ModelMapping      map[string]string     `json:"model_mapping"`
	SupportsResponses *bool                 `json:"supports_responses"`
	Timeouts          store.ChannelTimeouts `json:"timeout_config"`
}

func (a *API) createChannel(c *gin.Context) {
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	ch := &store.Channel{
		Name: req.Name, Type: req.Type, BaseURL: strings.TrimSuffix(req.BaseURL, "/"),
		Priority: derefInt(req.Priority, 0), Weight: derefInt(req.Weight, 1),
		Models: req.Models, ModelMapping: req.ModelMapping,
		Enabled: true, Status: store.ChannelStatusActive, SupportsResponses: derefBool(req.SupportsResponses, false),
		Timeouts: req.Timeouts,
	}
	if err := a.opt.DB.Create(ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建失败（名称重复？）")
		return
	}
	ok(c, a.toChannelDTO(ch, 0))
}

func (a *API) getChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var keys []*store.ChannelKey
	a.opt.DB.Where("channel_id = ?", ch.ID).Order("id asc").Find(&keys)
	keyItems := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		keyItems = append(keyItems, gin.H{
			"id": k.ID, "enabled": k.Enabled, "weight": k.Weight, "state": k.State,
			"consecutive_fails": k.ConsecutiveFails, "level": k.Level,
			"success_count": k.SuccessCount, "fail_count": k.FailCount,
			"last_error": k.LastError, "cooldown_until": k.CooldownUntil,
			"rpm_limit": k.RPMLimit, "last_used_at": k.LastUsedAt,
			// 红线：绝不返回 key_cipher / 任何明文（验收 #9）
		})
	}
	dto := a.toChannelDTO(ch, int64(len(keys)))
	ok(c, gin.H{"channel": dto, "keys": keyItems})
}

func (a *API) updateChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	ch.Name, ch.Type, ch.BaseURL = req.Name, req.Type, strings.TrimSuffix(req.BaseURL, "/")
	ch.Priority, ch.Weight = derefInt(req.Priority, ch.Priority), derefInt(req.Weight, ch.Weight)
	ch.Models, ch.ModelMapping, ch.Timeouts = req.Models, req.ModelMapping, req.Timeouts
	ch.SupportsResponses = derefBool(req.SupportsResponses, ch.SupportsResponses)
	if err := a.opt.DB.Save(ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	ok(c, a.toChannelDTO(ch, -1))
}

func (a *API) deleteChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	if err := a.opt.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", ch.ID).Delete(&store.ChannelKey{}).Error; err != nil {
			return err
		}
		return tx.Delete(ch).Error
	}); err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": true})
}

func (a *API) toggleChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	ch.Enabled = derefBool(req.Enabled, !ch.Enabled)
	if err := a.opt.DB.Model(ch).Update("enabled", ch.Enabled).Error; err != nil {
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	ok(c, gin.H{"enabled": ch.Enabled})
}

// testChannel 连通测试：取一把可用 key 请求 {base_url}/models（轻量、不耗推理额度）。
func (a *API) testChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var key *store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ? AND enabled = ?", ch.ID, true).Order("id asc").First(&key).Error; err != nil {
		fail(c, http.StatusBadRequest, "渠道没有启用的 key")
		return
	}
	plain, err := store.Decrypt(key.KeyCipher, a.opt.MasterKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "key 解密失败")
		return
	}
	url := strings.TrimSuffix(ch.BaseURL, "/") + "/models"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, herr := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if herr != nil {
		ok(c, gin.H{"ok": false, "latency_ms": latency, "error": herr.Error()})
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	ok(c, gin.H{"ok": resp.StatusCode == http.StatusOK, "status_code": resp.StatusCode, "latency_ms": latency})
}

func (a *API) loadChannel(c *gin.Context) (*store.Channel, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "非法 id")
		return nil, false
	}
	var ch store.Channel
	if err := a.opt.DB.First(&ch, id).Error; err != nil {
		fail(c, http.StatusNotFound, "渠道不存在")
		return nil, false
	}
	return &ch, true
}

// ---------- 凭据 ----------

// batchAddKeys 批量导入 key：多行纯文本，每行一把（任务书：支持多行批量粘贴）。
func (a *API) batchAddKeys(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req struct {
		Keys string `json:"keys" binding:"required"` // 多行纯文本
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	lines := strings.Split(strings.ReplaceAll(req.Keys, "\r\n", "\n"), "\n")
	added := 0
	var lastErr error
	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key == "" {
			continue
		}
		cipher, err := store.Encrypt(key, a.opt.MasterKey)
		if err != nil {
			lastErr = err
			continue
		}
		ck := &store.ChannelKey{ChannelID: ch.ID, KeyCipher: cipher, Enabled: true, Weight: 1, State: store.KeyStateActive}
		if err := a.opt.DB.Create(ck).Error; err != nil {
			lastErr = err
			continue
		}
		added++
	}
	if added == 0 && lastErr != nil {
		fail(c, http.StatusInternalServerError, "导入失败："+lastErr.Error())
		return
	}
	ok(c, gin.H{"added": added}) // 红线：不回显任何明文（验收 #9）
}

func (a *API) updateKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "非法 id")
		return
	}
	var key store.ChannelKey
	if err := a.opt.DB.First(&key, id).Error; err != nil {
		fail(c, http.StatusNotFound, "key 不存在")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
		Weight  *int  `json:"weight"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	updates := map[string]any{}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
		// 手动重新启用优先于自动状态：重置状态机（任务书 4.5③）
		if *req.Enabled && !key.Enabled {
			a.opt.Pool.Reset(key.ID)
		}
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if len(updates) > 0 {
		if err := a.opt.DB.Model(&key).Updates(updates).Error; err != nil {
			fail(c, http.StatusInternalServerError, "更新失败")
			return
		}
	}
	ok(c, gin.H{"updated": true})
}

func (a *API) deleteKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "非法 id")
		return
	}
	if err := a.opt.DB.Delete(&store.ChannelKey{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ---------- 下游令牌 ----------

type tokenDTO struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Enabled       bool       `json:"enabled"`
	Quota         int64      `json:"quota"`
	UsedQuota     int64      `json:"used_quota"`
	ExpiredAt     *time.Time `json:"expired_at"`
	AllowedModels []string   `json:"allowed_models"`
	Group         string     `json:"group"`
	CreatedAt     time.Time  `json:"created_at"`
}

func toTokenDTO(t *store.Token) tokenDTO {
	return tokenDTO{
		ID: t.ID, Name: t.Name, Enabled: t.Enabled, Quota: t.Quota, UsedQuota: t.UsedQuota,
		ExpiredAt: t.ExpiredAt, AllowedModels: t.AllowedModels, Group: t.Group, CreatedAt: t.CreatedAt,
	}
}

func (a *API) listTokens(c *gin.Context) {
	var tokens []*store.Token
	if err := a.opt.DB.Order("id desc").Find(&tokens).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	items := make([]tokenDTO, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, toTokenDTO(t))
	}
	ok(c, gin.H{"items": items})
}

func (a *API) createToken(c *gin.Context) {
	var req struct {
		Name          string     `json:"name" binding:"required"`
		Quota         *int64     `json:"quota"`
		ExpiredAt     *time.Time `json:"expired_at" time_format:"2006-01-02T15:04:05Z07:00"`
		AllowedModels []string   `json:"allowed_models"`
		Group         string     `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	// 生成 24 字节随机 key；SHA-256 入库；原文仅本次响应返回一次（任务书 4.3）
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		fail(c, http.StatusInternalServerError, "生成失败")
		return
	}
	plain := "eg-" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	group := req.Group
	if group == "" {
		group = "default"
	}
	t := &store.Token{
		Name: req.Name, KeyHash: hex.EncodeToString(sum[:]), Enabled: true,
		Quota: derefInt64(req.Quota, 0), ExpiredAt: req.ExpiredAt,
		AllowedModels: req.AllowedModels, Group: group,
	}
	if err := a.opt.DB.Create(t).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建失败")
		return
	}
	dto := toTokenDTO(t)
	ok(c, gin.H{"token": dto, "key": plain}) // 唯一一次明文返回
}

func (a *API) updateToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "非法 id")
		return
	}
	var t store.Token
	if err := a.opt.DB.First(&t, id).Error; err != nil {
		fail(c, http.StatusNotFound, "令牌不存在")
		return
	}
	var req struct {
		Name          *string    `json:"name"`
		Enabled       *bool      `json:"enabled"`
		Quota         *int64     `json:"quota"`
		ExpiredAt     *time.Time `json:"expired_at"`
		AllowedModels []string   `json:"allowed_models"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
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
		updates["quota"] = *req.Quota
	}
	if req.ExpiredAt != nil {
		updates["expired_at"] = *req.ExpiredAt
	}
	if req.AllowedModels != nil {
		updates["allowed_models"] = req.AllowedModels
	}
	if len(updates) > 0 {
		if err := a.opt.DB.Model(&t).Updates(updates).Error; err != nil {
			fail(c, http.StatusInternalServerError, "更新失败")
			return
		}
	}
	ok(c, gin.H{"updated": true})
}

func (a *API) deleteToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "非法 id")
		return
	}
	if err := a.opt.DB.Delete(&store.Token{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, "删除失败")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ---------- 请求日志 ----------

func (a *API) listLogs(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := a.opt.DB.Model(&store.RequestLog{})
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
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	var logs []*store.RequestLog
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询失败")
		return
	}
	ok(c, gin.H{"items": logs, "total": total, "page": page, "page_size": pageSize})
}

func (a *API) getLog(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "非法 id")
		return
	}
	var log store.RequestLog
	if err := a.opt.DB.First(&log, id).Error; err != nil {
		fail(c, http.StatusNotFound, "日志不存在")
		return
	}
	ok(c, gin.H{"log": log})
}

// ---------- 工具 ----------

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"ok": false, "message": msg})
}

func derefInt(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func derefInt64(p *int64, def int64) int64 {
	if p == nil {
		return def
	}
	return *p
}

func derefBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func queryInt(c *gin.Context, key string, def int) int {
	v, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return def
	}
	return v
}

func queryInt64(c *gin.Context, key string) int64 {
	v, _ := strconv.ParseInt(c.Query(key), 10, 64)
	return v
}

var _ = json.Marshal // 保持 import 稳定
