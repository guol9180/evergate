package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"evergate/internal/store"
)

func seedLog(t *testing.T, env *testEnv, status int, totalMS int64, model string) {
	t.Helper()
	if err := env.db.Create(&store.RequestLog{
		CreatedAt: time.Now().UTC(), StatusCode: status, Model: model, TotalMS: totalMS,
		ChannelName: "c", TokenName: "t", CostStatus: "priced", CostMicros: 12,
	}).Error; err != nil {
		t.Fatalf("seed log: %v", err)
	}
}

func readyCookie(t *testing.T, env *testEnv) string {
	t.Helper()
	cookie, _ := login(t, env, "admin", "admin123")
	env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie, _ = login(t, env, "admin", "newpass123")
	return cookie
}

func TestComputeLogStatsTableDriven(t *testing.T) {
	type row = struct {
		StatusCode int   `gorm:"column:status_code"`
		TotalMS    int64 `gorm:"column:total_ms"`
	}
	cases := []struct {
		name           string
		rows           []row
		total, success int64
		rate           *float64
		p50, p95, avg  *int64
	}{
		{name: "empty", rows: nil, total: 0, success: 0},
		{
			name: "mixed", rows: []row{{200, 100}, {200, 300}, {503, 200}, {200, 0}},
			total: 4, success: 3, rate: fptr(75), p50: iptr(200), p95: iptr(300), avg: iptr(200),
		},
		{
			name: "all failed", rows: []row{{500, 50}, {429, 70}},
			total: 2, success: 0, rate: fptr(0), p50: iptr(50), p95: iptr(70), avg: iptr(60),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeLogStats(tc.rows)
			if got.Total != tc.total || got.Success != tc.success {
				t.Fatalf("total/success = %d/%d, want %d/%d", got.Total, got.Success, tc.total, tc.success)
			}
			assertFloat(t, "rate", got.SuccessRate, tc.rate)
			assertInt(t, "p50", got.P50MS, tc.p50)
			assertInt(t, "p95", got.P95MS, tc.p95)
			assertInt(t, "avg", got.AvgTotalMS, tc.avg)
		})
	}
}

func TestLogsStatsAndCSVExport(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	seedLog(t, env, 200, 100, "m-a")
	seedLog(t, env, 200, 300, "m-a")
	seedLog(t, env, 200, 500, "m-b")
	seedLog(t, env, 503, 700, "m-b")

	code, body, _ := env.req(t, "GET", "/admin/api/logs/stats", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("stats: %d %s", code, body)
	}
	var resp struct {
		Data struct{ Stats logStats } `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	s := resp.Data.Stats
	if s.Total != 4 || s.Success != 3 || s.SuccessRate == nil || *s.SuccessRate != 75 {
		t.Fatalf("stats = %+v", s)
	}
	if s.P50MS == nil || *s.P50MS != 300 || s.P95MS == nil || *s.P95MS != 700 {
		t.Fatalf("percentiles = p50=%v p95=%v, want 300/700", s.P50MS, s.P95MS)
	}

	// 筛选参数与 /logs 共用：只看 m-b 应得 2 条、成功率 50%。
	code, body, _ = env.req(t, "GET", "/admin/api/logs/stats?model=m-b", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"total":2`) || !strings.Contains(body, `"success_rate":50`) {
		t.Fatalf("filtered stats: %d %s", code, body)
	}

	resp3, err := http.NewRequest("GET", env.gw.URL+"/admin/api/logs/export?model=m-b", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Header.Set("Cookie", CookieName+"="+cookie)
	httpResp, err := http.DefaultClient.Do(resp3)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("export status %d", httpResp.StatusCode)
	}
	if ct := httpResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content-type %q", ct)
	}
	if cd := httpResp.Header.Get("Content-Disposition"); !strings.Contains(cd, `attachment; filename="evergate-logs-`) {
		t.Fatalf("content-disposition %q", cd)
	}
	raw := make([]byte, 3)
	if _, err := httpResp.Body.Read(raw); err != nil {
		t.Fatal(err)
	}
	if raw[0] != 0xEF || raw[1] != 0xBB || raw[2] != 0xBF {
		t.Fatalf("CSV must start with UTF-8 BOM, got %x", raw)
	}
	rest := readAll(t, httpResp)
	lines := strings.Count(rest, "\n")
	// 1 表头 + 2 行；不含请求体或 key 字段
	if lines != 3 {
		t.Fatalf("CSV lines = %d, want 3:\n%s", lines, rest)
	}
	for _, forbidden := range []string{"key_cipher", "eg-", "request_body"} {
		if strings.Contains(rest, forbidden) {
			t.Fatalf("CSV leaked %q", forbidden)
		}
	}
}

func TestAuditLogRecordsWritesWithoutSecrets(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)

	code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie, `{"name":"audited","type":"openai-compatible","base_url":"https://example.com","models":["m"]}`)
	if code != http.StatusOK {
		t.Fatalf("create channel: %d %s", code, body)
	}
	code, body, _ = env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"audited-token"}`)
	if code != http.StatusOK || !strings.Contains(body, `"key":"eg-`) {
		t.Fatalf("create token: %d %s", code, body)
	}
	code, body, _ = env.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-secret-material"}`)
	if code != http.StatusOK {
		t.Fatalf("add key: %d %s", code, body)
	}
	code, body, _ = env.req(t, "DELETE", "/admin/api/channels/1", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("delete channel: %d %s", code, body)
	}
	// 失败的写操作不应产生审计噪音。
	env.req(t, "DELETE", "/admin/api/channels/999", cookie, "")

	code, body, _ = env.req(t, "GET", "/admin/api/audit-logs?page_size=50", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("list audit: %d %s", code, body)
	}
	for _, want := range []string{`"action":"channel.create"`, `"action":"token.create"`, `"action":"key.batch_add"`, `"action":"channel.delete"`, `"action":"admin.change_password"`, `"username":"admin"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("audit response missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "sk-secret-material") || strings.Contains(body, `"key":"eg-`) || strings.Contains(body, "key_hash") {
		t.Fatalf("audit leaked secret material: %s", body)
	}
	var count int64
	env.db.Model(&store.AuditLog{}).Where("action = ?", "channel.delete").Count(&count)
	if count != 1 {
		t.Fatalf("channel.delete audit rows = %d, want 1 (failed delete must not be audited)", count)
	}
	// 按动作筛选
	code, body, _ = env.req(t, "GET", "/admin/api/audit-logs?action=token.create", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"total":1`) {
		t.Fatalf("filter by action: %d %s", code, body)
	}
}

func TestConfigExportImportRoundTrip(t *testing.T) {
	src := newEnv(t)
	cookie := readyCookie(t, src)
	if code, body, _ := src.req(t, "POST", "/admin/api/channels", cookie, `{"name":"export-me","type":"openai-compatible","preset_id":"bigmodel","base_url":"https://example.com","models":["pub"],"model_mapping":{"pub":"up"},"priority":2,"weight":5,"supports_responses":true,"timeout_config":{"connect_sec":5}}`); code != http.StatusOK {
		t.Fatalf("create channel: %d %s", code, body)
	}
	if code, body, _ := src.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-upstream-secret"}`); code != http.StatusOK {
		t.Fatalf("add key: %d %s", code, body)
	}
	if code, body, _ := src.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"exported-token","quota":1000,"allowed_models":["pub"]}`); code != http.StatusOK {
		t.Fatalf("create token: %d %s", code, body)
	}
	if code, body, _ := src.req(t, "PUT", "/admin/api/model-fallbacks", cookie, `{"items":[{"model":"pub","budget_threshold":10,"fallbacks":["backup"]}]}`); code != http.StatusOK {
		t.Fatalf("fallbacks: %d %s", code, body)
	}

	code, body, _ := src.req(t, "GET", "/admin/api/config/export", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("export: %d %s", code, body)
	}
	for _, forbidden := range []string{"sk-upstream-secret", "key_cipher", "key_hash", `"key":"eg-`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("export leaked %q", forbidden)
		}
	}
	for _, want := range []string{`"version":1`, `"name":"export-me"`, `"preset_id":"bigmodel"`, `"model_mapping":{"pub":"up"}`, `"connect_sec":5`, `"name":"exported-token"`, `"quota":1000`, `"budget_threshold":10`} {
		if !strings.Contains(body, want) {
			t.Fatalf("export missing %s: %s", want, body)
		}
	}
	// 导出被审计（虽是 GET）
	if code, abody, _ := src.req(t, "GET", "/admin/api/audit-logs?action=config.export", cookie, ""); code != http.StatusOK || !strings.Contains(abody, `"total":1`) {
		t.Fatalf("export should be audited: %d %s", code, abody)
	}

	// 提取 data 作为导入载荷
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode export envelope: %v", err)
	}

	dst := newEnv(t)
	dcookie := readyCookie(t, dst)
	code, body, _ = dst.req(t, "POST", "/admin/api/config/import", dcookie, string(envelope.Data))
	if code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if !strings.Contains(body, `"created":2`) { // 渠道 + 令牌
		t.Fatalf("import result: %s", body)
	}
	if code, cbody, _ := dst.req(t, "GET", "/admin/api/channels/1", dcookie, ""); code != http.StatusOK || !strings.Contains(cbody, `"preset_id":"bigmodel"`) {
		t.Fatalf("import should keep preset_id: %d %s", code, cbody)
	}
	var ch store.Channel
	if err := dst.db.Where("name = ?", "export-me").First(&ch).Error; err != nil {
		t.Fatalf("imported channel missing: %v", err)
	}
	if ch.BaseURL != "https://example.com" || ch.Priority != 2 || ch.Weight != 5 || !ch.SupportsResponses || ch.Timeouts.ConnectSec == nil || *ch.Timeouts.ConnectSec != 5 {
		t.Fatalf("imported channel = %+v", ch)
	}
	if ch.ModelMapping["pub"] != "up" {
		t.Fatalf("mapping not imported: %v", ch.ModelMapping)
	}
	var keys int64
	dst.db.Model(&store.ChannelKey{}).Where("channel_id = ?", ch.ID).Count(&keys)
	if keys != 0 {
		t.Fatalf("import must not fabricate keys, got %d", keys)
	}
	var token store.Token
	if err := dst.db.Where("name = ?", "exported-token").First(&token).Error; err != nil {
		t.Fatalf("imported token missing: %v", err)
	}
	if token.Enabled || token.Quota != 1000 || len(token.AllowedModels) != 1 || len(token.KeyHash) != 64 {
		t.Fatalf("imported token = %+v", token)
	}
	var fb store.ModelFallback
	if err := dst.db.Where("model = ?", "pub").First(&fb).Error; err != nil || fb.Fallbacks[0] != "backup" {
		t.Fatalf("fallback not imported: %v %+v", err, fb)
	}

	// 幂等：再次导入应 update 渠道、skip 令牌，不产生重复
	code, body, _ = dst.req(t, "POST", "/admin/api/config/import", dcookie, string(envelope.Data))
	if code != http.StatusOK {
		t.Fatalf("re-import: %d %s", code, body)
	}
	// updated = 渠道 1 + 降级链 1 + 设置 1；令牌按名称已存在 → skipped
	if !strings.Contains(body, `"created":0`) || !strings.Contains(body, `"updated":3`) || !strings.Contains(body, `"skipped":1`) {
		t.Fatalf("re-import should be idempotent: %s", body)
	}
	var channels int64
	dst.db.Model(&store.Channel{}).Where("name = ?", "export-me").Count(&channels)
	if channels != 1 {
		t.Fatalf("duplicate channel after re-import: %d", channels)
	}

	// 非法渠道整体拒绝，不留半成品
	dst.req(t, "POST", "/admin/api/config/import", dcookie, `{"version":1,"channels":[{"name":"broken","type":"x","base_url":"not-a-url","models":[]}]}`)
	var broken int64
	dst.db.Model(&store.Channel{}).Where("name = ?", "broken").Count(&broken)
	if broken != 0 {
		t.Fatal("invalid import must not persist anything")
	}
	if code, body, _ := dst.req(t, "POST", "/admin/api/config/import", dcookie, `{"version":99}`); code != http.StatusBadRequest {
		t.Fatalf("unknown version: %d %s", code, body)
	}
}

func TestChannelsTestAllReportsEveryEnabledChannel(t *testing.T) {
	env := newEnv(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	cookie := readyCookie(t, env)

	if code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie, fmt.Sprintf(`{"name":"good","type":"openai-compatible","base_url":%q,"models":["m"]}`, upstream.URL)); code != http.StatusOK {
		t.Fatalf("create good: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-1"}`); code != http.StatusOK {
		t.Fatalf("add key: %d %s", code, body)
	}
	env.req(t, "POST", "/admin/api/channels", cookie, `{"name":"nokey","type":"openai-compatible","base_url":"https://example.invalid","models":["m"]}`)
	env.req(t, "POST", "/admin/api/channels", cookie, `{"name":"off","type":"openai-compatible","base_url":"https://example.invalid","models":["m"]}`)
	if err := env.db.Model(&store.Channel{}).Where("name = ?", "off").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	code, body, _ := env.req(t, "POST", "/admin/api/channels/test-all", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("test-all: %d %s", code, body)
	}
	var resp struct {
		Data struct{ Items []testAllItem } `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data.Items) != 2 {
		t.Fatalf("items = %d, want 2 (disabled channel excluded): %s", len(resp.Data.Items), body)
	}
	byName := map[string]testAllItem{}
	for _, item := range resp.Data.Items {
		byName[item.Name] = item
	}
	if g := byName["good"]; !g.Ok || g.Result != "success" || g.LatencyMS < 0 {
		t.Fatalf("good = %+v", g)
	}
	if n := byName["nokey"]; n.Ok || n.Result != "error" || !strings.Contains(n.Error, "key") {
		t.Fatalf("nokey = %+v", n)
	}
	if strings.Contains(body, "sk-1") {
		t.Fatalf("test-all leaked key material: %s", body)
	}
}

// ---------- 断言辅助 ----------

func fptr(v float64) *float64 { return &v }
func iptr(v int64) *int64     { return &v }

func assertFloat(t *testing.T, name string, got, want *float64) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Fatalf("%s = %v, want %v", name, got, want)
	case *got != *want:
		t.Fatalf("%s = %v, want %v", name, *got, *want)
	}
}

func assertInt(t *testing.T, name string, got, want *int64) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Fatalf("%s = %v, want %v", name, got, want)
	case *got != *want:
		t.Fatalf("%s = %v, want %v", name, *got, *want)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func TestPresetPriceCatalogAndApply(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie, `{"name":"openai-main","type":"openai","preset_id":"openai","base_url":"https://api.openai.com/v1","models":["gpt-4o","gpt-4o-mini"]}`)
	if code != http.StatusOK {
		t.Fatalf("create: %d %s", code, body)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/usage/prices", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"catalog"`) || !strings.Contains(body, `"as_of"`) || !strings.Contains(body, `"preset_id":"openai"`) {
		t.Fatalf("prices should expose the embedded catalog: %d %s", code, body)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/channels/1", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"source":"catalog"`) {
		t.Fatalf("channel detail should expose catalog-sourced effective prices: %d %s", code, body)
	}
	code, body, _ = env.req(t, "POST", "/admin/api/channels/1/prices/apply-preset", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"created":2`) {
		t.Fatalf("apply preset: %d %s", code, body)
	}
	code, body, _ = env.req(t, "POST", "/admin/api/channels/1/prices/apply-preset", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"created":0`) {
		t.Fatalf("apply preset twice must be idempotent: %d %s", code, body)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/audit-logs?action=price.apply_preset", cookie, "")
	if code != http.StatusOK || !strings.Contains(body, `"total":2`) {
		t.Fatalf("apply preset should be audited: %d %s", code, body)
	}
	// 自定义渠道没有预设价格
	code, body, _ = env.req(t, "POST", "/admin/api/channels", cookie, `{"name":"custom","type":"openai-compatible","base_url":"https://example.com/v1","models":["m"]}`)
	if code != http.StatusOK {
		t.Fatalf("create custom: %d %s", code, body)
	}
	if code, body, _ = env.req(t, "POST", "/admin/api/channels/2/prices/apply-preset", cookie, ""); code != http.StatusBadRequest {
		t.Fatalf("custom channel apply must be rejected: %d %s", code, body)
	}
}
