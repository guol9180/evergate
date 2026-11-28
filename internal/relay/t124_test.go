package relay

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

// unknownTokenGateway 起一个只挂 /v1/models 的鉴权网关，并返回关闭日志写入器后读出未知令牌日志的函数。
func unknownTokenGateway(t *testing.T, env *testEnv, enabled *bool) (hit func(string) int, logs func() []store.RequestLog) {
	t.Helper()
	writer := NewLogWriter(env.db, 256, slog.New(slog.NewTextHandler(io.Discard, nil)))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1/models", TokenAuth(env.db, writer, func() bool { return *enabled }), env.HandleModels)
	gw := httptest.NewServer(engine)
	t.Cleanup(gw.Close)
	hit = func(token string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, gw.URL+"/v1/models", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	logs = func() []store.RequestLog {
		t.Helper()
		writer.Close()
		var rows []store.RequestLog
		if err := env.db.Where("error_type = ?", "invalid_token").Order("id asc").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		return rows
	}
	return hit, logs
}

func TestUnknownTokenRequestsAreLoggedWithRateLimit(t *testing.T) {
	env := newEnv(t)
	t.Cleanup(env.logWriter.Close)
	unknownTokenLogs = fixedWindowCounter{}

	enabled := true
	hit, logs := unknownTokenGateway(t, env, &enabled)
	if code := hit("eg-unknown"); code != http.StatusUnauthorized {
		t.Fatalf("unknown token status = %d", code)
	}
	rows := logs()
	if len(rows) != 1 || rows[0].StatusCode != http.StatusUnauthorized || rows[0].TokenID != 0 || rows[0].IP == "" {
		t.Fatalf("unknown token should leave one 401 log without token identity: %#v", rows)
	}
	if strings.Contains(rows[0].ErrorMessage, "eg-unknown") {
		t.Fatalf("log must never contain the token plaintext: %q", rows[0].ErrorMessage)
	}

	// 关闭开关后不再落日志
	disabled := false
	hit, logs = unknownTokenGateway(t, env, &disabled)
	if code := hit("eg-unknown-2"); code != http.StatusUnauthorized {
		t.Fatalf("unknown token status = %d", code)
	}
	if rows = logs(); len(rows) != 1 {
		t.Fatalf("disabled switch should not add logs, got %d", len(rows))
	}

	// 被扫描时每分钟只记 budget 条
	enabled = true
	unknownTokenLogs = fixedWindowCounter{}
	hit, logs = unknownTokenGateway(t, env, &enabled)
	for i := 0; i < unknownTokenLogBudget+5; i++ {
		if code := hit(fmt.Sprintf("eg-scan-%d", i)); code != http.StatusUnauthorized {
			t.Fatalf("scan status = %d", code)
		}
	}
	if rows = logs(); len(rows) != 1+unknownTokenLogBudget {
		t.Fatalf("rate limit should cap unknown-token logs at %d, got %d", unknownTokenLogBudget, len(rows)-1)
	}

	// 已知令牌照常通过且不产生 invalid_token 日志
	_, plain := env.addToken(t, "known", nil)
	if code := hit(plain); code != http.StatusOK {
		t.Fatalf("known token status = %d", code)
	}
}
