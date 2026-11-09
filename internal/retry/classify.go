// Package retry 定义错误分类器与重试预算（任务书 4.5④⑤）。
// 分类学吸收了 bifrost core/failureclass.go 与 gpt-load internal/health 的生产经验：
// 服务器类故障（5xx/网络）不归咎于 key，模型不可用类换渠道不惩罚 key。
package retry

import (
	"errors"
	"strings"
	"time"
)

// ErrorKind 错误类别，决定重试行为（任务书 4.5④ 细分）。
type ErrorKind string

const (
	// KindTransient 服务器/网络类瞬时故障：5xx、529、408、409、网络错误、首 token 前超时。
	// 换 key 对服务器过载无益（bifrost：rotating would burn every key），
	// 因此跳过整个渠道且不计入 key 失败。
	KindTransient ErrorKind = "transient"
	// KindRateLimited 限流/配额：429、Retry-After、限流文案、quota 耗尽。
	// 冷却该 key，换下一个 key。
	KindRateLimited ErrorKind = "rate_limited"
	// KindAuth 凭据问题：401/403 或凭据错误标识 → 禁用该 key，换 key 继续。
	KindAuth ErrorKind = "auth"
	// KindModelUnavailable 模型不可用：model_not_found / DeploymentNotFound 等。
	// 该渠道没有此模型，换渠道继续，不惩罚 key（多渠道网关常态）。
	KindModelUnavailable ErrorKind = "model_unavailable"
	// KindNonRetryable 不可重试：400/404/413/422 等，请求本身有问题，换渠道也一样。
	KindNonRetryable ErrorKind = "non_retryable"
	// KindContentPolicy 内容策略：上游内容安全拦截，透传。
	KindContentPolicy ErrorKind = "content_policy"
)

// 首个 token 超时哨兵：属于瞬时故障（relay 在可重试窗口内返回）。
var ErrFirstTokenTimeout = errors.New("first token timeout")

// Result 一次上游尝试的结果摘要（供分类；StatusCode 为 0 表示未拿到 HTTP 响应）。
type Result struct {
	Err         error
	StatusCode  int
	RetryAfter  time.Duration // 429 时从 Retry-After 头解析；<=0 表示无/无效
	BodySnippet string        // 错误体片段（标识识别用）
}

// Classification 分类结论与附带参数。
type Classification struct {
	Kind       ErrorKind
	RetryAfter time.Duration // 仅限流/配额类 > 0（quota 用默认冷却）
}

// DefaultQuotaCooldown quota 类（账户余额/额度耗尽）的默认 key 冷却时长。
const DefaultQuotaCooldown = 2 * time.Minute

// Classify 按任务书 4.5④ + 参考项目生产经验分类一次上游结果。
func Classify(r Result) Classification {
	if r.Err != nil {
		// 网络错误 / TLS / 连接超时 / 首 token 前超时：服务器类瞬时故障，换渠道不烧 key。
		return Classification{Kind: KindTransient}
	}

	// 文案/标识识别（不限状态码）：bifrost 经验——同一故障各家状态码不一
	lower := strings.ToLower(r.BodySnippet)
	if strings.Contains(lower, "insufficient_quota") || containsAny(lower, quotaMarkers) {
		// 账户没钱/额度耗尽：该 key 冷却较久，换其他 key
		return Classification{Kind: KindRateLimited, RetryAfter: DefaultQuotaCooldown}
	}
	if r.StatusCode != 429 && containsAny(lower, rateLimitPatterns) && r.StatusCode >= 400 && r.StatusCode <= 499 {
		// 限流文案出现在非 429 的 4xx（Anthropic 400、Vertex 403 等）→ 按限流处理
		return Classification{Kind: KindRateLimited}
	}
	if containsAny(lower, credentialMarkers) {
		return Classification{Kind: KindAuth}
	}
	if isModelUnavailable(r.StatusCode, lower) {
		return Classification{Kind: KindModelUnavailable}
	}

	switch code := r.StatusCode; {
	case code == 408 || code == 409:
		return Classification{Kind: KindTransient}
	case code == 429:
		c := Classification{Kind: KindRateLimited}
		if r.RetryAfter > 0 {
			c.RetryAfter = r.RetryAfter
		}
		return c
	case code == 500 || code == 502 || code == 503 || code == 504 || code == 529:
		// 529 = Anthropic overloaded_error，服务器容量问题（bifrost transient 集合）
		return Classification{Kind: KindTransient}
	case code >= 500 && code <= 599:
		return Classification{Kind: KindTransient}
	case code == 401 || code == 403:
		return Classification{Kind: KindAuth}
	case isContentPolicy(code, r.BodySnippet):
		return Classification{Kind: KindContentPolicy}
	default:
		// 400/404/413/422 及其余未知 4xx：直接透传错误。
		return Classification{Kind: KindNonRetryable}
	}
}

// StreamErrorStatus 从 SSE 流内错误事件推断其等效 HTTP 状态（bifrost streamErrorStatus 思路）：
// 事件骑在已提交的 200 上，无状态码时按 provider 的 code/type 推断，供分类使用。
func StreamErrorStatus(errType, code string) int {
	s := strings.ToLower(errType + " " + code)
	switch {
	case strings.Contains(s, "rate_limit") || strings.Contains(s, "too_many_requests") || strings.Contains(s, "insufficient_quota"):
		return 429
	case strings.Contains(s, "invalid_request") || strings.Contains(s, "context_length") || strings.Contains(s, "invalid_prompt"):
		return 400
	case strings.Contains(s, "authentication") || strings.Contains(s, "invalid_api_key"):
		return 401
	case strings.Contains(s, "permission"):
		return 403
	case strings.Contains(s, "model_not_found") || strings.Contains(s, "not_found_error"):
		return 404
	case strings.Contains(s, "overloaded"):
		return 529
	default:
		return 502 // 未知按瞬时服务器错误，保持可重试（bifrost 同策略）
	}
}

// 限流文案（bifrost rateLimitPatterns 精简版）：不以 429 报限流的供应商。
var rateLimitPatterns = []string{
	"rate limit", "rate_limit", "too many requests", "quota exceeded", "quota_exceeded",
	"request limit", "throttled", "throttling", "rate exceeded", "rpm exceeded",
	"tpm exceeded", "requests per minute", "usage limit",
}

// 配额/余额耗尽标识（bifrost quotaMarkers/quotaCodes）。
var quotaMarkers = []string{
	"credit balance", "spend limit", "prepayment credits", "credits are depleted",
	"spending cap", "billing to be enabled", "account is not active",
}

// 凭据错误标识（bifrost credentialTypes/Codes/Markers 精简版）：
// 这些词出现在错误体中即为 key 问题，不限状态码。
var credentialMarkers = []string{
	"invalid_api_key", "api key not valid", "api_key_invalid", "api key expired",
	"invalid api key", "authentication_error", "authentication failed",
}

// 模型不可用标识（bifrost modelAccessCodes + gpt-load ModelUnavailable）。
var modelUnavailableMarkers = []string{
	"model_not_found", "deploymentnotfound", "deployment_not_found", "unknown_model",
	"model_not_available", "model not available", "model does not exist",
	"no such model", "model_permission_blocked",
}

func isModelUnavailable(status int, lowerBody string) bool {
	if status < 400 || status > 499 {
		return false
	}
	return containsAny(lowerBody, modelUnavailableMarkers)
}

// contentPolicyMarkers 常见上游内容安全拦截标识（小写匹配，可按需扩充）。
var contentPolicyMarkers = []string{
	"content_policy",
	"content_policy_violation",
	"content filter",
	"content_filter",
	"content management policy",
	"safety system",
	"input blocked",
	"output blocked",
}

// isContentPolicy 仅对 4xx 判定：换渠道重试同样的违规内容没有意义。
func isContentPolicy(status int, body string) bool {
	if status < 400 || status > 499 || body == "" {
		return false
	}
	lower := strings.ToLower(body)
	for _, m := range contentPolicyMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func containsAny(lower string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
