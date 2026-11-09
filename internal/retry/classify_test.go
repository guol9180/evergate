package retry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	netErr := errors.New("dial tcp: connection refused")
	cases := []struct {
		name string
		in   Result
		want Classification
	}{
		{"网络错误", Result{Err: netErr}, Classification{Kind: KindTransient}},
		{"首token超时", Result{Err: ErrFirstTokenTimeout}, Classification{Kind: KindTransient}},
		{"408", Result{StatusCode: 408}, Classification{Kind: KindTransient}},
		{"409", Result{StatusCode: 409}, Classification{Kind: KindTransient}},
		{"500", Result{StatusCode: 500}, Classification{Kind: KindTransient}},
		{"502", Result{StatusCode: 502}, Classification{Kind: KindTransient}},
		{"503", Result{StatusCode: 503}, Classification{Kind: KindTransient}},
		{"504", Result{StatusCode: 504}, Classification{Kind: KindTransient}},
		{"529-Anthropic过载", Result{StatusCode: 529}, Classification{Kind: KindTransient}},
		{"599", Result{StatusCode: 599}, Classification{Kind: KindTransient}},
		{"429无RetryAfter", Result{StatusCode: 429}, Classification{Kind: KindRateLimited}},
		{"429带RetryAfter", Result{StatusCode: 429, RetryAfter: 60 * time.Second}, Classification{Kind: KindRateLimited, RetryAfter: 60 * time.Second}},
		{"400带限流文案", Result{StatusCode: 400, BodySnippet: "Requests per minute limit exceeded"}, Classification{Kind: KindRateLimited}},
		{"403带限流文案", Result{StatusCode: 403, BodySnippet: "Rate limit reached"}, Classification{Kind: KindRateLimited}},
		{"quota标识-默认冷却", Result{StatusCode: 429, BodySnippet: `{"code":"insufficient_quota"}`}, Classification{Kind: KindRateLimited, RetryAfter: DefaultQuotaCooldown}},
		{"quota文案", Result{StatusCode: 400, BodySnippet: "Your credit balance is too low"}, Classification{Kind: KindRateLimited, RetryAfter: DefaultQuotaCooldown}},
		{"凭据标识不限状态码", Result{StatusCode: 400, BodySnippet: `{"code":"invalid_api_key"}`}, Classification{Kind: KindAuth}},
		{"凭据文案", Result{StatusCode: 403, BodySnippet: "API key not valid"}, Classification{Kind: KindAuth}},
		{"401", Result{StatusCode: 401}, Classification{Kind: KindAuth}},
		{"403", Result{StatusCode: 403}, Classification{Kind: KindAuth}},
		{"模型不存在404", Result{StatusCode: 404, BodySnippet: `{"code":"model_not_found"}`}, Classification{Kind: KindModelUnavailable}},
		{"模型不存在文案", Result{StatusCode: 400, BodySnippet: "The model unknown-model does not exist"}, Classification{Kind: KindModelUnavailable}},
		{"DeploymentNotFound", Result{StatusCode: 403, BodySnippet: "DeploymentNotFound"}, Classification{Kind: KindModelUnavailable}},
		{"400", Result{StatusCode: 400}, Classification{Kind: KindNonRetryable}},
		{"404非模型原因", Result{StatusCode: 404}, Classification{Kind: KindNonRetryable}},
		{"413", Result{StatusCode: 413}, Classification{Kind: KindNonRetryable}},
		{"422", Result{StatusCode: 422}, Classification{Kind: KindNonRetryable}},
		{"未知4xx默认透传", Result{StatusCode: 418}, Classification{Kind: KindNonRetryable}},
		{"内容策略标识", Result{StatusCode: 400, BodySnippet: `{"code":"content_policy_violation"}`}, Classification{Kind: KindContentPolicy}},
		{"内容策略大写标识", Result{StatusCode: 400, BodySnippet: "Request blocked by CONTENT FILTER"}, Classification{Kind: KindContentPolicy}},
		{"500带策略标识仍为瞬时", Result{StatusCode: 500, BodySnippet: "content_filter"}, Classification{Kind: KindTransient}},
		{"空body的400", Result{StatusCode: 400}, Classification{Kind: KindNonRetryable}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.in)
			if got.Kind != c.want.Kind {
				t.Fatalf("Kind = %s, want %s", got.Kind, c.want.Kind)
			}
			if got.RetryAfter != c.want.RetryAfter {
				t.Fatalf("RetryAfter = %v, want %v", got.RetryAfter, c.want.RetryAfter)
			}
		})
	}
}

func TestStreamErrorStatus(t *testing.T) {
	cases := []struct {
		typ, code string
		want      int
	}{
		{"rate_limit_error", "", 429},
		{"", "too_many_requests", 429},
		{"", "insufficient_quota", 429},
		{"invalid_request_error", "", 400},
		{"", "context_length_exceeded", 400},
		{"authentication_error", "", 401},
		{"", "invalid_api_key", 401},
		{"permission_error", "", 403},
		{"", "model_not_found", 404},
		{"overloaded_error", "", 529},
		{"weird_unknown", "", 502},
	}
	for _, c := range cases {
		if got := StreamErrorStatus(c.typ, c.code); got != c.want {
			t.Fatalf("StreamErrorStatus(%q,%q) = %d, want %d", c.typ, c.code, got, c.want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	rec := httptest.NewRecorder()
	rec.Header().Set("Retry-After", now.Add(90*time.Second).UTC().Format(http.TimeFormat))
	httpDate := rec.Header().Get("Retry-After")
	cases := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"秒数", "60", 60 * time.Second},
		{"零", "0", 0},
		{"负数", "-5", 0},
		{"空串", "", 0},
		{"非法", "soon", 0},
		{"HTTP日期", httpDate, 90 * time.Second},
		{"过去日期", now.Add(-time.Hour).UTC().Format(http.TimeFormat), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseRetryAfter(c.in, now); got != c.want {
				t.Fatalf("ParseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestBudget(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	b := NewBudget(3, 2, 60*time.Second, now)
	cases := []struct {
		name         string
		channel, key int
		at           time.Time
		want         bool
	}{
		{"第1渠道第1key", 1, 1, now, true},
		{"第3渠道第2key", 3, 2, now.Add(59 * time.Second), true},
		{"第4渠道", 4, 1, now, false},
		{"每渠道第3key", 1, 3, now, false},
		{"超总时长", 1, 1, now.Add(61 * time.Second), false},
		{"非法序号0", 0, 1, now, false},
		{"非法key序号0", 1, 0, now, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := b.Allow(c.channel, c.key, c.at); got != c.want {
				t.Fatalf("Allow(%d,%d) = %v, want %v", c.channel, c.key, got, c.want)
			}
		})
	}
	t.Run("不限总时长", func(t *testing.T) {
		b := NewBudget(3, 2, 0, now)
		if !b.Allow(1, 1, now.Add(100*time.Hour)) {
			t.Fatal("total<=0 应不限总时长")
		}
	})
}
