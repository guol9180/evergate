package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"evergate/internal/metrics"
	"evergate/internal/store"
)

func TestHealthzReports503WithoutLeakingInternalError(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "healthz.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// 关掉底层连接：Ping 失败，但错误细节（驱动原话）不应回显给探测方
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	engine := New(Options{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "database unavailable") || strings.Contains(body, "sql:") {
		t.Fatalf("healthz body should hide the driver error: %s", body)
	}
}

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

func TestSPAHandlerSupportsBasePath(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(`<html><head><base href="./" /><script src="./assets/app.js"></script></head></html>`)},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.NoRoute(spaHandler(webFS, "/eg"))

	check := func(path string, wantStatus int, wantBody, wantLocation string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("%s status = %d, want %d, body=%s", path, rec.Code, wantStatus, rec.Body.String())
		}
		if wantBody != "" && !strings.Contains(rec.Body.String(), wantBody) {
			t.Fatalf("%s body = %s, want %q", path, rec.Body.String(), wantBody)
		}
		if wantLocation != "" && rec.Header().Get("Location") != wantLocation {
			t.Fatalf("%s location = %q, want %q", path, rec.Header().Get("Location"), wantLocation)
		}
		return rec
	}

	check("/", http.StatusTemporaryRedirect, "", "/eg/")
	check("/eg/", http.StatusOK, `<base href="/eg/" />`, "")
	check("/eg/channels", http.StatusOK, `<base href="/eg/" />`, "")
	asset := check("/eg/assets/app.js", http.StatusOK, "console.log", "")
	if asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache = %q", asset.Header().Get("Cache-Control"))
	}
	check("/eg/assets/missing.js", http.StatusNotFound, "", "")
	check("/eg/admin/missing", http.StatusNotFound, `"code":"not_found"`, "")

	root := gin.New()
	root.NoRoute(spaHandler(webFS, ""))
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/channels", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<base href="/" />`) {
		t.Fatalf("root base path fallback = %d %s", rec.Code, rec.Body.String())
	}
}
