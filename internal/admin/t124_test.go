package admin

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/alert"
	"evergate/internal/store"
)

// secondAPI 在同一数据库上重建管理 API（新的 SessionManager），模拟进程重启后会话密钥版本仍从库里读。
func secondAPI(t *testing.T, env *testEnv, configure func(*Options)) *testEnv {
	t.Helper()
	opts := Options{
		DB: env.db, MasterKey: env.master, Pool: env.pool, Session: NewSessionManager("test-secret", time.Hour),
		Settings: env.setting, Usage: env.usage,
		ConfigPath: filepath.Join(t.TempDir(), "evergate.yml"), DBPath: env.dbPath,
		DataDir: filepath.Join(t.TempDir(), "data"), ConfigSource: "default",
		Playground: func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"stub": true}}) },
	}
	if configure != nil {
		configure(&opts)
	}
	api := New(opts)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api.RegisterRoutes(engine)
	gw := httptest.NewServer(engine)
	t.Cleanup(gw.Close)
	return &testEnv{engine: engine, db: env.db, dbPath: env.dbPath, gw: gw, pool: env.pool, setting: env.setting, usage: env.usage, master: env.master}
}

func sessionCookie(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw := resp.Header.Get("Set-Cookie")
	if !strings.Contains(raw, CookieName+"=") {
		t.Fatalf("response should set session cookie, got %q", raw)
	}
	return strings.Split(strings.Split(raw, CookieName+"=")[1], ";")[0]
}

func me(t *testing.T, env *testEnv, cookie string) int {
	t.Helper()
	code, body, _ := env.req(t, "GET", "/admin/api/auth/me", cookie, "")
	if code != http.StatusOK && code != http.StatusUnauthorized {
		t.Fatalf("me: %d %s", code, body)
	}
	return code
}

// D3：改密轮换会话密钥版本——其他会话立即 401，当前会话重签后继续可用，重启后旧 Cookie 仍 401。
func TestChangePasswordRevokesOtherSessionsAndSurvivesRestart(t *testing.T) {
	env := newEnv(t)
	cookieA, _ := login(t, env, "admin", "admin123")
	cookieB, _ := login(t, env, "admin", "admin123")

	code, body, resp := env.req(t, "PUT", "/admin/api/auth/password", cookieA, `{"old_password":"admin123","new_password":"newpass123"}`)
	if code != http.StatusOK {
		t.Fatalf("change password: %d %s", code, body)
	}
	renewed := sessionCookie(t, resp)
	if renewed == cookieA {
		t.Fatal("change password should re-issue the current session cookie")
	}
	for name, old := range map[string]string{"current": cookieA, "other": cookieB} {
		if got := me(t, env, old); got != http.StatusUnauthorized {
			t.Errorf("old %s session should be revoked, got %d", name, got)
		}
	}
	if got := me(t, env, renewed); got != http.StatusOK {
		t.Fatalf("renewed session should keep working, got %d", got)
	}

	// 模拟重启：新进程、新 SessionManager、同一数据库
	restarted := secondAPI(t, env, nil)
	if got := me(t, restarted, cookieB); got != http.StatusUnauthorized {
		t.Errorf("revoked session must stay revoked after restart, got %d", got)
	}
	if got := me(t, restarted, renewed); got != http.StatusOK {
		t.Errorf("renewed session must survive restart, got %d", got)
	}
}

// D4：反向代理终止 TLS 时，仅凭 X-Forwarded-Proto: https 会话 Cookie 也要带 Secure。
func TestSessionCookieSecureBehindTLSProxy(t *testing.T) {
	env := newEnv(t)
	loginWith := func(headers map[string]string) string {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, env.gw.URL+"/admin/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login status = %d", resp.StatusCode)
		}
		return resp.Header.Get("Set-Cookie")
	}
	if raw := loginWith(nil); strings.Contains(strings.ToLower(raw), "; secure") {
		t.Errorf("plain http login should not set Secure: %q", raw)
	}
	if raw := loginWith(map[string]string{"X-Forwarded-Proto": "https"}); !strings.Contains(strings.ToLower(raw), "; secure") {
		t.Errorf("https behind proxy should set Secure: %q", raw)
	}
	// 改密重签的 Cookie 同样跟随
	cookie := sessionCookie(t, mustPost(t, env, "/admin/api/auth/login", `{"username":"admin","password":"admin123"}`, nil))
	req, _ := http.NewRequest(http.MethodPut, env.gw.URL+"/admin/api/auth/password", strings.NewReader(`{"old_password":"admin123","new_password":"newpass123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", CookieName+"="+cookie)
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change password status = %d", resp.StatusCode)
	}
	if raw := resp.Header.Get("Set-Cookie"); !strings.Contains(strings.ToLower(raw), "; secure") {
		t.Errorf("re-issued cookie behind https proxy should set Secure: %q", raw)
	}
}

func mustPost(t *testing.T, env *testEnv, path, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, env.gw.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// B11 / 分页：审计列表支持按动作筛选；分页参数非法时 400 而不是静默夹逼。
func TestAuditListFiltersByActionAndRejectsBadPaging(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	for i, action := range []string{"token.create", "channel.create", "token.create"} {
		if err := env.db.Create(&store.AuditLog{Action: action, Username: "admin", IP: "127.0.0.1", CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// readyCookie 的改密本身已产生一条 admin.change_password
	total := func(query string) (int, string) {
		t.Helper()
		code, body, _ := env.req(t, "GET", "/admin/api/audit-logs"+query, cookie, "")
		return code, body
	}
	if code, body := total("?action=token.create"); code != http.StatusOK || !strings.Contains(body, `"total":2`) {
		t.Errorf("action filter: %d %s", code, body)
	}
	if code, body := total("?action="); code != http.StatusOK || !strings.Contains(body, `"total":4`) {
		t.Errorf("empty action filter: %d %s", code, body)
	}
	for _, bad := range []string{"?page=0", "?page=abc", "?page=-1", "?page_size=0", "?page_size=101", "?page_size=x"} {
		if code, body := total(bad); code != http.StatusBadRequest || !strings.Contains(body, `"invalid_param"`) {
			t.Errorf("%s should be 400 invalid_param: %d %s", bad, code, body)
		}
	}
	if code, body := total("?page=2&page_size=2"); code != http.StatusOK || !strings.Contains(body, `"page":2`) || !strings.Contains(body, `"page_size":2`) {
		t.Errorf("valid paging should echo the request: %d %s", code, body)
	}
}

// A10：告警配置（Webhook 地址是数据外发通道）变更入审计。
func TestAlertsUpdateIsAudited(t *testing.T) {
	env := newEnv(t)
	api := secondAPI(t, env, func(o *Options) { o.Alerts = alert.New(env.db, nil, nil) })
	cookie, _ := login(t, api, "admin", "admin123")
	_, _, resp := api.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie = sessionCookie(t, resp)
	if code, body, _ := api.req(t, "PUT", "/admin/api/alerts", cookie, `{"enabled":false}`); code != http.StatusOK {
		t.Fatalf("update alerts: %d %s", code, body)
	}
	code, body, _ := api.req(t, "GET", "/admin/api/audit-logs?action=alerts.update", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"total":1`) || !strings.Contains(body, `"action":"alerts.update"`) {
		t.Fatalf("alerts update should be audited: %d %s", code, body)
	}
}
