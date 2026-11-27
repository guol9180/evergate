// Package admin 实现管理台 REST API（任务书 4.4 /admin/api/*）：
// 会话鉴权、渠道与凭据管理、下游令牌管理、请求日志查询。
package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/health"
	"evergate/internal/httpx"
	"evergate/internal/pool"
	"evergate/internal/provider"
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
	// OnConfigChange 在会话写操作成功后调用，用于让转发热路径的配置快照失效。
	OnConfigChange func()
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

// invalidateOnWrite 在任何成功的非 GET 写操作后让转发配置快照失效。
// 挂在会话路由组上只声明一次，新增写接口不会漏掉失效。
func (a *API) invalidateOnWrite(c *gin.Context) {
	c.Next()
	if c.Request.Method != http.MethodGet && c.Writer.Status() < http.StatusBadRequest {
		a.opt.OnConfigChange()
	}
}

// RegisterRoutes 注册 /admin/api 全部路由（由 server.SetupRoutes 挂载）：
// 公开组只做本机访问限制（登录 / 首启 / 登出），会话组再套会话校验，中间件只在这里声明一次。
func (a *API) RegisterRoutes(r gin.IRouter) {
	public := r.Group("/admin/api", a.accessMiddleware())
	public.POST("/auth/login", a.login)
	public.GET("/auth/setup-status", a.setupStatus)
	public.POST("/auth/setup", a.setupAdmin)
	public.POST("/auth/logout", a.logout)
	// 会话保护区（sessionMiddleware 放行 me/password 以强制首登改密）
	session := r.Group("/admin/api", a.accessMiddleware(), a.sessionMiddleware())
	if a.opt.OnConfigChange != nil {
		session.Use(a.invalidateOnWrite)
	}
	session.GET("/auth/me", a.me)
	session.PUT("/auth/password", a.changePassword)
	session.GET("/system/update", a.systemUpdate)
	session.POST("/system/update/apply", a.applySystemUpdate)
	session.GET("/system/update/progress", a.systemUpdateProgress)
	session.POST("/system/update/cancel", a.cancelSystemUpdate)
	session.GET("/system/settings", a.getSystemSettings)
	session.PUT("/system/settings", a.updateSystemSettings)
	session.GET("/model-fallbacks", a.listModelFallbacks)
	session.PUT("/model-fallbacks", a.replaceModelFallbacks)
	session.GET("/usage/report", a.getUsageReport)
	session.GET("/usage/prices", a.listUsagePrices)
	session.PUT("/usage/prices/default", a.setDefaultUsagePrice)
	session.POST("/usage/prices/overrides", a.upsertUsagePriceOverride)
	session.DELETE("/usage/prices/overrides/:id", a.deleteUsagePriceOverride)
	session.POST("/channels/:id/prices/apply-preset", a.applyPresetPrice)
	session.GET("/channels", a.listChannels)
	session.GET("/model-catalog", a.modelCatalog)
	session.POST("/channels", a.createChannel)
	session.GET("/channels/:id", a.getChannel)
	session.PUT("/channels/:id", a.updateChannel)
	session.DELETE("/channels/:id", a.deleteChannel)
	session.POST("/channels/:id/toggle", a.toggleChannel)
	session.POST("/channels/:id/test", a.testChannel)
	session.POST("/channels/test-all", a.testAllChannels)
	session.POST("/channels/:id/models/preview", a.previewChannelModels)
	session.POST("/channels/:id/keys", a.batchAddKeys)
	session.PUT("/keys/:id", a.updateKey)
	session.DELETE("/keys/:id", a.deleteKey)
	session.GET("/tokens", a.listTokens)
	session.POST("/tokens", a.createToken)
	session.PUT("/tokens/:id", a.updateToken)
	session.DELETE("/tokens/:id", a.deleteToken)
	session.GET("/logs", a.listLogs)
	session.GET("/logs/stats", a.logStatsHandler)
	session.GET("/logs/models", a.logModelFacets)
	session.GET("/logs/export", a.exportLogs)
	session.GET("/logs/:id", a.getLog)
	session.GET("/config/export", a.exportConfig)
	session.POST("/config/import", a.importConfig)
	session.GET("/audit-logs", a.listAuditLogs)
	session.POST("/playground", a.playground)
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

// adminUsernamePattern 首启设置的账号规则：3–32 位字母、数字、下划线、点或连字符。
var adminUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

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

type modelPreviewItem struct {
	PublicModel   string `json:"public_model"`
	UpstreamModel string `json:"upstream_model"`
	// 上游目录若带 OpenRouter 风格 pricing，则给出微美元 / 百万 token，便于一键写入价格覆盖
	PriceMicros *provider.ModelPriceHint `json:"price_micros,omitempty"`
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

const (
	// maxKeyBatch 单次导入上限：超过就让用户分批，避免一次请求长时间占用写事务。
	maxKeyBatch = 1000
	// maxKeyChars 单把 key 的长度上限；超长行计入非法，不进入加密与入库。
	maxKeyChars = 4096
)

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
	// RPMLimit / ConcurrencyLimit 为 0 表示不限。
	RPMLimit         int       `json:"rpm_limit"`
	ConcurrencyLimit int       `json:"concurrency_limit"`
	CreatedAt        time.Time `json:"created_at"`
}

func ok(c *gin.Context, data any) {
	// 只有审计写路由需要摘要，其余响应（含列表 GET）不做任何额外处理
	if _, audited := auditActions[c.Request.Method+" "+c.FullPath()]; audited {
		stashAuditSummary(c, data)
	}
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
