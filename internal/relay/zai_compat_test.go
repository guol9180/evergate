package relay

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"evergate/internal/store"
)

func TestKtorZAIChatDisablesReasoningBeforeVisibleTimeout(t *testing.T) {
	oldInterval := keepaliveInterval
	keepaliveInterval = 20 * time.Millisecond
	t.Cleanup(func() { keepaliveInterval = oldInterval })

	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var wire struct {
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if err := json.Unmarshal([]byte(body), &wire); err != nil {
			t.Errorf("unmarshal upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if wire.Thinking.Type == "disabled" {
			_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"visible\"}}]}\n\n"))
		} else {
			_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"thinking\"}}]}\n\n"))
			w.(http.Flusher).Flush()
			time.Sleep(140 * time.Millisecond)
			_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{\"content\":\"late\"}}]}\n\n"))
		}
		_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":2,\"total_tokens\":10}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		w.(http.Flusher).Flush()
	})
	env.addChannel(t, "ZAI", up.server.URL+"/api/coding/paas/v4", 0, []string{"glm-5.3-flash"}, nil, "sk")
	token, plain := env.addToken(t, "ktor-visible-timeout", func(token *store.Token) { token.Quota = 20 })

	client, streamConn := trackedStreamClient(t)
	req, err := http.NewRequest(http.MethodPost, env.authGateway(t).URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"generate commit message"}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ktor-client")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	body := readSSEUntilDone(t, resp, streamConn(), 80*time.Millisecond, chatLineHasVisibleDelta)
	if !strings.Contains(body, `"content":"visible"`) || strings.Contains(body, `"content":"late"`) {
		t.Fatalf("visible content timeout was not prevented: %s", body)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if row := tokenRow(t, env, token.ID); row.UsedQuota == 10 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("used quota = %d, want 10", tokenRow(t, env, token.ID).UsedQuota)
}

func TestKtorZAIThinkingPolicyBoundaries(t *testing.T) {
	const zaiURL = "https://open.bigmodel.cn/api/coding/paas/v4"
	tests := []struct {
		name      string
		userAgent string
		baseURL   string
		stream    bool
		body      string
		want      string
	}{
		{
			name:      "ktor zai glm disables thinking",
			userAgent: "ktor-client",
			baseURL:   zaiURL,
			stream:    true,
			body:      `{"model":"glm-5.3-flash","stream":true}`,
			want:      `"thinking":{"type":"disabled"}`,
		},
		{
			name:      "explicit thinking is preserved",
			userAgent: "ktor-client",
			baseURL:   zaiURL,
			stream:    true,
			body:      `{"model":"glm-5.3-flash","stream":true,"thinking":{"type":"enabled"}}`,
			want:      `"thinking":{"type":"enabled"}`,
		},
		{
			name:      "explicit reasoning effort is preserved",
			userAgent: "ktor-client",
			baseURL:   zaiURL,
			stream:    true,
			body:      `{"model":"glm-5.3-flash","stream":true,"reasoning_effort":"high"}`,
			want:      "",
		},
		{
			name:      "non ktor is untouched",
			userAgent: "curl/8",
			baseURL:   zaiURL,
			stream:    true,
			body:      `{"model":"glm-5.3-flash","stream":true}`,
			want:      "",
		},
		{
			name:      "non zai coding path is untouched",
			userAgent: "ktor-client",
			baseURL:   "https://api.example.com/v1",
			stream:    true,
			body:      `{"model":"glm-5.3-flash","stream":true}`,
			want:      "",
		},
		{
			name:      "non glm model is untouched",
			userAgent: "ktor-client",
			baseURL:   zaiURL,
			stream:    true,
			body:      `{"model":"other-model","stream":true}`,
			want:      "",
		},
		{
			name:      "non stream is untouched",
			userAgent: "ktor-client",
			baseURL:   zaiURL,
			stream:    false,
			body:      `{"model":"glm-5.3-flash"}`,
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(applyKtorZAIThinkingPolicy(tt.userAgent, tt.baseURL, tt.stream, []byte(tt.body)))
			if tt.want == "" {
				if got != tt.body {
					t.Fatalf("body changed unexpectedly: %s", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func chatLineHasVisibleDelta(line string) bool {
	if !strings.HasPrefix(line, "data:") {
		return false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" || payload == "[DONE]" {
		return false
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content   string            `json:"content"`
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(payload), &chunk) != nil {
		return false
	}
	return len(chunk.Choices) > 0 &&
		(chunk.Choices[0].Delta.Content != "" || len(chunk.Choices[0].Delta.ToolCalls) > 0)
}
