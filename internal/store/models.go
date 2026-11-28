package store

import "time"

// 渠道健康状态（由健康任务 + 实时错误统计推导）。
const (
	ChannelStatusActive       = "active"
	ChannelStatusDegraded     = "degraded"
	ChannelStatusDown         = "down"
	ChannelStatusInconclusive = "inconclusive"
)

// key 状态机取值（active/cooldown/half_open/disabled，详见任务书 4.5③）。
const (
	KeyStateActive   = "active"
	KeyStateCooldown = "cooldown"
	KeyStateHalfOpen = "half_open"
	KeyStateDisabled = "disabled"
)

// Channel 是上游渠道（任务书 4.3 channels 表）。
type Channel struct {
	ID                  int64             `gorm:"primaryKey;autoIncrement" json:"id"`
	Name                string            `gorm:"size:100;not null;uniqueIndex" json:"name"`
	Type                string            `gorm:"size:32;not null" json:"type"`                 // openai / openai-compatible / deepseek / vllm / anthropic / gemini ...
	PresetID            string            `gorm:"size:64;not null;default:''" json:"preset_id"` // 供应商预设 id；自定义为空（T91）
	BaseURL             string            `gorm:"column:base_url;not null" json:"base_url"`
	Priority            int               `gorm:"not null;default:0;index" json:"priority"` // 越小越优先
	Weight              int               `gorm:"not null;default:1" json:"weight"`         // 同优先级内加权随机
	Models              []string          `gorm:"serializer:json" json:"models"`
	ModelMapping        map[string]string `gorm:"serializer:json" json:"model_mapping"` // {请求模型: 上游模型}
	Enabled             bool              `gorm:"not null;default:true" json:"enabled"`
	Status              string            `gorm:"size:16;not null;default:'active'" json:"status"`
	LastProbeAt         *time.Time        `json:"last_probe_at"`
	LastProbeResult     string            `gorm:"size:16;not null;default:''" json:"last_probe_result"`
	LastProbeStatusCode int               `gorm:"not null;default:0" json:"last_probe_status_code"`
	LastProbeLatencyMS  int64             `gorm:"not null;default:0" json:"last_probe_latency_ms"`
	LastProbeErrorType  string            `gorm:"size:32;not null;default:''" json:"last_probe_error_type"`
	LastProbeError      string            `json:"last_probe_error"`
	NextProbeAt         *time.Time        `gorm:"index" json:"next_probe_at"`
	// SupportsResponses 表示该渠道可原生接收 /responses；否则 /v1/responses 走 CC 桥接。
	SupportsResponses bool `gorm:"not null;default:false" json:"supports_responses"`
	// SupportsAnthropic 表示该渠道可原生接收 /v1/messages；否则 /v1/messages 走 CC 桥接。
	SupportsAnthropic bool `gorm:"not null;default:false" json:"supports_anthropic"`
	// SupportsGemini 表示该渠道可原生接收 /v1beta/models/*；否则 Gemini 入口走 CC 桥接。
	SupportsGemini bool            `gorm:"not null;default:false" json:"supports_gemini"`
	Timeouts       ChannelTimeouts `gorm:"column:timeout_config;serializer:json" json:"timeout_config"`
	// CatalogDiff 是最近一次上游目录同步与渠道模型的差异；只提示不自动改写模型列表，CheckedAt 零值 = 未同步。
	CatalogDiff ChannelCatalogDiff `gorm:"column:catalog_diff;serializer:json" json:"catalog_diff"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// ChannelCatalogDiff 上游目录相对渠道配置模型的差异（按上游模型名比较，已考虑 model_mapping）。
type ChannelCatalogDiff struct {
	Added         []string  `json:"added"`
	Removed       []string  `json:"removed"`
	UpstreamCount int       `json:"upstream_count"`
	CheckedAt     time.Time `json:"checked_at"`
}

// ChannelTimeouts 渠道级超时覆盖（秒；nil 表示跟随全局默认）。
type ChannelTimeouts struct {
	ConnectSec    *int `json:"connect_sec,omitempty"`
	FirstTokenSec *int `json:"first_token_sec,omitempty"`
	OverallSec    *int `json:"overall_sec,omitempty"`
}

// ChannelKey 是上游凭据池中的一条 key（任务书 4.3 channel_keys 表）。
type ChannelKey struct {
	ID               int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID        int64      `gorm:"not null;index" json:"channel_id"`
	KeyCipher        string     `gorm:"column:key_cipher;not null" json:"-"` // AES-GCM base64，任何接口不回显明文或密文
	Enabled          bool       `gorm:"not null;default:true" json:"enabled"`
	Weight           int        `gorm:"not null;default:1" json:"weight"`
	State            string     `gorm:"size:16;not null;default:'active'" json:"state"`
	ConsecutiveFails int        `gorm:"not null;default:0" json:"consecutive_fails"`
	Level            int        `gorm:"not null;default:0" json:"level"` // 退避档位（状态机持久化，重启恢复）
	SuccessCount     int64      `gorm:"not null;default:0" json:"success_count"`
	FailCount        int64      `gorm:"not null;default:0" json:"fail_count"`
	LastError        string     `json:"last_error"`
	CooldownUntil    *time.Time `json:"cooldown_until"`
	RPMLimit         int        `gorm:"not null;default:0" json:"rpm_limit"` // 0 = 不限
	LastUsedAt       *time.Time `json:"last_used_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Token 是发给客户端的下游令牌（任务书 4.3 tokens 表）。
type Token struct {
	ID            int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string     `gorm:"size:100;not null" json:"name"`
	KeyHash       string     `gorm:"size:64;not null;uniqueIndex" json:"-"` // SHA-256 hex，原文仅创建时展示一次
	Enabled       bool       `gorm:"not null;default:true" json:"enabled"`
	Quota         int64      `gorm:"not null;default:0" json:"quota"` // 0 = 不限
	UsedQuota     int64      `gorm:"not null;default:0" json:"used_quota"`
	ExpiredAt     *time.Time `json:"expired_at"`
	AllowedModels []string   `gorm:"serializer:json" json:"allowed_models"`
	AllowedIPs    []string   `gorm:"serializer:json" json:"allowed_ips"`
	// DisableFallback 为 true 时该令牌不参与模型降级，保持原模型失败语义。
	DisableFallback bool `gorm:"not null;default:false" json:"disable_fallback"`
	// RPMLimit / ConcurrencyLimit 为 0 表示不限；只做单进程内存计数，不跨实例。
	RPMLimit         int       `gorm:"not null;default:0" json:"rpm_limit"`
	ConcurrencyLimit int       `gorm:"not null;default:0" json:"concurrency_limit"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RetryAttempt 是重试轨迹中的一次尝试（存于 RequestLog.RetryTrace）。
type RetryAttempt struct {
	ChannelID    int64  `json:"channel_id"`
	ChannelName  string `json:"channel_name"`
	KeyID        int64  `json:"key_id"`
	Attempt      int    `json:"attempt"`
	StatusCode   int    `json:"status_code,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	ElapsedMS    int64  `json:"elapsed_ms"`
	// Reason / FromModel / ToModel 仅在模型降级边界记录，普通上游尝试为空。
	Reason    string `json:"reason,omitempty"`
	FromModel string `json:"from_model,omitempty"`
	ToModel   string `json:"to_model,omitempty"`
}

// RequestLog 是请求日志（任务书 4.3 request_logs 表，核心调试体验）。
type RequestLog struct {
	ID               int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt        time.Time      `gorm:"index" json:"created_at"`
	TokenID          int64          `gorm:"index" json:"token_id"`
	TokenName        string         `gorm:"size:100" json:"token_name"`
	Source           string         `gorm:"size:16;not null;default:''" json:"source,omitempty"` // api / playground
	ChannelID        int64          `gorm:"index" json:"channel_id"`
	ChannelName      string         `gorm:"size:100" json:"channel_name"`
	KeyID            int64          `json:"key_id"`
	OriginalModel    string         `gorm:"size:100" json:"original_model,omitempty"`
	Model            string         `gorm:"size:100;index" json:"model"`
	UpstreamModel    string         `gorm:"size:100" json:"upstream_model"`
	Stream           bool           `json:"stream"`
	StatusCode       int            `gorm:"index" json:"status_code"`
	ErrorType        string         `gorm:"size:32" json:"error_type"`
	ErrorMessage     string         `json:"error_message"`
	RetryTrace       []RetryAttempt `gorm:"serializer:json" json:"retry_trace"`
	FirstTokenMS     int64          `json:"first_token_ms"`
	TotalMS          int64          `json:"total_ms"`
	PromptTokens     int64          `json:"prompt_tokens"`
	CompletionTokens int64          `json:"completion_tokens"`
	TotalTokens      int64          `json:"total_tokens"`
	CachedTokens     int64          `json:"cached_tokens"`
	CacheWriteTokens int64          `json:"cache_write_tokens"`
	ReasoningTokens  int64          `json:"reasoning_tokens"`
	Cost             float64        `json:"cost"`
	CostMicros       int64          `gorm:"not null;default:0" json:"cost_micros"`
	CostStatus       string         `gorm:"size:32;not null;default:''" json:"cost_status"`
	PriceSource      string         `gorm:"size:16;not null;default:''" json:"price_source"` // override / default / catalog（T92）
	IP               string         `gorm:"size:45" json:"ip"`
	UserAgent        string         `json:"user_agent"`
	ConfigOverride   string         `gorm:"size:512" json:"config_override,omitempty"` // 请求级覆盖脱敏摘要
}

// UsageDaily 是按 UTC 天 + 渠道 + 请求模型 + 令牌的幂等聚合。
type UsageDaily struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Date                time.Time `gorm:"not null;uniqueIndex:idx_usage_daily_uq" json:"date"` // UTC 零点
	ChannelID           int64     `gorm:"not null;default:0;uniqueIndex:idx_usage_daily_uq" json:"channel_id"`
	Model               string    `gorm:"size:100;not null;uniqueIndex:idx_usage_daily_uq" json:"model"`
	TokenID             int64     `gorm:"not null;default:0;uniqueIndex:idx_usage_daily_uq" json:"token_id"`
	Requests            int64     `gorm:"not null;default:0" json:"requests"`
	Success             int64     `gorm:"not null;default:0" json:"success"`
	TokensIn            int64     `gorm:"not null;default:0" json:"tokens_in"`
	TokensOut           int64     `gorm:"not null;default:0" json:"tokens_out"`
	CacheReadTokens     int64     `gorm:"not null;default:0" json:"cache_read_tokens"`
	CacheWriteTokens    int64     `gorm:"not null;default:0" json:"cache_write_tokens"`
	Cost                float64   `gorm:"not null;default:0" json:"cost"`
	CostMicros          int64     `gorm:"not null;default:0" json:"cost_micros"`
	PricedRequests      int64     `gorm:"not null;default:0" json:"priced_requests"`
	UnpricedRequests    int64     `gorm:"not null;default:0" json:"unpriced_requests"`
	NoUsageRequests     int64     `gorm:"not null;default:0" json:"no_usage_requests"`
	UnknownCostRequests int64     `gorm:"not null;default:0" json:"unknown_cost_requests"`
}

// TableName 固定为任务书表名 usage_daily（GORM 默认复数会得到 usage_dailies）。
func (UsageDaily) TableName() string { return "usage_daily" }

// ModelPrice 是每百万 token 的固定精度价格（micro-USD）。
// (channel_id=0, model="") 为全局默认；非零 channel_id + 非空 model 为精确覆盖。
// 缓存读写价为 0 表示未设置：计费时按输入价全价计，宁可高估不低估。
type ModelPrice struct {
	ID                         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID                  int64     `gorm:"not null;default:0;uniqueIndex:idx_model_price_uq" json:"channel_id"`
	Model                      string    `gorm:"size:100;not null;default:'';uniqueIndex:idx_model_price_uq" json:"model"`
	InputMicrosPerMillion      int64     `gorm:"not null;default:0" json:"input_micros_per_million"`
	OutputMicrosPerMillion     int64     `gorm:"not null;default:0" json:"output_micros_per_million"`
	CacheReadMicrosPerMillion  int64     `gorm:"not null;default:0" json:"cache_read_micros_per_million"`
	CacheWriteMicrosPerMillion int64     `gorm:"not null;default:0" json:"cache_write_micros_per_million"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// ModelFallback 是模型降级链：令牌余额低于阈值或原模型渠道耗尽时按序尝试。
type ModelFallback struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Model           string    `gorm:"size:100;not null;uniqueIndex" json:"model"`
	BudgetThreshold int64     `gorm:"not null;default:0" json:"budget_threshold"` // 剩余 token 配额阈值；0 = 不按预算触发
	Fallbacks       []string  `gorm:"serializer:json" json:"fallbacks"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// RuntimeSetting 是安全运行时配置的 SQLite 单例（ID 固定为 1）。
// YAML 只在首次初始化该行时提供种子；此后 SQLite 是唯一事实源。
type RuntimeSetting struct {
	ID                     int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ConnectTimeoutMS       int64 `gorm:"not null" json:"connect_timeout_ms"`
	FirstTokenTimeoutMS    int64 `gorm:"not null" json:"first_token_timeout_ms"`
	OverallTimeoutMS       int64 `gorm:"not null" json:"overall_timeout_ms"`
	RetryChannels          int   `gorm:"not null" json:"retry_channels"`
	RetryKeysPerChannel    int   `gorm:"not null" json:"retry_keys_per_channel"`
	RetryTotalMS           int64 `gorm:"not null" json:"retry_total_ms"`
	CooldownFailThreshold  int   `gorm:"not null" json:"cooldown_fail_threshold"`
	CooldownBaseMS         int64 `gorm:"not null" json:"cooldown_base_ms"`
	CooldownMaxMS          int64 `gorm:"not null" json:"cooldown_max_ms"`
	LogRetentionDays       int   `gorm:"not null" json:"log_retention_days"`
	HealthProbeIntervalMS  int64 `gorm:"not null" json:"health_probe_interval_ms"`
	HealthProbeConcurrency int   `gorm:"not null" json:"health_probe_concurrency"`
	HealthProbeTimeoutMS   int64 `gorm:"not null" json:"health_probe_timeout_ms"`
	// LogUnknownTokens 默认开启：AutoMigrate 为既有行回填 true；map 更新显式写 false 不受默认值影响。
	LogUnknownTokens bool      `gorm:"not null;default:true" json:"log_unknown_tokens"`
	Source           string    `gorm:"size:32;not null" json:"source"`
	InitializedAt    time.Time `json:"initialized_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Admin 是管理台账号（首次启动空表时创建 admin，密码为随机生成或 EVERGATE_INITIAL_ADMIN_PASSWORD，首登强制改密）。
type Admin struct {
	ID                 int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username           string    `gorm:"size:50;not null;uniqueIndex" json:"username"`
	PasswordHash       string    `gorm:"not null" json:"-"` // bcrypt；json:"-" 防序列化泄露
	MustChangePassword bool      `gorm:"not null;default:true" json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// AuditLog 记录管理台写操作（渠道/密钥/令牌/设置/降级链/价格/配置导入导出/改密），供管理台查询与追溯。
type AuditLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	Username  string    `gorm:"size:50;not null;default:''" json:"username"`
	Action    string    `gorm:"size:64;not null;index" json:"action"` // channel.create / token.delete / settings.update ...
	Target    string    `gorm:"size:200;not null;default:''" json:"target"`
	Summary   string    `gorm:"type:text" json:"summary"` // ≤512 字符，不含任何密钥
	IP        string    `gorm:"size:45;not null;default:''" json:"ip"`
}

// SchemaMeta 是迁移元数据单表（key/value）；app_version 记录最近一次完成 AutoMigrate 的应用版本，用于升级前备份判断。
type SchemaMeta struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

// AlertSetting 是告警通道配置（单例行，id 固定 1）。
type AlertSetting struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	Enabled   bool      `gorm:"not null;default:false" json:"enabled"`
	Preset    string    `gorm:"size:32;not null;default:'generic'" json:"preset"`
	URL       string    `gorm:"not null;default:''" json:"url"`
	Events    []string  `gorm:"serializer:json" json:"events"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AlertRecord 是告警发送记录（只保留最近若干条，用于设置页回看）。
type AlertRecord struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
	Event      string    `gorm:"size:64;not null" json:"event"`
	Preset     string    `gorm:"size:32;not null;default:''" json:"preset"`
	Result     string    `gorm:"size:16;not null" json:"result"` // sent / failed / test
	StatusCode int       `json:"status_code"`
	Error      string    `json:"error"`
	Message    string    `json:"message"`
}
