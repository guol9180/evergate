package retry

import (
	"errors"
	"net/http"
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
		{"网络错误", Result{Err: netErr}, Classification{Kind: KindRetryable}},
		{"首token超时", Result{Err: ErrFirstTokenTimeout}, Classification{Kind: KindRetryable}},
		{"408", Result{StatusCode: 408}, Classification{Kind: KindRetryable}},
		{"409", Result{StatusCode: 409}, Classification{Kind: KindRetryable}},
		{"429无RetryAfter", Result{StatusCode: 429}, Classification{Kind: KindRetryable}},
		{"429带RetryAfter", Result{StatusCode: 429, RetryAfter: 60 * time.Second}, Classification{Kind: KindRetryable, RetryAfter: 60 * time.Second}},
		{"500", Result{StatusCode: 500}, Classification{Kind: KindRetryable}},
		{"502", Result{StatusCode: 502}, Classification{Kind: KindRetryable}},
		{"599", Result{StatusCode: 599}, Classification{Kind: KindRetryable}},
		{"401", Result{StatusCode: 401}, Classification{Kind: KindAuth}},
		{"403", Result{StatusCode: 403}, Classification{Kind: KindAuth}},
		{"400", Result{StatusCode: 400}, Classification{Kind: KindNonRetryable}},
		{"404", Result{StatusCode: 404}, Classification{Kind: KindNonRetryable}},
		{"413", Result{StatusCode: 413}, Classification{Kind: KindNonRetryable}},
		{"422", Result{StatusCode: 422}, Classification{Kind: KindNonRetryable}},
		{"未知4xx默认透传", Result{StatusCode: 418}, Classification{Kind: KindNonRetryable}},
		{"内容策略标识", Result{StatusCode: 400, BodySnippet: `{"code":"content_policy_violation"}`}, Classification{Kind: KindContentPolicy}},
		{"内容策略大写标识", Result{StatusCode: 400, BodySnippet: "Request blocked by CONTENT FILTER"}, Classification{Kind: KindContentPolicy}},
		{"500带策略标识仍可重试", Result{StatusCode: 500, BodySnippet: "content_filter"}, Classification{Kind: KindRetryable}},
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

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
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
		{"HTTP日期", now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{"过去日期", now.Add(-time.Hour).Format(http.TimeFormat), 0},
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
