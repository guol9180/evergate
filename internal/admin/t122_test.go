package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"evergate/internal/store"
)

func createChannelForTest(t *testing.T, env *testEnv, cookie, name string) int64 {
	t.Helper()
	code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie,
		`{"name":"`+name+`","type":"openai-compatible","base_url":"https://example.com","models":[]}`)
	if code != http.StatusOK {
		t.Fatalf("create channel %q: %d %s", name, code, body)
	}
	var out struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out.Data.ID
}

// 日志四个入口共用区间校验：start > end 必须 400 end_before_start，不能静默返回空结果。
func TestLogEndpointsRejectEndBeforeStart(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	seedLog(t, env, 200, 10, "m")
	const bad = "start=2026-11-10T00:00:00Z&end=2026-11-09T00:00:00Z"
	for _, path := range []string{"/admin/api/logs", "/admin/api/logs/stats", "/admin/api/logs/models", "/admin/api/logs/export"} {
		code, body, _ := env.req(t, "GET", path+"?"+bad, cookie, "")
		if code != http.StatusBadRequest || !strings.Contains(body, `"end_before_start"`) {
			t.Errorf("%s: want 400 end_before_start, got %d %s", path, code, body)
		}
	}
	// start == end 合法；单边缺省合法；格式错误仍是 invalid_param
	for _, query := range []string{
		"start=2026-11-10T00:00:00Z&end=2026-11-10T00:00:00Z",
		"start=2026-11-10T00:00:00Z",
		"end=2026-11-10T00:00:00Z",
	} {
		if code, body, _ := env.req(t, "GET", "/admin/api/logs?"+query, cookie, ""); code != http.StatusOK {
			t.Errorf("%s should be accepted: %d %s", query, code, body)
		}
	}
	if code, body, _ := env.req(t, "GET", "/admin/api/logs?start=2026-11-10", cookie, ""); code != http.StatusBadRequest || !strings.Contains(body, `"invalid_param"`) {
		t.Fatalf("malformed start should stay invalid_param: %d %s", code, body)
	}
}

func TestConfigExportImportCarriesTokenLimits(t *testing.T) {
	src := newEnv(t)
	cookie := readyCookie(t, src)
	if code, body, _ := src.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"limited","rpm_limit":60,"concurrency_limit":3}`); code != http.StatusOK {
		t.Fatalf("create limited token: %d %s", code, body)
	}
	code, body, _ := src.req(t, "GET", "/admin/api/config/export", cookie, "")
	if code != http.StatusOK {
		t.Fatalf("export: %d %s", code, body)
	}
	if !strings.Contains(body, `"rpm_limit":60`) || !strings.Contains(body, `"concurrency_limit":3`) {
		t.Fatalf("export should carry token limits: %s", body)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatal(err)
	}

	dst := newEnv(t)
	dcookie := readyCookie(t, dst)
	if code, body, _ := dst.req(t, "POST", "/admin/api/config/import", dcookie, string(envelope.Data)); code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	var imported store.Token
	if err := dst.db.Where("name = ?", "limited").First(&imported).Error; err != nil {
		t.Fatal(err)
	}
	if imported.RPMLimit != 60 || imported.ConcurrencyLimit != 3 {
		t.Fatalf("imported token limits = %d/%d, want 60/3", imported.RPMLimit, imported.ConcurrencyLimit)
	}
	// 版本 1 的旧文件没有限流字段：导入不报错，按 0（不限）落地
	if code, body, _ := dst.req(t, "POST", "/admin/api/config/import", dcookie, `{"version":1,"tokens":[{"name":"legacy","quota":5}]}`); code != http.StatusOK {
		t.Fatalf("legacy import should succeed: %d %s", code, body)
	}
	var legacy store.Token
	if err := dst.db.Where("name = ?", "legacy").First(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.RPMLimit != 0 || legacy.ConcurrencyLimit != 0 || legacy.Quota != 5 {
		t.Fatalf("legacy token = %+v", legacy)
	}
	// 负数限流整体拒绝
	if code, body, _ := dst.req(t, "POST", "/admin/api/config/import", dcookie, `{"version":1,"tokens":[{"name":"bad","rpm_limit":-1}]}`); code != http.StatusBadRequest || !strings.Contains(body, `"config_tokens_invalid"`) {
		t.Fatalf("negative limit should be rejected: %d %s", code, body)
	}
}

func TestChannelNameConflictReturnsStableCode(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	first := createChannelForTest(t, env, cookie, "dup")
	second := createChannelForTest(t, env, cookie, "other")

	code, body, _ := env.req(t, "POST", "/admin/api/channels", cookie,
		`{"name":"dup","type":"openai-compatible","base_url":"https://example.com","models":[]}`)
	if code != http.StatusConflict || !strings.Contains(body, `"channel_name_taken"`) || strings.Contains(body, "名称重复？") {
		t.Fatalf("duplicate create: %d %s", code, body)
	}
	// 更新撞名同样稳定码；改回自己的名字不算冲突
	update := func(id int64, name string) (int, string) {
		code, body, _ := env.req(t, "PUT", fmt.Sprintf("/admin/api/channels/%d", id), cookie,
			`{"name":"`+name+`","type":"openai-compatible","base_url":"https://example.com","models":[]}`)
		return code, body
	}
	if code, body := update(second, "dup"); code != http.StatusConflict || !strings.Contains(body, `"channel_name_taken"`) {
		t.Fatalf("rename onto existing name: %d %s", code, body)
	}
	if code, body := update(second, "other"); code != http.StatusOK {
		t.Fatalf("keeping own name should succeed: %d %s", code, body)
	}
	if code, body := update(first, "dup"); code != http.StatusOK {
		t.Fatalf("keeping own name should succeed: %d %s", code, body)
	}
}

func TestDeleteMissingKeyAndTokenReturns404(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	createChannelForTest(t, env, cookie, "c")
	if code, body, _ := env.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-t122"}`); code != http.StatusOK {
		t.Fatalf("add key: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"t"}`); code != http.StatusOK {
		t.Fatalf("create token: %d %s", code, body)
	}
	cases := []struct {
		path string
		code string
	}{
		{"/admin/api/keys/9999", "key_not_found"},
		{"/admin/api/tokens/9999", "token_not_found"},
	}
	for _, tc := range cases {
		if code, body, _ := env.req(t, "DELETE", tc.path, cookie, ""); code != http.StatusNotFound || !strings.Contains(body, `"`+tc.code+`"`) {
			t.Errorf("DELETE %s: want 404 %s, got %d %s", tc.path, tc.code, code, body)
		}
	}
	// 存在的资源正常删除，再删一次变 404
	if code, body, _ := env.req(t, "DELETE", "/admin/api/keys/1", cookie, ""); code != http.StatusOK {
		t.Fatalf("delete existing key: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "DELETE", "/admin/api/tokens/1", cookie, ""); code != http.StatusOK {
		t.Fatalf("delete existing token: %d %s", code, body)
	}
	for _, path := range []string{"/admin/api/keys/1", "/admin/api/tokens/1"} {
		if code, _, _ := env.req(t, "DELETE", path, cookie, ""); code != http.StatusNotFound {
			t.Errorf("second DELETE %s should be 404, got %d", path, code)
		}
	}
}

func TestKeyWeightAndTokenNameValidation(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	createChannelForTest(t, env, cookie, "c")
	if code, body, _ := env.req(t, "POST", "/admin/api/channels/1/keys", cookie, `{"keys":"sk-t122"}`); code != http.StatusOK {
		t.Fatalf("add key: %d %s", code, body)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"t"}`); code != http.StatusOK {
		t.Fatalf("create token: %d %s", code, body)
	}
	weightCases := []struct {
		value int
		ok    bool
	}{{0, false}, {10001, false}, {1, true}, {5, true}, {10000, true}}
	for _, tc := range weightCases {
		weight, ok := tc.value, tc.ok
		code, body, _ := env.req(t, "PUT", "/admin/api/keys/1", cookie, `{"weight":`+strconv.Itoa(weight)+`}`)
		if ok && code != http.StatusOK {
			t.Errorf("weight %d should be accepted: %d %s", weight, code, body)
		}
		if !ok && (code != http.StatusBadRequest || !strings.Contains(body, `"invalid_param"`) || !strings.Contains(body, "weight")) {
			t.Errorf("weight %d should be rejected with weight hint: %d %s", weight, code, body)
		}
	}
	var key store.ChannelKey
	if err := env.db.First(&key, 1).Error; err != nil {
		t.Fatal(err)
	}
	if key.Weight != 10000 {
		t.Fatalf("last accepted weight should persist, got %d", key.Weight)
	}

	nameCases := []struct {
		body string
		ok   bool
	}{
		{`{"name":"   "}`, false},
		{`{"name":""}`, false},
		{`{"name":"` + strings.Repeat("长", 101) + `"}`, false},
		{`{"name":"  renamed  "}`, true},
	}
	for _, tc := range nameCases {
		code, body, _ := env.req(t, "PUT", "/admin/api/tokens/1", cookie, tc.body)
		if tc.ok && code != http.StatusOK {
			t.Errorf("%s should be accepted: %d %s", tc.body, code, body)
		}
		if !tc.ok && (code != http.StatusBadRequest || !strings.Contains(body, `"invalid_param"`) || !strings.Contains(body, "名称")) {
			t.Errorf("%s should be rejected with name hint: %d %s", tc.body, code, body)
		}
	}
	var token store.Token
	if err := env.db.First(&token, 1).Error; err != nil {
		t.Fatal(err)
	}
	if token.Name != "renamed" {
		t.Fatalf("token name should be trimmed, got %q", token.Name)
	}
	if code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"   "}`); code != http.StatusBadRequest || !strings.Contains(body, `"invalid_param"`) {
		t.Fatalf("blank token name on create should be rejected: %d %s", code, body)
	}
}
