package relay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
	"evergate/internal/usage"
)

type playgroundEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Response json.RawMessage `json:"response"`
		Meta     playgroundMeta  `json:"meta"`
	} `json:"data"`
	Message string `json:"message"`
}

func (e *testEnv) playground(t *testing.T, body string) (*httptest.ResponseRecorder, playgroundEnvelope) {
	return e.playgroundWithHeaders(t, body, nil)
}

func (e *testEnv) playgroundWithHeaders(t *testing.T, body string, headers map[string]string) (*httptest.ResponseRecorder, playgroundEnvelope) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/api/playground", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		c.Request.Header.Set(name, value)
	}
	e.HandlePlayground(c)
	var env playgroundEnvelope
	if strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode playground envelope: %v (%s)", err, rec.Body.String())
		}
	}
	return rec, env
}

func TestPlaygroundChatNonStream(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		if req.Model != "gpt-upstream" || len(req.Messages) != 2 ||
			req.Messages[0].Role != "system" || req.Messages[0].Content != "be brief" ||
			req.Messages[1].Role != "user" || req.Messages[1].Content != "hello" {
			t.Errorf("unexpected upstream request: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-pg","choices":[{"message":{"role":"assistant","content":"playground ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`))
	})
	env.addChannel(t, "pg", up.server.URL, 0, []string{"gpt-x"}, map[string]string{"gpt-x": "gpt-upstream"}, "sk-pg")
	if err := env.usage.SetDefault(context.Background(), usage.PriceInput{Input: 1_000_000, Output: 2_000_000}); err != nil {
		t.Fatalf("set default price: %v", err)
	}

	rec, envelope := env.playground(t, `{"protocol":"chat","model":"gpt-x","system":"be brief","prompt":"hello","stream":false}`)
	if rec.Code != http.StatusOK || !envelope.OK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(string(envelope.Data.Response), "playground ok") {
		t.Fatalf("response = %s", envelope.Data.Response)
	}
	meta := envelope.Data.Meta
	if meta.Status != 200 || meta.ChannelName != "pg" || meta.UpstreamModel != "gpt-upstream" {
		t.Fatalf("meta = %+v", meta)
	}
	if meta.Usage.PromptTokens != 5 || meta.Usage.CompletionTokens != 3 || !meta.Usage.UsageSeen {
		t.Fatalf("usage = %+v", meta.Usage)
	}
	if meta.CostMicros != 11 || meta.CostStatus != usage.CostStatusPriced {
		t.Fatalf("cost = %d %s", meta.CostMicros, meta.CostStatus)
	}
	if len(meta.RetryTrace) != 1 || meta.RetryTrace[0].ErrorType != "" {
		t.Fatalf("retry trace = %+v", meta.RetryTrace)
	}

	env.logWriter.Close()
	var log store.RequestLog
	if err := env.db.Order("id desc").First(&log).Error; err != nil {
		t.Fatalf("read request log: %v", err)
	}
	if log.Source != "playground" || log.TokenID != 0 || log.Model != "gpt-x" {
		t.Fatalf("playground 日志标记错误: %+v", log)
	}
	if strings.Contains(log.ChannelName+log.ErrorMessage+log.UserAgent, "sk-pg") {
		t.Fatalf("日志不应包含上游 key 明文: %+v", log)
	}
}

func TestPlaygroundRealHTTPServer(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("real http"))
	env.addChannel(t, "pg-http", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-http")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/admin/api/playground", env.HandlePlayground)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)

	resp, err := http.Post(server.URL+"/admin/api/playground", "application/json",
		strings.NewReader(`{"protocol":"chat","model":"gpt-x","prompt":"hi","stream":false}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(data), "real http") {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, data)
	}
}

func TestPlaygroundChatStream(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"he\"}}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"llo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		fl.Flush()
	})
	env.addChannel(t, "pg-stream", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-stream")

	rec, _ := env.playground(t, `{"protocol":"chat","model":"gpt-x","prompt":"hi","stream":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, want := range []string{`"content":"he"`, `"content":"llo"`, "event: evergate.meta"} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE 缺少 %q:\n%s", want, body)
		}
	}
	meta := parsePlaygroundMeta(t, body)
	if meta.Status != 200 || meta.Usage.TotalTokens != 4 || !meta.Usage.UsageSeen {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestPlaygroundResponsesNonStreamAndStream(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			t.Errorf("path = %q, want /responses", r.URL.Path)
		}
		var req map[string]any
		_ = json.Unmarshal([]byte(body), &req)
		if req["model"] != "gpt-x" || req["input"] != "hello" || req["instructions"] != "be brief" {
			t.Errorf("unexpected Responses body: %s", body)
		}
		if req["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fl := w.(http.Flusher)
			_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_pg\"}}\n\n"))
			_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"native ok\"}\n\n"))
			_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_pg\",\"usage\":{\"input_tokens\":4,\"output_tokens\":2,\"total_tokens\":6}}}\n\n"))
			fl.Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_pg","object":"response","output":[{"type":"message","content":[{"type":"output_text","text":"native ok"}]}],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}`))
	})
	env.addResponsesChannel(t, "pg-native", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-native")

	rec, envelope := env.playground(t, `{"protocol":"responses","model":"gpt-x","system":"be brief","prompt":"hello","stream":false}`)
	if rec.Code != http.StatusOK || !envelope.OK || !strings.Contains(string(envelope.Data.Response), "native ok") {
		t.Fatalf("non-stream: %d %s", rec.Code, rec.Body.String())
	}
	if envelope.Data.Meta.Usage.TotalTokens != 6 || envelope.Data.Meta.Protocol != "responses" {
		t.Fatalf("meta = %+v", envelope.Data.Meta)
	}

	rec, _ = env.playground(t, `{"protocol":"responses","model":"gpt-x","system":"be brief","prompt":"hello","stream":true}`)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "response.output_text.delta") || !strings.Contains(body, "event: evergate.meta") {
		t.Fatalf("stream: %d %s", rec.Code, body)
	}
	if meta := parsePlaygroundMeta(t, body); meta.Usage.TotalTokens != 6 {
		t.Fatalf("stream meta = %+v", meta)
	}
}

func TestPlaygroundMessagesNonStreamAndStream(t *testing.T) {
	env := newEnv(t)
	var upstreamBodies []string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		upstreamBodies = append(upstreamBodies, body)
		var req map[string]any
		_ = json.Unmarshal([]byte(body), &req)
		if req["model"] != "claude-upstream" || req["max_tokens"] != float64(1024) ||
			req["system"] != "be brief" || req["stream"] == true {
			if req["stream"] != true {
				t.Errorf("unexpected Messages body: %s", body)
			}
		}
		if req["stream"] == true {
			writeSSEEvents(w,
				"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_pg\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-upstream\",\"content\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":1}}}\n\n",
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"native messages\"}}\n\n",
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\n",
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_pg","type":"message","role":"assistant","content":[{"type":"text","text":"native messages"}],"model":"claude-upstream","usage":{"input_tokens":4,"output_tokens":2}}`))
	})
	env.addAnthropicChannel(t, "pg-messages", up.server.URL, 0, []string{"claude-x"}, map[string]string{"claude-x": "claude-upstream"}, "sk-ant")

	rec, envelope := env.playground(t, `{"protocol":"anthropic","model":"claude-x","system":"be brief","prompt":"hello","stream":false}`)
	if rec.Code != http.StatusOK || !envelope.OK || !strings.Contains(string(envelope.Data.Response), "native messages") {
		t.Fatalf("non-stream: %d %s", rec.Code, rec.Body.String())
	}
	if envelope.Data.Meta.Protocol != "anthropic" || envelope.Data.Meta.UpstreamModel != "claude-upstream" ||
		envelope.Data.Meta.Usage.TotalTokens != 6 {
		t.Fatalf("meta = %+v", envelope.Data.Meta)
	}

	rec, _ = env.playground(t, `{"protocol":"anthropic","model":"claude-x","system":"be brief","prompt":"hello","stream":true}`)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "content_block_delta") ||
		!strings.Contains(body, "native messages") || !strings.Contains(body, "event: evergate.meta") {
		t.Fatalf("stream: %d %s", rec.Code, body)
	}
	if meta := parsePlaygroundMeta(t, body); meta.Usage.TotalTokens != 7 {
		t.Fatalf("stream meta = %+v", meta)
	}
	if len(upstreamBodies) != 2 {
		t.Fatalf("upstream requests = %d", len(upstreamBodies))
	}
}

func TestPlaygroundGeminiNonStreamAndStream(t *testing.T) {
	env := newEnv(t)
	var paths []string
	var bodies []string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		bodies = append(bodies, body)
		var req map[string]any
		_ = json.Unmarshal([]byte(body), &req)
		if req["systemInstruction"] == nil || req["contents"] == nil || req["generationConfig"] == nil {
			t.Errorf("unexpected Gemini body: %s", body)
		}
		if strings.HasSuffix(r.URL.Path, ":streamGenerateContent") {
			writeSSEEvents(w,
				"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"native gemini\"}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":2,\"thoughtsTokenCount\":1,\"totalTokenCount\":8}}\n\n",
			)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"native gemini"}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"thoughtsTokenCount":1,"totalTokenCount":8}}`))
	})
	env.addGeminiChannel(t, "pg-gemini", up.server.URL, 0, []string{"gemini-x"}, map[string]string{"gemini-x": "gemini-upstream"}, "sk-gem")

	rec, envelope := env.playground(t, `{"protocol":"gemini","model":"gemini-x","system":"be brief","prompt":"hello","stream":false}`)
	if rec.Code != http.StatusOK || !envelope.OK || !strings.Contains(string(envelope.Data.Response), "native gemini") {
		t.Fatalf("non-stream: %d %s", rec.Code, rec.Body.String())
	}
	if envelope.Data.Meta.Protocol != "gemini" || envelope.Data.Meta.UpstreamModel != "gemini-upstream" ||
		envelope.Data.Meta.Usage.TotalTokens != 8 {
		t.Fatalf("meta = %+v", envelope.Data.Meta)
	}

	rec, _ = env.playground(t, `{"protocol":"gemini","model":"gemini-x","system":"be brief","prompt":"hello","stream":true}`)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "native gemini") ||
		!strings.Contains(body, "event: evergate.meta") {
		t.Fatalf("stream: %d %s", rec.Code, body)
	}
	if meta := parsePlaygroundMeta(t, body); meta.Usage.TotalTokens != 8 {
		t.Fatalf("stream meta = %+v", meta)
	}
	if len(paths) != 2 ||
		!strings.HasSuffix(paths[0], "/v1beta/models/gemini-upstream:generateContent") ||
		!strings.HasSuffix(paths[1], "/v1beta/models/gemini-upstream:streamGenerateContent") {
		t.Fatalf("paths = %#v", paths)
	}
	if len(bodies) != 2 {
		t.Fatalf("bodies = %d", len(bodies))
	}
}

func TestPlaygroundFailoverAndInvalidModel(t *testing.T) {
	env := newEnv(t)
	primary := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	})
	secondary := newUpstream(t, chatOKHandler("recovered"))
	env.addChannel(t, "primary", primary.server.URL, 0, []string{"gpt-x"}, nil, "sk-p")
	env.addChannel(t, "secondary", secondary.server.URL, 1, []string{"gpt-x"}, nil, "sk-s")

	rec, envelope := env.playground(t, `{"protocol":"chat","model":"gpt-x","prompt":"hi"}`)
	if rec.Code != http.StatusOK || !envelope.OK || !strings.Contains(string(envelope.Data.Response), "recovered") {
		t.Fatalf("failover: %d %s", rec.Code, rec.Body.String())
	}
	if len(envelope.Data.Meta.RetryTrace) != 2 {
		t.Fatalf("retry trace = %+v", envelope.Data.Meta.RetryTrace)
	}
	if envelope.Data.Meta.ChannelName != "secondary" {
		t.Fatalf("final channel = %q", envelope.Data.Meta.ChannelName)
	}

	rec, envelope = env.playground(t, `{"protocol":"chat","model":"missing-model","prompt":"hi"}`)
	if rec.Code != http.StatusOK || !envelope.OK {
		t.Fatalf("invalid model envelope: %d %s", rec.Code, rec.Body.String())
	}
	if envelope.Data.Meta.Status != http.StatusServiceUnavailable ||
		!strings.Contains(envelope.Data.Meta.ErrorMessage, "all upstream channels exhausted") {
		t.Fatalf("invalid model meta = %+v", envelope.Data.Meta)
	}
}

func TestPlaygroundValidationLimits(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("never"))
	env.addChannel(t, "pg", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-pg")

	cases := []struct {
		name string
		body string
		want string
	}{
		{"bad protocol", `{"protocol":"grpc","model":"gpt-x","prompt":"hi"}`, "protocol"},
		{"empty prompt", `{"protocol":"chat","model":"gpt-x","prompt":"  "}`, "prompt"},
		{"long prompt", `{"protocol":"chat","model":"gpt-x","prompt":"` + strings.Repeat("x", playgroundMaxPrompt+1) + `"}`, "上限"},
		{"bad json", `{"protocol":`, "JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, _ := env.playground(t, c.body)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.want) {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
	if hits, _, _ := up.snapshot(); hits != 0 {
		t.Fatalf("非法 Playground 请求不得触达上游, hits=%d", hits)
	}
}

func parsePlaygroundMeta(t *testing.T, body string) playgroundMeta {
	t.Helper()
	for _, block := range strings.Split(body, "\n\n") {
		if !strings.Contains(block, "event: evergate.meta") {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}
			var meta playgroundMeta
			if err := json.Unmarshal([]byte(data), &meta); err != nil {
				t.Fatalf("decode meta: %v (%s)", err, data)
			}
			return meta
		}
	}
	t.Fatalf("SSE 缺少 evergate.meta 事件:\n%s", body)
	return playgroundMeta{}
}
