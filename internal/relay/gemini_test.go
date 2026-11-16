package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGeminiNativePassthroughNonStream(t *testing.T) {
	env := newEnv(t)
	var gotPath, gotAPIKey, gotAuthorization, gotBody string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotAuthorization = r.Header.Get("Authorization")
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"thoughtsTokenCount":1,"totalTokenCount":8},"modelVersion":"gemini-2.5-pro"}`))
	})
	env.addGeminiChannel(t, "gemini", up.server.URL, 0, []string{"gemini-alias"}, map[string]string{"gemini-alias": "gemini-2.5-pro"}, "sk-gemini")
	token, plain := env.addToken(t, "gemini-native", nil)
	gateway := env.authGateway(t)

	request := `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"systemInstruction":{"parts":[{"text":"be nice"}]},"safetySettings":[{"category":"HARM_CATEGORY_HATE_SPEECH","threshold":"BLOCK_NONE"}],"generationConfig":{"temperature":0.2},"tools":[{"functionDeclarations":[{"name":"lookup","description":"d","parameters":{"type":"object","properties":{}}}]}]}`
	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:generateContent", plain, nil, request)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"gemini-2.5-pro"`) {
		t.Fatalf("native response: %d %s", resp.StatusCode, body)
	}
	if gotPath != "/v1beta/models/gemini-2.5-pro:generateContent" || gotAPIKey != "sk-gemini" || gotAuthorization != "" {
		t.Fatalf("upstream path/auth = %s / %q / %q", gotPath, gotAPIKey, gotAuthorization)
	}
	for _, want := range []string{`"systemInstruction"`, `"safetySettings"`, `"generationConfig"`, `"functionDeclarations"`} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("native body missing %q: %s", want, gotBody)
		}
	}
	if strings.Contains(gotBody, `"model":`) || strings.Contains(gotBody, `"stream":`) {
		t.Fatalf("native body must not be injected with dispatch fields: %s", gotBody)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].PromptTokens != 5 || logs[0].CompletionTokens != 3 {
		t.Fatalf("gemini usage log = %#v", logs)
	}
}

func TestGeminiNativeStreamPassthrough(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w,
			"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"hel\"}]},\"index\":0}]}\n\n",
			"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"lo\"}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":7,\"candidatesTokenCount\":2,\"thoughtsTokenCount\":1,\"totalTokenCount\":10}}\n\n",
		)
	})
	env.addGeminiChannel(t, "gemini", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-gemini")
	token, plain := env.addToken(t, "gemini-stream", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:streamGenerateContent", plain, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"text":"hel"`) || !strings.Contains(body, `"finishReason":"STOP"`) {
		t.Fatalf("native stream missing frames: %s", body)
	}
	if strings.Contains(body, "[DONE]") {
		t.Fatalf("gemini stream must not contain [DONE]: %s", body)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].PromptTokens != 7 || logs[0].CompletionTokens != 3 {
		t.Fatalf("gemini stream usage log = %#v", logs)
	}
}

func TestGeminiNativeStreamFailoverBeforeFirstFrame(t *testing.T) {
	env := newEnv(t)
	failing := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w, "data: {\"error\":{\"code\":503,\"message\":\"busy\",\"status\":\"UNAVAILABLE\"}}\n\n")
	})
	healthy := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w, "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\",\"index\":0}]}\n\n")
	})
	env.addGeminiChannel(t, "primary", failing.server.URL, 0, []string{"gemini-alias"}, nil, "sk-a")
	env.addGeminiChannel(t, "backup", healthy.server.URL, 1, []string{"gemini-alias"}, nil, "sk-b")
	token, plain := env.addToken(t, "gemini-failover", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:streamGenerateContent", plain, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"text":"ok"`) {
		t.Fatalf("failover response: %d %s", resp.StatusCode, body)
	}
	if failing.hits.Load() != 1 || healthy.hits.Load() != 1 {
		t.Fatalf("hits = %d/%d", failing.hits.Load(), healthy.hits.Load())
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || len(logs[0].RetryTrace) != 2 || logs[0].RetryTrace[0].ErrorType != "transient" {
		t.Fatalf("failover trace = %#v", logs)
	}
}

func TestGeminiNativeStreamDoesNotRetryAfterFirstFrame(t *testing.T) {
	env := newEnv(t)
	first := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w, "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"partial\"}]},\"index\":0}]}\n\n")
	})
	second := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not retry after first Gemini frame")
	})
	env.addGeminiChannel(t, "primary", first.server.URL, 0, []string{"gemini-alias"}, nil, "sk-a")
	env.addGeminiChannel(t, "backup", second.server.URL, 1, []string{"gemini-alias"}, nil, "sk-b")
	_, plain := env.addToken(t, "gemini-no-retry", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:streamGenerateContent", plain, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"text":"partial"`) {
		t.Fatalf("stream response: %d %s", resp.StatusCode, body)
	}
	if first.hits.Load() != 1 || second.hits.Load() != 0 {
		t.Fatalf("hits = %d/%d", first.hits.Load(), second.hits.Load())
	}
}

func TestGeminiBridgeNonStreamWithTools(t *testing.T) {
	env := newEnv(t)
	var gotPath, gotAuthorization, gotBody string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"checking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`))
	})
	env.addChannel(t, "chat", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-chat")
	_, plain := env.addToken(t, "gemini-bridge", nil)
	gateway := env.authGateway(t)

	request := `{"contents":[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"q":"x"}}}]},{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{"result":"ok"}}},{"text":"now"}]}],"systemInstruction":{"parts":[{"text":"sys"}]},"tools":[{"functionDeclarations":[{"name":"lookup","description":"d","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}]}],"toolConfig":{"functionCallingConfig":{"mode":"AUTO"}}}`
	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:generateContent", plain, nil, request)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if gotPath != "/chat/completions" || gotAuthorization != "Bearer sk-chat" {
		t.Fatalf("bridge upstream = %s / %q", gotPath, gotAuthorization)
	}
	for _, want := range []string{`"role":"system"`, `"tool_calls"`, `"tool_call_id":"call_1_lookup"`, `"role":"tool"`, `"lookup"`} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("bridge request missing %q: %s", want, gotBody)
		}
	}
	var response struct {
		Candidates []struct {
			Content struct {
				Role  string `json:"role"`
				Parts []struct {
					Text         string `json:"text"`
					FunctionCall *struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		Usage struct {
			PromptTokenCount     int64 `json:"promptTokenCount"`
			CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode gemini response: %v body=%s", err, body)
	}
	if len(response.Candidates) != 1 || response.Candidates[0].FinishReason != "STOP" {
		t.Fatalf("gemini candidates = %#v", response.Candidates)
	}
	parts := response.Candidates[0].Content.Parts
	if len(parts) != 2 || parts[0].Text != "checking" || parts[1].FunctionCall == nil || parts[1].FunctionCall.Args["q"] != "x" {
		t.Fatalf("gemini parts = %#v", parts)
	}
	if response.Usage.PromptTokenCount != 9 || response.Usage.CandidatesTokenCount != 4 {
		t.Fatalf("gemini usage = %#v", response.Usage)
	}
}

func TestGeminiBridgeStream(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		writeSSEEvents(w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"q\\\":\"}}]}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"x\\\"}\"}}]}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4,\"total_tokens\":12}}\n\n",
			"data: [DONE]\n\n",
		)
	})
	env.addChannel(t, "chat", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-chat")
	token, plain := env.addToken(t, "gemini-bridge-stream", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:streamGenerateContent", plain, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	for _, want := range []string{`"text":"Hel"`, `"text":"lo"`, `"functionCall"`, `"name":"lookup"`, `"q":"x"`, `"finishReason":"STOP"`, `"usageMetadata"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("bridge stream missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "[DONE]") || strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Fatalf("bridge stream leaked Chat chunks: %s", body)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].PromptTokens != 8 || logs[0].CompletionTokens != 4 {
		t.Fatalf("gemini bridge usage log = %#v", logs)
	}
}

func TestGeminiBridgeRejectsUnsupportedFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "safetySettings",
			body: `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"safetySettings":[{"category":"HARM_CATEGORY_HATE_SPEECH","threshold":"BLOCK_NONE"}]}`,
			want: "safetySettings",
		},
		{
			name: "thinkingConfig",
			body: `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"thinkingConfig":{"thinkingBudget":1024}}}`,
			want: "thinkingConfig",
		},
		{
			name: "inlineData",
			body: `{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"aGk="}}]}]}`,
			want: "inlineData",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
				t.Errorf("unsupported bridge field must not reach upstream")
			})
			env.addChannel(t, "chat", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-chat")
			_, plain := env.addToken(t, "gemini-unsupported", nil)
			gateway := env.authGateway(t)

			resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:generateContent", plain, nil, tc.body)
			if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, tc.want) {
				t.Fatalf("status = %d body = %s", resp.StatusCode, body)
			}
			if hits := up.hits.Load(); hits != 0 {
				t.Fatalf("upstream hits = %d", hits)
			}
		})
	}
}

func TestGeminiAuthAndQueryKey(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP","index":0}]}`))
	})
	env.addGeminiChannel(t, "gemini", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-gemini")
	_, plain := env.addToken(t, "gemini-auth", nil)
	gateway := env.authGateway(t)

	req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1beta/models/gemini-alias:generateContent",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-goog-api-key", plain)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("x-goog-api-key auth: %d %s", resp.StatusCode, data)
	}
	resp.Body.Close()

	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:generateContent?key=abc", plain, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"code":"api_key_in_query"`) {
		t.Fatalf("query key response: %d %s", resp.StatusCode, body)
	}
}

func TestGeminiCountTokensNativeOnly(t *testing.T) {
	env := newEnv(t)
	var gotPath string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalTokens":42}`))
	})
	env.addGeminiChannel(t, "gemini", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-gemini")
	_, plain := env.addToken(t, "gemini-count", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:countTokens", plain, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"totalTokens":42`) {
		t.Fatalf("countTokens response: %d %s", resp.StatusCode, body)
	}
	if gotPath != "/v1beta/models/gemini-alias:countTokens" {
		t.Fatalf("countTokens path = %s", gotPath)
	}

	env2 := newEnv(t)
	env2.addChannel(t, "chat", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-chat")
	_, plain2 := env2.addToken(t, "gemini-count-bridge", nil)
	gateway2 := env2.authGateway(t)
	resp, body = postWithHeaders(t, gateway2, "/v1beta/models/gemini-alias:countTokens", plain2, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"code":"gemini_native_required"`) {
		t.Fatalf("bridge countTokens response: %d %s", resp.StatusCode, body)
	}
}

func TestGeminiUpstreamErrorUsesOpenAIEnvelope(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"contents is required","status":"INVALID_ARGUMENT"}}`))
	})
	env.addGeminiChannel(t, "gemini", up.server.URL, 0, []string{"gemini-alias"}, nil, "sk-gemini")
	_, plain := env.addToken(t, "gemini-error", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1beta/models/gemini-alias:generateContent", plain, nil,
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"code":"INVALID_ARGUMENT"`) || !strings.Contains(body, `"message":"contents is required"`) {
		t.Fatalf("gemini error envelope = %s", body)
	}
}

func TestGeminiUsageAndFinishReasonMapping(t *testing.T) {
	usage := geminiUsage{
		PromptTokenCount: 10, ToolUsePromptTokenCount: 2,
		CandidatesTokenCount: 4, ThoughtsTokenCount: 3, TotalTokenCount: 19,
	}
	prompt, completion, total, _ := usage.normalize()
	if prompt != 12 || completion != 7 || total != 19 {
		t.Fatalf("gemini usage = prompt %d completion %d total %d", prompt, completion, total)
	}
	cases := map[string]string{"stop": "STOP", "length": "MAX_TOKENS", "content_filter": "SAFETY"}
	for input, want := range cases {
		if got := geminiFinishReason(input); got != want {
			t.Fatalf("finish reason %q = %q, want %q", input, got, want)
		}
	}
}
