// Package admin 实现管理台 REST API（任务书 4.4 /admin/api/*）：
// 会话鉴权、渠道与凭据管理、下游令牌管理、请求日志查询。
package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/health"
	"evergate/internal/httpx"
	"evergate/internal/pool"
	"evergate/internal/provider"
	"evergate/internal/router"
	"evergate/internal/selfupdate"
	"evergate/internal/settings"
	"evergate/internal/store"
	"evergate/internal/update"
	"evergate/internal/usage"
)

// Options 管理 API 装配依赖。
type Options struct {
	DB        *gorm.DB
	MasterKey string        // 解密上游 key 用于连通测试
	Pool      *pool.Manager // 渠道健康推导 / key 重置
	// AllowRemote 为 true 时才允许非 loopback 客户端访问管理 API。
	AllowRemote bool
	Now         func() time.Time
	// ModelListClient 拉取上游模型目录；nil 时使用 http.DefaultClient。
	ModelListClient provider.HTTPClient
	Session         *SessionManager
	Update          *update.Checker
	Settings        *settings.Manager
	Health          *health.Prober
	Usage           *usage.Manager
	SelfUpdate      *selfupdate.Manager
	ConfigPath      string
	DataDir         string
	ConfigSource    string
	// Playground 由 relay.HandlePlayground 提供；nil 表示未启用（测试或裁剪构建）。
	Playground gin.HandlerFunc
	// Logger 用于审计写入失败等非业务告警；nil 时静默。
	Logger *slog.Logger
}

// API 管理台接口集合。
type API struct {
	opt          Options
	now          func() time.Time
	loginLimiter *loginLimiter
	// setupMu 保证并发首启设置只有一个成功（单进程内串行；跨进程由用户名唯一索引兜底）。
	setupMu sync.Mutex
}

// New 创建管理 API。
func New(opt Options) *API {
	if opt.ModelListClient == nil {
		opt.ModelListClient = http.DefaultClient
	}
	if opt.Now == nil {
		opt.Now = func() time.Time { return time.Now().UTC() }
	}
	return &API{opt: opt, now: opt.Now, loginLimiter: newLoginLimiter()}
}

// RegisterRoutes 注册 /admin/api 全部路由（由 server.SetupRoutes 挂载）。
func (a *API) RegisterRoutes(r gin.IRoutes) {
	r.POST("/admin/api/auth/login", a.accessMiddleware(), a.login)
	r.GET("/admin/api/auth/setup-status", a.accessMiddleware(), a.setupStatus)
	r.POST("/admin/api/auth/setup", a.accessMiddleware(), a.setupAdmin)
	r.POST("/admin/api/auth/logout", a.accessMiddleware(), a.logout)
	// 会话保护区（sessionMiddleware 放行 me/password 以强制首登改密）
	r.GET("/admin/api/auth/me", a.accessMiddleware(), a.sessionMiddleware(), a.me)
	r.PUT("/admin/api/auth/password", a.accessMiddleware(), a.sessionMiddleware(), a.changePassword)
	r.GET("/admin/api/system/update", a.accessMiddleware(), a.sessionMiddleware(), a.systemUpdate)
	r.POST("/admin/api/system/update/apply", a.accessMiddleware(), a.sessionMiddleware(), a.applySystemUpdate)
	r.GET("/admin/api/system/update/progress", a.accessMiddleware(), a.sessionMiddleware(), a.systemUpdateProgress)
	r.POST("/admin/api/system/update/cancel", a.accessMiddleware(), a.sessionMiddleware(), a.cancelSystemUpdate)
	r.GET("/admin/api/system/settings", a.accessMiddleware(), a.sessionMiddleware(), a.getSystemSettings)
	r.PUT("/admin/api/system/settings", a.accessMiddleware(), a.sessionMiddleware(), a.updateSystemSettings)
	r.GET("/admin/api/model-fallbacks", a.accessMiddleware(), a.sessionMiddleware(), a.listModelFallbacks)
	r.PUT("/admin/api/model-fallbacks", a.accessMiddleware(), a.sessionMiddleware(), a.replaceModelFallbacks)
	r.GET("/admin/api/usage/report", a.accessMiddleware(), a.sessionMiddleware(), a.getUsageReport)
	r.GET("/admin/api/usage/prices", a.accessMiddleware(), a.sessionMiddleware(), a.listUsagePrices)
	r.PUT("/admin/api/usage/prices/default", a.accessMiddleware(), a.sessionMiddleware(), a.setDefaultUsagePrice)
	r.POST("/admin/api/usage/prices/overrides", a.accessMiddleware(), a.sessionMiddleware(), a.upsertUsagePriceOverride)
	r.DELETE("/admin/api/usage/prices/overrides/:id", a.accessMiddleware(), a.sessionMiddleware(), a.deleteUsagePriceOverride)
	r.POST("/admin/api/channels/:id/prices/apply-preset", a.accessMiddleware(), a.sessionMiddleware(), a.applyPresetPrice)
	r.GET("/admin/api/channels", a.accessMiddleware(), a.sessionMiddleware(), a.listChannels)
	r.GET("/admin/api/model-catalog", a.accessMiddleware(), a.sessionMiddleware(), a.modelCatalog)
	r.POST("/admin/api/channels", a.accessMiddleware(), a.sessionMiddleware(), a.createChannel)
	r.GET("/admin/api/channels/:id", a.accessMiddleware(), a.sessionMiddleware(), a.getChannel)
	r.PUT("/admin/api/channels/:id", a.accessMiddleware(), a.sessionMiddleware(), a.updateChannel)
	r.DELETE("/admin/api/channels/:id", a.accessMiddleware(), a.sessionMiddleware(), a.deleteChannel)
	r.POST("/admin/api/channels/:id/toggle", a.accessMiddleware(), a.sessionMiddleware(), a.toggleChannel)
	r.POST("/admin/api/channels/:id/test", a.accessMiddleware(), a.sessionMiddleware(), a.testChannel)
	r.POST("/admin/api/channels/test-all", a.accessMiddleware(), a.sessionMiddleware(), a.testAllChannels)
	r.POST("/admin/api/channels/:id/models/preview", a.accessMiddleware(), a.sessionMiddleware(), a.previewChannelModels)
	r.POST("/admin/api/channels/:id/keys", a.accessMiddleware(), a.sessionMiddleware(), a.batchAddKeys)
	r.PUT("/admin/api/keys/:id", a.accessMiddleware(), a.sessionMiddleware(), a.updateKey)
	r.DELETE("/admin/api/keys/:id", a.accessMiddleware(), a.sessionMiddleware(), a.deleteKey)
	r.GET("/admin/api/tokens", a.accessMiddleware(), a.sessionMiddleware(), a.listTokens)
	r.POST("/admin/api/tokens", a.accessMiddleware(), a.sessionMiddleware(), a.createToken)
	r.PUT("/admin/api/tokens/:id", a.accessMiddleware(), a.sessionMiddleware(), a.updateToken)
	r.DELETE("/admin/api/tokens/:id", a.accessMiddleware(), a.sessionMiddleware(), a.deleteToken)
	r.GET("/admin/api/logs", a.accessMiddleware(), a.sessionMiddleware(), a.listLogs)
	r.GET("/admin/api/logs/stats", a.accessMiddleware(), a.sessionMiddleware(), a.logStatsHandler)
	r.GET("/admin/api/logs/models", a.accessMiddleware(), a.sessionMiddleware(), a.logModelFacets)
	r.GET("/admin/api/logs/export", a.accessMiddleware(), a.sessionMiddleware(), a.exportLogs)
	r.GET("/admin/api/logs/:id", a.accessMiddleware(), a.sessionMiddleware(), a.getLog)
	r.GET("/admin/api/config/export", a.accessMiddleware(), a.sessionMiddleware(), a.exportConfig)
	r.POST("/admin/api/config/import", a.accessMiddleware(), a.sessionMiddleware(), a.importConfig)
	r.GET("/admin/api/audit-logs", a.accessMiddleware(), a.sessionMiddleware(), a.listAuditLogs)
	r.POST("/admin/api/playground", a.accessMiddleware(), a.sessionMiddleware(), a.playground)
}

// playground 仅负责会话保护；具体调度与流式输出由 relay 提供。
func (a *API) playground(c *gin.Context) {
	if a.opt.Playground == nil {
		fail(c, http.StatusNotImplemented, "playground_disabled", "Playground 未启用")
		return
	}
	a.opt.Playground(c)
}

// ---------- 会话 ----------

func (a *API) accessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.opt.AllowRemote || isLoopback(c.ClientIP()) {
			c.Next()
			return
		}
		fail(c, http.StatusForbidden, "admin_local_only", "管理 API 仅允许本机访问；远程管理请显式关闭 admin_loopback_only")
		c.Abort()
	}
}

func (a *API) login(c *gin.Context) {
	clientIP := c.ClientIP()
	if wait := a.loginLimiter.blockedFor(clientIP, a.now()); wait > 0 {
		seconds := (wait + time.Second - 1) / time.Second
		c.Header("Retry-After", strconv.FormatInt(int64(seconds), 10))
		fail(c, http.StatusTooManyRequests, "login_rate_limited", "登录尝试过多，请稍后重试")
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "credentials_required", "请输入用户名和密码")
		return
	}
	admin, err := store.GetAdminByUsername(a.opt.DB, req.Username)
	if err != nil || !store.CheckAdminPassword(admin, req.Password) {
		a.loginLimiter.recordFailure(clientIP, a.now())
		fail(c, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	a.loginLimiter.recordSuccess(clientIP)
	now := a.now()
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(CookieName, a.opt.Session.Issue(req.Username, now), int(a.opt.Session.ttl.Seconds()), "/", "", c.Request.TLS != nil, true)
	ok(c, gin.H{"username": admin.Username, "must_change_password": admin.MustChangePassword})
}

// adminUsernamePattern 首启设置的账号规则：3–32 位字母、数字、下划线、点或连字符。
var adminUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

// setupStatus 供登录页判断是否需要先完成首启管理员设置。
func (a *API) setupStatus(c *gin.Context) {
	required, err := store.SetupRequired(a.opt.DB)
	if err != nil {
		fail(c, http.StatusInternalServerError, "setup_status_failed", "查询初始化状态失败")
		return
	}
	ok(c, gin.H{"setup_required": required})
}

// setupAdmin 首启管理员设置：仅在 admins 为空且请求来自本机时允许；成功后不自动登录。
func (a *API) setupAdmin(c *gin.Context) {
	if !isLoopback(c.ClientIP()) {
		fail(c, http.StatusForbidden, "setup_local_only", "初始管理员设置仅允许本机访问")
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	username := strings.TrimSpace(req.Username)
	if !adminUsernamePattern.MatchString(username) {
		fail(c, http.StatusBadRequest, "invalid_username", "用户名需为 3–32 位字母、数字、下划线、点或连字符")
		return
	}
	if err := store.ValidateAdminPassword(req.Password); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	// 单进程内串行，保证并发设置只有一个成功；跨进程由用户名唯一索引兜底
	a.setupMu.Lock()
	defer a.setupMu.Unlock()
	required, err := store.SetupRequired(a.opt.DB)
	if err != nil {
		fail(c, http.StatusInternalServerError, "setup_status_failed", "查询初始化状态失败")
		return
	}
	if !required {
		fail(c, http.StatusForbidden, "already_initialized", "管理员已存在，无法重复初始化")
		return
	}
	if err := store.CreateAdmin(a.opt.DB, username, req.Password); err != nil {
		if stillRequired, _ := store.SetupRequired(a.opt.DB); !stillRequired {
			fail(c, http.StatusForbidden, "already_initialized", "管理员已存在，无法重复初始化")
			return
		}
		fail(c, http.StatusInternalServerError, "setup_failed", "创建管理员失败")
		return
	}
	c.Set("admin", username)
	a.recordAudit(c, "setup_admin", "", "username="+username)
	ok(c, gin.H{"username": username, "setup": true})
}

func (a *API) logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(CookieName, "", -1, "/", "", false, true)
	ok(c, gin.H{"logout": true})
}

func (a *API) me(c *gin.Context) {
	admin, err := store.GetAdminByUsername(a.opt.DB, c.GetString("admin"))
	if err != nil {
		fail(c, http.StatusUnauthorized, "session_invalid", "会话无效")
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
		fail(c, http.StatusBadRequest, "password_too_short", "参数错误（新密码至少 8 位）")
		return
	}
	admin, err := store.GetAdminByUsername(a.opt.DB, c.GetString("admin"))
	if err != nil || !store.CheckAdminPassword(admin, req.OldPassword) {
		fail(c, http.StatusUnauthorized, "wrong_old_password", "旧密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "hash_failed", "加密失败")
		return
	}
	if err := a.opt.DB.Model(&store.Admin{}).Where("id = ?", admin.ID).
		Updates(map[string]any{"password_hash": string(hash), "must_change_password": false}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
		return
	}
	ok(c, gin.H{"changed": true})
}

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

// sessionMiddleware 校验会话 Cookie；首登未改密前仅放行 me/password（强制改密）。
// 请求结束后对白名单内的写操作落审计（T75）。
func (a *API) sessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.authenticateSession(c) {
			return
		}
		c.Next()
		a.auditWrite(c)
	}
}

// authenticateSession 校验会话并在需要时中止请求；返回 false 表示已中止。
func (a *API) authenticateSession(c *gin.Context) bool {
	token, err := c.Cookie(CookieName)
	if err != nil || token == "" {
		fail(c, http.StatusUnauthorized, "unauthorized", "未登录")
		c.Abort()
		return false
	}
	username, verr := a.opt.Session.Verify(token, time.Now())
	if verr != nil {
		fail(c, http.StatusUnauthorized, "session_expired", "会话无效或已过期")
		c.Abort()
		return false
	}
	c.Set("admin", username)
	// 强制首登改密：me/password 放行，其余一律 403
	path := c.FullPath()
	if strings.HasSuffix(path, "/auth/me") || strings.HasSuffix(path, "/auth/password") {
		return true
	}
	admin, aerr := store.GetAdminByUsername(a.opt.DB, username)
	if aerr == nil && admin.MustChangePassword {
		httpx.FailCode(c, http.StatusForbidden, "must_change_password", "首次登录必须修改密码")
		c.Abort()
		return false
	}
	return true
}

// ---------- 渠道 ----------

type channelDTO struct {
	PresetID            string                `json:"preset_id"`
	ID                  int64                 `json:"id"`
	Name                string                `json:"name"`
	Type                string                `json:"type"`
	BaseURL             string                `json:"base_url"`
	Priority            int                   `json:"priority"`
	Weight              int                   `json:"weight"`
	Models              []string              `json:"models"`
	ModelMapping        map[string]string     `json:"model_mapping"`
	Enabled             bool                  `json:"enabled"`
	Status              string                `json:"status"`
	AvailableKeyCount   int64                 `json:"available_key_count"`
	LastProbeAt         *time.Time            `json:"last_probe_at"`
	LastProbeResult     string                `json:"last_probe_result"`
	LastProbeStatusCode int                   `json:"last_probe_status_code"`
	LastProbeLatencyMS  int64                 `json:"last_probe_latency_ms"`
	LastProbeErrorType  string                `json:"last_probe_error_type"`
	LastProbeError      string                `json:"last_probe_error"`
	NextProbeAt         *time.Time            `json:"next_probe_at"`
	SupportsResponses   bool                  `json:"supports_responses"`
	SupportsAnthropic   bool                  `json:"supports_anthropic"`
	SupportsGemini      bool                  `json:"supports_gemini"`
	Timeouts            store.ChannelTimeouts `json:"timeout_config"`
	KeyCount            int64                 `json:"key_count"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

func (a *API) toChannelDTO(ch *store.Channel, keyCount, availableKeyCount int64, status string) channelDTO {
	return channelDTO{
		ID: ch.ID, Name: ch.Name, Type: ch.Type, BaseURL: ch.BaseURL,
		PresetID: ch.PresetID,
		Priority: ch.Priority, Weight: ch.Weight, Models: ch.Models, ModelMapping: ch.ModelMapping,
		Enabled: ch.Enabled, Status: status, SupportsResponses: ch.SupportsResponses,
		SupportsAnthropic: ch.SupportsAnthropic, SupportsGemini: ch.SupportsGemini,
		AvailableKeyCount: availableKeyCount,
		LastProbeAt:       ch.LastProbeAt, LastProbeResult: ch.LastProbeResult,
		LastProbeStatusCode: ch.LastProbeStatusCode, LastProbeLatencyMS: ch.LastProbeLatencyMS,
		LastProbeErrorType: ch.LastProbeErrorType, LastProbeError: ch.LastProbeError,
		NextProbeAt: ch.NextProbeAt,
		Timeouts:    ch.Timeouts, KeyCount: keyCount,
		CreatedAt: ch.CreatedAt, UpdatedAt: ch.UpdatedAt,
	}
}

func (a *API) listChannels(c *gin.Context) {
	var channels []*store.Channel
	if err := a.opt.DB.Order("priority asc, id asc").Find(&channels).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	var keys []*store.ChannelKey
	if err := a.opt.DB.Order("id asc").Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	keysByChannel := make(map[int64][]*store.ChannelKey)
	for _, key := range keys {
		keysByChannel[key.ChannelID] = append(keysByChannel[key.ChannelID], key)
	}
	out := make([]channelDTO, 0, len(channels))
	for _, ch := range channels {
		channelKeys := keysByChannel[ch.ID]
		status := health.DeriveStatus(ch, channelKeys, a.opt.Pool.StateMachine().State)
		out = append(out, a.toChannelDTO(ch, int64(len(channelKeys)), int64(health.AvailableKeyCount(channelKeys, a.opt.Pool.StateMachine().State)), status))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if health.StatusRank(out[i].Status) != health.StatusRank(out[j].Status) {
			return health.StatusRank(out[i].Status) < health.StatusRank(out[j].Status)
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	ok(c, gin.H{"items": out})
}

type channelReq struct {
	Name              string                `json:"name" binding:"required"`
	Type              string                `json:"type" binding:"required"`
	PresetID          *string               `json:"preset_id"`
	BaseURL           string                `json:"base_url" binding:"required,url"`
	Priority          *int                  `json:"priority"`
	Weight            *int                  `json:"weight"`
	Models            []string              `json:"models"` // 允许留空：保存并导入 key 后再「从上游拉取」（T89）
	ModelMapping      map[string]string     `json:"model_mapping"`
	SupportsResponses *bool                 `json:"supports_responses"`
	SupportsAnthropic *bool                 `json:"supports_anthropic"`
	SupportsGemini    *bool                 `json:"supports_gemini"`
	Timeouts          store.ChannelTimeouts `json:"timeout_config"`
}

func (r *channelReq) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Type = strings.TrimSpace(r.Type)
	if r.PresetID != nil {
		*r.PresetID = strings.TrimSpace(*r.PresetID)
	}
	r.BaseURL = strings.TrimSpace(strings.TrimSuffix(r.BaseURL, "/"))
	if r.Name == "" || r.Type == "" {
		return errors.New("name 和 type 不能为空")
	}
	parsed, err := url.Parse(r.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("base_url 必须是 http(s) 地址")
	}
	if priority := derefInt(r.Priority, 0); priority < -1000 || priority > 1000 {
		return errors.New("priority 必须在 -1000-1000")
	}
	if weight := derefInt(r.Weight, 1); weight < 1 || weight > 10000 {
		return errors.New("weight 必须在 1-10000")
	}
	if len(r.Models) > 500 {
		return errors.New("models 最多 500 个")
	}
	models := make([]string, 0, len(r.Models))
	modelSet := make(map[string]bool, len(r.Models))
	for i, model := range r.Models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 200 {
			return fmt.Errorf("models[%d] 必须非空且不超过 200 字符", i)
		}
		if modelSet[model] {
			continue
		}
		modelSet[model] = true
		models = append(models, model)
	}
	// 允许空模型列表：保存并导入 key 后再「从上游拉取」模型目录（T89）
	if len(r.ModelMapping) > 500 {
		return errors.New("model_mapping 最多 500 项")
	}
	mapping := make(map[string]string, len(r.ModelMapping))
	for publicModel, upstreamModel := range r.ModelMapping {
		publicModel = strings.TrimSpace(publicModel)
		upstreamModel = strings.TrimSpace(upstreamModel)
		if publicModel == "" || upstreamModel == "" || len(publicModel) > 200 || len(upstreamModel) > 200 {
			return errors.New("model_mapping 的键和值必须非空且不超过 200 字符")
		}
		if !modelSet[publicModel] {
			return fmt.Errorf("model_mapping 的公开模型 %q 不在 models 中", publicModel)
		}
		mapping[publicModel] = upstreamModel
	}
	if err := validateChannelTimeouts(r.Timeouts); err != nil {
		return err
	}
	r.Models = models
	r.ModelMapping = mapping
	return nil
}

func validateChannelTimeouts(t store.ChannelTimeouts) error {
	connect, first, overall := 0, 0, 0
	if t.ConnectSec != nil {
		connect = *t.ConnectSec
		if connect < 1 || connect > 300 {
			return errors.New("timeout_config.connect_sec 必须在 1-300")
		}
	}
	if t.FirstTokenSec != nil {
		first = *t.FirstTokenSec
		if first < 1 || first > 3600 {
			return errors.New("timeout_config.first_token_sec 必须在 1-3600")
		}
	}
	if t.OverallSec != nil {
		overall = *t.OverallSec
		if overall < 1 || overall > 86400 {
			return errors.New("timeout_config.overall_sec 必须在 1-86400")
		}
		if (connect > 0 && overall < connect) || (first > 0 && overall < first) {
			return errors.New("timeout_config.overall_sec 不能小于 connect_sec/first_token_sec")
		}
	}
	return nil
}

func (a *API) validateEffectiveChannelTimeouts(t store.ChannelTimeouts) error {
	defaults := config.Default().Defaults
	if a.opt.Settings != nil {
		defaults = a.opt.Settings.Snapshot().ConfigDefaults()
	}
	connect := defaults.ConnectTimeout.Duration
	first := defaults.FirstTokenTimeout.Duration
	overall := defaults.OverallTimeout.Duration
	if t.ConnectSec != nil {
		connect = time.Duration(*t.ConnectSec) * time.Second
	}
	if t.FirstTokenSec != nil {
		first = time.Duration(*t.FirstTokenSec) * time.Second
	}
	if t.OverallSec != nil {
		overall = time.Duration(*t.OverallSec) * time.Second
	}
	if overall < connect || overall < first {
		return errors.New("timeout_config.overall_sec 不能小于有效 connect/first_token 超时")
	}
	return nil
}

func (a *API) createChannel(c *gin.Context) {
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	if err := req.normalize(); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	if err := a.validateEffectiveChannelTimeouts(req.Timeouts); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	ch := &store.Channel{
		Name: req.Name, Type: req.Type, BaseURL: req.BaseURL,
		PresetID: derefString(req.PresetID, ""),
		Priority: derefInt(req.Priority, 0), Weight: derefInt(req.Weight, 1),
		Models: req.Models, ModelMapping: req.ModelMapping,
		Enabled: true, Status: store.ChannelStatusActive, SupportsResponses: derefBool(req.SupportsResponses, false),
		SupportsAnthropic: derefBool(req.SupportsAnthropic, false),
		SupportsGemini:    derefBool(req.SupportsGemini, false),
		Timeouts:          req.Timeouts,
	}
	if err := a.opt.DB.Create(ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, "channel_create_failed", "创建失败（名称重复？）")
		return
	}
	a.reloadPrices()
	ok(c, a.toChannelDTO(ch, 0, 0, store.ChannelStatusActive))
}

func (a *API) getChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var keys []*store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ?", ch.ID).Order("id asc").Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "key_query_failed", "查询 key 失败")
		return
	}
	keyItems := make([]gin.H, 0, len(keys))
	stateFn := a.opt.Pool.StateMachine().State
	for _, k := range keys {
		remaining := a.opt.Pool.StateMachine().CooldownRemaining(k.ID)
		keyItems = append(keyItems, gin.H{
			"id": k.ID, "enabled": k.Enabled, "weight": k.Weight, "state": stateFn(k.ID),
			"consecutive_fails": k.ConsecutiveFails, "level": k.Level,
			"success_count": k.SuccessCount, "fail_count": k.FailCount,
			"last_error": k.LastError, "cooldown_until": k.CooldownUntil,
			"cooldown_remaining_sec": int64(remaining / time.Second),
			"rpm_limit":              k.RPMLimit, "last_used_at": k.LastUsedAt,
			// 红线：绝不返回 key_cipher / 任何明文（验收 #9）
		})
	}
	status := health.DeriveStatus(ch, keys, a.opt.Pool.StateMachine().State)
	dto := a.toChannelDTO(ch, int64(len(keys)), int64(health.AvailableKeyCount(keys, a.opt.Pool.StateMachine().State)), status)
	ok(c, gin.H{"channel": dto, "keys": keyItems, "prices": a.effectivePrices(ch)})
}

// upstreamModels 把对外模型名换成供应商实际计费的上游模型名（无映射时同名）。
func upstreamModels(ch *store.Channel) []string {
	out := make([]string, 0, len(ch.Models))
	for _, model := range ch.Models {
		if upstream := ch.ModelMapping[model]; upstream != "" {
			out = append(out, upstream)
		} else {
			out = append(out, model)
		}
	}
	return out
}

func (a *API) effectivePrices(ch *store.Channel) []usage.EffectivePrice {
	if a.opt.Usage == nil {
		return nil
	}
	return a.opt.Usage.Effective(ch.ID, upstreamModels(ch))
}

// reloadPrices 渠道的 preset_id 决定内置价格目录能否命中，渠道增删改后刷新计价快照。
func (a *API) reloadPrices() {
	if a.opt.Usage != nil {
		_ = a.opt.Usage.Reload()
	}
}

// applyPresetPrice 把渠道预设目录里的价格一键写成该渠道的模型覆盖（只补缺不覆盖）。
func (a *API) applyPresetPrice(c *gin.Context) {
	if a.opt.Usage == nil {
		fail(c, http.StatusServiceUnavailable, "pricing_not_ready", "计价配置未初始化")
		return
	}
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	if ch.PresetID == "" {
		fail(c, http.StatusBadRequest, "channel_not_preset", "该渠道不是预设渠道，没有可用的预设价格")
		return
	}
	created, err := a.opt.Usage.ApplyPreset(c.Request.Context(), ch.ID, ch.PresetID, upstreamModels(ch))
	if err != nil {
		fail(c, http.StatusInternalServerError, "preset_price_write_failed", "写入预设价格失败")
		return
	}
	ok(c, gin.H{"created": created, "prices": a.effectivePrices(ch)})
}

func (a *API) updateChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	if err := req.normalize(); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	if err := a.validateEffectiveChannelTimeouts(req.Timeouts); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	ch.Name, ch.Type, ch.BaseURL = req.Name, req.Type, req.BaseURL
	ch.PresetID = derefString(req.PresetID, "")
	ch.Priority, ch.Weight = derefInt(req.Priority, ch.Priority), derefInt(req.Weight, ch.Weight)
	ch.Models, ch.ModelMapping, ch.Timeouts = req.Models, req.ModelMapping, req.Timeouts
	ch.SupportsResponses = derefBool(req.SupportsResponses, ch.SupportsResponses)
	ch.SupportsAnthropic = derefBool(req.SupportsAnthropic, ch.SupportsAnthropic)
	ch.SupportsGemini = derefBool(req.SupportsGemini, ch.SupportsGemini)
	if err := a.opt.DB.Save(ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
		return
	}
	a.reloadPrices()
	var keys []*store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ?", ch.ID).Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	status := health.DeriveStatus(ch, keys, a.opt.Pool.StateMachine().State)
	ok(c, a.toChannelDTO(ch, int64(len(keys)), int64(health.AvailableKeyCount(keys, a.opt.Pool.StateMachine().State)), status))
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
		fail(c, http.StatusInternalServerError, "delete_failed", "删除失败")
		return
	}
	a.reloadPrices()
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
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	ch.Enabled = derefBool(req.Enabled, !ch.Enabled)
	if err := a.opt.DB.Model(ch).Update("enabled", ch.Enabled).Error; err != nil {
		fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
		return
	}
	ok(c, gin.H{"enabled": ch.Enabled})
}

// testChannel 手动连通测试：与后台健康探测共用协议化模型列表 GET，
// 结果同样驱动 half_open/cooldown 状态机并持久化渠道健康摘要。
func (a *API) testChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	if a.opt.Health == nil {
		fail(c, http.StatusInternalServerError, "prober_not_ready", "健康探测器未初始化")
		return
	}
	var keys []*store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ? AND enabled = ?", ch.ID, true).Order("id asc").Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	timeout, nextInterval := 10*time.Second, 5*time.Minute
	if a.opt.Settings != nil {
		current := a.opt.Settings.Snapshot()
		timeout = time.Duration(current.HealthProbeTimeoutMS) * time.Millisecond
		nextInterval = time.Duration(current.HealthProbeIntervalMS) * time.Millisecond
	}
	result, err := a.opt.Health.ProbeChannel(
		c.Request.Context(), ch, keys, timeout, nextInterval,
	)
	if err != nil {
		if errors.Is(err, health.ErrNoAvailableKey) {
			fail(c, http.StatusBadRequest, "channel_no_key", "渠道没有可用 key")
			return
		}
		if errors.Is(err, health.ErrChannelDisabled) {
			fail(c, http.StatusBadRequest, "channel_disabled", "渠道已停用")
			return
		}
		if errors.Is(err, context.Canceled) {
			fail(c, http.StatusRequestTimeout, "test_cancelled", "测试请求已取消")
			return
		}
		fail(c, http.StatusInternalServerError, "test_failed", "测试失败")
		return
	}
	ok(c, gin.H{
		"ok": result.Result == health.ProbeResultSuccess, "channel_id": result.ChannelID,
		"key_id": result.KeyID, "result": result.Result, "status": result.Status,
		"status_code": result.StatusCode, "latency_ms": result.LatencyMS,
		"error_type": result.ErrorType, "error": result.Error,
		"probed_at": result.ProbedAt, "next_probe_at": result.NextProbeAt,
	})
}

type modelPreviewItem struct {
	PublicModel   string `json:"public_model"`
	UpstreamModel string `json:"upstream_model"`
	// 上游目录若带 OpenRouter 风格 pricing，则给出微美元 / 百万 token，便于一键写入价格覆盖
	PriceMicros *provider.ModelPriceHint `json:"price_micros,omitempty"`
}

// previewChannelModels 拉取渠道上游模型目录并按现有映射反查对外别名。
// 该操作只读取一把启用 key，不经过 pool，不改变 key 调度状态。
func (a *API) previewChannelModels(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var key store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ? AND enabled = ?", ch.ID, true).
		Order("id asc").First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusBadRequest, "channel_no_enabled_key", "渠道没有启用的 key，请先导入 key")
			return
		}
		fail(c, http.StatusInternalServerError, "key_query_failed", "查询 key 失败")
		return
	}
	plainKey, err := store.Decrypt(key.KeyCipher, a.opt.MasterKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "key_decrypt_failed", "解密上游 key 失败")
		return
	}

	timeout := 10 * time.Second
	if a.opt.Settings != nil {
		timeout = time.Duration(a.opt.Settings.Snapshot().HealthProbeTimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	result, err := provider.ListModels(ctx, a.opt.ModelListClient, ch, plainKey)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			fail(c, http.StatusGatewayTimeout, "models_fetch_timeout", "拉取模型目录超时")
		case errors.Is(err, context.Canceled):
			fail(c, http.StatusRequestTimeout, "models_fetch_cancelled", "拉取模型目录已取消")
		default:
			fail(c, http.StatusBadGateway, "models_fetch_failed", "拉取模型目录失败："+err.Error())
		}
		return
	}

	aliasesByUpstream := make(map[string][]string)
	for publicModel, upstreamModel := range ch.ModelMapping {
		if publicModel == "" || upstreamModel == "" {
			continue
		}
		aliasesByUpstream[upstreamModel] = append(aliasesByUpstream[upstreamModel], publicModel)
	}
	items := make([]modelPreviewItem, 0, len(result.Models))
	for _, upstreamModel := range result.Models {
		aliases := aliasesByUpstream[upstreamModel]
		var hint *provider.ModelPriceHint
		if price, ok := result.Prices[upstreamModel]; ok {
			hint = &price
		}
		if len(aliases) == 0 {
			items = append(items, modelPreviewItem{PublicModel: upstreamModel, UpstreamModel: upstreamModel, PriceMicros: hint})
			continue
		}
		sort.Strings(aliases)
		for _, publicModel := range aliases {
			items = append(items, modelPreviewItem{PublicModel: publicModel, UpstreamModel: upstreamModel, PriceMicros: hint})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublicModel != items[j].PublicModel {
			return items[i].PublicModel < items[j].PublicModel
		}
		return items[i].UpstreamModel < items[j].UpstreamModel
	})
	ok(c, gin.H{
		"items": items, "protocol": result.Protocol,
		"pages": result.Pages, "truncated": result.Truncated,
	})
}

type catalogChannelRef struct {
	ChannelID     int64  `json:"channel_id"`
	ChannelName   string `json:"channel_name"`
	UpstreamModel string `json:"upstream_model"`
}

type catalogModel struct {
	Model    string              `json:"model"`
	Channels []catalogChannelRef `json:"channels"`
}

// modelCatalog 为令牌模型选择器提供启用渠道 Models 并集；与下游 /v1/models 共用归一化逻辑。
func (a *API) modelCatalog(c *gin.Context) {
	var channels []*store.Channel
	if err := a.opt.DB.Where("enabled = ?", true).Order("id asc").Find(&channels).Error; err != nil {
		fail(c, http.StatusInternalServerError, "catalog_query_failed", "查询模型目录失败")
		return
	}
	models := router.PublicModelCatalog(channels)
	items := make([]catalogModel, 0, len(models))
	for _, model := range models {
		item := catalogModel{Model: model, Channels: []catalogChannelRef{}}
		for _, ch := range channels {
			if !router.SupportsModel(ch, model) {
				continue
			}
			item.Channels = append(item.Channels, catalogChannelRef{
				ChannelID: ch.ID, ChannelName: ch.Name,
				UpstreamModel: router.UpstreamModel(ch.ModelMapping, model),
			})
		}
		items = append(items, item)
	}
	ok(c, gin.H{"items": items})
}

func (a *API) loadChannel(c *gin.Context) (*store.Channel, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return nil, false
	}
	var ch store.Channel
	if err := a.opt.DB.First(&ch, id).Error; err != nil {
		fail(c, http.StatusNotFound, "channel_not_found", "渠道不存在")
		return nil, false
	}
	return &ch, true
}

// ---------- 凭据 ----------

const (
	// maxKeyBatch 单次导入上限：超过就让用户分批，避免一次请求长时间占用写事务。
	maxKeyBatch = 1000
	// maxKeyChars 单把 key 的长度上限；超长行计入非法，不进入加密与入库。
	maxKeyChars = 4096
)

// batchAddKeys 批量导入 key：多行纯文本，每行一把；批内与渠道存量都去重，
// 单事务批量写入，返回新增 / 重复 / 非法计数（不回显任何明文）。
func (a *API) batchAddKeys(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req struct {
		Keys string `json:"keys" binding:"required"` // 多行纯文本
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	lines := strings.Split(strings.ReplaceAll(req.Keys, "\r\n", "\n"), "\n")
	seen := make(map[string]bool, len(lines))
	plaintexts := make([]string, 0, len(lines))
	duplicates, invalid := 0, 0
	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key == "" {
			continue
		}
		if len(key) > maxKeyChars {
			invalid++
			continue
		}
		if seen[key] {
			duplicates++
			continue
		}
		seen[key] = true
		plaintexts = append(plaintexts, key)
	}
	if len(plaintexts) > maxKeyBatch {
		fail(c, http.StatusBadRequest, "key_batch_too_large", fmt.Sprintf("单次最多导入 %d 把 key，请分批粘贴", maxKeyBatch))
		return
	}
	// 与渠道存量 key 去重：同一把上游 key 重复入库会变成双倍权重，只能解密比对。
	if len(plaintexts) > 0 {
		var existingKeys []*store.ChannelKey
		if err := a.opt.DB.Where("channel_id = ?", ch.ID).Find(&existingKeys).Error; err != nil {
			fail(c, http.StatusInternalServerError, "key_query_failed", "查询已有 key 失败")
			return
		}
		existing := make(map[string]bool, len(existingKeys))
		for _, k := range existingKeys {
			if plain, derr := store.Decrypt(k.KeyCipher, a.opt.MasterKey); derr == nil {
				existing[plain] = true
			}
		}
		kept := plaintexts[:0]
		for _, key := range plaintexts {
			if existing[key] {
				duplicates++
				continue
			}
			kept = append(kept, key)
		}
		plaintexts = kept
	}
	items := make([]*store.ChannelKey, 0, len(plaintexts))
	for _, key := range plaintexts {
		cipher, err := store.Encrypt(key, a.opt.MasterKey)
		if err != nil {
			if a.opt.Logger != nil {
				a.opt.Logger.Warn("encrypt channel key failed", slog.Any("error", err))
			}
			fail(c, http.StatusInternalServerError, "key_encrypt_failed", "加密 key 失败")
			return
		}
		items = append(items, &store.ChannelKey{ChannelID: ch.ID, KeyCipher: cipher, Enabled: true, Weight: 1, State: store.KeyStateActive})
	}
	if len(items) > 0 {
		if err := a.opt.DB.Transaction(func(tx *gorm.DB) error {
			return tx.CreateInBatches(items, 100).Error
		}); err != nil {
			if a.opt.Logger != nil {
				a.opt.Logger.Warn("batch add channel keys failed", slog.Any("error", err))
			}
			fail(c, http.StatusInternalServerError, "key_import_rolled_back", "导入失败，已回滚")
			return
		}
	}
	ok(c, gin.H{"added": len(items), "duplicates": duplicates, "invalid": invalid})
}

func (a *API) updateKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return
	}
	var key store.ChannelKey
	if err := a.opt.DB.First(&key, id).Error; err != nil {
		fail(c, http.StatusNotFound, "key_not_found", "key 不存在")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
		Weight  *int  `json:"weight"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
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
			fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
			return
		}
	}
	ok(c, gin.H{"updated": true})
}

func (a *API) deleteKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return
	}
	if err := a.opt.DB.Delete(&store.ChannelKey{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, "delete_failed", "删除失败")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// ---------- 下游令牌 ----------

type tokenDTO struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	Quota           int64      `json:"quota"`
	UsedQuota       int64      `json:"used_quota"`
	RemainingQuota  int64      `json:"remaining_quota"`
	Status          string     `json:"status"`
	ExpiredAt       *time.Time `json:"expired_at"`
	AllowedModels   []string   `json:"allowed_models"`
	AllowedIPs      []string   `json:"allowed_ips"`
	DisableFallback bool       `json:"disable_fallback"`
	CreatedAt       time.Time  `json:"created_at"`
}

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
		fail(c, http.StatusBadRequest, "invalid_param", filterErr.Error())
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

func ok(c *gin.Context, data any) {
	stashAuditSummary(c, data)
	httpx.OK(c, data)
}

// fail 是管理台错误信封的唯一出口：每个调用点必须给出稳定 code，前端按 code 本地化，message 只作兜底。
func fail(c *gin.Context, status int, code, msg string) {
	httpx.FailCode(c, status, code, msg)
}

func derefString(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
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

// parseQueryInt64 解析可选的整数查询参数：缺省返回 0，非法返回带字段名的错误。
func parseQueryInt64(c *gin.Context, key string) (int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s 必须是整数", key)
	}
	return v, nil
}
