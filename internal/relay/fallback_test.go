package relay

import (
	"net/http"
	"strings"
	"testing"

	"evergate/internal/store"
	"evergate/internal/usage"
)

func TestIsInternalTraceType(t *testing.T) {
	for _, typ := range []string{"internal", "decrypt", "responses_bridge", "messages_bridge", "gemini_bridge"} {
		if !isInternalTraceType(typ) {
			t.Fatalf("%q should be treated as internal", typ)
		}
	}
	for _, typ := range []string{"", "transient", "rate_limited", "auth", "non_retryable"} {
		if isInternalTraceType(typ) {
			t.Fatalf("%q must not be treated as internal", typ)
		}
	}
}

func TestFilterChannelsByIDAndModel(t *testing.T) {
	channels := []*store.Channel{
		{ID: 1, Models: []string{"a"}},
		{ID: 2, Models: []string{"b"}},
		{ID: 3, Models: []string{"a"}},
	}
	got := filterChannelsByIDAndModel(channels, "a", []int64{1, 2, 3, 99})
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("filtered channels = %+v, want ids [1 3]", got)
	}
}

func TestBudgetThresholdFallsBackBeforeOriginalModel(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(body, `"model":"cheap"`) {
			t.Errorf("upstream model body = %s, want cheap", body)
		}
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"cheap ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})
	env.addChannel(t, "cheap", up.server.URL, 0, []string{"cheap"}, nil, "sk-cheap")
	if err := env.db.Create(&store.ModelFallback{Model: "expensive", BudgetThreshold: 20, Fallbacks: []string{"cheap"}}).Error; err != nil {
		t.Fatal(err)
	}
	token, plain := env.addToken(t, "budget", func(token *store.Token) {
		token.Quota, token.UsedQuota = 100, 85
	})
	if err := env.usage.SetDefault(t.Context(), usage.PriceInput{Input: 1_000_000, Output: 1_000_000}); err != nil {
		t.Fatal(err)
	}
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"expensive","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if hits := up.hits.Load(); hits != 1 {
		t.Fatalf("upstream hits = %d, want 1", hits)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 {
		t.Fatalf("logs = %d", len(logs))
	}
	log := logs[0]
	if log.OriginalModel != "expensive" || log.Model != "cheap" || log.UpstreamModel != "cheap" {
		t.Fatalf("model fields = %#v", log)
	}
	if len(log.RetryTrace) != 2 || log.RetryTrace[0].Reason != fallbackReasonBudget || log.RetryTrace[1].StatusCode != http.StatusOK {
		t.Fatalf("retry trace = %#v", log.RetryTrace)
	}
	if log.CostStatus != usage.CostStatusPriced || log.CostMicros != 5 {
		t.Fatalf("cost = %d %s", log.CostMicros, log.CostStatus)
	}
}

func TestChannelExhaustedFallsBackInConfiguredOrder(t *testing.T) {
	env := newEnv(t)
	expensive := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		http.Error(w, "expensive down", http.StatusServiceUnavailable)
	})
	mid := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mid down", http.StatusServiceUnavailable)
	})
	cheap := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"third model ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	env.addChannel(t, "expensive", expensive.server.URL, 0, []string{"expensive"}, nil, "sk-expensive")
	env.addChannel(t, "mid", mid.server.URL, 1, []string{"mid"}, nil, "sk-mid")
	env.addChannel(t, "cheap", cheap.server.URL, 2, []string{"cheap"}, nil, "sk-cheap")
	if err := env.db.Create(&store.ModelFallback{Model: "expensive", Fallbacks: []string{"mid", "cheap"}}).Error; err != nil {
		t.Fatal(err)
	}
	token, plain := env.addToken(t, "exhausted", nil)
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"expensive","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if expensive.hits.Load() != 1 || mid.hits.Load() != 1 || cheap.hits.Load() != 1 {
		t.Fatalf("hits expensive=%d mid=%d cheap=%d", expensive.hits.Load(), mid.hits.Load(), cheap.hits.Load())
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 {
		t.Fatalf("logs = %d", len(logs))
	}
	trace := logs[0].RetryTrace
	if len(trace) != 5 || trace[1].Reason != fallbackReasonExhausted || trace[1].ToModel != "mid" || trace[3].Reason != fallbackReasonExhausted || trace[3].ToModel != "cheap" {
		t.Fatalf("fallback trace = %#v", trace)
	}
}

func TestResponsesEntranceUsesSameFallbackChain(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if strings.Contains(body, `"model":"responses-expensive"`) {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"responses fallback ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	})
	env.addChannel(t, "expensive", up.server.URL, 0, []string{"responses-expensive"}, nil, "sk")
	env.addChannel(t, "cheap", up.server.URL, 1, []string{"responses-cheap"}, nil, "sk")
	if err := env.db.Create(&store.ModelFallback{Model: "responses-expensive", Fallbacks: []string{"responses-cheap"}}).Error; err != nil {
		t.Fatal(err)
	}
	token, plain := env.addToken(t, "responses-fallback", nil)
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/responses", plain, `{"model":"responses-expensive","input":"hi","stream":false}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "responses fallback ok") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].OriginalModel != "responses-expensive" || logs[0].Model != "responses-cheap" {
		t.Fatalf("logs = %#v", logs)
	}
	if len(logs[0].RetryTrace) != 3 || logs[0].RetryTrace[1].Reason != fallbackReasonExhausted {
		t.Fatalf("trace = %#v", logs[0].RetryTrace)
	}
}

func TestFallbackRuleUpdateTakesEffectWithoutRestart(t *testing.T) {
	env := newEnv(t)
	expensive := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	})
	cheap := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	env.addChannel(t, "expensive", expensive.server.URL, 0, []string{"expensive"}, nil, "sk")
	env.addChannel(t, "cheap", cheap.server.URL, 1, []string{"cheap"}, nil, "sk")
	rule := &store.ModelFallback{Model: "expensive", Fallbacks: []string{"cheap"}}
	if err := env.db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
	_, plain := env.addToken(t, "hot-rule", nil)
	gateway := env.authGateway(t)

	if resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"expensive","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("before update: %d %s", resp.StatusCode, body)
	}
	if err := env.db.Model(rule).Update("fallbacks", []string{}).Error; err != nil {
		t.Fatal(err)
	}
	env.InvalidateConfig() // 测试直接改库，模拟管理台写操作后的失效
	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"expensive","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "all_channels_exhausted") {
		t.Fatalf("after update: %d %s", resp.StatusCode, body)
	}
	if cheap.hits.Load() != 1 {
		t.Fatalf("cheap hits after rule update = %d, want 1", cheap.hits.Load())
	}
}

func TestTokenCanDisableModelFallback(t *testing.T) {
	env := newEnv(t)
	expensive := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	})
	cheap := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		t.Error("cheap upstream should not be hit")
	})
	env.addChannel(t, "expensive", expensive.server.URL, 0, []string{"expensive"}, nil, "sk")
	env.addChannel(t, "cheap", cheap.server.URL, 1, []string{"cheap"}, nil, "sk")
	if err := env.db.Create(&store.ModelFallback{Model: "expensive", Fallbacks: []string{"cheap"}}).Error; err != nil {
		t.Fatal(err)
	}
	_, plain := env.addToken(t, "no-fallback", func(token *store.Token) { token.DisableFallback = true })
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"expensive","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "all_channels_exhausted") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if cheap.hits.Load() != 0 {
		t.Fatalf("cheap hits = %d, want 0", cheap.hits.Load())
	}
}

func TestNoFallbackRuleKeepsExisting503Semantics(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	})
	env.addChannel(t, "only", up.server.URL, 0, []string{"expensive"}, nil, "sk")
	_, plain := env.addToken(t, "no-rule", nil)
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"expensive","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "all_channels_exhausted") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
}
