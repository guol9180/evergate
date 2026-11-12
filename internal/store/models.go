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
	Type                string            `gorm:"size:32;not null" json:"type"` // openai / openai-compatible / deepseek / vllm / anthropic / gemini ...
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
	SupportsResponses bool            `gorm:"not null;default:false" json:"supports_responses"`
	Timeouts          ChannelTimeouts `gorm:"column:timeout_config;serializer:json" json:"timeout_config"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
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
	KeyCipher        string     `gorm:"column:key_cipher;not null" json:"key_cipher"` // AES-GCM base64，任何接口不回显明文
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
	KeyHash       string     `gorm:"size:64;not null;uniqueIndex" json:"key_hash"` // SHA-256 hex，原文仅创建时展示一次
	Enabled       bool       `gorm:"not null;default:true" json:"enabled"`
	Quota         int64      `gorm:"not null;default:0" json:"quota"` // 0 = 不限
	UsedQuota     int64      `gorm:"not null;default:0" json:"used_quota"`
	ExpiredAt     *time.Time `json:"expired_at"`
	AllowedModels []string   `gorm:"serializer:json" json:"allowed_models"`
	AllowedIPs    []string   `gorm:"serializer:json" json:"allowed_ips"`
	Group         string     `gorm:"size:50;not null;default:'default'" json:"group"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
}

// RequestLog 是请求日志（任务书 4.3 request_logs 表，核心调试体验）。
type RequestLog struct {
	ID               int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt        time.Time      `gorm:"index" json:"created_at"`
	TokenID          int64          `gorm:"index" json:"token_id"`
	TokenName        string         `gorm:"size:100" json:"token_name"`
	ChannelID        int64          `gorm:"index" json:"channel_id"`
	ChannelName      string         `gorm:"size:100" json:"channel_name"`
	KeyID            int64          `json:"key_id"`
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
	ReasoningTokens  int64          `json:"reasoning_tokens"`
	Cost             float64        `json:"cost"`
	CostMicros       int64          `gorm:"not null;default:0" json:"cost_micros"`
	CostStatus       string         `gorm:"size:32;not null;default:''" json:"cost_status"`
	IP               string         `gorm:"size:45" json:"ip"`
	UserAgent        string         `json:"user_agent"`
}

// UsageDaily 是按 UTC 天 + 渠道 + 请求模型 + 令牌的幂等聚合。
type UsageDaily struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Date             time.Time `gorm:"not null;uniqueIndex:idx_usage_daily_uq" json:"date"` // UTC 零点
	ChannelID        int64     `gorm:"not null;default:0;uniqueIndex:idx_usage_daily_uq" json:"channel_id"`
	Model            string    `gorm:"size:100;not null;uniqueIndex:idx_usage_daily_uq" json:"model"`
	TokenID          int64     `gorm:"not null;default:0;uniqueIndex:idx_usage_daily_uq" json:"token_id"`
	Requests         int64     `gorm:"not null;default:0" json:"requests"`
	Success          int64     `gorm:"not null;default:0" json:"success"`
	TokensIn         int64     `gorm:"not null;default:0" json:"tokens_in"`
	TokensOut        int64     `gorm:"not null;default:0" json:"tokens_out"`
	Cost             float64   `gorm:"not null;default:0" json:"cost"`
	CostMicros       int64     `gorm:"not null;default:0" json:"cost_micros"`
	PricedRequests   int64     `gorm:"not null;default:0" json:"priced_requests"`
	UnpricedRequests int64     `gorm:"not null;default:0" json:"unpriced_requests"`
}

// TableName 固定为任务书表名 usage_daily（GORM 默认复数会得到 usage_dailies）。
func (UsageDaily) TableName() string { return "usage_daily" }

// ModelPrice 是每百万 token 的固定精度价格（micro-USD）。
// (channel_id=0, model="") 为全局默认；非零 channel_id + 非空 model 为精确覆盖。
type ModelPrice struct {
	ID                     int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID              int64     `gorm:"not null;default:0;uniqueIndex:idx_model_price_uq" json:"channel_id"`
	Model                  string    `gorm:"size:100;not null;default:'';uniqueIndex:idx_model_price_uq" json:"model"`
	InputMicrosPerMillion  int64     `gorm:"not null;default:0" json:"input_micros_per_million"`
	OutputMicrosPerMillion int64     `gorm:"not null;default:0" json:"output_micros_per_million"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// RuntimeSetting 是安全运行时配置的 SQLite 单例（ID 固定为 1）。
// YAML 只在首次初始化该行时提供种子；此后 SQLite 是唯一事实源。
type RuntimeSetting struct {
	ID                     int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ConnectTimeoutMS       int64     `gorm:"not null" json:"connect_timeout_ms"`
	FirstTokenTimeoutMS    int64     `gorm:"not null" json:"first_token_timeout_ms"`
	OverallTimeoutMS       int64     `gorm:"not null" json:"overall_timeout_ms"`
	RetryChannels          int       `gorm:"not null" json:"retry_channels"`
	RetryKeysPerChannel    int       `gorm:"not null" json:"retry_keys_per_channel"`
	RetryTotalMS           int64     `gorm:"not null" json:"retry_total_ms"`
	CooldownFailThreshold  int       `gorm:"not null" json:"cooldown_fail_threshold"`
	CooldownBaseMS         int64     `gorm:"not null" json:"cooldown_base_ms"`
	CooldownMaxMS          int64     `gorm:"not null" json:"cooldown_max_ms"`
	LogRetentionDays       int       `gorm:"not null" json:"log_retention_days"`
	HealthProbeIntervalMS  int64     `gorm:"not null" json:"health_probe_interval_ms"`
	HealthProbeConcurrency int       `gorm:"not null" json:"health_probe_concurrency"`
	HealthProbeTimeoutMS   int64     `gorm:"not null" json:"health_probe_timeout_ms"`
	Source                 string    `gorm:"size:32;not null" json:"source"`
	InitializedAt          time.Time `json:"initialized_at"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// Admin 是管理台账号（首次启动空表时创建默认 admin/admin123，首登强制改密）。
type Admin struct {
	ID                 int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username           string    `gorm:"size:50;not null;uniqueIndex" json:"username"`
	PasswordHash       string    `gorm:"not null" json:"-"` // bcrypt；json:"-" 防序列化泄露
	MustChangePassword bool      `gorm:"not null;default:true" json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
