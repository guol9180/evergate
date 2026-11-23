package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginLimiterBlocksAfterRepeatedFailures(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Unix(1000, 0)
	for i := 0; i < loginFailureLimit; i++ {
		limiter.recordFailure("127.0.0.1", now)
	}
	if wait := limiter.blockedFor("127.0.0.1", now); wait != loginBlockDuration {
		t.Fatalf("blocked duration = %s, want %s", wait, loginBlockDuration)
	}
	if wait := limiter.blockedFor("127.0.0.1", now.Add(loginBlockDuration)); wait != 0 {
		t.Fatalf("expired block = %s, want 0", wait)
	}
	limiter.recordSuccess("127.0.0.1")
	if wait := limiter.blockedFor("127.0.0.1", now); wait != 0 {
		t.Fatalf("successful login did not clear limiter: %s", wait)
	}
}

func TestAdminAccessMiddlewareRestrictsRemoteClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := New(Options{})
	r := gin.New()
	r.Use(api.accessMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.20:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback status = %d, want 200", rec.Code)
	}
}
