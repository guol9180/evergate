package retry

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ParseRetryAfter 解析上游 Retry-After 头：秒数或 HTTP 日期。
// 无效或已过期返回 0。
func ParseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
