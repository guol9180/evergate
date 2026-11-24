package admin

import (
	"strings"
	"testing"

	"evergate/internal/store"
)

func TestModelFallbackRulesReplaceHotConfiguration(t *testing.T) {
	env := newEnv(t)
	cookie := readyLogin(t, env)

	code, body, _ := env.req(t, "PUT", "/admin/api/model-fallbacks", cookie, `{"items":[{"model":"gpt-expensive","budget_threshold":1000,"fallbacks":["gpt-standard","gpt-mini"]}]}`)
	if code != 200 || !strings.Contains(body, `"model":"gpt-expensive"`) {
		t.Fatalf("replace fallbacks: %d %s", code, body)
	}
	var rows []store.ModelFallback
	if err := env.db.Order("model asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].BudgetThreshold != 1000 || len(rows[0].Fallbacks) != 2 {
		t.Fatalf("rows = %#v", rows)
	}

	code, body, _ = env.req(t, "PUT", "/admin/api/model-fallbacks", cookie, `{"items":[{"model":"gpt-expensive","budget_threshold":0,"fallbacks":["gpt-mini"]}]}`)
	if code != 200 {
		t.Fatalf("update fallbacks: %d %s", code, body)
	}
	code, body, _ = env.req(t, "GET", "/admin/api/model-fallbacks", cookie, "")
	if code != 200 || !strings.Contains(body, `"fallbacks":["gpt-mini"]`) {
		t.Fatalf("list fallbacks: %d %s", code, body)
	}
}

func TestModelFallbackValidationRejectsDuplicatesAndBadValues(t *testing.T) {
	env := newEnv(t)
	cookie := readyLogin(t, env)

	cases := []string{
		`{"items":[{"model":"","fallbacks":["m"]}]}`,
		`{"items":[{"model":"m","budget_threshold":-1,"fallbacks":["m2"]}]}`,
		`{"items":[{"model":"m","fallbacks":["m"]}]}`,
		`{"items":[{"model":"m","fallbacks":[]}]}`,
		`{"items":[{"model":"m","fallbacks":["a"]},{"model":"m","fallbacks":["b"]}]}`,
	}
	for _, body := range cases {
		code, out, _ := env.req(t, "PUT", "/admin/api/model-fallbacks", cookie, body)
		if code != 400 {
			t.Fatalf("body %s: code = %d out = %s, want 400", body, code, out)
		}
	}
	var count int64
	if err := env.db.Model(&store.ModelFallback{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid writes should not persist: count=%d err=%v", count, err)
	}
}

func TestTokenDisableFallbackFieldRoundTrip(t *testing.T) {
	env := newEnv(t)
	cookie := readyLogin(t, env)
	code, body, _ := env.req(t, "POST", "/admin/api/tokens", cookie, `{"name":"fallback-off","quota":100,"disable_fallback":true}`)
	if code != 200 || !strings.Contains(body, `"disable_fallback":true`) {
		t.Fatalf("create token: %d %s", code, body)
	}
	code, body, _ = env.req(t, "PUT", "/admin/api/tokens/1", cookie, `{"disable_fallback":false}`)
	if code != 200 {
		t.Fatalf("update token: %d %s", code, body)
	}
	var token store.Token
	if err := env.db.First(&token, 1).Error; err != nil {
		t.Fatal(err)
	}
	if token.DisableFallback {
		t.Fatalf("token disable fallback = true, want false")
	}
}

func readyLogin(t *testing.T, env *testEnv) string {
	t.Helper()
	cookie, _ := login(t, env, store.DefaultAdminUsername, "admin123")
	if code, body, _ := env.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`); code != 200 {
		t.Fatalf("change password: %d %s", code, body)
	}
	cookie, _ = login(t, env, store.DefaultAdminUsername, "newpass123")
	return cookie
}
