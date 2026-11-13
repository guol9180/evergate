package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func render(t *testing.T, r *Registry) string {
	t.Helper()
	var sb strings.Builder
	if err := r.WriteText(&sb); err != nil {
		t.Fatalf("write text: %v", err)
	}
	return sb.String()
}

func TestPrometheusTextExposition(t *testing.T) {
	r := New()
	r.Requests.Inc("chat", "200")
	r.Requests.Inc("chat", "200")
	r.Requests.Inc("responses", "503")
	r.RequestErrors.Inc("all_channels_exhausted")
	r.ActiveRequests.Set(3)
	r.FirstToken.Observe(0.12, "chat")
	r.FirstToken.Observe(2.5, "chat")

	out := render(t, r)
	for _, want := range []string{
		"# TYPE evergate_requests_total counter",
		`evergate_requests_total{protocol="chat",status="200"} 2`,
		`evergate_requests_total{protocol="responses",status="503"} 1`,
		`evergate_request_errors_total{type="all_channels_exhausted"} 1`,
		"evergate_active_requests 3",
		`evergate_first_token_seconds_bucket{protocol="chat",le="0.1"} 0`,
		`evergate_first_token_seconds_bucket{protocol="chat",le="0.25"} 1`,
		`evergate_first_token_seconds_bucket{protocol="chat",le="+Inf"} 2`,
		`evergate_first_token_seconds_sum{protocol="chat"} 2.62`,
		`evergate_first_token_seconds_count{protocol="chat"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in exposition:\n%s", want, out)
		}
	}
}

func TestMetricsLoopbackRestriction(t *testing.T) {
	r := New()
	r.Requests.Inc("chat", "200")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:51234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "evergate_requests_total") {
		t.Fatalf("loopback body missing metrics: %s", rec.Body.String())
	}

	for _, remote := range []string{"192.168.1.9:443", "10.0.0.2:1", "::1%25bad"} {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("remote %q status = %d, want 403", remote, rec.Code)
		}
	}
}

func TestLoopbackAddrIPv6(t *testing.T) {
	for _, addr := range []string{"[::1]:8080", "127.0.0.1:1", "::1"} {
		if !IsLoopbackAddr(addr) {
			t.Fatalf("%q should be loopback", addr)
		}
	}
	for _, addr := range []string{"", "bad", "8.8.8.8:53", "[fe80::1]:80"} {
		if IsLoopbackAddr(addr) {
			t.Fatalf("%q should not be loopback", addr)
		}
	}
}

func TestLowCardinalityFoldsOverflow(t *testing.T) {
	r := New()
	seenOther := false
	first := ""
	for i := 0; i < maxLabelValues+10; i++ {
		v := r.LowCardinality("model", "model-"+strings.Repeat("x", i%5)+string(rune('a'+i%26))+"-"+itoa(i))
		if first == "" {
			first = v
		}
		if v == otherLabelValue {
			seenOther = true
		}
	}
	if !seenOther {
		t.Fatal("expected overflow values to fold into other")
	}
	if got := r.LowCardinality("model", first); got != first {
		t.Fatalf("existing value should be preserved, got %q", got)
	}
	if got := r.LowCardinality("other-kind", "free\"text"); got != otherLabelValue {
		t.Fatalf("unsafe label value = %q, want other", got)
	}
}

func TestSeriesLimitDropsUpdates(t *testing.T) {
	r := New()
	r.seriesLimit = 2
	for _, status := range []string{"200", "201", "202"} {
		r.Requests.Inc("chat", status)
	}
	if r.Dropped() == 0 {
		t.Fatal("expected dropped series count > 0")
	}
	out := render(t, r)
	if !strings.Contains(out, "evergate_metrics_series_dropped_total") {
		t.Fatalf("dropped metric missing:\n%s", out)
	}
}

func TestCollectorSamplesRendered(t *testing.T) {
	r := New()
	r.AddCollector(func() []Sample {
		return []Sample{
			{Name: "evergate_test_value", Help: "test gauge", Labels: map[string]string{"channel": "1", "state": "active"}, Value: 2},
		}
	})
	out := render(t, r)
	if !strings.Contains(out, `evergate_test_value{channel="1",state="active"} 2`) {
		t.Fatalf("collector sample missing:\n%s", out)
	}
}

func TestConcurrentIncrements(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.Requests.Inc("chat", "200")
				r.FirstToken.Observe(0.2, "chat")
			}
		}()
	}
	wg.Wait()
	out := render(t, r)
	if !strings.Contains(out, `evergate_requests_total{protocol="chat",status="200"} 2000`) {
		t.Fatalf("concurrent counter mismatch:\n%s", out)
	}
	if !strings.Contains(out, `evergate_first_token_seconds_count{protocol="chat"} 2000`) {
		t.Fatalf("concurrent histogram mismatch:\n%s", out)
	}
}

func itoa(v int) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v%10]
		v /= 10
	}
	return string(buf[i:])
}
