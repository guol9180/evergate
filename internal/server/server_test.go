package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"evergate/internal/metrics"
)

func TestMetricsRouteMountedAndLoopbackOnly(t *testing.T) {
	reg := metrics.New()
	reg.Requests.Inc("chat", "200")
	engine := New(Options{Metrics: reg})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:4567"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "evergate_requests_total") {
		t.Fatalf("metrics body = %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "192.168.0.8:4567"
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status = %d, want 403", rec.Code)
	}
}

func TestMetricsRouteUnderBasePath(t *testing.T) {
	reg := metrics.New()
	engine := New(Options{BasePath: "/eg", Metrics: reg})
	req := httptest.NewRequest(http.MethodGet, "/eg/metrics", nil)
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
