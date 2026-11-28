package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"evergate/internal/alert"
)

// 设置页「测试发送」带未保存的草稿配置：发到草稿地址、记录标注 draft、已保存配置不变；非法草稿 400；无请求体沿用已保存配置。
func TestAlertsTestAcceptsDraftWithoutSaving(t *testing.T) {
	env := newEnv(t)
	api := secondAPI(t, env, func(o *Options) { o.Alerts = alert.New(env.db, nil, nil) })
	cookie, _ := login(t, api, "admin", "admin123")
	_, _, resp := api.req(t, "PUT", "/admin/api/auth/password", cookie, `{"old_password":"admin123","new_password":"newpass123"}`)
	cookie = sessionCookie(t, resp)

	var savedHits, draftHits atomic.Int32
	count := func(counter *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
	}
	savedServer, draftServer := count(&savedHits), count(&draftHits)
	t.Cleanup(savedServer.Close)
	t.Cleanup(draftServer.Close)

	body := `{"enabled":true,"preset":"generic","url":"` + savedServer.URL + `","events":["channel_down"]}`
	if code, respBody, _ := api.req(t, "PUT", "/admin/api/alerts", cookie, body); code != http.StatusOK {
		t.Fatalf("save alerts: %d %s", code, respBody)
	}

	body = `{"enabled":false,"preset":"wecom","url":"` + draftServer.URL + `","events":[]}`
	code, respBody, _ := api.req(t, "POST", "/admin/api/alerts/test", cookie, body)
	if code != http.StatusOK || !strings.Contains(respBody, `"draft":true`) || !strings.Contains(respBody, `"preset":"wecom"`) {
		t.Fatalf("draft test: %d %s", code, respBody)
	}
	if savedHits.Load() != 0 || draftHits.Load() != 1 {
		t.Fatalf("saved=%d draft=%d hits, want 0/1", savedHits.Load(), draftHits.Load())
	}
	code, respBody, _ = api.req(t, "GET", "/admin/api/alerts", cookie, "")
	if code != http.StatusOK || !strings.Contains(respBody, `"url":"`+savedServer.URL+`"`) || strings.Contains(respBody, draftServer.URL) {
		t.Fatalf("draft test must not change the saved setting: %d %s", code, respBody)
	}

	code, respBody, _ = api.req(t, "POST", "/admin/api/alerts/test", cookie, `{"preset":"slack","url":"https://x"}`)
	if code != http.StatusBadRequest || !strings.Contains(respBody, "invalid_param") {
		t.Fatalf("invalid draft should be 400 invalid_param: %d %s", code, respBody)
	}
	if draftHits.Load() != 1 {
		t.Fatalf("invalid draft must not send, hits=%d", draftHits.Load())
	}

	code, respBody, _ = api.req(t, "POST", "/admin/api/alerts/test", cookie, "")
	if code != http.StatusOK || !strings.Contains(respBody, `"draft":false`) {
		t.Fatalf("test without body should use the saved setting: %d %s", code, respBody)
	}
	if savedHits.Load() != 1 {
		t.Fatalf("saved webhook hits=%d, want 1", savedHits.Load())
	}
}
