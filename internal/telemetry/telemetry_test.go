package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFromEnv(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318/")
	t.Setenv("OTEL_SERVICE_NAME", "evergate-test")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "x-api-key=abc,authorization=Bearer%20token")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "x-scope=traces")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "")

	cfg := FromEnv()
	if cfg.Endpoint != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.ServiceName != "evergate-test" {
		t.Fatalf("service name = %q", cfg.ServiceName)
	}
	if cfg.Headers["x-api-key"] != "abc" || cfg.Headers["authorization"] != "Bearer token" || cfg.Headers["x-scope"] != "traces" {
		t.Fatalf("headers = %#v", cfg.Headers)
	}
	if ProtocolUnsupported() {
		t.Fatal("http/json should be supported")
	}

	t.Setenv("OTEL_SDK_DISABLED", "true")
	if FromEnv().Endpoint != "" {
		t.Fatal("OTEL_SDK_DISABLED should disable exporter")
	}
	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "grpc")
	if !ProtocolUnsupported() {
		t.Fatal("grpc without grpc exporter should be reported unsupported")
	}
}

func TestDisabledTracerIsNoop(t *testing.T) {
	tr := New(Config{}, nil)
	if tr.Enabled() {
		t.Fatal("empty endpoint must disable tracer")
	}
	ctx, span := tr.Start(context.Background(), "noop", KindServer, String("a", "b"))
	span.SetAttr(Int("n", 1))
	span.End(nil)
	if ctx == nil {
		t.Fatal("ctx should be preserved")
	}
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

type capturedPayload struct {
	mu      sync.Mutex
	body    []byte
	ctype   string
	headers http.Header
}

func (c *capturedPayload) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.body = body
		c.ctype = r.Header.Get("Content-Type")
		c.headers = r.Header.Clone()
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func TestExportsOTLPJSONWithParentChild(t *testing.T) {
	capture := &capturedPayload{}
	srv := httptest.NewServer(capture.handler())
	defer srv.Close()

	tr := New(Config{
		Endpoint: srv.URL + "/v1/traces", ServiceName: "evergate",
		ServiceVersion: "v9.9.9", Headers: map[string]string{"x-token": "secret-header"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer func() { _ = tr.Shutdown(context.Background()) }()

	ctx, parentHandle := tr.Start(context.Background(), "relay.request", KindServer,
		String("model", "gpt-test"), Int("stream", 1), Float("ratio", 0.5))
	_, childHandle := tr.Start(ctx, "relay.attempt", KindClient, String("channel", "7"))
	childHandle.End(nil)
	parentHandle.End(errors.New("boom: upstream failed"))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tr.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	capture.mu.Lock()
	body := append([]byte(nil), capture.body...)
	ctype := capture.ctype
	headers := capture.headers.Clone()
	capture.mu.Unlock()
	if len(body) == 0 {
		t.Fatal("collector received no payload")
	}
	if !strings.HasPrefix(ctype, "application/json") {
		t.Fatalf("content type = %q", ctype)
	}
	if headers.Get("x-token") != "secret-header" {
		t.Fatalf("export headers not applied: %#v", headers)
	}

	var payload struct {
		ResourceSpans []struct {
			Resource struct {
				Attributes []struct {
					Key   string `json:"key"`
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
				} `json:"attributes"`
			} `json:"resource"`
			ScopeSpans []struct {
				Spans []struct {
					TraceID      string `json:"traceId"`
					SpanID       string `json:"spanId"`
					ParentSpanID string `json:"parentSpanId"`
					Name         string `json:"name"`
					Kind         int    `json:"kind"`
					Status       struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
					} `json:"status"`
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string  `json:"stringValue"`
							IntValue    string  `json:"intValue"`
							DoubleValue float64 `json:"doubleValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v\n%s", err, body)
	}
	if len(payload.ResourceSpans) != 1 || len(payload.ResourceSpans[0].ScopeSpans) != 1 {
		t.Fatalf("unexpected payload shape: %+v", payload)
	}
	spans := payload.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 2 {
		t.Fatalf("span count = %d, want 2", len(spans))
	}
	var parentIdx, childIdx = -1, -1
	for i, span := range spans {
		switch span.Name {
		case "relay.request":
			parentIdx = i
		case "relay.attempt":
			childIdx = i
		}
	}
	if parentIdx < 0 || childIdx < 0 {
		t.Fatalf("missing spans: %+v", spans)
	}
	p, child := spans[parentIdx], spans[childIdx]
	if p.TraceID == "" || len(p.TraceID) != 32 || len(p.SpanID) != 16 {
		t.Fatalf("invalid ids: %+v", p)
	}
	if child.TraceID != p.TraceID || child.ParentSpanID != p.SpanID {
		t.Fatalf("parent/child linkage broken: parent=%+v child=%+v", p, child)
	}
	if p.Status.Code != 2 || !strings.Contains(p.Status.Message, "boom") {
		t.Fatalf("parent status = %+v", p.Status)
	}
	if child.Status.Code != 1 {
		t.Fatalf("child status = %+v", child.Status)
	}
	if p.Kind != KindServer || child.Kind != KindClient {
		t.Fatalf("kinds = %d/%d", p.Kind, child.Kind)
	}
	attrs := make(map[string]string)
	for _, attr := range p.Attributes {
		attrs[attr.Key] = attr.Value.StringValue
	}
	if attrs["model"] != "gpt-test" {
		t.Fatalf("model attr = %q", attrs["model"])
	}
	resourceAttrs := make(map[string]string)
	for _, attr := range payload.ResourceSpans[0].Resource.Attributes {
		resourceAttrs[attr.Key] = attr.Value.StringValue
	}
	if resourceAttrs["service.name"] != "evergate" || resourceAttrs["service.version"] != "v9.9.9" {
		t.Fatalf("resource attrs = %#v", resourceAttrs)
	}
}

func TestExportFailureOnlyWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	endpoint := srv.URL + "/v1/traces"
	srv.Close() // 端口立即关闭，导出必然失败

	var logs bytes.Buffer
	tr := New(Config{Endpoint: endpoint, ServiceName: "evergate"},
		slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	_, span := tr.Start(context.Background(), "relay.request", KindServer)
	span.End(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tr.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown should not fail on export errors: %v", err)
	}
	if !strings.Contains(logs.String(), "otel export failed") {
		t.Fatalf("expected export warning, logs = %s", logs.String())
	}
}

func TestTruncateAndHeaders(t *testing.T) {
	long := strings.Repeat("x", 1000)
	if got := truncate(long, 10); len(got) != 10 {
		t.Fatalf("truncate len = %d", len(got))
	}
	headers := parseHeaders(" a = b , , bad , c=d=e")
	if headers["a"] != "b" || headers["c"] != "d=e" {
		t.Fatalf("headers = %#v", headers)
	}
	if _, ok := headers["bad"]; ok {
		t.Fatalf("invalid header should be skipped: %#v", headers)
	}
}
