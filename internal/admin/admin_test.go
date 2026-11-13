package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/health"
	"evergate/internal/pool"
	"evergate/internal/settings"
	"evergate/internal/store"
	"evergate/internal/usage"
)

type testEnv struct {
	engine  *gin.Engine
	db      *gorm.DB
	gw      *httptest.Server
	pool    *pool.Manager
	setting *settings.Manager
	usage   *usage.Manager
	master  string
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := store.EnsureAdmin(db, store.DefaultAdminUsername, store.DefaultAdminPassword); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	master, _ := store.GenerateRandomKey()
	settingMgr, err := settings.Load(db, config.Default(), func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	pl := pool.NewManager(3, pool.DefaultCooldownSchedule, func() time.Time { return time.Now().UTC() })
	prober := health.NewProber(health.ProberOptions{
		DB: db, MasterKey: master, Pool: pl, Now: func() time.Time { return time.Now().UTC() },
	})
	usageMgr, err := usage.Load(db)
	if err != nil {
		t.Fatalf("load usage: %v", err)
	}
	api := New(Options{
		DB: db, MasterKey: master, Pool: pl, Session: NewSessionManager("test-secret", time.Hour),
		Settings: settingMgr, Health: prober, Usage: usageMgr,
		Playground: func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"stub": true}}) },
	})
	gin.SetMode(gin.TestMode)
	e := gin.New()
	api.RegisterRoutes(e)
	gw := httptest.NewServer(e)
	t.Cleanup(gw.Close)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return &testEnv{engine: e, db: db, gw: gw, pool: pl, setting: settingMgr, usage: usageMgr, master: master}
}

func (e *testEnv) req(t *testing.T, method, path, cookie, body string) (int, string, *http.Response) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, e.gw.URL+path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", CookieName+"="+cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, string(data), resp
}

func login(t *testing.T, e *testEnv, user, pass string) (string, string) {
	code, body, resp := e.req(t, "POST", "/admin/api/auth/login", "", fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass))
	if code != 200 {
		t.Fatalf("login: %d %s", code, body)
	}
	cookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, CookieName+"=") {
		t.Fatal("login should set session cookie")
	}
	value := strings.Split(strings.Split(cookie, CookieName+"=")[1], ";")[0]
	return value, body
}

func TestAuthFlowAndForcedPasswordChange(t *testing.T) {
	env := newEnv(t)
	// 错误密码
	if code, body, _ := env.req(t, "POST", "/admin/api/auth/login", "", `{"username":"admin","password":"wrong"}`); code != 401 {
		t.Fatalf("wrong password: %d %s", code, body)
	}
	// 正确登录 → 返回 must_change_password
	cookie, body := login(t, env, "admin", "admin123")
	if !strings.Contains(body, `"must_change_password":true`) {
		t.Fatalf("首次登录应要求改密: %s", body)
	}
	// me 可用
	if code, body, _ := env.req(t, "GET", "/admin/api/auth/me", cookie, ""); code != 200 || !strings.Contains(body, "admin") {
		t.Fatalf("me: %d %s", code, body)
	}
	// 未改密前其他接口 403
	if code, body, _ := env.req(t, "GET", "/admin/api/channels", cookie, ""); code != 403 || !strings.Contains(body, "must_change_password") {
		t.Fatalf("改密前应 403: %d %s", code, body)
	}
	// 旧密码错误
	if code, _, _ := env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"bad","new_password":"newpass123"}`); code != 401 {
		t.Fatal("旧密码错误应 401")
	}
	// 改密成功
	if code, body, _ := env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`); code != 200 {
		t.Fatalf("change password: %d %s", code, body)
	}
	// 改密后可访问
	if code, body, _ := env.req(t, "GET", "/admin/api/channels", cookie, ""); code != 200 {
		t.Fatalf("改密后应放行: %d %s", code, body)
	}
	// 旧密码不能登录，新密码可以
	if code, _, _ := env.req(t, "POST", "/admin/api/auth/login", "", `{"username":"admin","password":"admin123"}`); code != 401 {
		t.Fatal("旧密码应失效")
	}
	login(t, env, "admin", "newpass123")
	// 未登录访问
	if code, _, _ := env.req(t, "GET", "/admin/api/channels", "", ""); code != 401 {
		t.Fatal("未登录应 401")
	}
}

func TestSystemUpdateStatusRequiresAuth(t *testing.T) {
	env := newEnv(t)
	code, _, _ := env.req(t, "GET", "/admin/api/system/update", "", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("update status without session = %d, want 401", code)
	}
}

func TestPlaygroundRequiresAdminSession(t *testing.T) {
	env := newEnv(t)
	if code, _, _ := env.req(t, "POST", "/admin/api/playground", "", `{"model":"m","prompt":"hi"}`); code != http.StatusUnauthorized {
		t.Fatalf("未登录调用 playground = %d, want 401", code)
	}
	// 关闭首登改密标记，验证已登录会话可进入 Playground handler。
	if err := env.db.Model(&store.Admin{}).Where("username = ?", "admin").Update("must_change_password", false).Error; err != nil {
		t.Fatalf("update admin: %v", err)
	}
	cookie, _ := login(t, env, "admin", "admin123")
	code, body, _ := env.req(t, "POST", "/admin/api/playground", cookie, `{"model":"m","prompt":"hi"}`)
	if code != http.StatusOK || !strings.Contains(body, `"stub":true`) {
		t.Fatalf("登录后 playground = %d %s", code, body)
	}
}

func TestSystemSettingsHotUpdate(t *testing.T) {
	env := newEnv(t)
	// 未登录禁止读取
	if code, _, _ := env.req(t, "GET", "/admin/api/system/settings", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("settings without session = %d, want 401", code)
	}
	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")

	code, body, _ := env.req(t, "GET", "/admin/api/system/settings", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("get settings: %d %s", code, body)
	}
	if strings.Contains(body, "master_key") || strings.Contains(body, "session_secret") || strings.Contains(body, "sqlite_path") {
		t.Fatalf("系统设置接口泄漏引导敏感字段: %s", body)
	}
	if !strings.Contains(body, `"source":"evergate.yml"`) || !strings.Contains(body, `"hot_update_enabled":true`) {
		t.Fatalf("设置响应缺少来源/热更新说明: %s", body)
	}

	current := env.setting.Snapshot()
	invalid := current
	invalid.ConnectTimeoutMS = 0
	payload, _ := json.Marshal(invalid)
	if code, body, _ = env.req(t, "PUT", "/admin/api/system/settings", cookie, string(payload)); code != http.StatusBadRequest {
		t.Fatalf("invalid settings = %d %s", code, body)
	}
	if env.setting.Snapshot().ConnectTimeoutMS != current.ConnectTimeoutMS {
		t.Fatal("非法配置不应产生部分更新")
	}

	next := current
	next.FirstTokenTimeoutMS = 45_000
	next.RetryChannels = 5
	next.CooldownFailThreshold = 4
	next.LogRetentionDays = 21
	payload, _ = json.Marshal(next)
	if code, body, _ = env.req(t, "PUT", "/admin/api/system/settings", cookie, string(payload)); code != http.StatusOK {
		t.Fatalf("update settings: %d %s", code, body)
	}
	got := env.setting.Snapshot()
	if got.FirstTokenTimeoutMS != 45_000 || got.RetryChannels != 5 || got.CooldownFailThreshold != 4 || got.LogRetentionDays != 21 {
		t.Fatalf("热更新快照不正确: %#v", got)
	}
	if got.Source != settings.SourceDatabase {
		t.Fatalf("更新后 source = %q, want database", got.Source)
	}
	var row store.RuntimeSetting
	if err := env.db.First(&row, "id = ?", 1).Error; err != nil {
		t.Fatalf("read settings row: %v", err)
	}
	if row.FirstTokenTimeoutMS != 45_000 || row.Source != settings.SourceDatabase {
		t.Fatalf("设置未持久化: %#v", row)
	}
}

func TestChannelAndKeyManagement(t *testing.T) {
	env := newEnv(t)
	login(t, env, "admin", "admin123")
	// 改密解锁
	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")

	// 创建渠道
	code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie, `{"name":"test-ch","type":"openai-compatible","base_url":"https://api.example.com/v1/","models":["gpt-x","gpt-y"],"priority":1,"weight":2,"supports_responses":true}`)
	if code != 200 || !strings.Contains(body, `"id":1`) {
		t.Fatalf("create channel: %d %s", code, body)
	}
	if !strings.Contains(body, `"supports_responses":true`) {
		t.Fatalf("create channel should expose Responses capability: %s", body)
	}
	// 批量导入 key（多行）
	code, body, _ = env.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-plaintext-aaa\r\nsk-plaintext-bbb\n\n  \r\nsk-plaintext-ccc"}`)
	if code != 200 || !strings.Contains(body, `"added":3`) {
		t.Fatalf("batch keys: %d %s", code, body)
	}
	// 详情：不回显密文与明文（验收 #9）
	code, body, _ = env.req(t, "GET", "/admin/api/channels/1", cookie, "")
	if code != 200 {
		t.Fatalf("get channel: %d %s", code, body)
	}
	if strings.Contains(body, "sk-plaintext") || strings.Contains(body, "key_cipher") {
		t.Fatalf("接口泄漏明文或密文: %s", body)
	}
	if !strings.Contains(body, `"key_count":3`) {
		t.Fatalf("key 计数错误: %s", body)
	}
	// key 状态字段存在
	if !strings.Contains(body, `"state":"active"`) {
		t.Fatalf("key 应含状态: %s", body)
	}
	// 更新渠道
	if code, body, _ = env.req(t, "PUT", "/admin/api/channels/1", cookie, `{"name":"test-ch","type":"openai-compatible","base_url":"https://api.example.com/v1","models":["gpt-z"],"priority":0,"weight":1,"supports_responses":false}`); code != 200 || !strings.Contains(body, `"supports_responses":false`) {
		t.Fatal("update channel failed")
	}
	// 启停
	if code, body, _ = env.req(t, "POST", "/admin/api/channels/1/toggle", cookie, `{"enabled":false}`); code != 200 || !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("toggle: %d %s", code, body)
	}
	// 列表含状态与 key 计数
	if code, body, _ = env.req(t, "GET", "/admin/api/channels", cookie, ""); code != 200 || !strings.Contains(body, `"key_count":3`) {
		t.Fatalf("list channels: %d %s", code, body)
	}
	// 删除单个 key
	if code, _, _ = env.req(t, "DELETE", "/admin/api/keys/1", cookie, ""); code != 200 {
		t.Fatal("delete key failed")
	}
	// 删除渠道（级联 keys）
	if code, _, _ = env.req(t, "DELETE", "/admin/api/channels/1", cookie, ""); code != 200 {
		t.Fatal("delete channel failed")
	}
	var keyCount int64
	env.db.Model(&store.ChannelKey{}).Count(&keyCount)
	if keyCount != 0 {
		t.Fatalf("渠道删除应级联删除 key: %d", keyCount)
	}
}

func TestChannelManualProbePersistsHealthView(t *testing.T) {
	env := newEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer sk-probe" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")

	code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie, fmt.Sprintf(`{"name":"probe","type":"openai-compatible","base_url":%q,"models":["test-model"]}`, server.URL))
	if code != http.StatusOK {
		t.Fatalf("create channel: %d %s", code, body)
	}
	if code, body, _ = env.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-probe"}`); code != http.StatusOK {
		t.Fatalf("add key: %d %s", code, body)
	}
	code, body, _ = env.req(t, "POST", "/admin/api/channels/1/test", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("manual probe: %d %s", code, body)
	}
	for _, want := range []string{`"ok":true`, `"result":"success"`, `"status":"active"`, `"status_code":200`} {
		if !strings.Contains(body, want) {
			t.Fatalf("manual probe response missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "sk-probe") || strings.Contains(body, "key_cipher") {
		t.Fatalf("manual probe leaked key material: %s", body)
	}

	code, body, _ = env.req(t, "GET", "/admin/api/channels", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("list channels: %d %s", code, body)
	}
	for _, want := range []string{`"available_key_count":1`, `"last_probe_result":"success"`, `"next_probe_at":`} {
		if !strings.Contains(body, want) {
			t.Fatalf("channel health view missing %s: %s", want, body)
		}
	}
	var ch store.Channel
	if err := env.db.First(&ch, 1).Error; err != nil {
		t.Fatalf("read channel: %v", err)
	}
	if ch.Status != store.ChannelStatusActive || ch.LastProbeStatusCode != http.StatusOK || ch.LastProbeAt == nil || ch.NextProbeAt == nil {
		t.Fatalf("channel probe row = %#v", ch)
	}

	cipher, err := store.Encrypt("sk-bad", env.master)
	if err != nil {
		t.Fatalf("encrypt bad key: %v", err)
	}
	bad := &store.Channel{
		Name: "bad", Type: "openai-compatible", BaseURL: server.URL,
		Models: []string{"test-model"}, Enabled: true, Status: store.ChannelStatusActive,
		LastProbeResult: health.ProbeResultFailure,
	}
	if err := env.db.Create(bad).Error; err != nil {
		t.Fatalf("create bad channel: %v", err)
	}
	if err := env.db.Create(&store.ChannelKey{ChannelID: bad.ID, KeyCipher: cipher, Enabled: true, State: store.KeyStateActive}).Error; err != nil {
		t.Fatalf("create bad key: %v", err)
	}
	if err := env.db.Model(bad).Updates(map[string]any{
		"last_probe_result": health.ProbeResultFailure, "last_probe_error": "upstream unavailable",
	}).Error; err != nil {
		t.Fatalf("mark bad channel: %v", err)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/channels", cookie, "")
	if code != http.StatusOK || strings.Index(body, `"name":"bad"`) >= strings.Index(body, `"name":"probe"`) {
		t.Fatalf("abnormal channel should sort first: %d %s", code, body)
	}
}

func TestTokenManagement(t *testing.T) {
	env := newEnv(t)
	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")

	code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"my-token"}`)
	if code != 200 {
		t.Fatalf("create token: %d %s", code, body)
	}
	var out struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || !strings.HasPrefix(out.Data.Key, "eg-") {
		t.Fatalf("create token 应返回一次明文: %s", body)
	}
	// 列表不返回明文
	code, body, _ = env.req(t, "GET", "/admin/api/tokens", cookie, "")
	if code != 200 || strings.Contains(body, out.Data.Key) || strings.Contains(body, "key_hash") {
		t.Fatalf("列表泄漏明文或哈希: %s", body)
	}
	// 更新与删除
	if code, _, _ = env.req(t, "PUT", "/admin/api/tokens/1", cookie, `{"enabled":false}`); code != 200 {
		t.Fatal("update token failed")
	}
	if code, _, _ = env.req(t, "DELETE", "/admin/api/tokens/1", cookie, ""); code != 200 {
		t.Fatal("delete token failed")
	}
}

func TestTokenGovernanceManagement(t *testing.T) {
	env := newEnv(t)
	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")

	code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie,
		`{"name":"governed","quota":10,"allowed_models":["m","m",""],"allowed_ips":["127.0.0.1","10.0.0.0/8","127.0.0.1"]}`)
	if code != http.StatusOK {
		t.Fatalf("create governed token: %d %s", code, body)
	}
	for _, want := range []string{
		`"quota":10`, `"remaining_quota":10`, `"status":"active"`,
		`"allowed_models":["m"]`, `"allowed_ips":["127.0.0.1","10.0.0.0/8"]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("governed token response missing %s: %s", want, body)
		}
	}

	if err := env.db.Model(&store.Token{}).Where("id = 1").Update("used_quota", 10).Error; err != nil {
		t.Fatalf("exhaust token: %v", err)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/tokens", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"remaining_quota":0`) || !strings.Contains(body, `"status":"exhausted"`) {
		t.Fatalf("exhausted token view: %d %s", code, body)
	}

	if code, body, _ = env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"bad-ip","allowed_ips":["localhost"]}`); code != http.StatusBadRequest || !strings.Contains(body, "CIDR") {
		t.Fatalf("invalid ip should 400: %d %s", code, body)
	}
	if code, body, _ = env.req(t, "PUT", "/admin/api/tokens/1", cookie, `{"quota":-1}`); code != http.StatusBadRequest {
		t.Fatalf("negative quota should 400: %d %s", code, body)
	}
	if code, body, _ = env.req(t, "PUT", "/admin/api/tokens/1", cookie, `{"allowed_ips":["127.0.0.1:8080"]}`); code != http.StatusBadRequest {
		t.Fatalf("port rule should 400: %d %s", code, body)
	}
	code, body, _ = env.req(t, "PUT", "/admin/api/tokens/1", cookie, `{"allowed_models":[],"allowed_ips":[]}`)
	if code != http.StatusOK {
		t.Fatalf("clear rules: %d %s", code, body)
	}
	var token store.Token
	if err := env.db.First(&token, 1).Error; err != nil {
		t.Fatalf("read token: %v", err)
	}
	if len(token.AllowedModels) != 0 || len(token.AllowedIPs) != 0 {
		t.Fatalf("rules should be cleared: %#v", token)
	}
}

func TestLogPaginationAndFilters(t *testing.T) {
	env := newEnv(t)
	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")

	for i := 1; i <= 25; i++ {
		log := &store.RequestLog{
			Model: fmt.Sprintf("m-%d", i%3), StatusCode: 200 + (i % 2), ChannelID: int64(i % 2),
			ChannelName: "ch", IP: "127.0.0.1", RetryTrace: []store.RetryAttempt{{ChannelID: 1, Attempt: 1}},
		}
		if err := env.db.Create(log).Error; err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}
	// 分页
	code, body, _ := env.req(t, "GET", "/admin/api/logs?page=1&page_size=10", cookie, "")
	if code != 200 || !strings.Contains(body, `"total":25`) {
		t.Fatalf("分页: %d %s", code, body)
	}
	var page struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(body), &page)
	if len(page.Data.Items) != 10 {
		t.Fatalf("page size = %d", len(page.Data.Items))
	}
	// 筛选
	code, body, _ = env.req(t, "GET", "/admin/api/logs?model=m-0", cookie, "")
	if code != 200 {
		t.Fatalf("filter: %d", code)
	}
	if !strings.Contains(body, `"total":9`) && !strings.Contains(body, `"total":8`) {
		t.Fatalf("model 筛选计数异常: %s", body)
	}
	// 详情含 retry_trace
	code, body, _ = env.req(t, "GET", "/admin/api/logs/1", cookie, "")
	if code != 200 || !strings.Contains(body, "retry_trace") {
		t.Fatalf("log detail: %d %s", code, body)
	}
}

func TestSessionTamper(t *testing.T) {
	s := NewSessionManager("secret", time.Hour)
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	tok := s.Issue("admin", now)
	if user, err := s.Verify(tok, now); err != nil || user != "admin" {
		t.Fatalf("verify: %v", err)
	}
	if _, err := s.Verify(tok+"x", now); err == nil {
		t.Fatal("篡改令牌应失效")
	}
	if _, err := s.Verify(tok, now.Add(2*time.Hour)); err == nil {
		t.Fatal("过期应失效")
	}
}
