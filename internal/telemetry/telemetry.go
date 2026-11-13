// Package telemetry 提供可选的 OpenTelemetry 链路导出（OTLP/HTTP JSON）。
//
// 默认完全关闭：仅当标准环境变量 OTEL_EXPORTER_OTLP_ENDPOINT 或
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT 存在时才创建导出 goroutine。
// 不引入 OpenTelemetry SDK（会显著增大单二进制体积），本包直接生成
// OTLP/HTTP JSON payload，兼容标准 Collector / 后端接收端（T26 验收 #2/#4）。
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// KindInternal 进程内部 span（网关编排）。
	KindInternal = 1
	// KindServer 服务端入口 span。
	KindServer = 2
	// KindClient 上游调用 span。
	KindClient = 3

	exportTimeout = 5 * time.Second
	flushInterval = 5 * time.Second
	maxBatch      = 256
	maxQueue      = 4096
	maxAttrValue  = 512
)

// Config 是 OTLP 导出配置；Endpoint 为空表示关闭。
type Config struct {
	Endpoint       string // 完整 traces 端点，例如 http://127.0.0.1:4318/v1/traces
	Headers        map[string]string
	ServiceName    string
	ServiceVersion string
}

// FromEnv 读取标准 OpenTelemetry 环境变量。
func FromEnv() Config {
	cfg := Config{ServiceName: "evergate"}
	if truthy(os.Getenv("OTEL_SDK_DISABLED")) {
		return Config{}
	}
	cfg.Endpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	if cfg.Endpoint == "" {
		if base := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); base != "" {
			cfg.Endpoint = strings.TrimSuffix(base, "/") + "/v1/traces"
		}
	}
	if name := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); name != "" {
		cfg.ServiceName = name
	}
	cfg.Headers = parseHeaders(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	for key, value := range parseHeaders(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS")) {
		cfg.Headers[key] = value
	}
	return cfg
}

// ProtocolUnsupported 报告是否显式请求了本包不支持的 OTLP 协议。
// 支持 OTLP/HTTP JSON；显式 grpc 或 http/protobuf 时调用方应关闭导出并告警。
func ProtocolUnsupported() bool {
	for _, key := range []string{"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_EXPORTER_OTLP_PROTOCOL"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "", "http/json":
			continue
		default:
			return true
		}
	}
	return false
}

// Attr 是 span 属性；仅支持标量，避免把请求体或密钥带进导出。
type Attr struct {
	Key   string
	str   string
	num   float64
	isNum bool
}

// String 创建字符串属性。
func String(key, value string) Attr { return Attr{Key: key, str: value} }

// Int 创建整数属性。
func Int(key string, value int64) Attr { return Attr{Key: key, num: float64(value), isNum: true} }

// Float 创建浮点属性。
func Float(key string, value float64) Attr { return Attr{Key: key, num: value, isNum: true} }

type spanKey struct{}

type spanData struct {
	traceID  [16]byte
	spanID   [8]byte
	parentID [8]byte
	name     string
	kind     int
	start    time.Time
	end      time.Time
	attrs    []Attr
	status   int
	message  string
}

// Tracer 是轻量 OTLP 导出器；禁用态所有方法为 no-op，且不创建 goroutine。
type Tracer struct {
	cfg     Config
	log     *slog.Logger
	client  *http.Client
	enabled bool

	ch     chan spanData
	stop   chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
	closed atomic.Bool
	drops  atomic.Uint64
}

// New 创建 Tracer。Endpoint 为空时返回禁用态。
func New(cfg Config, logger *slog.Logger) *Tracer {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "evergate"
	}
	t := &Tracer{cfg: cfg, log: logger, enabled: cfg.Endpoint != ""}
	if !t.enabled {
		return t
	}
	t.client = &http.Client{Timeout: exportTimeout}
	t.ch = make(chan spanData, maxQueue)
	t.stop = make(chan struct{})
	t.wg.Add(1)
	go t.run()
	return t
}

// Enabled 报告导出是否启用。
func (t *Tracer) Enabled() bool { return t != nil && t.enabled }

// Start 创建 span；禁用态仍返回可安全调用的 Span，但不写入 context。
func (t *Tracer) Start(ctx context.Context, name string, kind int, attrs ...Attr) (context.Context, *Span) {
	span := &Span{tracer: t, data: spanData{name: name, kind: kind, start: time.Now(), attrs: attrs}}
	if t == nil || !t.enabled || ctx == nil {
		return ctx, span
	}
	if parent, ok := ctx.Value(spanKey{}).(*Span); ok && parent != nil {
		span.data.traceID = parent.data.traceID
		span.data.parentID = parent.data.spanID
	}
	if span.data.traceID == ([16]byte{}) {
		fillRandom(span.data.traceID[:])
	}
	fillRandom(span.data.spanID[:])
	return context.WithValue(ctx, spanKey{}, span), span
}

// Span 是单个 span 句柄；End 幂等。
type Span struct {
	tracer *Tracer
	data   spanData
	mu     sync.Mutex
	ended  atomic.Bool
}

// SetAttr 在 span 结束前追加属性。
func (s *Span) SetAttr(attr Attr) {
	if s == nil || s.tracer == nil || !s.tracer.enabled {
		return
	}
	s.mu.Lock()
	s.data.attrs = append(s.data.attrs, attr)
	s.mu.Unlock()
}

// End 结束 span 并入队导出；err 非 nil 时标记为错误状态。
func (s *Span) End(err error) {
	if s == nil || s.tracer == nil || !s.tracer.enabled {
		return
	}
	if !s.ended.CompareAndSwap(false, true) {
		return
	}
	s.mu.Lock()
	s.data.end = time.Now()
	if err != nil {
		s.data.status = 2
		s.data.message = truncate(err.Error(), maxAttrValue)
	} else {
		s.data.status = 1
	}
	data := s.data
	s.mu.Unlock()
	s.tracer.enqueue(data)
}

func (t *Tracer) enqueue(data spanData) {
	if t.closed.Load() {
		return
	}
	select {
	case t.ch <- data:
	default:
		if n := t.drops.Add(1); n%100 == 1 {
			t.log.Warn("otel span queue full, dropping spans", slog.Uint64("dropped", n))
		}
	}
}

func (t *Tracer) run() {
	defer t.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	batch := make([]spanData, 0, maxBatch)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		t.export(batch)
		batch = make([]spanData, 0, maxBatch)
	}
	for {
		select {
		case span := <-t.ch:
			batch = append(batch, span)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-t.stop:
			for {
				select {
				case span := <-t.ch:
					batch = append(batch, span)
					if len(batch) >= maxBatch {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// Shutdown 停止接收新 span，排空队列并等待导出 goroutine 退出。
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t == nil || !t.enabled {
		return nil
	}
	t.closed.Store(true)
	t.once.Do(func() { close(t.stop) })
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ---------- OTLP/HTTP JSON ----------

type otlpPayload struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource    `json:"resource"`
	ScopeSpans []otlpScopeSpan `json:"scopeSpans"`
}

type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes"`
}

type otlpScopeSpan struct {
	Scope otlpScope  `json:"scope"`
	Spans []otlpSpan `json:"spans"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type otlpSpan struct {
	TraceID           string         `json:"traceId"`
	SpanID            string         `json:"spanId"`
	ParentSpanID      string         `json:"parentSpanId,omitempty"`
	Name              string         `json:"name"`
	Kind              int            `json:"kind"`
	StartTimeUnixNano string         `json:"startTimeUnixNano"`
	EndTimeUnixNano   string         `json:"endTimeUnixNano"`
	Attributes        []otlpKeyValue `json:"attributes,omitempty"`
	Status            otlpSpanStatus `json:"status"`
}

type otlpSpanStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type otlpKeyValue struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
}

func (t *Tracer) export(batch []spanData) {
	payload := t.payload(batch)
	body, err := json.Marshal(payload)
	if err != nil {
		t.log.Warn("otel marshal payload failed", slog.Any("error", err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		t.log.Warn("otel build request failed", slog.Any("error", err), slog.String("endpoint", t.cfg.Endpoint))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range t.cfg.Headers {
		req.Header.Set(key, value)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		// 验收 #4：导出失败只告警，不影响请求成功。
		t.log.Warn("otel export failed", slog.Any("error", err), slog.String("endpoint", t.cfg.Endpoint))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.log.Warn("otel exporter rejected payload", slog.Int("status", resp.StatusCode), slog.String("endpoint", t.cfg.Endpoint))
	}
}

func (t *Tracer) payload(batch []spanData) otlpPayload {
	spans := make([]otlpSpan, 0, len(batch))
	for _, data := range batch {
		span := otlpSpan{
			TraceID:      hex.EncodeToString(data.traceID[:]),
			SpanID:       hex.EncodeToString(data.spanID[:]),
			ParentSpanID: hex.EncodeToString(data.parentID[:]),
			Name:         data.name, Kind: data.kind,
			StartTimeUnixNano: nano(data.start),
			EndTimeUnixNano:   nano(data.end),
			Status:            otlpSpanStatus{Code: data.status, Message: data.message},
		}
		if data.parentID == ([8]byte{}) {
			span.ParentSpanID = ""
		}
		for _, attr := range data.attrs {
			if attr.Key == "" {
				continue
			}
			kv := otlpKeyValue{Key: attr.Key}
			if attr.isNum {
				v := truncate(formatFloat(attr.num), maxAttrValue)
				if attr.num == float64(int64(attr.num)) {
					kv.Value.IntValue = &v
				} else {
					kv.Value.DoubleValue = &attr.num
				}
			} else {
				v := truncate(attr.str, maxAttrValue)
				kv.Value.StringValue = &v
			}
			span.Attributes = append(span.Attributes, kv)
		}
		spans = append(spans, span)
	}
	return otlpPayload{ResourceSpans: []otlpResourceSpans{{
		Resource: otlpResource{Attributes: []otlpKeyValue{
			stringKV("service.name", t.cfg.ServiceName),
			stringKV("service.version", t.cfg.ServiceVersion),
			stringKV("telemetry.sdk.name", "evergate-otlp-http-json"),
			stringKV("telemetry.sdk.language", "go"),
		}},
		ScopeSpans: []otlpScopeSpan{{
			Scope: otlpScope{Name: "evergate", Version: t.cfg.ServiceVersion},
			Spans: spans,
		}},
	}}}
}

func stringKV(key, value string) otlpKeyValue {
	v := truncate(value, maxAttrValue)
	return otlpKeyValue{Key: key, Value: otlpValue{StringValue: &v}}
}

func nano(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

func formatFloat(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func fillRandom(b []byte) {
	if _, err := rand.Read(b); err != nil {
		// 退化路径（测试或熵源异常）：仍保证非零 ID，避免空 trace/span id。
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (8 * (i % 8)))
		}
	}
}

func truncate(v string, max int) string {
	if len(v) <= max {
		return v
	}
	return v[:max]
}

func parseHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return headers
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// StartSpan 是便捷入口：无 Tracer 时返回原始 ctx。
func StartSpan(ctx context.Context, tracer *Tracer, name string, kind int, attrs ...Attr) (context.Context, *Span) {
	if tracer == nil {
		return ctx, &Span{}
	}
	return tracer.Start(ctx, name, kind, attrs...)
}
