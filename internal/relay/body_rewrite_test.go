package relay

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type rewrittenBody struct {
	Model         string `json:"model"`
	StreamOptions struct {
		IncludeUsage *bool `json:"include_usage"`
	} `json:"stream_options"`
	Messages []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

func decodeRewritten(t *testing.T, body []byte) rewrittenBody {
	t.Helper()
	var out rewrittenBody
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode rewritten body: %v", err)
	}
	return out
}

func TestBodyRewriterCachesPerUpstreamModel(t *testing.T) {
	original := []byte(`{"model":"public","stream":true,"messages":[{"content":"hi"}],"stream_options":{"include_usage":false}}`)
	rw := newBodyRewriter(original)

	first, err := rw.rewrite(original, "upstream-1", true)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	got := decodeRewritten(t, first)
	if got.Model != "upstream-1" || got.StreamOptions.IncludeUsage == nil || !*got.StreamOptions.IncludeUsage {
		t.Fatalf("rewritten body = %+v", got)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "hi" {
		t.Fatalf("payload changed: %+v", got.Messages)
	}
	if !strings.Contains(string(original), `"model":"public"`) {
		t.Fatal("original body must not be mutated")
	}

	second, err := rw.rewrite(original, "upstream-1", true)
	if err != nil || &second[0] != &first[0] {
		t.Fatalf("second rewrite should hit the cache (err=%v)", err)
	}
	other, err := rw.rewrite(original, "upstream-2", true)
	if err != nil || decodeRewritten(t, other).Model != "upstream-2" {
		t.Fatalf("per-model cache miss: %v %s", err, other)
	}

	plain, err := rw.rewrite(original, "upstream-1", false)
	if err != nil {
		t.Fatalf("rewrite without usage: %v", err)
	}
	if usage := decodeRewritten(t, plain).StreamOptions.IncludeUsage; usage == nil || *usage {
		t.Fatalf("injectUsage=false should restore the original stream_options, got %v", usage)
	}
}

func TestBodyRewriterFallsBackForBridgeBodies(t *testing.T) {
	original := []byte(`{"model":"public","messages":[]}`)
	rw := newBodyRewriter(original)
	bridge := []byte(`{"model":"public","messages":[{"content":"bridged"}]}`)

	out, err := rw.rewrite(bridge, "upstream", true)
	if err != nil {
		t.Fatalf("rewrite bridge: %v", err)
	}
	got := decodeRewritten(t, out)
	if got.Model != "upstream" || got.Messages[0].Content != "bridged" {
		t.Fatalf("bridge rewrite = %+v", got)
	}
	if again, _ := rw.rewrite(bridge, "upstream", true); &again[0] == &out[0] {
		t.Fatal("bridge bodies must not be cached across attempts")
	}
}

// 对比每次尝试重建（旧路径）与一次解析 + 缓存命中（新路径）在 1MB / 8MB 请求体下的成本。
func BenchmarkBodyRewrite(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		body := bigChatBody(size)
		b.Run(sizeName(size)+"/legacy", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				if _, err := rewriteUpstreamBody(body, "upstream", true); err != nil {
					b.Fatalf("legacy rewrite: %v", err)
				}
			}
		})
		b.Run(sizeName(size)+"/cached", func(b *testing.B) {
			rw := newBodyRewriter(body)
			if _, err := rw.rewrite(body, "upstream", true); err != nil {
				b.Fatalf("warm cache: %v", err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := rw.rewrite(body, "upstream", true); err != nil {
					b.Fatalf("cached rewrite: %v", err)
				}
			}
		})
		b.Run(sizeName(size)+"/first-attempt", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				rw := newBodyRewriter(body)
				if _, err := rw.rewrite(body, "upstream", true); err != nil {
					b.Fatalf("first attempt: %v", err)
				}
			}
		})
	}
}

func bigChatBody(size int) []byte {
	content := strings.Repeat("x", size)
	return []byte(`{"model":"public","stream":true,"messages":[{"role":"user","content":"` + content + `"}]}`)
}

func sizeName(size int) string {
	return fmt.Sprintf("%dMB", size>>20)
}
