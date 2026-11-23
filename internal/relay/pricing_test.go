package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"evergate/internal/store"
	"evergate/internal/usage"
)

func TestRequestCostUsesSnapshotAndDoesNotRewriteHistory(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})
	env.addChannel(t, "cost", up.server.URL, 0, []string{"m"}, nil, "sk")
	if err := env.usage.SetDefault(context.Background(), 1_000_000, 2_000_000); err != nil {
		t.Fatalf("set default price: %v", err)
	}

	if resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: %d %s", resp.StatusCode, body)
	}
	if err := env.usage.SetDefault(context.Background(), 2_000_000, 4_000_000); err != nil {
		t.Fatalf("update default price: %v", err)
	}
	if resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("second request: %d %s", resp.StatusCode, body)
	}

	env.logWriter.Close()
	var logs []store.RequestLog
	if err := env.db.Order("id asc").Find(&logs).Error; err != nil {
		t.Fatalf("read logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs = %d, want 2", len(logs))
	}
	if logs[0].CostStatus != usage.CostStatusPriced || logs[0].CostMicros != 7 || logs[1].CostMicros != 14 {
		t.Fatalf("cost history should use price at request time: %#v", logs)
	}
}

func TestChannelModelPriceOverridesDefault(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})
	env.addChannel(t, "override", up.server.URL, 0, []string{"m"}, nil, "sk")
	if err := env.usage.SetDefault(context.Background(), 1_000_000, 1_000_000); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if _, err := env.usage.UpsertOverride(context.Background(), 1, "m", 10_000_000, 20_000_000); err != nil {
		t.Fatalf("set override: %v", err)
	}
	if resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("request: %d %s", resp.StatusCode, body)
	}
	env.logWriter.Close()
	var log store.RequestLog
	if err := env.db.First(&log).Error; err != nil {
		t.Fatalf("read log: %v", err)
	}
	if log.CostMicros != 70 || log.CostStatus != usage.CostStatusPriced {
		t.Fatalf("override cost = %#v, want 70 micros", log)
	}
}

func TestChatStreamRequestsUsageForCost(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		var wire struct {
			StreamOptions struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
		}
		if err := json.Unmarshal([]byte(body), &wire); err != nil {
			t.Errorf("unmarshal upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n"))
		if wire.StreamOptions.IncludeUsage {
			_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
	env.addChannel(t, "stream-cost", up.server.URL, 0, []string{"m"}, nil, "sk")
	token, plain := env.addToken(t, "stream-cost", nil)
	if err := env.usage.SetDefault(context.Background(), 1_000_000, 2_000_000); err != nil {
		t.Fatalf("set default price: %v", err)
	}

	gateway := env.authGateway(t)
	resp, body := authPost(t, gateway, "/v1/chat/completions", plain,
		`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"content":"ok"`) {
		t.Fatalf("stream response: %d %s", resp.StatusCode, body)
	}

	env.logWriter.Close()
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 {
		t.Fatalf("logs = %d, want 1", len(logs))
	}
	if logs[0].PromptTokens != 3 || logs[0].CompletionTokens != 2 ||
		logs[0].CostMicros != 7 || logs[0].CostStatus != usage.CostStatusPriced {
		t.Fatalf("stream usage/cost = %#v", logs[0])
	}
}

func TestRequestCostAcrossAllProtocols(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testEnv, string)
		path   string
		non    string
		stream string
	}{
		{
			name: "chat",
			setup: func(env *testEnv, baseURL string) {
				env.addChannel(t, "chat-cost", baseURL, 0, []string{"m"}, nil, "sk")
			},
			path:   "/v1/chat/completions",
			non:    `{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":false}`,
			stream: `{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`,
		},
		{
			name: "responses",
			setup: func(env *testEnv, baseURL string) {
				env.addResponsesChannel(t, "responses-cost", baseURL, 0, []string{"m"}, nil, "sk")
			},
			path:   "/v1/responses",
			non:    `{"model":"m","input":"hi","stream":false}`,
			stream: `{"model":"m","input":"hi","stream":true}`,
		},
		{
			name: "anthropic",
			setup: func(env *testEnv, baseURL string) {
				env.addAnthropicChannel(t, "messages-cost", baseURL, 0, []string{"m"}, nil, "sk")
			},
			path:   "/v1/messages",
			non:    `{"model":"m","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":false}`,
			stream: `{"model":"m","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":true}`,
		},
		{
			name: "gemini",
			setup: func(env *testEnv, baseURL string) {
				env.addGeminiChannel(t, "gemini-cost", baseURL, 0, []string{"m"}, nil, "sk")
			},
			path:   "/v1beta/models/m:generateContent",
			non:    `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
			stream: `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newEnv(t)
			up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
				stream := strings.Contains(body, `"stream":true`) || strings.HasSuffix(r.URL.Path, ":streamGenerateContent")
				switch tc.name {
				case "chat":
					w.Header().Set("Content-Type", "text/event-stream")
					if !stream {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`))
						return
					}
					writeSSEEvents(w,
						"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n",
						"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n",
						"data: [DONE]\n\n",
					)
				case "responses":
					if !stream {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"id":"resp","usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4}}`))
						return
					}
					writeSSEEvents(w,
						"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp\"}}\n\n",
						"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp\",\"usage\":{\"input_tokens\":2,\"output_tokens\":2,\"total_tokens\":4}}}\n\n",
					)
				case "anthropic":
					if !stream {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"id":"msg","type":"message","role":"assistant","content":[],"usage":{"input_tokens":2,"output_tokens":2}}`))
						return
					}
					writeSSEEvents(w,
						"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n",
						"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n",
						"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
					)
				case "gemini":
					if !stream {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"candidates":[],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"thoughtsTokenCount":1,"totalTokenCount":4}}`))
						return
					}
					writeSSEEvents(w,
						"data: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":1,\"thoughtsTokenCount\":1,\"totalTokenCount\":4}}\n\n",
					)
				}
			})
			tc.setup(env, up.server.URL)
			token, plain := env.addToken(t, tc.name, nil)
			if err := env.usage.SetDefault(context.Background(), 1_000_000, 2_000_000); err != nil {
				t.Fatalf("set default price: %v", err)
			}
			gateway := env.authGateway(t)

			resp, body := authPost(t, gateway, tc.path, plain, tc.non)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("non-stream: %d %s", resp.StatusCode, body)
			}
			streamPath := tc.path
			if tc.name == "gemini" {
				streamPath = "/v1beta/models/m:streamGenerateContent"
			}
			resp, body = authPost(t, gateway, streamPath, plain, tc.stream)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("stream: %d %s", resp.StatusCode, body)
			}

			env.logWriter.Close()
			logs := logsForToken(t, env, token.ID)
			if len(logs) != 2 {
				t.Fatalf("logs = %d, want 2", len(logs))
			}
			for _, log := range logs {
				if log.PromptTokens != 2 || log.CompletionTokens != 2 ||
					log.CostMicros != 6 || log.CostStatus != usage.CostStatusPriced {
					t.Fatalf("usage/cost = %#v", log)
				}
			}
		})
	}
}
