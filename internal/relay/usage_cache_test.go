package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"evergate/internal/store"
	"evergate/internal/usage"
)

func ptrOf(v int64) *int64 { return &v }

// 表驱动：各协议 usage 的输入侧拆分口径。缓存读 / 缓存写必须进可计费输入，
// 且已含于输入计数的协议（OpenAI / Responses / DeepSeek）不得重复累加。
func TestUsageNormalizationSplitsBillableInput(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want normalizedUsage
	}{
		{
			name: "openai chat cached is subset of prompt",
			wire: `{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":80}}`,
			want: normalizedUsage{Prompt: 100, CacheRead: 80, Completion: 10, Total: 110},
		},
		{
			name: "openai responses cached is subset of input",
			wire: `{"input_tokens":11,"output_tokens":7,"total_tokens":18,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":4}}`,
			want: normalizedUsage{Prompt: 11, CacheRead: 3, Completion: 7, Total: 18, Reasoning: 4},
		},
		{
			name: "deepseek cache hit is subset of prompt",
			wire: `{"prompt_tokens":1000,"completion_tokens":20,"total_tokens":1020,"prompt_cache_hit_tokens":900}`,
			want: normalizedUsage{Prompt: 1000, CacheRead: 900, Completion: 20, Total: 1020},
		},
		{
			name: "anthropic cache read and write are separate from input",
			wire: `{"input_tokens":3,"cache_creation_input_tokens":20,"cache_read_input_tokens":200,"output_tokens":7}`,
			want: normalizedUsage{Prompt: 223, CacheRead: 200, CacheWrite: 20, Completion: 7, Total: 230},
		},
		{
			name: "anthropic output-only frame leaves input to the merge",
			wire: `{"output_tokens":12}`,
			want: normalizedUsage{Completion: 12, Total: 12},
		},
		{
			name: "inconsistent cached above prompt is clamped",
			wire: `{"prompt_tokens":10,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":50}}`,
			want: normalizedUsage{Prompt: 10, CacheRead: 10, Completion: 1, Total: 11},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var wire usageWire
			if err := json.Unmarshal([]byte(tc.wire), &wire); err != nil {
				t.Fatalf("unmarshal usage: %v", err)
			}
			if got := wire.normalize().withTotal(); got != tc.want {
				t.Fatalf("normalize() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// 桥接回写给 Anthropic 客户端的 usage 必须遵循 Anthropic 口径：input_tokens 不含缓存读。
func TestMessagesBridgeUsageExcludesCacheReadFromInput(t *testing.T) {
	data := []byte(`{"id":"chatcmpl-x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_tokens_details":{"cached_tokens":80}}}`)
	out, err := convertChatCompletionToMessages(data, "m", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	var resp struct {
		Usage map[string]int64 `json:"usage"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Usage["input_tokens"] != 20 || resp.Usage["cache_read_input_tokens"] != 80 || resp.Usage["output_tokens"] != 5 {
		t.Fatalf("bridged usage = %#v, want input 20 / cache read 80 / output 5", resp.Usage)
	}
}

// 集成：缓存 token 进入成本与令牌额度；缓存价未设置时按输入价全价计（宁可高估）。
func TestCacheTokensBilledAndChargedAcrossProtocols(t *testing.T) {
	t.Run("anthropic native bills cache read and write", func(t *testing.T) {
		env := newEnv(t)
		up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
			if strings.Contains(body, `"stream":true`) {
				w.Header().Set("Content-Type", "text/event-stream")
				writeSSEEvents(w,
					"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":3,\"cache_creation_input_tokens\":20,\"cache_read_input_tokens\":200,\"output_tokens\":1}}}\n\n",
					"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n",
					"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
				)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msg","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],` +
				`"usage":{"input_tokens":3,"cache_creation_input_tokens":20,"cache_read_input_tokens":200,"output_tokens":7}}`))
		})
		env.addAnthropicChannel(t, "anthropic-cache", up.server.URL, 0, []string{"m"}, nil, "sk")
		token, plain := env.addToken(t, "anthropic-cache", func(tok *store.Token) { tok.Quota = 10_000 })
		if err := env.usage.SetDefault(context.Background(), usage.PriceInput{
			Input: 1_000_000, Output: 2_000_000, CacheRead: ptrOf(100_000), CacheWrite: ptrOf(1_250_000),
		}); err != nil {
			t.Fatalf("set default price: %v", err)
		}
		gateway := env.authGateway(t)
		for _, stream := range []bool{false, true} {
			body := fmt.Sprintf(`{"model":"m","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
			resp, respBody := authPost(t, gateway, "/v1/messages", plain, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("stream=%v: %d %s", stream, resp.StatusCode, respBody)
			}
		}
		env.logWriter.Close()
		logs := logsForToken(t, env, token.ID)
		if len(logs) != 2 {
			t.Fatalf("logs = %d, want 2", len(logs))
		}
		for _, log := range logs {
			// 普通 3×$1 + 缓存读 200×$0.1 + 缓存写 20×$1.25 + 输出 7×$2 = 62 微美元
			if log.PromptTokens != 223 || log.CachedTokens != 200 || log.CacheWriteTokens != 20 || log.CompletionTokens != 7 ||
				log.CostMicros != 62 || log.CostStatus != usage.CostStatusPriced {
				t.Fatalf("anthropic cache usage/cost = %#v", log)
			}
		}
		var stored store.Token
		if err := env.db.First(&stored, token.ID).Error; err != nil {
			t.Fatalf("read token: %v", err)
		}
		// 额度按输入 + 输出计且输入侧含缓存读写：每次 223 + 7 = 230
		if stored.UsedQuota != 460 {
			t.Fatalf("used quota = %d, want 460", stored.UsedQuota)
		}
	})

	t.Run("openai cached falls back to full input price until a cache price is set", func(t *testing.T) {
		env := newEnv(t)
		up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],` +
				`"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":80}}}`))
		})
		env.addChannel(t, "openai-cache", up.server.URL, 0, []string{"m"}, nil, "sk")
		token, plain := env.addToken(t, "openai-cache", func(tok *store.Token) { tok.Quota = 10_000 })
		if err := env.usage.SetDefault(context.Background(), usage.PriceInput{Input: 1_000_000, Output: 2_000_000}); err != nil {
			t.Fatalf("set default price: %v", err)
		}
		gateway := env.authGateway(t)
		const body = `{"model":"m","messages":[{"role":"user","content":"hi"}]}`
		if resp, respBody := authPost(t, gateway, "/v1/chat/completions", plain, body); resp.StatusCode != http.StatusOK {
			t.Fatalf("first request: %d %s", resp.StatusCode, respBody)
		}
		if _, err := env.usage.UpsertOverride(context.Background(), 1, "m", usage.PriceInput{
			Input: 1_000_000, Output: 2_000_000, CacheRead: ptrOf(100_000),
		}); err != nil {
			t.Fatalf("set cache price: %v", err)
		}
		if resp, respBody := authPost(t, gateway, "/v1/chat/completions", plain, body); resp.StatusCode != http.StatusOK {
			t.Fatalf("second request: %d %s", resp.StatusCode, respBody)
		}
		env.logWriter.Close()
		logs := logsForToken(t, env, token.ID)
		if len(logs) != 2 {
			t.Fatalf("logs = %d, want 2", len(logs))
		}
		// 未设缓存价：100×$1 + 10×$2 = 120；设后：普通 20×$1 + 缓存读 80×$0.1 + 输出 10×$2 = 48
		if logs[0].CostMicros != 120 || logs[1].CostMicros != 48 || logs[1].PriceSource != usage.PriceSourceOverride {
			t.Fatalf("cost progression = %d → %d (%s), want 120 → 48", logs[0].CostMicros, logs[1].CostMicros, logs[1].PriceSource)
		}
		for _, log := range logs {
			if log.PromptTokens != 100 || log.CachedTokens != 80 || log.CacheWriteTokens != 0 || log.CompletionTokens != 10 {
				t.Fatalf("openai cached usage = %#v", log)
			}
		}
		var stored store.Token
		if err := env.db.First(&stored, token.ID).Error; err != nil {
			t.Fatalf("read token: %v", err)
		}
		if stored.UsedQuota != 220 {
			t.Fatalf("used quota = %d, want 220", stored.UsedQuota)
		}
	})

	t.Run("catalog supplies cache prices for preset channels", func(t *testing.T) {
		env := newEnv(t)
		up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],` +
				`"usage":{"prompt_tokens":1000000,"completion_tokens":1000,"total_tokens":1001000,"prompt_tokens_details":{"cached_tokens":900000}}}`))
		})
		env.addChannel(t, "sonnet", up.server.URL, 0, []string{"claude-sonnet-4-5"}, nil, "sk")
		if err := env.db.Model(&store.Channel{}).Where("id = ?", 1).Update("preset_id", "anthropic").Error; err != nil {
			t.Fatalf("set preset: %v", err)
		}
		if err := env.usage.Reload(); err != nil {
			t.Fatalf("reload prices: %v", err)
		}
		if resp, respBody := env.chat(t, `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
			t.Fatalf("request: %d %s", resp.StatusCode, respBody)
		}
		env.logWriter.Close()
		var log store.RequestLog
		if err := env.db.First(&log).Error; err != nil {
			t.Fatalf("read log: %v", err)
		}
		// 目录价：普通 10 万×$3 + 缓存读 90 万×$0.3 + 输出 1000×$15 = 585000 微美元
		if log.CostMicros != 585_000 || log.PriceSource != usage.PriceSourceCatalog || log.CostStatus != usage.CostStatusPriced {
			t.Fatalf("catalog cache cost = %#v, want 585000 catalog", log)
		}
	})
}
