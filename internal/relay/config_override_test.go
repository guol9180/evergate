package relay

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"evergate/internal/reqconfig"
	"evergate/internal/store"
)

func overrideHeader(cfg string) map[string]string {
	return map[string]string{"X-EverGate-Config": cfg}
}

func TestRequestConfigRestrictsChannelsAndRetryBudget(t *testing.T) {
	env := newEnv(t)
	upA := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	})
	upB := newUpstream(t, chatOKHandler("b"))
	env.addChannel(t, "a", upA.server.URL, 0, []string{"gpt-x"}, nil, "sk-a")
	env.addChannel(t, "b", upB.server.URL, 1, []string{"gpt-x"}, nil, "sk-b")
	idA, idB := channelIDByName(t, env, "a"), channelIDByName(t, env, "b")
	_, plain := env.addToken(t, "cfg-channels", nil)
	gateway := env.authGateway(t)

	// 只允许 B：A 即使优先级更高也不得被尝试。
	resp, body := postWithHeaders(t, gateway, "/v1/chat/completions", plain,
		overrideHeader(fmt.Sprintf(`{"channels":[%d]}`, idB)),
		`{"model":"gpt-x","messages":[]}`)
	if resp.StatusCode != 200 || !strings.Contains(body, `"b"`) {
		t.Fatalf("channels 覆盖应只命中 B: %d %s", resp.StatusCode, body)
	}
	if hits, _, _ := upA.snapshot(); hits != 0 {
		t.Fatalf("A 不应被尝试, hits=%d", hits)
	}

	// 允许 A+B 但重试预算只有 1 个渠道：A 失败后直接 503，不得继续 B。
	hitsBBefore, _, _ := upB.snapshot()
	resp, body = postWithHeaders(t, gateway, "/v1/chat/completions", plain,
		overrideHeader(fmt.Sprintf(`{"channels":[%d,%d],"retry":{"channels":1}}`, idA, idB)),
		`{"model":"gpt-x","messages":[]}`)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "all_channels_exhausted") {
		t.Fatalf("retry.channels=1 应在 A 失败后耗尽: %d %s", resp.StatusCode, body)
	}
	hitsBAfter, _, _ := upB.snapshot()
	if hitsBAfter != hitsBBefore {
		t.Fatalf("预算用尽不应尝试 B: before=%d after=%d", hitsBBefore, hitsBAfter)
	}
}

func TestRequestConfigValidateChannelTargets(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-only")
	id := channelIDByName(t, env, "only")
	_, plain := env.addToken(t, "cfg-targets", nil)
	gateway := env.authGateway(t)

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"unknown channel", `{"channels":[999999]}`, "channels[0]"},
		{"unsupported model", fmt.Sprintf(`{"channels":[%d]}`, id), "不支持模型"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			model := "gpt-x"
			if c.name == "unsupported model" {
				model = "gpt-other"
			}
			resp, body := postWithHeaders(t, gateway, "/v1/chat/completions", plain, overrideHeader(c.header),
				fmt.Sprintf(`{"model":%q,"messages":[]}`, model))
			if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, c.want) {
				t.Fatalf("status = %d, body = %s, want 400 + %q", resp.StatusCode, body, c.want)
			}
		})
	}
	if hits, _, _ := up.snapshot(); hits != 0 {
		t.Fatalf("非法渠道覆盖不得触达上游, hits=%d", hits)
	}

	// 停用渠道不可作为候选。
	if err := env.db.Model(&store.Channel{}).Where("id = ?", id).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable channel: %v", err)
	}
	resp, body := postWithHeaders(t, gateway, "/v1/chat/completions", plain,
		overrideHeader(fmt.Sprintf(`{"channels":[%d]}`, id)), `{"model":"gpt-x","messages":[]}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "不存在或未启用") {
		t.Fatalf("停用渠道应 400: %d %s", resp.StatusCode, body)
	}
}

func TestRequestConfigRejectsInvalidWithoutUpstream(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-only")
	_, plain := env.addToken(t, "cfg-invalid", nil)
	gateway := env.authGateway(t)

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"bad json", `{"timeouts":`, "JSON"},
		{"unknown api key", `{"api_key":"sk-leak"}`, "api_key"},
		{"unknown model whitelist", `{"allowed_models":["gpt-x"]}`, "allowed_models"},
		{"bad type", `{"retry":{"channels":"2"}}`, "retry.channels"},
		{"out of range", `{"timeouts":{"connect":"61s"}}`, "timeouts.connect"},
		{"overall before first token", `{"timeouts":{"first_token":"60s","overall":"30s"}}`, "timeouts.overall"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, body := postWithHeaders(t, gateway, "/v1/chat/completions", plain, overrideHeader(c.header),
				`{"model":"gpt-x","messages":[]}`)
			if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, c.want) {
				t.Fatalf("status = %d, body = %s, want 400 + %q", resp.StatusCode, body, c.want)
			}
			if !strings.Contains(body, "invalid_request_config") {
				t.Fatalf("应返回 invalid_request_config: %s", body)
			}
		})
	}
	if hits, _, _ := up.snapshot(); hits != 0 {
		t.Fatalf("非法覆盖不得触达上游, hits=%d", hits)
	}
}

func TestRequestConfigCannotBypassTokenGovernance(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 0, []string{"gpt-x", "gpt-allowed"}, nil, "sk-only")
	_, plain := env.addToken(t, "cfg-governance", func(token *store.Token) {
		token.AllowedModels = []string{"gpt-allowed"}
	})
	gateway := env.authGateway(t)

	id := channelIDByName(t, env, "only")
	resp, body := postWithHeaders(t, gateway, "/v1/chat/completions", plain,
		overrideHeader(fmt.Sprintf(`{"channels":[%d],"retry":{"channels":10}}`, id)),
		`{"model":"gpt-x","messages":[]}`)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "model_not_allowed") {
		t.Fatalf("覆盖不得绕过模型白名单: %d %s", resp.StatusCode, body)
	}

	// 未通过鉴权时先 401，不会泄漏配置校验细节。
	resp, body = postWithHeaders(t, gateway, "/v1/chat/completions", "eg-invalid",
		overrideHeader(`{"api_key":"sk-leak"}`), `{"model":"gpt-x","messages":[]}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无效 token 应 401: %d %s", resp.StatusCode, body)
	}
	if hits, _, _ := up.snapshot(); hits != 0 {
		t.Fatalf("治理拒绝不得触达上游, hits=%d", hits)
	}
}

func TestRequestConfigSessionAffinitySwitch(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-only")
	token, plain := env.addToken(t, "cfg-affinity", nil)
	gateway := env.authGateway(t)

	headers := map[string]string{
		"X-Session-Id":      "sess-cfg-off",
		"X-EverGate-Config": `{"session_affinity":false}`,
	}
	if resp, body := postWithHeaders(t, gateway, "/v1/chat/completions", plain, headers, `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if env.opt.Affinity.Len() != 0 {
		t.Fatal("session_affinity=false 不应写入亲和记录")
	}
	if _, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-cfg-off"); ok {
		t.Fatal("关闭亲和的请求不得创建记录")
	}

	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-cfg-off", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if env.opt.Affinity.Len() != 1 {
		t.Fatalf("未关闭亲和的请求应恢复记录, len = %d", env.opt.Affinity.Len())
	}
}

func TestRequestConfigLoggedSummary(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-only")
	id := channelIDByName(t, env, "only")
	token, plain := env.addToken(t, "cfg-log", nil)
	gateway := env.authGateway(t)

	header := fmt.Sprintf(`{"timeouts":{"connect":"15s","first_token":"45s"},"retry":{"keys_per_channel":1},"channels":[%d],"session_affinity":true}`, id)
	if resp, body := postWithHeaders(t, gateway, "/v1/chat/completions", plain, overrideHeader(header), `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) == 0 {
		t.Fatal("请求日志缺失")
	}
	got := logs[len(logs)-1].ConfigOverride
	if got == "" || len(got) > reqconfig.MaxSummaryBytes {
		t.Fatalf("覆盖摘要缺失或超长: %q", got)
	}
	for _, want := range []string{`"connect_ms":15000`, `"first_token_ms":45000`, `"keys_per_channel":1`, `"channels":[`, `"session_affinity":true`} {
		if !strings.Contains(got, want) {
			t.Fatalf("摘要缺少 %q: %s", want, got)
		}
	}
	if strings.Contains(got, "sk-") || strings.Contains(got, `"15s"`) {
		t.Fatalf("摘要不得包含密钥或原始时长串: %s", got)
	}
}

func TestRequestConfigResponsesEndpoint(t *testing.T) {
	env := newEnv(t)
	nativeA := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_a","object":"response","output":[]}`))
	})
	nativeB := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_b","object":"response","output":[]}`))
	})
	env.addResponsesChannel(t, "ra", nativeA.server.URL, 0, []string{"gpt-x"}, nil, "sk-ra")
	env.addResponsesChannel(t, "rb", nativeB.server.URL, 1, []string{"gpt-x"}, nil, "sk-rb")
	idB := channelIDByName(t, env, "rb")
	_, plain := env.addToken(t, "cfg-responses", nil)
	gateway := env.authGateway(t)

	resp, body := postWithHeaders(t, gateway, "/v1/responses", plain,
		overrideHeader(fmt.Sprintf(`{"channels":[%d]}`, idB)), `{"model":"gpt-x","input":"hi"}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "resp_b") {
		t.Fatalf("Responses 渠道覆盖失败: %d %s", resp.StatusCode, body)
	}
	if hits, _, _ := nativeA.snapshot(); hits != 0 {
		t.Fatalf("未选中的渠道不得被调用, hits=%d", hits)
	}
}

func TestApplyTimeoutOverride(t *testing.T) {
	connect, first, overall := 10*time.Second, 30*time.Second, 300*time.Second
	if c, f, o := applyTimeoutOverride(connect, first, overall, nil); c != connect || f != first || o != overall {
		t.Fatalf("nil 覆盖不应改变超时: %v %v %v", c, f, o)
	}
	ov := &reqconfig.Timeouts{}
	c15, f45, o120 := 15*time.Second, 45*time.Second, 120*time.Second
	ov.Connect, ov.FirstToken, ov.Overall = &c15, &f45, &o120
	if c, f, o := applyTimeoutOverride(connect, first, overall, ov); c != c15 || f != f45 || o != o120 {
		t.Fatalf("请求级超时应覆盖渠道/默认值: %v %v %v", c, f, o)
	}
	ov2 := &reqconfig.Timeouts{Connect: &c15}
	if c, f, o := applyTimeoutOverride(connect, first, overall, ov2); c != c15 || f != first || o != overall {
		t.Fatalf("未提供的字段应保持原值: %v %v %v", c, f, o)
	}
}
