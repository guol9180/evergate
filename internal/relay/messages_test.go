package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func writeSSEEvents(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, event := range events {
		_, _ = io.WriteString(w, event)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func TestMessagesNativePassthroughNonStream(t *testing.T) {
	env := newEnv(t)
	var gotPath, gotAPIKey, gotAuthorization, gotVersion, gotBeta, gotBody string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotAuthorization = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		gotBeta = r.Header.Get("anthropic-beta")
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"model":"claude-3-5-sonnet","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":2}}`))
	})
	env.addAnthropicChannel(t, "anthropic", up.server.URL, 0, []string{"claude-3-5-sonnet"}, nil, "sk-ant")
	_, plain := env.addToken(t, "messages-native", nil)
	gateway := env.authGateway(t)

	request := `{"model":"claude-3-5-sonnet","max_tokens":64,"system":[{"type":"text","text":"be nice","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}],"thinking":{"type":"enabled","budget_tokens":1024},"stream":false}`
	resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, map[string]string{
		"anthropic-version": "2023-06-01",
		"anthropic-beta":    "prompt-caching-2024-07-31",
	}, request)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"msg_1"`) {
		t.Fatalf("native response not passed through: %s", body)
	}
	if gotPath != "/v1/messages" || gotAPIKey != "sk-ant" || gotAuthorization != "" {
		t.Fatalf("upstream auth/path = path %s api-key %q authorization %q", gotPath, gotAPIKey, gotAuthorization)
	}
	if gotVersion != "2023-06-01" || gotBeta != "prompt-caching-2024-07-31" {
		t.Fatalf("anthropic headers = version %q beta %q", gotVersion, gotBeta)
	}
	if !strings.Contains(gotBody, `"thinking"`) || !strings.Contains(gotBody, `"cache_control"`) {
		t.Fatalf("native body lost state fields: %s", gotBody)
	}
	if hits := up.hits.Load(); hits != 1 {
		t.Fatalf("upstream hits = %d, want 1", hits)
	}
}

func TestMessagesNativeStreamPassthrough(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w,
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-3-5-sonnet\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":7,\"output_tokens\":1}}}\n\n",
			"event: ping\ndata: {\"type\":\"ping\"}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":3}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		)
	})
	env.addAnthropicChannel(t, "anthropic", up.server.URL, 0, []string{"claude-3-5-sonnet"}, nil, "sk-ant")
	token, plain := env.addToken(t, "messages-stream", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, nil,
		`{"model":"claude-3-5-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	for _, want := range []string{"event: message_start", "event: ping", "event: content_block_delta", "event: message_stop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "[DONE]") {
		t.Fatalf("messages stream must not synthesize [DONE]: %s", body)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].PromptTokens != 7 || logs[0].CompletionTokens != 3 {
		t.Fatalf("messages stream usage log = %#v", logs)
	}
}

func TestMessagesNativeStreamFailoverBeforeFirstEvent(t *testing.T) {
	env := newEnv(t)
	failing := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n")
	})
	healthy := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w,
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_2\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-3-5-sonnet\",\"content\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		)
	})
	env.addAnthropicChannel(t, "primary", failing.server.URL, 0, []string{"claude-3-5-sonnet"}, nil, "sk-a")
	env.addAnthropicChannel(t, "backup", healthy.server.URL, 1, []string{"claude-3-5-sonnet"}, nil, "sk-b")
	token, plain := env.addToken(t, "messages-failover", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, nil,
		`{"model":"claude-3-5-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"msg_2"`) {
		t.Fatalf("failover response: %d %s", resp.StatusCode, body)
	}
	if failing.hits.Load() != 1 || healthy.hits.Load() != 1 {
		t.Fatalf("hits = %d/%d, want 1/1", failing.hits.Load(), healthy.hits.Load())
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || len(logs[0].RetryTrace) != 2 {
		t.Fatalf("failover trace = %#v", logs)
	}
	if logs[0].RetryTrace[0].ErrorType != "transient" {
		t.Fatalf("first attempt error type = %q, want transient", logs[0].RetryTrace[0].ErrorType)
	}
}

func TestMessagesBridgeNonStreamWithTools(t *testing.T) {
	env := newEnv(t)
	var gotPath, gotAuthorization, gotBody string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"let me check","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	})
	env.addChannel(t, "chat", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-chat")
	_, plain := env.addToken(t, "messages-bridge", nil)
	gateway := env.authGateway(t)

	request := `{"model":"gpt-x","max_tokens":128,"system":"sys","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_0","name":"get_weather","input":{"city":"LA"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_0","content":"sunny"},{"type":"text","text":"now SF"}]}],"tools":[{"name":"get_weather","description":"weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}],"tool_choice":{"type":"auto"}}`
	resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, nil, request)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if gotPath != "/chat/completions" || gotAuthorization != "Bearer sk-chat" {
		t.Fatalf("bridge upstream = path %s authorization %q", gotPath, gotAuthorization)
	}
	for _, want := range []string{`"tool_calls"`, `"tool_call_id":"call_0"`, `"role":"tool"`, `"get_weather"`} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("bridge request missing %q: %s", want, gotBody)
		}
	}
	var response struct {
		Type       string `json:"type"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode bridge response: %v body = %s", err, body)
	}
	if response.Type != "message" || response.StopReason != "tool_use" {
		t.Fatalf("bridge response envelope = %#v", response)
	}
	if len(response.Content) != 2 || response.Content[0].Text != "let me check" || response.Content[1].Type != "tool_use" || response.Content[1].Input["city"] != "SF" {
		t.Fatalf("bridge response content = %#v", response.Content)
	}
	if response.Usage.InputTokens != 10 || response.Usage.OutputTokens != 5 {
		t.Fatalf("bridge response usage = %#v", response.Usage)
	}
}

func TestMessagesBridgeStreamEmitsAnthropicEvents(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"q\\\":\"}}]}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"x\\\"}\"}}]}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4,\"total_tokens\":12}}\n\n",
			"data: [DONE]\n\n",
		)
	})
	env.addChannel(t, "chat", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-chat")
	token, plain := env.addToken(t, "messages-bridge-stream", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, nil,
		`{"model":"gpt-x","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	ordered := []string{
		"event: message_start",
		"event: content_block_start",
		"text_delta",
		"event: content_block_stop",
		"\"type\":\"tool_use\"",
		"input_json_delta",
		"\"stop_reason\":\"tool_use\"",
		"event: message_stop",
	}
	offset := 0
	for _, want := range ordered {
		index := strings.Index(body[offset:], want)
		if index < 0 {
			t.Fatalf("stream missing %q after offset %d: %s", want, offset, body)
		}
		offset += index + len(want)
	}
	if strings.Contains(body, "[DONE]") || strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Fatalf("bridge stream leaked Chat chunks: %s", body)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].PromptTokens != 8 || logs[0].CompletionTokens != 4 {
		t.Fatalf("bridge stream usage log = %#v", logs)
	}
}

func TestMessagesBridgeRejectsUnsupportedStateFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "thinking",
			body: `{"model":"gpt-x","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"enabled","budget_tokens":1024}}`,
			want: "thinking",
		},
		{
			name: "cache_control",
			body: `{"model":"gpt-x","max_tokens":32,"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}]}`,
			want: "cache_control",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
				t.Errorf("unsupported bridge field must not reach upstream")
			})
			env.addChannel(t, "chat", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-chat")
			_, plain := env.addToken(t, "messages-unsupported", nil)
			gateway := env.authGateway(t)

			resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, nil, tc.body)
			if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, tc.want) {
				t.Fatalf("status = %d body = %s", resp.StatusCode, body)
			}
			if !strings.Contains(body, `"code":"invalid_request_error"`) {
				t.Fatalf("error body = %s", body)
			}
			if hits := up.hits.Load(); hits != 0 {
				t.Fatalf("upstream hits = %d, want 0", hits)
			}
		})
	}
}

func TestMessagesStreamDoesNotRetryAfterFirstEvent(t *testing.T) {
	env := newEnv(t)
	first := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w,
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-3-5-sonnet\",\"content\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n",
		)
	})
	second := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not retry after first event was written")
	})
	env.addAnthropicChannel(t, "primary", first.server.URL, 0, []string{"claude-3-5-sonnet"}, nil, "sk-a")
	env.addAnthropicChannel(t, "backup", second.server.URL, 1, []string{"claude-3-5-sonnet"}, nil, "sk-b")
	_, plain := env.addToken(t, "messages-no-retry", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, nil,
		`{"model":"claude-3-5-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: error") {
		t.Fatalf("stream should start then surface interruption: %s", body)
	}
	if first.hits.Load() != 1 || second.hits.Load() != 0 {
		t.Fatalf("hits = %d/%d, want 1/0", first.hits.Load(), second.hits.Load())
	}
}

func TestMessagesAcceptsXAPIKeyAuth(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[],"usage":{"input_tokens":1,"output_tokens":1}}`))
	})
	env.addAnthropicChannel(t, "anthropic", up.server.URL, 0, []string{"claude-3-5-sonnet"}, nil, "sk-ant")
	_, plain := env.addToken(t, "messages-x-api-key", nil)
	gateway := env.authGateway(t)

	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages",
		strings.NewReader(`{"model":"claude-3-5-sonnet","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-api-key", plain)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, data)
	}
}

func TestMessagesCountTokensNativeOnly(t *testing.T) {
	env := newEnv(t)
	var gotPath string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":42}`))
	})
	env.addAnthropicChannel(t, "anthropic", up.server.URL, 0, []string{"claude-3-5-sonnet"}, nil, "sk-ant")
	_, plain := env.addToken(t, "messages-count", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1/messages/count_tokens", plain, nil,
		`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"input_tokens":42`) {
		t.Fatalf("count_tokens response: %d %s", resp.StatusCode, body)
	}
	if gotPath != "/v1/messages/count_tokens" {
		t.Fatalf("count_tokens path = %s", gotPath)
	}

	env2 := newEnv(t)
	env2.addChannel(t, "chat", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-chat")
	_, plain2 := env2.addToken(t, "messages-count-bridge", nil)
	gateway2 := env2.authGateway(t)
	resp, body = postWithHeaders(t, gateway2, "/v1/messages/count_tokens", plain2, nil,
		`{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"code":"messages_native_required"`) {
		t.Fatalf("bridge count_tokens response: %d %s", resp.StatusCode, body)
	}
}

func TestMessagesUpstreamErrorUsesOpenAIEnvelope(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens is required"}}`))
	})
	env.addAnthropicChannel(t, "anthropic", up.server.URL, 0, []string{"claude-3-5-sonnet"}, nil, "sk-ant")
	_, plain := env.addToken(t, "messages-error", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1/messages", plain, nil,
		`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	for _, want := range []string{`"code":"invalid_request_error"`, `"message":"max_tokens is required"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("error envelope missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `"type":"error","error"`) {
		t.Fatalf("Anthropic error body leaked upstream shape: %s", body)
	}
}
