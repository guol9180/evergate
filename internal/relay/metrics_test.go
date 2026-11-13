package relay

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"evergate/internal/store"
	"evergate/internal/telemetry"
)

func renderRelayMetrics(t *testing.T, env *testEnv) string {
	t.Helper()
	var sb strings.Builder
	if err := env.opt.Metrics.WriteText(&sb); err != nil {
		t.Fatalf("render metrics: %v", err)
	}
	return sb.String()
}

func TestRelayMetricsStreamingSuccess(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
		w.(http.Flusher).Flush()
	})
	env.addChannel(t, "metrics-ok", up.server.URL, 0, []string{"gateway-model"}, nil, "sk-secret-value")

	resp, body := env.chat(t, `{"model":"gateway-model","stream":true,"messages":[]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var channel store.Channel
	if err := env.db.Order("id asc").First(&channel).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	text := renderRelayMetrics(t, env)
	for _, want := range []string{
		`evergate_requests_total{protocol="chat",status="200"} 1`,
		`evergate_first_token_seconds_count{protocol="chat"} 1`,
		`evergate_upstream_attempts_total{channel="` + itoa64(channel.ID) + `",model="gateway-model",result="success"} 1`,
		"evergate_active_requests 0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in metrics:\n%s", want, text)
		}
	}
	if strings.Contains(text, "sk-secret-value") {
		t.Fatalf("metrics must never contain upstream key plaintext:\n%s", text)
	}
}

func TestRelayMetricsExhaustedRequest(t *testing.T) {
	env := newEnv(t)
	for _, name := range []string{"primary", "secondary"} {
		up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"upstream boom"}}`))
		})
		env.addChannel(t, name, up.server.URL, 0, []string{"gateway-model"}, nil, "sk-"+name)
	}

	resp, body := env.chat(t, `{"model":"gateway-model","stream":false,"messages":[]}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	text := renderRelayMetrics(t, env)
	for _, want := range []string{
		`evergate_requests_total{protocol="chat",status="503"} 1`,
		`evergate_request_errors_total{type="transient"} 1`,
		`evergate_upstream_errors_total{kind="transient"} 2`,
		`result="transient"} 1`,
		"evergate_active_requests 0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in metrics:\n%s", want, text)
		}
	}
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestRelayEmitsRequestAndAttemptSpans(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-span","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	env.addChannel(t, "span", up.server.URL, 0, []string{"gateway-model"}, nil, "sk-span")

	var mu sync.Mutex
	var payload []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		payload = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	tracer := telemetry.New(telemetry.Config{Endpoint: srv.URL + "/v1/traces", ServiceName: "evergate"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	env.opt.Tracer = tracer

	resp, body := env.chat(t, `{"model":"gateway-model","stream":false,"messages":[]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tracer.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown tracer: %v", err)
	}
	mu.Lock()
	exported := string(payload)
	mu.Unlock()
	for _, want := range []string{`"name":"relay.request"`, `"name":"relay.attempt"`, `"evergate.channel_id"`, `"service.name"`} {
		if !strings.Contains(exported, want) {
			t.Fatalf("missing %q in exported spans: %s", want, exported)
		}
	}
	if strings.Contains(exported, "sk-span") {
		t.Fatalf("span payload must not contain key plaintext: %s", exported)
	}
}
