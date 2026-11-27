package relay

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"evergate/internal/store"
)

const goodChatChunk = "data: {\"id\":\"chatcmpl-good\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
	"data: {\"id\":\"chatcmpl-good\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n" +
	"data: [DONE]\n\n"

// shrink 把上游读取上限调小，测试不必真的构造 MB 级响应；测试结束自动恢复。
func shrink[T int | int64](t *testing.T, get func() T, set func(T), value T) {
	t.Helper()
	original := get()
	set(value)
	t.Cleanup(func() { set(original) })
}

func TestUpstreamReadLimits(t *testing.T) {
	t.Run("line reader", func(t *testing.T) {
		shrink(t, func() int { return maxSSELineBytes }, func(v int) { maxSSELineBytes = v }, 64)

		under := newSSELineReader(strings.NewReader(strings.Repeat("a", 60) + "\nnext\n"))
		line, err := under.readLine()
		if err != nil || len(line) != 61 {
			t.Fatalf("line within limit: len=%d err=%v", len(line), err)
		}
		line, err = under.readLine()
		if err != nil || string(line) != "next\n" {
			t.Fatalf("second line: %q err=%v", line, err)
		}

		over := newSSELineReader(strings.NewReader(strings.Repeat("a", 65) + "\n"))
		if _, err := over.readLine(); !errors.Is(err, errSSELineTooLarge) {
			t.Fatalf("line over limit should fail: %v", err)
		}

		// EOF 带尾部字节：与 bufio.ReadBytes 语义一致
		eof := newSSELineReader(strings.NewReader("tail"))
		line, err = eof.readLine()
		if string(line) != "tail" || !errors.Is(err, io.EOF) {
			t.Fatalf("eof tail: %q err=%v", line, err)
		}
	})

	t.Run("event and preamble", func(t *testing.T) {
		shrink(t, func() int { return maxSSEEventBytes }, func(v int) { maxSSEEventBytes = v }, 48)
		shrink(t, func() int { return maxSSEPreambleBytes }, func(v int) { maxSSEPreambleBytes = v }, 48)
		// 多行都没超单行上限，但事件累计超过事件上限
		events := newSSELineReader(strings.NewReader("event: a\n" + strings.Repeat("x", 40) + "\n\n"))
		if _, err := readSSEEvent(events); !errors.Is(err, errSSEEventTooLarge) {
			t.Fatalf("event over limit should fail: %v", err)
		}
		// 首个 data 之前的前导累计超限
		preamble := newSSELineReader(strings.NewReader(strings.Repeat(": ping\n", 20) + "data: {}\n\n"))
		if _, _, err := readFirstSSEDataLine(preamble); !errors.Is(err, errSSEPreambleTooLarge) {
			t.Fatalf("preamble over limit should fail: %v", err)
		}
		// 正常前导 + data 行照常返回
		ok := newSSELineReader(strings.NewReader(": ping\n\ndata: {\"a\":1}\n\n"))
		raw, data, err := readFirstSSEDataLine(ok)
		if err != nil || !strings.Contains(string(data), `"a":1`) || !strings.Contains(string(raw), ": ping") {
			t.Fatalf("preamble within limit: raw=%q data=%q err=%v", raw, data, err)
		}
	})

	t.Run("non-stream body", func(t *testing.T) {
		shrink(t, func() int64 { return maxNonStreamBodyBytes }, func(v int64) { maxNonStreamBodyBytes = v }, 32)
		if _, err := readUpstreamBody(strings.NewReader(strings.Repeat("a", 33))); !errors.Is(err, errUpstreamBodyTooLarge) {
			t.Fatalf("body over limit should fail: %v", err)
		}
		data, err := readUpstreamBody(strings.NewReader(strings.Repeat("a", 32)))
		if err != nil || len(data) != 32 {
			t.Fatalf("body within limit: len=%d err=%v", len(data), err)
		}
	})
}

// 非流式响应体超限：按上游失败处理，换渠道后客户端拿到正常结果，trace 记录超限。
func TestNonStreamBodyOverLimitFailsOver(t *testing.T) {
	shrink(t, func() int64 { return maxNonStreamBodyBytes }, func(v int64) { maxNonStreamBodyBytes = v }, 1024)
	env := newEnv(t)
	huge := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chatcmpl-huge","choices":[{"index":0,"message":{"role":"assistant","content":"` + strings.Repeat("x", 2048) + `"}}]}`))
	})
	healthy := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chatcmpl-good","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	env.addChannel(t, "huge", huge.server.URL, 0, []string{"m"}, nil, "sk-a")
	env.addChannel(t, "healthy", healthy.server.URL, 1, []string{"m"}, nil, "sk-b")

	resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"content":"ok"`) {
		t.Fatalf("failover response: %d %s", resp.StatusCode, body)
	}
	if huge.hits.Load() != 1 || healthy.hits.Load() != 1 {
		t.Fatalf("hits = %d/%d", huge.hits.Load(), healthy.hits.Load())
	}
	logs := logsForModel(t, env)
	if len(logs) != 1 || len(logs[0].RetryTrace) != 2 || logs[0].RetryTrace[0].ErrorType != "transient" ||
		!strings.Contains(logs[0].RetryTrace[0].ErrorMessage, "exceeds") {
		t.Fatalf("trace should record the oversized body then the failover: %#v", logs)
	}
}

// 流式首 chunk 前：单行超限或前导超限都按上游失败换渠道。
func TestSSEOverLimitBeforeFirstChunkFailsOver(t *testing.T) {
	shrink(t, func() int { return maxSSELineBytes }, func(v int) { maxSSELineBytes = v }, 1024)
	shrink(t, func() int { return maxSSEPreambleBytes }, func(v int) { maxSSEPreambleBytes = v }, 1024)
	cases := map[string]func(w http.ResponseWriter){
		"line": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: " + strings.Repeat("x", 4096) + "\n\n"))
		},
		"preamble": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(strings.Repeat(": ping\n", 200) + goodChatChunk))
		},
	}
	for name, respond := range cases {
		t.Run(name, func(t *testing.T) {
			env := newEnv(t)
			bad := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) { respond(w) })
			healthy := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(goodChatChunk))
			})
			env.addChannel(t, "bad", bad.server.URL, 0, []string{"m"}, nil, "sk-a")
			env.addChannel(t, "healthy", healthy.server.URL, 1, []string{"m"}, nil, "sk-b")

			resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`)
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"content":"ok"`) {
				t.Fatalf("failover response: %d %s", resp.StatusCode, body)
			}
			logs := logsForModel(t, env)
			if len(logs) != 1 || len(logs[0].RetryTrace) != 2 || logs[0].RetryTrace[0].ErrorType != "transient" {
				t.Fatalf("trace should record the oversized upstream then the failover: %#v", logs)
			}
		})
	}
}

// 首 chunk 之后的超长行：SSE 红线，不重试，以错误事件明确告知客户端。
func TestSSELineOverLimitAfterFirstChunkReportsErrorWithoutRetry(t *testing.T) {
	shrink(t, func() int { return maxSSELineBytes }, func(v int) { maxSSELineBytes = v }, 1024)
	env := newEnv(t)
	first := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte(goodChatChunk))
		flusher.Flush()
		_, _ = w.Write([]byte("data: " + strings.Repeat("x", 4096) + "\n\n"))
	})
	second := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not retry after first chunk")
	})
	env.addChannel(t, "first", first.server.URL, 0, []string{"m"}, nil, "sk-a")
	env.addChannel(t, "second", second.server.URL, 1, []string{"m"}, nil, "sk-b")

	resp, body := env.chat(t, `{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"content":"ok"`) || !strings.Contains(body, "upstream_stream_error") {
		t.Fatalf("client should get the first chunk then an explicit SSE error: %s", body)
	}
	if strings.Contains(body, strings.Repeat("x", 2048)) {
		t.Fatalf("oversized line must not be forwarded to the client")
	}
	if second.hits.Load() != 0 {
		t.Fatalf("second channel must not be hit after first chunk, hits = %d", second.hits.Load())
	}
}

// 上游 3xx 原样透传：不跟随，不把凭据头带到重定向目标。
func TestUpstreamRedirectPassesThroughWithoutFollowing(t *testing.T) {
	env := newEnv(t)
	target := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		t.Errorf("redirect target must not be requested, got Authorization %q", auth)
	})
	redirector := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.server.URL+"/v1/chat/completions", http.StatusFound)
	})
	env.addChannel(t, "redirector", redirector.server.URL, 0, []string{"m"}, nil, "sk-a")

	// 网关只负责把 3xx 交回；测试客户端自己也得不跟随，才能断言看到的就是网关的响应
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Post(env.gateway.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	resp, body := res, string(raw)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Location"); got != target.server.URL+"/v1/chat/completions" {
		t.Fatalf("Location = %q", got)
	}
	if target.hits.Load() != 0 {
		t.Fatalf("redirect target was requested %d times", target.hits.Load())
	}
	logs := logsForModel(t, env)
	if len(logs) != 1 || len(logs[0].RetryTrace) != 1 || logs[0].RetryTrace[0].StatusCode != http.StatusFound {
		t.Fatalf("trace should record the 302 passthrough: %#v", logs)
	}
}

func logsForModel(t *testing.T, env *testEnv) []store.RequestLog {
	t.Helper()
	env.logWriter.Close()
	var logs []store.RequestLog
	if err := env.db.Order("id asc").Find(&logs).Error; err != nil {
		t.Fatalf("read logs: %v", err)
	}
	return logs
}
