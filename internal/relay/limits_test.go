package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/ratelimit"
	"evergate/internal/store"
)

func limitsEngine(t *testing.T, token store.Token, handler gin.HandlerFunc) (*gin.Engine, *ratelimit.Limiter) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	clock := &fakeClock{t: time.Unix(2000, 0).UTC()}
	limiter := ratelimit.New(clock.Now)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(CtxToken, token)
		c.Set(CtxTokenID, token.ID)
		c.Set(CtxTokenName, token.Name)
	})
	r.POST("/v1/chat/completions", TokenLimits(limiter, nil), handler)
	return r, limiter
}

func TestTokenLimitsRPMReturnsOpenAI429(t *testing.T) {
	token := store.Token{ID: 5, Name: "limited", RPMLimit: 2}
	r, _ := limitsEngine(t, token, func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over-limit request = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
	var body struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Type != "rate_limit_exceeded" || body.Error.Code != "rpm_limit_exceeded" {
		t.Fatalf("error payload = %+v", body.Error)
	}
}

func TestTokenLimitsConcurrencyHoldsUntilHandlerReturns(t *testing.T) {
	token := store.Token{ID: 6, Name: "concurrent", ConcurrencyLimit: 1}
	release := make(chan struct{})
	entered := make(chan struct{})
	r, limiter := limitsEngine(t, token, func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusOK)
	})

	go func() {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
	}()
	<-entered
	if limiter.Inflight(token.ID) != 1 {
		t.Fatalf("inflight = %d, want 1", limiter.Inflight(token.ID))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second concurrent request = %d, want 429", rec.Code)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for limiter.Inflight(token.ID) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if limiter.Inflight(token.ID) != 0 {
		t.Fatalf("inflight after release = %d, want 0", limiter.Inflight(token.ID))
	}
}

// 上游 key 达到 RPM 上限后本轮不参与调度：单 key 渠道直接 503。
func TestUpstreamKeyRPMExcludesKey(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "limited", up.server.URL, 1, []string{"gpt-x"}, nil, "sk-limited")
	if err := env.db.Model(&store.ChannelKey{}).Where("channel_id = ?", 1).Update("rpm_limit", 1).Error; err != nil {
		t.Fatalf("set rpm limit: %v", err)
	}
	token, plain := env.addToken(t, "key-rpm", nil)
	_ = token
	gateway := env.authGateway(t)

	if resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"gpt-x","messages":[]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: %d %s", resp.StatusCode, body)
	}
	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"gpt-x","messages":[]}`)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "all_channels_exhausted") {
		t.Fatalf("second request should skip the rate-limited key: %d %s", resp.StatusCode, body)
	}
}
