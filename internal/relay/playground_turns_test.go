package relay

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// 多轮 + 参数：历史轮次按顺序进入上游 messages，temperature / top_p / max_tokens 透传，
// 且 meta 回显实际发往上游的请求（模型已替换为上游名，不含 key）。
func TestPlaygroundMultiTurnAndParams(t *testing.T) {
	env := newEnv(t)
	var upstreamBody string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, _ *http.Request) {
		upstreamBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"turn ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	env.addChannel(t, "pg", up.server.URL, 0, []string{"gpt-x"}, map[string]string{"gpt-x": "gpt-upstream"}, "sk-pg")

	req := `{"protocol":"chat","model":"gpt-x","stream":false,"temperature":0.2,"top_p":0.9,"max_tokens":64,
		"history":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}],"prompt":"again"}`
	rec, envelope := env.playground(t, req)
	if rec.Code != http.StatusOK || !envelope.OK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var sent struct {
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature"`
		TopP        float64 `json:"top_p"`
		MaxTokens   int     `json:"max_tokens"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(upstreamBody), &sent); err != nil {
		t.Fatalf("decode upstream body: %v", err)
	}
	if sent.Model != "gpt-upstream" || sent.Temperature != 0.2 || sent.TopP != 0.9 || sent.MaxTokens != 64 {
		t.Fatalf("params not forwarded: %s", upstreamBody)
	}
	if len(sent.Messages) != 3 || sent.Messages[0].Role != "user" || sent.Messages[1].Role != "assistant" || sent.Messages[2].Content != "again" {
		t.Fatalf("history not forwarded in order: %s", upstreamBody)
	}

	snapshot := envelope.Data.Meta.UpstreamRequest
	if snapshot == nil || snapshot.Method != http.MethodPost || !strings.HasSuffix(snapshot.URL, "/chat/completions") {
		t.Fatalf("upstream snapshot = %+v", snapshot)
	}
	if !strings.Contains(snapshot.Body, `"model":"gpt-upstream"`) || !strings.Contains(snapshot.Body, `"temperature":0.2`) {
		t.Fatalf("snapshot body should be the rewritten upstream body: %s", snapshot.Body)
	}
	if strings.Contains(snapshot.Body, "sk-pg") || snapshot.Truncated {
		t.Fatalf("snapshot leaked key or wrongly truncated: %+v", snapshot)
	}
}

// Responses 多轮用 easy-input 数组；单轮仍是字符串（与现有契约一致）。
func TestPlaygroundResponsesInputShapeByTurns(t *testing.T) {
	env := newEnv(t)
	var inputs []any
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, _ *http.Request) {
		var req map[string]any
		_ = json.Unmarshal([]byte(body), &req)
		inputs = append(inputs, req["input"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	})
	env.addResponsesChannel(t, "pg-resp", up.server.URL, 0, []string{"gpt-x"}, nil, "sk")

	if rec, _ := env.playground(t, `{"protocol":"responses","model":"gpt-x","prompt":"one","stream":false}`); rec.Code != http.StatusOK {
		t.Fatalf("single turn: %d", rec.Code)
	}
	if rec, _ := env.playground(t, `{"protocol":"responses","model":"gpt-x","prompt":"two","stream":false,
		"history":[{"role":"user","content":"one"},{"role":"assistant","content":"ok"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("multi turn: %d", rec.Code)
	}
	if single, ok := inputs[0].(string); !ok || single != "one" {
		t.Fatalf("single-turn input = %#v, want string", inputs[0])
	}
	arr, ok := inputs[1].([]any)
	if !ok || len(arr) != 3 {
		t.Fatalf("multi-turn input = %#v, want 3-item array", inputs[1])
	}
}

// 回显的请求体脱敏（sk- / Bearer 模式）并按上限截断。
func TestPlaygroundUpstreamRequestRedactedAndTruncated(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "pg-redact", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-real")

	payload, _ := json.Marshal(map[string]any{
		"protocol": "chat", "model": "gpt-x", "stream": false,
		"prompt": "use sk-abcdef1234567890 and Bearer abc.def.ghi please",
	})
	_, envelope := env.playground(t, string(payload))
	snapshot := envelope.Data.Meta.UpstreamRequest
	if snapshot == nil || strings.Contains(snapshot.Body, "sk-abcdef1234567890") || strings.Contains(snapshot.Body, "abc.def.ghi") {
		t.Fatalf("credentials not redacted: %+v", snapshot)
	}
	if !strings.Contains(snapshot.Body, "***") {
		t.Fatalf("redaction marker missing: %s", snapshot.Body)
	}

	// 单条 prompt 受 16000 字节上限约束，用多轮历史把上游请求体顶过 16KB 回显上限 → 截断并标记
	long := strings.Repeat("a", 15000)
	payload, _ = json.Marshal(map[string]any{
		"protocol": "chat", "model": "gpt-x", "stream": false, "prompt": long,
		"history": []map[string]string{
			{"role": "user", "content": long},
			{"role": "assistant", "content": long},
			{"role": "user", "content": long},
		},
	})
	_, envelope = env.playground(t, string(payload))
	snapshot = envelope.Data.Meta.UpstreamRequest
	if snapshot == nil || !snapshot.Truncated || len(snapshot.Body) > playgroundMaxUpstreamBody {
		t.Fatalf("long body not truncated: %+v", snapshot)
	}
}

func TestPlaygroundRejectsBadParamsAndHistory(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "pg-valid", up.server.URL, 0, []string{"gpt-x"}, nil, "sk")
	cases := []string{
		`{"protocol":"chat","model":"gpt-x","prompt":"hi","temperature":3}`,
		`{"protocol":"chat","model":"gpt-x","prompt":"hi","top_p":0}`,
		`{"protocol":"chat","model":"gpt-x","prompt":"hi","max_tokens":0}`,
		`{"protocol":"chat","model":"gpt-x","prompt":"hi","history":[{"role":"system","content":"x"}]}`,
		`{"protocol":"chat","model":"gpt-x","prompt":"hi","history":[{"role":"user","content":""}]}`,
	}
	for _, body := range cases {
		rec, envelope := env.playground(t, body)
		if rec.Code != http.StatusBadRequest || envelope.OK {
			t.Fatalf("%s: status = %d body = %s", body, rec.Code, rec.Body.String())
		}
	}
	if hits, _, _ := up.snapshot(); hits != 0 {
		t.Fatalf("invalid requests must not reach upstream, hits=%d", hits)
	}
}
