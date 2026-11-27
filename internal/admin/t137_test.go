package admin

import (
	"net/http"
	"strings"
	"testing"

	"evergate/internal/store"
)

// 回归：规则列是 JSON serializer 字段，必须经结构体更新；此前 map 更新写成裸值后整行无法读出。
func TestUpdateTokenRulesStayReadable(t *testing.T) {
	env := newEnv(t)
	cookie := readyCookie(t, env)
	if code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"rules","allowed_models":["a"],"allowed_ips":["10.0.0.0/8"]}`); code != http.StatusOK {
		t.Fatalf("create: %d %s", code, body)
	}
	steps := []struct {
		body  string
		wants []string
	}{
		{`{"allowed_models":["m","n"]}`, []string{`"allowed_models":["m","n"]`, `"allowed_ips":["10.0.0.0/8"]`}},
		{`{"allowed_ips":["127.0.0.1"]}`, []string{`"allowed_models":["m","n"]`, `"allowed_ips":["127.0.0.1"]`}},
		{`{"allowed_models":[],"allowed_ips":[]}`, []string{`"allowed_models":[]`, `"allowed_ips":[]`}},
		{`{"name":"renamed","enabled":false,"quota":5,"rpm_limit":7,"concurrency_limit":2,"disable_fallback":true,"expired_at":"2030-01-01T00:00:00Z"}`,
			[]string{`"name":"renamed"`, `"enabled":false`, `"quota":5`, `"rpm_limit":7`, `"concurrency_limit":2`, `"disable_fallback":true`, `"expired_at":"2030-01-01T00:00:00Z"`}},
		{`{"clear_expired_at":true}`, []string{`"expired_at":null`}},
		// 非法规则被拒绝且不碰已有数据
		{`{"allowed_ips":["localhost"]}`, []string{`"allowed_ips":[]`, `"name":"renamed"`}},
	}
	for i, step := range steps {
		code, body, _ := env.req(t, "PUT", "/admin/api/tokens/1", cookie, step.body)
		wantStatus := http.StatusOK
		if i == len(steps)-1 {
			wantStatus = http.StatusBadRequest
		}
		if code != wantStatus {
			t.Fatalf("step %d %s: %d %s", i, step.body, code, body)
		}
		code, body, _ = env.req(t, "GET", "/admin/api/tokens", cookie, "")
		if code != http.StatusOK {
			t.Fatalf("step %d: list must stay readable after update, got %d %s", i, code, body)
		}
		for _, want := range step.wants {
			if !strings.Contains(body, want) {
				t.Errorf("step %d: list missing %s: %s", i, want, body)
			}
		}
	}
	var raw string
	if err := env.db.Model(&store.Token{}).Select("allowed_models").Where("id = ?", 1).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "[") {
		t.Fatalf("allowed_models must be stored as JSON, got %q", raw)
	}
}
