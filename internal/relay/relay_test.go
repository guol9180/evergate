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

func (f *fakeClock) Advance(d time.Duration) { f.mu.Lock(); defer f.mu.Unlock(); f.t = f.t.Add(d) }

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
	rel := New(Options{DB: db, Router: router.New(42), Pool: pl, MasterKey: master, Defaults: d, Now: clock.Now})
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.POST("/v1/chat/completions", rel.HandleChatCompletions)
	e.POST("/v1/responses", rel.HandleResponses)
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

func (e *testEnv) responses(t *testing.T, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(e.gateway.URL+"/v1/responses", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post responses: %v", err)
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

func TestResponsesNonStreamConvertsToChatCompletions(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Errorf("unmarshal upstream request: %v", err)
		}
		if req["stream"] != false {
			t.Errorf("upstream stream = %v, want false", req["stream"])
		}
		if req["model"] != "upstream-model" {
			t.Errorf("upstream model = %v, want upstream-model", req["model"])
		}
		msgs, _ := req["messages"].([]any)
		if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["role"] != "user" {
			t.Errorf("upstream messages = %#v, want instructions + input message", req["messages"])
		}
		tools, _ := req["tools"].([]any)
		if len(tools) != 1 || tools[0].(map[string]any)["type"] != "function" {
			t.Errorf("upstream tools = %#v, want converted Responses function tool", req["tools"])
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl-upstream","object":"chat.completion","created":123,"model":"upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"gateway ok","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Shanghai\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, map[string]string{"gateway-model": "upstream-model"}, "sk-good")

	resp, body := env.responses(t, `{
		"model":"gateway-model",
		"instructions":"You are concise.",
		"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],
		"tools":[{"type":"function","name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}],
		"stream":false
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		ID     string `json:"id"`
		Object string `json:"object"`
		Status string `json:"status"`
		Model  string `json:"model"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Status  string `json:"status"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"output"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("unmarshal response: %v\n%s", err, body)
	}
	if !strings.HasPrefix(out.ID, "resp_") || out.Object != "response" || out.Status != "completed" || out.Model != "gateway-model" {
		t.Fatalf("unexpected response envelope: %+v", out)
	}
	if len(out.Output) != 2 || out.Output[0].Type != "message" || out.Output[0].Content[0].Text != "gateway ok" ||
		out.Output[1].Type != "function_call" || out.Output[1].CallID != "call_1" || out.Output[1].Name != "get_weather" {
		t.Fatalf("unexpected output: %#v", out.Output)
	}
	if out.Usage.InputTokens != 3 || out.Usage.OutputTokens != 4 || out.Usage.TotalTokens != 7 {
		t.Fatalf("unexpected usage: %+v", out.Usage)
	}
}

func TestResponsesStreamConvertsChatSSE(t *testing.T) {
	env := newEnv(t)
	payload := strings.Join([]string{
		`data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
		`data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
		`data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
		"",
	}, "\n\n")
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, body := env.responses(t, `{"model":"gateway-model","input":"hello","stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	got := sseEventTypes(body)
	want := []string{
		"response.created",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("event types = %v\nwant %v\nbody:\n%s", got, want, body)
	}
	if !strings.Contains(body, `"delta":"Hel"`) || !strings.Contains(body, `"delta":"lo"`) ||
		!strings.Contains(body, `"text":"Hello"`) {
		t.Fatalf("stream did not preserve and aggregate text: %s", body)
	}
	if strings.Contains(body, "[DONE]") || strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("Responses stream must not leak Chat Completions framing: %s", body)
	}
}

func TestResponsesCustomToolNonStream(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tools []chatTool `json:"tools"`
		}
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Errorf("unmarshal custom tool request: %v", err)
		}
		if len(req.Tools) != 1 || req.Tools[0].Type != "function" || req.Tools[0].Function.Name != "exec_command" {
			t.Fatalf("custom tool must be converted to a function tool: %#v", req.Tools)
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl-custom","object":"chat.completion","created":123,"model":"upstream-model","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_custom","type":"function","function":{"name":"exec_command","arguments":"{\"input\":\"echo EverGate\"}"}}]},"finish_reason":"tool_calls"}]}`))
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, body := env.responses(t, `{
		"model":"gateway-model",
		"input":"run custom tool",
		"tools":[{"type":"custom","name":"exec_command","description":"Run a command"}],
		"stream":false
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Output []struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
			Name   string `json:"name"`
			Input  string `json:"input"`
		} `json:"output"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("unmarshal response: %v\n%s", err, body)
	}
	if len(out.Output) != 1 || out.Output[0].Type != "custom_tool_call" ||
		out.Output[0].CallID != "call_custom" || out.Output[0].Name != "exec_command" ||
		out.Output[0].Input != "echo EverGate" {
		t.Fatalf("unexpected custom tool output: %#v", out.Output)
	}
}

func TestResponsesApplyPatchCustomGrammarStream(t *testing.T) {
	env := newEnv(t)
	grammar := "start: apply_patch\napply_patch: \"*** Begin Patch\""
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tools []chatTool `json:"tools"`
		}
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Errorf("unmarshal apply_patch request: %v", err)
		}
		if len(req.Tools) != 1 || req.Tools[0].Function.Name != "apply_patch" {
			t.Fatalf("apply_patch must stay a named Chat function tool: %#v", req.Tools)
		}
		parameters := req.Tools[0].Function.Parameters
		properties, _ := parameters["properties"].(map[string]any)
		input, _ := properties["input"].(map[string]any)
		description, _ := input["description"].(string)
		if !strings.Contains(description, "grammar syntax: lark") ||
			!strings.Contains(description, "grammar definition:") ||
			!strings.Contains(description, grammar) {
			t.Fatalf("apply_patch grammar was silently dropped: %q", description)
		}
		if parameters["additionalProperties"] != false {
			t.Fatalf("apply_patch schema must reject extra fields: %#v", parameters)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		payload := strings.Join([]string{
			`data: {"id":"chatcmpl-patch","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_patch","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
			"",
		}, "\n\n")
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, body := env.responses(t, `{
		"model":"gateway-model",
		"input":"apply a patch",
		"tools":[{
			"type":"custom",
			"name":"apply_patch",
			"description":"Apply a patch",
			"format":{"type":"grammar","syntax":"lark","definition":"start: apply_patch\napply_patch: \"*** Begin Patch\""}
		}],
		"stream":true
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply_patch grammar status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"type":"custom_tool_call"`) ||
		!strings.Contains(body, `"name":"apply_patch"`) ||
		!strings.Contains(body, `"input":"*** Begin Patch"`) ||
		!strings.Contains(body, "response.custom_tool_call_input.delta") ||
		!strings.Contains(body, "response.custom_tool_call_input.done") {
		t.Fatalf("apply_patch grammar response is incomplete: %s", body)
	}
}

func TestResponsesStreamToolCallRoundTrip(t *testing.T) {
	env := newEnv(t)
	var phase atomic.Int64
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		var payload string
		if phase.Add(1) == 1 {
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-tool","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"echo ok\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		} else {
			var req struct {
				Messages []struct {
					Role       string          `json:"role"`
					ToolCallID string          `json:"tool_call_id"`
					ToolCalls  []chatToolCall  `json:"tool_calls"`
					Content    json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Errorf("unmarshal second upstream request: %v", err)
			}
			if len(req.Messages) != 2 || req.Messages[0].Role != "assistant" || len(req.Messages[0].ToolCalls) != 1 ||
				req.Messages[0].ToolCalls[0].ID != "call_1" || req.Messages[0].ToolCalls[0].Function.Name != "exec_command" {
				t.Errorf("function_call was not replayed as assistant tool_calls: %#v", req.Messages)
			}
			if len(req.Messages) != 2 || req.Messages[1].Role != "tool" || req.Messages[1].ToolCallID != "call_1" {
				t.Errorf("function_call_output was not replayed as tool message: %#v", req.Messages)
			}
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-after-tool","object":"chat.completion.chunk","created":124,"model":"upstream-model","choices":[{"index":0,"delta":{"content":"tool ok"}}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		}
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	_, first := env.responses(t, `{"model":"gateway-model","input":"run tool","stream":true}`)
	wantFirst := []string{
		"response.created", "response.output_item.added", "response.function_call_arguments.delta",
		"response.function_call_arguments.done", "response.output_item.done", "response.completed",
	}
	got := sseEventTypes(first)
	if len(got) != len(wantFirst) {
		t.Fatalf("tool event types = %#v\nwant %#v\nbody:\n%s", got, wantFirst, first)
	}
	for i := range wantFirst {
		if got[i] != wantFirst[i] {
			t.Fatalf("tool event[%d] = %q, want %q\nbody:\n%s", i, got[i], wantFirst[i], first)
		}
	}
	if !strings.Contains(first, `"call_id":"call_1"`) || !strings.Contains(first, `"name":"exec_command"`) ||
		!strings.Contains(first, `"arguments":"{\"cmd\":\"echo ok\"}"`) {
		t.Fatalf("tool output item is incomplete: %s", first)
	}

	_, second := env.responses(t, `{
		"model":"gateway-model","stream":true,
		"input":[
			{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"echo ok\"}","status":"completed"},
			{"type":"function_call_output","id":"fco_1","call_id":"call_1","output":"command output"}
		]
	}`)
	if !strings.Contains(second, `"delta":"tool ok"`) {
		t.Fatalf("second Responses turn did not return final text: %s", second)
	}
}

func TestResponsesCustomToolStreamRoundTrip(t *testing.T) {
	env := newEnv(t)
	var phase atomic.Int64
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		var payload string
		if phase.Add(1) == 1 {
			var req struct {
				Tools []chatTool `json:"tools"`
			}
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Errorf("unmarshal custom tool request: %v", err)
			}
			if len(req.Tools) != 1 || req.Tools[0].Type != "function" || req.Tools[0].Function.Name != "exec_command" {
				t.Fatalf("custom tool must be converted to a Chat function tool: %#v", req.Tools)
			}
			params := req.Tools[0].Function.Parameters
			if params["type"] != "object" {
				t.Fatalf("custom tool parameters = %#v", params)
			}
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-custom","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_custom","type":"function","function":{"name":"exec_command","arguments":"{\"input\":\"echo EverGate\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		} else {
			var req struct {
				Messages []struct {
					Role       string          `json:"role"`
					ToolCallID string          `json:"tool_call_id"`
					ToolCalls  []chatToolCall  `json:"tool_calls"`
					Content    json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Errorf("unmarshal second custom tool request: %v", err)
			}
			if len(req.Messages) != 2 ||
				req.Messages[0].ToolCalls[0].Function.Arguments != `{"input":"echo EverGate"}` ||
				req.Messages[1].Role != "tool" || req.Messages[1].ToolCallID != "call_custom" ||
				string(req.Messages[1].Content) != `"EverGate tool ok"` {
				t.Fatalf("custom tool round trip was not converted to Chat messages: %#v", req.Messages)
			}
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-after-custom","object":"chat.completion.chunk","created":124,"model":"upstream-model","choices":[{"index":0,"delta":{"content":"custom tool ok"}}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		}
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, first := env.responses(t, `{
		"model":"gateway-model",
		"input":"run custom tool",
		"tools":[{"type":"custom","name":"exec_command","description":"Run a command"}],
		"stream":true
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("custom tool request status = %d, body = %s", resp.StatusCode, first)
	}
	wantFirst := []string{
		"response.created", "response.output_item.added", "response.custom_tool_call_input.delta",
		"response.custom_tool_call_input.done", "response.output_item.done", "response.completed",
	}
	if got := sseEventTypes(first); strings.Join(got, ",") != strings.Join(wantFirst, ",") {
		t.Fatalf("custom tool event types = %v, want %v\nbody:\n%s", got, wantFirst, first)
	}
	if !strings.Contains(first, `"type":"custom_tool_call"`) ||
		!strings.Contains(first, `"call_id":"call_custom"`) ||
		!strings.Contains(first, `"name":"exec_command"`) ||
		!strings.Contains(first, `"input":"echo EverGate"`) {
		t.Fatalf("custom tool output item is incomplete: %s", first)
	}

	_, second := env.responses(t, `{
		"model":"gateway-model","stream":true,
		"input":[
			{"type":"custom_tool_call","id":"ctc_1","call_id":"call_custom","name":"exec_command","input":"echo EverGate"},
			{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_custom","output":"EverGate tool ok"}
		]
	}`)
	if !strings.Contains(second, `"delta":"custom tool ok"`) {
		t.Fatalf("second custom tool turn did not return final text: %s", second)
	}
}

func TestResponsesCustomToolRawArgumentsFallback(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload := strings.Join([]string{
			`data: {"id":"chatcmpl-raw","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_raw","type":"function","function":{"name":"apply_patch","arguments":"*** Begin Patch *** End Patch"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
			"",
		}, "\n\n")
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, body := env.responses(t, `{
		"model":"gateway-model",
		"input":"apply patch",
		"tools":[{"type":"custom","name":"apply_patch"}],
		"stream":true
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("raw custom arguments status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"delta":"*** Begin Patch *** End Patch"`) ||
		!strings.Contains(body, `"input":"*** Begin Patch *** End Patch"`) {
		t.Fatalf("raw custom arguments were dropped: %s", body)
	}
}

func TestResponsesNamespaceToolsPreserveCallNamespace(t *testing.T) {
	env := newEnv(t)
	var phase atomic.Int64
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		var payload string
		if phase.Add(1) == 1 {
			var req struct {
				Tools []chatTool `json:"tools"`
			}
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Errorf("unmarshal namespace request: %v", err)
			}
			if len(req.Tools) != 2 || req.Tools[0].Function.Name != "multi_agent_v1__spawn_agent" || req.Tools[1].Function.Name != "multi_agent_v1__apply_patch" {
				t.Fatalf("namespace tools were not flattened safely: %#v", req.Tools)
			}
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-namespace","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_function","type":"function","function":{"name":"multi_agent_v1__spawn_agent","arguments":"{\"agent\":\"a\"}"}},{"index":1,"id":"call_patch","type":"function","function":{"name":"multi_agent_v1__apply_patch","arguments":"{\"input\":\"patch\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		} else {
			var req struct {
				Messages []chatMessage `json:"messages"`
			}
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Errorf("unmarshal namespace continuation: %v", err)
			}
			if len(req.Messages) != 4 ||
				req.Messages[0].ToolCalls[0].Function.Name != "multi_agent_v1__spawn_agent" ||
				req.Messages[1].Role != "tool" ||
				req.Messages[2].ToolCalls[0].Function.Name != "multi_agent_v1__apply_patch" ||
				req.Messages[3].Role != "tool" {
				t.Fatalf("namespace continuation was not replayed: %#v", req.Messages)
			}
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-namespace-final","object":"chat.completion.chunk","created":124,"model":"upstream-model","choices":[{"index":0,"delta":{"content":"namespace ok"}}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		}
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	tools := `[
		{"type":"namespace","name":"multi_agent_v1","description":"Agents","tools":[
			{"type":"function","name":"spawn_agent","description":"Spawn","parameters":{"type":"object","properties":{"agent":{"type":"string"}},"required":["agent"]}},
			{"type":"custom","name":"apply_patch","description":"Patch"}
		]}
	]`
	_, first := env.responses(t, `{"model":"gateway-model","input":"use tools","stream":true,"tools":`+tools+`}`)
	if !strings.Contains(first, `"namespace":"multi_agent_v1"`) ||
		!strings.Contains(first, `"type":"function_call"`) ||
		!strings.Contains(first, `"type":"custom_tool_call"`) {
		t.Fatalf("namespace was not restored to Responses output: %s", first)
	}

	_, second := env.responses(t, `{
		"model":"gateway-model","stream":true,
		"input":[
			{"type":"function_call","call_id":"call_function","name":"spawn_agent","namespace":"multi_agent_v1","arguments":"{\"agent\":\"a\"}"},
			{"type":"function_call_output","call_id":"call_function","output":"started"},
			{"type":"custom_tool_call","call_id":"call_patch","name":"apply_patch","namespace":"multi_agent_v1","input":"patch"},
			{"type":"custom_tool_call_output","call_id":"call_patch","output":"done"}
		]
	}`)
	if !strings.Contains(second, `"delta":"namespace ok"`) {
		t.Fatalf("namespace continuation did not finish: %s", second)
	}
}

func TestResponsesDuplicateNamespacedToolNamesRoundTrip(t *testing.T) {
	env := newEnv(t)
	var phase atomic.Int64
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		var payload string
		if phase.Add(1) == 1 {
			var req struct {
				Tools []chatTool `json:"tools"`
			}
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Errorf("unmarshal duplicate-name request: %v", err)
			}
			if len(req.Tools) != 2 ||
				req.Tools[0].Function.Name != "mcp__node_repl__js" ||
				req.Tools[1].Function.Name != "mcp__cua_repl__js" {
				t.Fatalf("duplicate namespaced tools were not made unique: %#v", req.Tools)
			}
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-duplicate","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_fn","type":"function","function":{"name":"mcp__node_repl__js","arguments":"{\"code\":\"1+1\"}"}},{"index":1,"id":"call_custom","type":"function","function":{"name":"mcp__cua_repl__js","arguments":"{\"input\":\"state\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		} else {
			var req struct {
				Messages []chatMessage `json:"messages"`
			}
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Errorf("unmarshal duplicate-name continuation: %v", err)
			}
			if len(req.Messages) != 4 ||
				req.Messages[0].ToolCalls[0].Function.Name != "mcp__node_repl__js" ||
				req.Messages[1].Role != "tool" ||
				req.Messages[2].ToolCalls[0].Function.Name != "mcp__cua_repl__js" ||
				req.Messages[3].Role != "tool" {
				t.Fatalf("duplicate-name continuation was not replayed with upstream aliases: %#v", req.Messages)
			}
			payload = strings.Join([]string{
				`data: {"id":"chatcmpl-duplicate-final","object":"chat.completion.chunk","created":124,"model":"upstream-model","choices":[{"index":0,"delta":{"content":"duplicate namespace ok"}}]}`,
				`data: [DONE]`,
				"",
			}, "\n\n")
		}
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	tools := `[
		{"type":"namespace","name":"mcp__node_repl","description":"Node","tools":[
			{"type":"function","name":"js","description":"Run JS","parameters":{"type":"object","properties":{"code":{"type":"string"}},"required":["code"]}}
		]},
		{"type":"namespace","name":"mcp__cua_repl","description":"CUA","tools":[
			{"type":"custom","name":"js","description":"Control UI"}
		]}
	]`
	resp, first := env.responses(t, `{"model":"gateway-model","input":"use both","stream":true,"tools":`+tools+`}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate namespaced tools status = %d, body = %s", resp.StatusCode, first)
	}
	if !strings.Contains(first, `"namespace":"mcp__node_repl"`) ||
		!strings.Contains(first, `"namespace":"mcp__cua_repl"`) ||
		!strings.Contains(first, `"type":"function_call"`) ||
		!strings.Contains(first, `"type":"custom_tool_call"`) {
		t.Fatalf("duplicate namespaced tools were not restored: %s", first)
	}

	_, second := env.responses(t, `{
		"model":"gateway-model","stream":true,
		"input":[
			{"type":"function_call","call_id":"call_fn","name":"js","namespace":"mcp__node_repl","arguments":"{\"code\":\"1+1\"}"},
			{"type":"function_call_output","call_id":"call_fn","output":"2"},
			{"type":"custom_tool_call","call_id":"call_custom","name":"js","namespace":"mcp__cua_repl","input":"state"},
			{"type":"custom_tool_call_output","call_id":"call_custom","output":"ok"}
		]
	}`)
	if !strings.Contains(second, `"delta":"duplicate namespace ok"`) {
		t.Fatalf("duplicate-name continuation did not finish: %s", second)
	}
}

func TestResponsesAdditionalToolsItemIsHoisted(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []chatMessage `json:"messages"`
			Tools    []chatTool    `json:"tools"`
		}
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Errorf("unmarshal additional_tools request: %v", err)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Fatalf("additional_tools item leaked into messages: %#v", req.Messages)
		}
		if len(req.Tools) != 2 || req.Tools[0].Function.Name != "lookup" || req.Tools[1].Function.Name != "apply_patch" {
			t.Fatalf("additional_tools were not hoisted: %#v", req.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		payload := strings.Join([]string{
			`data: {"id":"chatcmpl-additional","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_patch","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"patch\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
			"",
		}, "\n\n")
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, body := env.responses(t, `{
		"model":"gateway-model",
		"stream":true,
		"input":[
			{"type":"additional_tools","id":"at_1","role":"developer","tools":[
				{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string"}}}},
				{"type":"custom","name":"apply_patch","format":{"type":"grammar","syntax":"lark","definition":"start: patch"}}
			]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"use tool"}]}
		]
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("additional_tools status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"type":"custom_tool_call"`) || !strings.Contains(body, `"input":"patch"`) {
		t.Fatalf("additional_tools custom output was incomplete: %s", body)
	}
}

func TestResponsesConfigurationUpdateOverridesReasoningEffort(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages        []chatMessage `json:"messages"`
			ReasoningEffort string        `json:"reasoning_effort"`
			Verbosity       string        `json:"verbosity"`
		}
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Errorf("unmarshal configuration_update request: %v", err)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Fatalf("configuration_update leaked into messages: %#v", req.Messages)
		}
		if req.ReasoningEffort != "high" {
			t.Fatalf("reasoning_effort = %q, want high", req.ReasoningEffort)
		}
		if req.Verbosity != "low" {
			t.Fatalf("verbosity = %q, want low", req.Verbosity)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		payload := strings.Join([]string{
			`data: {"id":"chatcmpl-config","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"content":"config ok"}}]}`,
			`data: [DONE]`,
			"",
		}, "\n\n")
		_, _ = w.Write([]byte(payload))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, body := env.responses(t, `{
		"model":"gateway-model",
		"stream":true,
		"reasoning":{"effort":"low"},
		"text":{"verbosity":"low"},
		"input":[
			{"type":"configuration_update","reasoning":{"effort":"high"}},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"delta":"config ok"`) {
		t.Fatalf("configuration_update was not accepted: %d %s", resp.StatusCode, body)
	}
}

func TestResponsesRejectsUnsupportedCustomGrammarSyntax(t *testing.T) {
	env := newEnv(t)
	resp, body := env.responses(t, `{
		"model":"gateway-model",
		"input":"apply patch",
		"tools":[{"type":"custom","name":"apply_patch","format":{"type":"grammar","syntax":"json","definition":"start"}}]
	}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "grammar syntax") {
		t.Fatalf("unsupported grammar syntax should be diagnostic: %d %s", resp.StatusCode, body)
	}
}

func TestResponsesStreamMidstreamErrorEmitsResponseFailed(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"upstream exploded\",\"type\":\"server_error\"}}\n\n"))
		w.(http.Flusher).Flush()
	})
	env.addChannel(t, "openai", up.server.URL, 1, []string{"gateway-model"}, nil, "sk-good")

	resp, body := env.responses(t, `{"model":"gateway-model","input":"hello","stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "event: response.failed") || !strings.Contains(body, "upstream exploded") {
		t.Fatalf("midstream error should become response.failed: %s", body)
	}
	if strings.Contains(body, "event: response.completed") {
		t.Fatalf("failed stream must not emit completed: %s", body)
	}
}

func TestResponsesRejectsUnsupportedRequestField(t *testing.T) {
	env := newEnv(t)
	resp, body := env.responses(t, `{"model":"gateway-model","input":"hello","future_feature":true}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "unsupported Responses request field") || !strings.Contains(body, "future_feature") {
		t.Fatalf("unsupported field should be explicitly diagnosable: %s", body)
	}
}

func TestResponsesRejectsInvalidToken(t *testing.T) {
	env := newEnv(t)
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.POST("/v1/responses", TokenAuth(env.db), env.HandleResponses)
	gw := httptest.NewServer(e)
	defer gw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/responses", strings.NewReader(`{"model":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer eg-wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
}

func TestResponsesStreamFailsOverBeforeFirstEvent(t *testing.T) {
	env := newEnv(t)
	primary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream boom"}}`))
	})
	secondary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-recovered\",\"choices\":[{\"delta\":{\"content\":\"recovered\"}}]}\n\ndata: [DONE]\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	env.addChannel(t, "primary", primary.server.URL, 0, []string{"gateway-model"}, nil, "sk-p")
	env.addChannel(t, "secondary", secondary.server.URL, 1, []string{"gateway-model"}, nil, "sk-s")

	resp, body := env.responses(t, `{"model":"gateway-model","input":"hello","stream":true}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"delta":"recovered"`) {
		t.Fatalf("Responses stream should fail over before first event: %d %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "upstream boom") {
		t.Fatalf("primary error must not leak after retry: %s", body)
	}
}

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

func TestStreamFirstTokenMeasuredFromUpstreamRequestStart(t *testing.T) {
	for name, responses := range map[string]bool{"chat": false, "responses": true} {
		t.Run(name, func(t *testing.T) {
			env := newEnv(t)
			entered := make(chan struct{})
			release := make(chan struct{})
			up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-release
				w.Header().Set("Content-Type", "text/event-stream")
				payload := strings.Join([]string{
					`data: {"id":"chatcmpl-first-token","object":"chat.completion.chunk","created":123,"model":"upstream-model","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"}}]}`,
					`data: [DONE]`,
					"",
				}, "\n\n")
				_, _ = w.Write([]byte(payload))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			})
			env.addChannel(t, "slow-first-token", up.server.URL, 0, []string{"gateway-model"}, nil, "sk-good")
			go func() {
				<-entered
				env.clock.Advance(80 * time.Millisecond)
				close(release)
			}()

			var ch store.Channel
			if err := env.db.First(&ch, "name = ?", "slow-first-token").Error; err != nil {
				t.Fatalf("find channel: %v", err)
			}
			var key store.ChannelKey
			if err := env.db.First(&key, "channel_id = ?", ch.ID).Error; err != nil {
				t.Fatalf("find key: %v", err)
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gateway-model","stream":true,"messages":[]}`))
			if responses {
				c.Set("evergate.protocol", responsesProtocol)
			}

			hit := &hitRecord{}
			trace := make([]store.RetryAttempt, 0, 1)
			outcome := env.attemptOne(c, router.Candidate{Channel: &ch, UpstreamModel: "upstream-model"}, &key,
				[]byte(`{"model":"gateway-model","stream":true,"messages":[]}`), "gateway-model", true, &trace, hit)
			if outcome != outcomeDone {
				t.Fatalf("outcome = %v, want done", outcome)
			}
			if hit.firstTokenMS < 80 {
				t.Fatalf("first_token_ms = %d, want >= 80 (from upstream request start), trace=%#v", hit.firstTokenMS, trace)
			}
			if len(trace) != 1 || trace[0].ElapsedMS < 80 {
				t.Fatalf("success trace should include time before first token: %#v", trace)
			}
		})
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
	if !strings.Contains(errResp.Error.Message, "http 502") {
		t.Fatalf("message 应携带最后一次上游状态而非只有耗尽: %+v", errResp)
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

func sseEventTypes(body string) []string {
	types := make([]string, 0, 8)
	for _, line := range strings.Split(body, "\n") {
		if after, ok := strings.CutPrefix(line, "event: "); ok {
			types = append(types, strings.TrimSpace(after))
		}
	}
	return types
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
