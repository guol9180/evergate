// Package retry 定义错误分类器与重试预算（任务书 4.5④⑤）。
package retry

import (
	"errors"
	"strings"
	"time"
)

// ErrorKind 错误类别，决定重试行为（任务书 4.5④）。
type ErrorKind string

const (
	// KindRetryable 可切换重试：网络错误/TLS/连接超时、408、409、429、5xx、首 token 前超时。
	KindRetryable ErrorKind = "retryable"
	// KindAuth 凭据问题：401/403 → 禁用该 key，换 key 继续。
	KindAuth ErrorKind = "auth"
	// KindNonRetryable 不可重试：400/404/413/422 等，请求本身有问题，换渠道也一样。
	KindNonRetryable ErrorKind = "non_retryable"
	// KindContentPolicy 内容策略：上游内容安全拦截，透传。
	KindContentPolicy ErrorKind = "content_policy"
)

// Result 一次上游尝试的结果摘要（供分类；StatusCode 为 0 表示未拿到 HTTP 响应）。
type Result struct {
	Err         error
	StatusCode  int
	RetryAfter  time.Duration // 429 时从 Retry-After 头解析；<=0 表示无/无效
	BodySnippet string        // 错误体片段（内容策略识别用）
}

// Classification 分类结论与附带参数。
type Classification struct {
	Kind       ErrorKind
	RetryAfter time.Duration // 仅 429 且解析成功时 > 0
}

// Classify 按任务书 4.5④ 分类一次上游结果。
func Classify(r Result) Classification {
	if r.Err != nil {
		// 网络错误 / TLS / 连接超时 / 首 token 前超时：全部可切换重试。
		return Classification{Kind: KindRetryable}
	}
	switch code := r.StatusCode; {
	case code == 408 || code == 409 || code == 429:
		c := Classification{Kind: KindRetryable}
		if code == 429 && r.RetryAfter > 0 {
			c.RetryAfter = r.RetryAfter
		}
		return c
	case code >= 500 && code <= 599:
		return Classification{Kind: KindRetryable}
	case code == 401 || code == 403:
		return Classification{Kind: KindAuth}
	case isContentPolicy(code, r.BodySnippet):
		return Classification{Kind: KindContentPolicy}
	default:
		// 400/404/413/422 及其余未知 4xx：直接透传错误。
		return Classification{Kind: KindNonRetryable}
	}
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

// ErrFirstTokenTimeout 首 token 前超时哨兵：属于可重试错误（relay 在可重试窗口内返回）。
var ErrFirstTokenTimeout = errors.New("first token timeout")
