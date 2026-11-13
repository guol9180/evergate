// Package metrics 提供进程内 Prometheus 指标注册表与 /metrics 文本曝光。
//
// 不引入第三方 Prometheus 客户端：单二进制体积是项目硬约束（<30MB），
// 本包实现的指标类型（counter/gauge/fixed-bucket histogram）已覆盖网关需求，
// 文本协议稳定且可测试。新增指标必须保持低基数标签，禁止把 key、令牌、
// 请求体或用户可控长文本放入标签。
package metrics

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// maxSeriesPerFamily 单指标族最大时间序列数，防止标签组合爆炸（T26 验收 #3）。
	maxSeriesPerFamily = 1024
	// maxLabelValues 单个标签维度允许出现过的不同取值数，超出折叠为 other。
	maxLabelValues  = 128
	otherLabelValue = "other"
)

type metricType string

const (
	typeCounter   metricType = "counter"
	typeGauge     metricType = "gauge"
	typeHistogram metricType = "histogram"
)

// DefaultFirstTokenBuckets 首 token 延迟直方图桶（秒），覆盖本地百毫秒到慢渠道慢响应。
var DefaultFirstTokenBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60}

type series struct {
	labels []string
	value  float64
	counts []uint64 // 直方图累计桶计数
	sum    float64
	count  uint64
}

type family struct {
	registry   *Registry
	name       string
	help       string
	typ        metricType
	labelNames []string
	buckets    []float64
	series     map[string]*series
	order      []string
}

// Sample 是抓取时动态采集的 gauge 样本（如渠道状态、key 状态）。
type Sample struct {
	Name   string
	Help   string
	Labels map[string]string
	Value  float64
}

// Collector 在每次 /metrics 抓取时执行；不得阻塞过久或产生外部网络请求。
type Collector func() []Sample

// Registry 是指标注册表，同时实现 http.Handler（仅允许 loopback 访问）。
type Registry struct {
	mu         sync.Mutex
	families   map[string]*family
	collectors []Collector

	limMu  sync.Mutex
	limits map[string]map[string]struct{}

	seriesLimit  int
	dropped      atomic.Uint64
	droppedGauge *Gauge

	// 预置指标：relay 直接引用，避免字符串字面量散落。
	Requests         *Counter
	RequestErrors    *Counter
	ActiveRequests   *Gauge
	UpstreamAttempts *Counter
	UpstreamErrors   *Counter
	FirstToken       *Histogram
}

// New 创建注册表并注册网关预置指标。
func New() *Registry {
	r := &Registry{
		families:    make(map[string]*family),
		limits:      make(map[string]map[string]struct{}),
		seriesLimit: maxSeriesPerFamily,
	}
	r.Requests = r.Counter("evergate_requests_total",
		"网关完成的请求数，按入口协议与最终 HTTP 状态码统计", "protocol", "status")
	r.RequestErrors = r.Counter("evergate_request_errors_total",
		"网关请求终态错误数，按错误类型统计", "type")
	r.ActiveRequests = r.Gauge("evergate_active_requests",
		"当前正在处理的网关请求数")
	r.UpstreamAttempts = r.Counter("evergate_upstream_attempts_total",
		"上游尝试次数，按渠道、上游模型与结果统计", "channel", "model", "result")
	r.UpstreamErrors = r.Counter("evergate_upstream_errors_total",
		"上游尝试失败次数，按错误分类统计", "kind")
	r.FirstToken = r.Histogram("evergate_first_token_seconds",
		"上游首 token 延迟（秒），从发起上游请求到首条 SSE data 或非流式响应首字节",
		DefaultFirstTokenBuckets, "protocol")
	r.droppedGauge = r.Gauge("evergate_metrics_series_dropped_total",
		"因标签基数上限被丢弃的时间序列更新次数")
	return r
}

// Counter 注册/返回一个 counter。
func (r *Registry) Counter(name, help string, labels ...string) *Counter {
	return &Counter{fam: r.family(name, help, typeCounter, nil, labels)}
}

// Gauge 注册/返回一个 gauge。
func (r *Registry) Gauge(name, help string, labels ...string) *Gauge {
	return &Gauge{fam: r.family(name, help, typeGauge, nil, labels)}
}

// Histogram 注册/返回一个固定桶直方图。
func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *Histogram {
	if len(buckets) == 0 {
		buckets = DefaultFirstTokenBuckets
	}
	return &Histogram{fam: r.family(name, help, typeHistogram, buckets, labels)}
}

// AddCollector 注册抓取时采集器。
func (r *Registry) AddCollector(c Collector) {
	if c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors = append(r.collectors, c)
}

// LowCardinality 限制单个标签维度的取值数量；超限折叠为 other。
// 用于渠道 ID、模型名等可能随业务增长的标签。
func (r *Registry) LowCardinality(kind, value string) string {
	if value == "" {
		value = "unknown"
	}
	if strings.ContainsAny(value, "\n\r\"\\") || len(value) > 64 {
		return otherLabelValue
	}
	r.limMu.Lock()
	defer r.limMu.Unlock()
	set := r.limits[kind]
	if set == nil {
		set = make(map[string]struct{})
		r.limits[kind] = set
	}
	if _, ok := set[value]; ok {
		return value
	}
	if len(set) >= maxLabelValues {
		return otherLabelValue
	}
	set[value] = struct{}{}
	return value
}

// Dropped 返回被丢弃的序列更新次数（测试与自诊断用）。
func (r *Registry) Dropped() uint64 { return r.dropped.Load() }

func (r *Registry) family(name, help string, typ metricType, buckets []float64, labels []string) *family {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.families[name]; ok {
		return f
	}
	f := &family{
		registry: r, name: name, help: help, typ: typ,
		labelNames: append([]string(nil), labels...),
		buckets:    buckets,
		series:     make(map[string]*series),
	}
	r.families[name] = f
	return f
}

func (r *Registry) lookup(f *family, values []string) *series {
	if len(values) != len(f.labelNames) {
		r.dropped.Add(1)
		return nil
	}
	key := strings.Join(values, "\x1f")
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := f.series[key]; ok {
		return s
	}
	if len(f.series) >= r.seriesLimit {
		r.dropped.Add(1)
		return nil
	}
	s := &series{labels: append([]string(nil), values...)}
	if f.typ == typeHistogram {
		s.counts = make([]uint64, len(f.buckets))
	}
	f.series[key] = s
	f.order = append(f.order, key)
	return s
}

// Counter 单调递增计数器。
type Counter struct{ fam *family }

// Inc 计数 +1。
func (c *Counter) Inc(values ...string) { c.Add(1, values...) }

// Add 计数 +v。
func (c *Counter) Add(v float64, values ...string) {
	s := c.registry().lookup(c.fam, values)
	if s == nil {
		return
	}
	c.registry().mu.Lock()
	s.value += v
	c.registry().mu.Unlock()
}

func (c *Counter) registry() *Registry { return c.fam.registry }

// Gauge 可增可减的瞬时值。
type Gauge struct{ fam *family }

// Set 设置当前值。
func (g *Gauge) Set(v float64, values ...string) {
	s := g.registry().lookup(g.fam, values)
	if s == nil {
		return
	}
	g.registry().mu.Lock()
	s.value = v
	g.registry().mu.Unlock()
}

// Add 当前值 +v。
func (g *Gauge) Add(v float64, values ...string) {
	s := g.registry().lookup(g.fam, values)
	if s == nil {
		return
	}
	g.registry().mu.Lock()
	s.value += v
	g.registry().mu.Unlock()
}

func (g *Gauge) registry() *Registry { return g.fam.registry }

// Histogram 固定桶直方图。
type Histogram struct{ fam *family }

// Observe 记录一次观测值（秒）。
func (h *Histogram) Observe(v float64, values ...string) {
	s := h.registry().lookup(h.fam, values)
	if s == nil {
		return
	}
	reg := h.registry()
	reg.mu.Lock()
	defer reg.mu.Unlock()
	s.count++
	s.sum += v
	for i, upper := range h.fam.buckets {
		if v <= upper {
			s.counts[i]++
		}
	}
}

func (h *Histogram) registry() *Registry { return h.fam.registry }

// ServeHTTP 输出 Prometheus 文本格式；非本机直连访问一律 403（T26 验收 #1）。
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !IsLoopbackAddr(req.RemoteAddr) {
		http.Error(w, "metrics endpoint is restricted to loopback clients", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = r.WriteText(w)
}

// IsLoopbackAddr 判断 RemoteAddr 是否为 loopback（不信任 X-Forwarded-For）。
func IsLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// WriteText 渲染全部指标。采集器在锁外执行，避免 DB 查询阻塞指标写入。
func (r *Registry) WriteText(w io.Writer) error {
	type rendered struct {
		help  string
		typ   metricType
		lines []string
	}
	out := make(map[string]*rendered)

	r.mu.Lock()
	for name, f := range r.families {
		if name == r.droppedGauge.fam.name {
			continue // 动态值在锁外读取
		}
		rd := &rendered{help: f.help, typ: f.typ}
		keys := append([]string(nil), f.order...)
		sort.Strings(keys)
		for _, key := range keys {
			s := f.series[key]
			switch f.typ {
			case typeHistogram:
				rd.lines = append(rd.lines, renderHistogram(f, s)...)
			default:
				rd.lines = append(rd.lines, renderSample(f.name, f.labelNames, s.labels, s.value))
			}
		}
		out[name] = rd
	}
	collectors := append([]Collector(nil), r.collectors...)
	r.mu.Unlock()

	r.droppedGauge.Set(float64(r.dropped.Load()))
	r.mu.Lock()
	if f := r.droppedGauge.fam; f != nil {
		rd := out[f.name]
		if rd == nil {
			rd = &rendered{help: f.help, typ: f.typ}
			out[f.name] = rd
		}
		keys := append([]string(nil), f.order...)
		sort.Strings(keys)
		for _, key := range keys {
			s := f.series[key]
			rd.lines = append(rd.lines, renderSample(f.name, f.labelNames, s.labels, s.value))
		}
	}
	r.mu.Unlock()

	for _, collect := range collectors {
		for _, sample := range collect() {
			if sample.Name == "" {
				continue
			}
			rd := out[sample.Name]
			if rd == nil {
				rd = &rendered{help: sample.Help, typ: typeGauge}
				out[sample.Name] = rd
			}
			keys := make([]string, 0, len(sample.Labels))
			for k := range sample.Labels {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			labels := make([]string, 0, len(keys))
			for _, k := range keys {
				labels = append(labels, k+"="+quoteLabel(sample.Labels[k]))
			}
			line := sample.Name
			if len(labels) > 0 {
				line += "{" + strings.Join(labels, ",") + "}"
			}
			rd.lines = append(rd.lines, line+" "+formatFloat(sample.Value))
		}
	}

	names := make([]string, 0, len(out))
	for name := range out {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rd := out[name]
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, escapeHelp(rd.help), name, rd.typ); err != nil {
			return err
		}
		for _, line := range rd.lines {
			if _, err := io.WriteString(w, line+"\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderHistogram(f *family, s *series) []string {
	leNames := append(append([]string(nil), f.labelNames...), "le")
	lines := make([]string, 0, len(f.buckets)+2)
	for i, upper := range f.buckets {
		labels := append(append([]string(nil), s.labels...), formatFloat(upper))
		lines = append(lines, renderSample(f.name+"_bucket", leNames, labels, float64(s.counts[i])))
	}
	infLabels := append(append([]string(nil), s.labels...), "+Inf")
	lines = append(lines, renderSample(f.name+"_bucket", leNames, infLabels, float64(s.count)))
	lines = append(lines, renderSample(f.name+"_sum", f.labelNames, s.labels, s.sum))
	lines = append(lines, renderSample(f.name+"_count", f.labelNames, s.labels, float64(s.count)))
	return lines
}

// renderSample 按标签声明顺序渲染；labelNames 与 values 等长。
func renderSample(name string, labelNames, values []string, v float64) string {
	var sb strings.Builder
	sb.WriteString(name)
	if len(labelNames) > 0 {
		sb.WriteByte('{')
		for i, label := range labelNames {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(label)
			sb.WriteByte('=')
			sb.WriteString(quoteLabel(values[i]))
		}
		sb.WriteByte('}')
	}
	sb.WriteByte(' ')
	sb.WriteString(formatFloat(v))
	return sb.String()
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func quoteLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return `"` + v + `"`
}

func escapeHelp(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}
