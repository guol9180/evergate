package relay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/pool"
	"evergate/internal/router"
	"evergate/internal/store"
)

// ---------- 测试环境 ----------

type testEnv struct {
	*Relay
	db        *gorm.DB
	masterKey string
	engine    *gin.Engine
	pool      *pool.Manager
	clock     *fakeClock
	gateway   *httptest.Server
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) Now() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.t }

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	master, err := store.GenerateRandomKey()
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	clock := &fakeClock{t: time.Unix(1000000, 0)}
	d := config.Default().Defaults
	d.ConnectTimeout = config.Duration{Duration: 1 * time.Second}
	d.FirstTokenTimeout = config.Duration{Duration: 300 * time.Millisecond}
	d.OverallTimeout = config.Duration{Duration: 3 * time.Second}
	d.RetryBudget = config.RetryBudget{Channels: 3, KeysPerChannel: 2, Total: config.Duration{Duration: 60 * time.Second}}
	pl := pool.NewManager(3, pool.DefaultCooldownSchedule, clock.Now)
	rel := New(Options{DB: db, Router: router.New(42), Pool: pl, MasterKey: master, Defaults: d})
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.POST("/v1/chat/completions", rel.HandleChatCompletions)
	e.GET("/v1/models", rel.HandleModels)
	gw := httptest.NewServer(e)
	t.Cleanup(gw.Close)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close() // Windows 下未关闭会阻塞 TempDir 清理
		}
	})
	return &testEnv{Relay: rel, db: db, masterKey: master, engine: e, pool: pl, clock: clock, gateway: gw}
}

func (e *testEnv) addChannel(t *testing.T, name, baseURL string, priority int, models []string, mapping map[string]string, keys ...string) {
	t.Helper()
	ch := &store.Channel{Name: name, Type: "openai-compatible", BaseURL: baseURL, Priority: priority, Weight: 1, Models: models, ModelMapping: mapping, Enabled: true, Status: store.ChannelStatusActive}
	if err := e.db.Create(ch).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	for _, k := range keys {
		cipher, err := store.Encrypt(k, e.masterKey)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		ck := &store.ChannelKey{ChannelID: ch.ID, KeyCipher: cipher, Enabled: true, Weight: 1, State: store.KeyStateActive}
		if err := e.db.Create(ck).Error; err != nil {
			t.Fatalf("create key: %v", err)
		}
	}
}

func (e *testEnv) chat(t *testing.T, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(e.gateway.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(data)
}

// countingUpstream 记录命中次数与收到的 Authorization / body。
type countingUpstream struct {
	server  *httptest.Server
	hits    atomic.Int64
	mu      sync.Mutex
	auths   []string
	bodies  []string
	handler func(auth string, body string, w http.ResponseWriter, r *http.Request)
}

func newUpstream(t *testing.T, handler func(auth string, body string, w http.ResponseWriter, r *http.Request)) *countingUpstream {
	t.Helper()
	u := &countingUpstream{handler: handler}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.auths = append(u.auths, r.Header.Get("Authorization"))
		u.bodies = append(u.bodies, string(body))
		u.mu.Unlock()
		handler(r.Header.Get("Authorization"), string(body), w, r)
	}))
	t.Cleanup(u.server.Close)
	return u
}

func (u *countingUpstream) snapshot() (int64, []string, []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits.Load(), append([]string(nil), u.auths...), append([]string(nil), u.bodies...)
}

// ---------- 验收 #2：非流式故障切换 ----------

func TestAcceptance2NonStreamFailover(t *testing.T) {
	env := newEnv(t)
	primary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream boom"}}`))
	})
	secondary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if auth != "Bearer sk-good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req map[string]any
		_ = json.Unmarshal([]byte(body), &req)
		if req["model"] != "gpt-upstream" { // 模型映射必须生效
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf("wrong model %v", req["model"])))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"hello from secondary"}}]}`))
	})
	env.addChannel(t, "primary", primary.server.URL, 0, []string{"gpt-x"}, nil, "sk-primary")
	env.addChannel(t, "secondary", secondary.server.URL, 1, []string{"gpt-x"}, map[string]string{"gpt-x": "gpt-upstream"}, "sk-good")

	resp, body := env.chat(t, `{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "hello from secondary") {
		t.Fatalf("应收到次渠道响应: %s", body)
	}
	if ph, _, _ := primary.snapshot(); ph != 1 {
		t.Fatalf("主渠道应被尝试 1 次, got %d", ph)
	}
	if sh, auths, _ := secondary.snapshot(); sh != 1 || len(auths) != 1 {
		t.Fatalf("次渠道应命中 1 次, got hits=%d auths=%v", sh, auths)
	}
}

// ---------- 验收 #3：流式首 token 前超时切换 ----------

func TestAcceptance3StreamFailoverBeforeFirstToken(t *testing.T) {
	env := newEnv(t)
	primary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done() // 建连后挂起，不返回任何数据
	})
	secondary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		fl.Flush()
	})
	env.addChannel(t, "primary", primary.server.URL, 0, []string{"gpt-x"}, nil, "sk-p")
	env.addChannel(t, "secondary", secondary.server.URL, 1, []string{"gpt-x"}, nil, "sk-s")

	resp, body := env.chat(t, `{"model":"gpt-x","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "\"hi\"") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("应收到次渠道完整流: %s", body)
	}
	if strings.Count(body, "hi") != 1 {
		t.Fatalf("流内容不应重复: %s", body)
	}
	if ph, _, _ := primary.snapshot(); ph != 1 {
		t.Fatalf("主渠道应被尝试 1 次, got %d", ph)
	}
}

// ---------- 验收 #4：首 token 后失败，SSE 错误且不重复 ----------

func TestAcceptance4StreamFailAfterFirstTokenNoRetry(t *testing.T) {
	env := newEnv(t)
	primary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"part1\"}}]}\n\n"))
		fl.Flush()
		panic(http.ErrAbortHandler) // 发送一个 chunk 后连接中断
	})
	secondary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: SECONDARY\n\n"))
		w.(http.Flusher).Flush()
	})
	env.addChannel(t, "primary", primary.server.URL, 0, []string{"gpt-x"}, nil, "sk-p")
	env.addChannel(t, "secondary", secondary.server.URL, 1, []string{"gpt-x"}, nil, "sk-s")

	resp, body := env.chat(t, `{"model":"gpt-x","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 { // 首 chunk 已写出，状态码无法改变
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "part1") {
		t.Fatalf("客户端应已收到第一段内容: %s", body)
	}
	if !strings.Contains(body, "upstream_stream_error") {
		t.Fatalf("客户端应收到明确 SSE 错误事件: %s", body)
	}
	if strings.Contains(body, "SECONDARY") {
		t.Fatalf("首 token 后禁止切换重试: %s", body)
	}
	if sh, _, _ := secondary.snapshot(); sh != 0 {
		t.Fatalf("次渠道不应被调用, got %d", sh)
	}
}

// ---------- 验收 #5：429 + Retry-After → cooldown，其他 key 服务 ----------

func TestAcceptance5RateLimitCooldown(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if auth == "Bearer sk-a" {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"content":"ok"}`))
	})
	env.addChannel(t, "single", up.server.URL, 0, []string{"m"}, nil, "sk-a", "sk-b")

	// 请求 1：key a 429 → 冷却；预算内换 key b 成功
	resp, body := env.chat(t, `{"model":"m","messages":[]}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "ok") {
		t.Fatalf("请求1应经 key b 成功: %d %s", resp.StatusCode, body)
	}
	// key a 应 cooldown 至 now+60s
	now := env.clock.Now()
	found := false
	for _, s := range env.pool.StateMachine().Snapshot() {
		if s.KeyID == lookupKeyID(t, env, "sk-a") {
			found = true
			if s.State != store.KeyStateCooldown {
				t.Fatalf("key a state = %s, want cooldown", s.State)
			}
			remaining := s.CooldownUntil.Sub(now)
			if remaining < 55*time.Second || remaining > 65*time.Second {
				t.Fatalf("cooldown remaining = %v, want ~60s", remaining)
			}
		}
	}
	if !found {
		t.Fatal("key a 不在快照中")
	}
	// 请求 2：key a 仍在冷却，自动用 key b
	resp2, body2 := env.chat(t, `{"model":"m","messages":[]}`)
	if resp2.StatusCode != 200 || !strings.Contains(body2, "ok") {
		t.Fatalf("请求2应由 key b 服务: %d %s", resp2.StatusCode, body2)
	}
	_, auths, _ := up.snapshot()
	aUses := 0
	for _, a := range auths {
		if a == "Bearer sk-a" {
			aUses++
		}
	}
	if aUses != 1 {
		t.Fatalf("key a 应只在首次请求被使用 1 次（此后冷却）, got %d, auths=%v", aUses, auths)
	}
}

// ---------- 验收 #6：401 → key disabled，其余 key 正常 ----------

func TestAcceptance6AuthFailureDisablesKey(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if auth == "Bearer sk-bad" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"content":"ok"}`))
	})
	env.addChannel(t, "single", up.server.URL, 0, []string{"m"}, nil, "sk-bad", "sk-good")

	resp, body := env.chat(t, `{"model":"m","messages":[]}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "ok") {
		t.Fatalf("请求1应换 key 成功: %d %s", resp.StatusCode, body)
	}
	if state := env.pool.StateMachine().State(lookupKeyID(t, env, "sk-bad")); state != store.KeyStateDisabled {
		t.Fatalf("sk-bad state = %s, want disabled", state)
	}
	// 后续请求永不使用 disabled key
	for i := 0; i < 3; i++ {
		resp, body := env.chat(t, `{"model":"m","messages":[]}`)
		if resp.StatusCode != 200 {
			t.Fatalf("请求%d 失败: %d %s", i+2, resp.StatusCode, body)
		}
	}
	_, auths, _ := up.snapshot()
	badUses := 0
	for _, a := range auths {
		if a == "Bearer sk-bad" {
			badUses++
		}
	}
	if badUses != 1 {
		t.Fatalf("坏 key 应只被使用 1 次（此后 disabled）, got %d, auths=%v", badUses, auths)
	}
}

// ---------- 验收 #7：全部渠道耗尽 → 503 OpenAI 错误体 ----------

func TestAcceptance7AllChannelsExhausted(t *testing.T) {
	env := newEnv(t)
	up1 := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	up2 := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	env.addChannel(t, "a", up1.server.URL, 0, []string{"m"}, nil, "sk-1")
	env.addChannel(t, "b", up2.server.URL, 1, []string{"m"}, nil, "sk-2")

	resp, body := env.chat(t, `{"model":"m","messages":[]}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", resp.StatusCode, body)
	}
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &errResp); err != nil {
		t.Fatalf("503 响应不是合法 JSON: %v, body=%s", err, body)
	}
	if errResp.Error.Type != "server_error" || errResp.Error.Code != "all_channels_exhausted" {
		t.Fatalf("错误体字段不符: %+v", errResp)
	}
	if !strings.Contains(errResp.Error.Message, "exhausted") {
		t.Fatalf("message 应说明耗尽: %+v", errResp)
	}
	// 连打 3 次后两 key 均进入 cooldown，第 4 次直接无可用 key（不再打上游）
	for i := 0; i < 2; i++ {
		if resp, _ := env.chat(t, `{"model":"m","messages":[]}`); resp.StatusCode != 503 {
			t.Fatalf("第 %d 次仍应 503", i+2)
		}
	}
	h1, _, _ := up1.snapshot()
	h2, _, _ := up2.snapshot()
	if h1 != 3 || h2 != 3 {
		t.Fatalf("3 次失败后 key 应冷却（各渠道恰被尝试 3 次）: up1=%d up2=%d", h1, h2)
	}
	resp4, _ := env.chat(t, `{"model":"m","messages":[]}`)
	if resp4.StatusCode != 503 {
		t.Fatalf("冷却后应仍返回 503")
	}
	if h1, _, _ = up1.snapshot(); h1 != 3 {
		t.Fatalf("冷却期上游不应再被调用, got %d", h1)
	}
}

// ---------- 透传：不可重试 4xx 原样返回 ----------

func TestNonRetryablePassThrough(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"max token limit exceeded","type":"invalid_request_error"}}`))
	})
	env.addChannel(t, "only", up.server.URL, 0, []string{"m"}, nil, "sk-1")
	resp, body := env.chat(t, `{"model":"m","messages":[]}`)
	if resp.StatusCode != 400 || !strings.Contains(body, "max token limit") {
		t.Fatalf("400 应原样透传: %d %s", resp.StatusCode, body)
	}
	if h, _, _ := up.snapshot(); h != 1 {
		t.Fatalf("不可重试错误只应尝试 1 次, got %d", h)
	}
}

// ---------- /v1/models 去重合并 ----------

func TestModelsEndpoint(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {})
	env.addChannel(t, "a", up.server.URL, 0, []string{"m2", "m1"}, nil, "sk-1")
	env.addChannel(t, "b", up.server.URL, 0, []string{"m1", "m3"}, nil, "sk-2")
	resp, err := http.Get(env.gateway.URL + "/v1/models")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Object != "list" || len(out.Data) != 3 {
		t.Fatalf("models 应去重为 3 个: %+v", out)
	}
	if out.Data[0].ID != "m1" || out.Data[1].ID != "m2" || out.Data[2].ID != "m3" {
		t.Fatalf("models 应排序: %+v", out)
	}
}

// ---------- SSE 首行为多行 chunk 的健壮性（bufio 逐行读） ----------

func TestStreamMultiLineDataPreserved(t *testing.T) {
	env := newEnv(t)
	payload := "data: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\ndata: {\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\ndata: [DONE]\n"
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
		w.(http.Flusher).Flush()
	})
	env.addChannel(t, "s", up.server.URL, 0, []string{"m"}, nil, "sk-1")
	resp, body := env.chat(t, `{"model":"m","stream":true,"messages":[]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	want := []string{"\"A\"", "\"B\"", "[DONE]"}
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Fatalf("缺少 %s: %q", w, body)
		}
	}
	if strings.Count(body, "data:") != 3 {
		t.Fatalf("data 行数应为 3: %q", body)
	}
}

// ---------- 辅助 ----------

func lookupKeyID(t *testing.T, env *testEnv, plainKey string) int64 {
	t.Helper()
	var keys []*store.ChannelKey
	if err := env.db.Find(&keys).Error; err != nil {
		t.Fatalf("find keys: %v", err)
	}
	for _, k := range keys {
		if plain, err := store.Decrypt(k.KeyCipher, env.masterKey); err == nil && plain == plainKey {
			return k.ID
		}
	}
	t.Fatalf("key %s not found", plainKey)
	return 0
}

var _ = bytes.NewBuffer // 保留 imports 一致性
var _ = bufio.NewReader

// ---------- 学习成果回归：404 model_not_found 应换渠道（bifrost ModelAccess） ----------

func TestModelNotFoundFailsOverToNextChannel(t *testing.T) {
	env := newEnv(t)
	primary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"The model 'gpt-x' does not exist","type":"invalid_request_error","code":"model_not_found"}}`))
	})
	secondary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":"served-by-secondary"}`))
	})
	env.addChannel(t, "primary", primary.server.URL, 0, []string{"gpt-x"}, nil, "sk-p")
	env.addChannel(t, "secondary", secondary.server.URL, 1, []string{"gpt-x"}, nil, "sk-s")

	resp, body := env.chat(t, `{"model":"gpt-x","messages":[]}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "served-by-secondary") {
		t.Fatalf("model_not_found 应换渠道成功: %d %s", resp.StatusCode, body)
	}
	// key 不应被惩罚
	if state := env.pool.StateMachine().State(lookupKeyID(t, env, "sk-p")); state != store.KeyStateActive {
		t.Fatalf("model_not_found 不应惩罚 key, state = %s", state)
	}
}

// ---------- 学习成果回归：瞬时错误单请求内跳渠道 + 跨请求阈值冷却（混合策略） ----------

func TestTransientSkipsSecondKeyAndCoolsAfterThreshold(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	env.addChannel(t, "only", up.server.URL, 0, []string{"m"}, nil, "sk-1", "sk-2")

	// 单请求内：key1 500 后应跳过渠道（不再尝试同渠道 key2）
	resp, _ := env.chat(t, `{"model":"m","messages":[]}`)
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if h, _, auths := up.snapshot(); h != 1 || len(auths) != 1 {
		t.Fatalf("瞬时错误应跳过同渠道第二把 key: hits=%d auths=%v", h, auths)
	}
	// WRR 轮流选 key：6 次请求后两把 key 各计满 3 次失败进入冷却
	for i := 0; i < 5; i++ {
		if resp, _ := env.chat(t, `{"model":"m","messages":[]}`); resp.StatusCode != 503 {
			t.Fatalf("第 %d 次应 503", i+2)
		}
	}
	hits, _, _ := up.snapshot()
	if hits != 6 {
		t.Fatalf("6 次请求应合计 6 次上游调用（每请求恰一把 key）: %d", hits)
	}
	if state := env.pool.StateMachine().State(lookupKeyID(t, env, "sk-1")); state != store.KeyStateCooldown {
		t.Fatalf("持续 5xx 达阈值应冷却 key1: %s", state)
	}
	if state := env.pool.StateMachine().State(lookupKeyID(t, env, "sk-2")); state != store.KeyStateCooldown {
		t.Fatalf("持续 5xx 达阈值应冷却 key2: %s", state)
	}
	// 冷却后不再打上游
	if resp, _ := env.chat(t, `{"model":"m","messages":[]}`); resp.StatusCode != 503 {
		t.Fatal("冷却期应 503")
	}
	if h, _, _ := up.snapshot(); h != 6 {
		t.Fatalf("冷却期上游不应再被调用: %d", h)
	}
}

// ---------- 学习成果回归：SSE 首 chunk 错误事件（bifrost stream_error） ----------

func TestStreamFirstChunkErrorEventFailsOver(t *testing.T) {
	env := newEnv(t)
	primary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}` + "\n\n"))
		w.(http.Flusher).Flush()
	})
	secondary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"delta\":\"recovered\"}\n\ndata: [DONE]\n\n"))
		w.(http.Flusher).Flush()
	})
	env.addChannel(t, "primary", primary.server.URL, 0, []string{"m"}, nil, "sk-p")
	env.addChannel(t, "secondary", secondary.server.URL, 1, []string{"m"}, nil, "sk-s")

	resp, body := env.chat(t, `{"model":"m","stream":true,"messages":[]}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "recovered") {
		t.Fatalf("首 chunk 为错误事件应无感切换次渠道: %d %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "rate limit exceeded") {
		t.Fatalf("错误事件不应透出给客户端（首 token 前可重试）: %s", body)
	}
	// rate_limit_error 语义应冷却主渠道 key（SSE 状态推断 429）
	if state := env.pool.StateMachine().State(lookupKeyID(t, env, "sk-p")); state != store.KeyStateCooldown {
		t.Fatalf("SSE rate_limit_error 应按 429 冷却 key, state = %s", state)
	}
}
