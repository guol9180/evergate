package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"evergate/internal/affinity"
	"evergate/internal/router"
	"evergate/internal/store"
)

// postWithSession 通过带 TokenAuth 的公共 HTTP 边界发起请求，可附加 X-Session-Id。
func postWithSession(t *testing.T, gateway *httptest.Server, path, token, session, body string) (*http.Response, string) {
	t.Helper()
	headers := map[string]string{}
	if session != "" {
		headers["X-Session-Id"] = session
	}
	return postWithHeaders(t, gateway, path, token, headers, body)
}

// postWithHeaders 通过带 TokenAuth 的公共 HTTP 边界发起请求。
func postWithHeaders(t *testing.T, gateway *httptest.Server, path, token string, headers map[string]string, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, gateway.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(data)
}

func chatOKHandler(content string) func(auth, body string, w http.ResponseWriter, r *http.Request) {
	return func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-affinity","choices":[{"message":{"role":"assistant","content":"` + content + `"}}]}`))
	}
}

func TestSessionAffinityStickyChannelAndKey(t *testing.T) {
	env := newEnv(t)
	upA := newUpstream(t, chatOKHandler("a"))
	upB := newUpstream(t, chatOKHandler("b"))
	env.addChannel(t, "a", upA.server.URL, 1, []string{"gpt-x"}, nil, "sk-a1", "sk-a2")
	env.addChannel(t, "b", upB.server.URL, 1, []string{"gpt-x"}, nil, "sk-b1", "sk-b2")
	token, plain := env.addToken(t, "affinity-sticky", nil)
	gateway := env.authGateway(t)

	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-sticky", `{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != 200 {
		t.Fatalf("first request status = %d, body = %s", resp.StatusCode, body)
	}
	entry, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-sticky")
	if !ok || entry.ChannelID == 0 || entry.KeyID == 0 {
		t.Fatalf("首次成功应写入亲和记录, got %+v %v", entry, ok)
	}

	for i := 0; i < 4; i++ {
		if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-sticky", `{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != 200 {
			t.Fatalf("request %d status = %d, body = %s", i+2, resp.StatusCode, body)
		}
	}

	hitsA, authsA, _ := upA.snapshot()
	hitsB, authsB, _ := upB.snapshot()
	var gotHits int64
	var gotAuths []string
	switch entry.ChannelID {
	case channelIDByName(t, env, "a"):
		gotHits, gotAuths = hitsA, authsA
	case channelIDByName(t, env, "b"):
		gotHits, gotAuths = hitsB, authsB
	default:
		t.Fatalf("亲和记录指向未知渠道 %d", entry.ChannelID)
	}
	if gotHits != 5 {
		t.Fatalf("粘性渠道应命中 5 次, got %d (a=%d b=%d)", gotHits, hitsA, hitsB)
	}
	if hitsA+hitsB != 5 {
		t.Fatalf("非粘性渠道不应被命中: a=%d b=%d", hitsA, hitsB)
	}
	for _, auth := range gotAuths {
		if auth != gotAuths[0] {
			t.Fatalf("同一 session 应固定同一 key, got %v", gotAuths)
		}
	}
}

func TestSessionAffinityTokenIsolation(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 1, []string{"gpt-x"}, nil, "sk-only")
	token1, plain1 := env.addToken(t, "affinity-token-1", nil)
	token2, plain2 := env.addToken(t, "affinity-token-2", nil)
	gateway := env.authGateway(t)

	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain1, "shared-session", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("token1 status = %d, body = %s", resp.StatusCode, body)
	}
	if _, ok := env.opt.Affinity.Lookup(token1.ID, "gpt-x", "shared-session"); !ok {
		t.Fatal("token1 应写入亲和")
	}
	if _, ok := env.opt.Affinity.Lookup(token2.ID, "gpt-x", "shared-session"); ok {
		t.Fatal("不同下游令牌即使 session 相同也不能复用记录")
	}
	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain2, "shared-session", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("token2 status = %d, body = %s", resp.StatusCode, body)
	}
}

func TestSessionAffinityFailureFailsOverAndRebinds(t *testing.T) {
	env := newEnv(t)
	var failA atomic.Bool
	upA := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if failA.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
			return
		}
		chatOKHandler("a")(auth, body, w, r)
	})
	upB := newUpstream(t, chatOKHandler("b"))
	env.addChannel(t, "a", upA.server.URL, 1, []string{"gpt-x"}, nil, "sk-a")
	token, plain := env.addToken(t, "affinity-failover", nil)
	gateway := env.authGateway(t)

	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-fail", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("first status = %d, body = %s", resp.StatusCode, body)
	}
	first, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-fail")
	if !ok || first.ChannelID != channelIDByName(t, env, "a") {
		t.Fatalf("首次应粘到渠道 a, got %+v %v", first, ok)
	}

	failA.Store(true)
	env.addChannel(t, "b", upB.server.URL, 1, []string{"gpt-x"}, nil, "sk-b")
	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-fail", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 || !strings.Contains(body, `"b"`) {
		t.Fatalf("粘性渠道失败应切换到 b: %d %s", resp.StatusCode, body)
	}
	second, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-fail")
	if !ok || second.ChannelID != channelIDByName(t, env, "b") {
		t.Fatalf("成功切换后应更新亲和到 b, got %+v %v", second, ok)
	}

	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-fail", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("third status = %d, body = %s", resp.StatusCode, body)
	}
	hitsA, _, _ := upA.snapshot()
	hitsB, _, _ := upB.snapshot()
	if hitsA != 2 || hitsB != 2 {
		t.Fatalf("第三次应优先命中 b: a=%d b=%d", hitsA, hitsB)
	}
}

func TestSessionAffinityKeyCooldownFallsBackWithinChannel(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 1, []string{"gpt-x"}, nil, "sk-k1", "sk-k2")
	token, plain := env.addToken(t, "affinity-key", nil)
	gateway := env.authGateway(t)

	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-key", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("first status = %d, body = %s", resp.StatusCode, body)
	}
	first, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-key")
	if !ok {
		t.Fatal("首次应写入亲和")
	}
	env.pool.StateMachine().OnRateLimited(first.KeyID, time.Minute)

	if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-key", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("粘性 key 冷却后应回退同渠道其他 key: %d %s", resp.StatusCode, body)
	}
	second, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-key")
	if !ok || second.ChannelID != first.ChannelID || second.KeyID == first.KeyID {
		t.Fatalf("应更新为同渠道另一 key, first=%+v second=%+v ok=%v", first, second, ok)
	}
	_, auths, _ := up.snapshot()
	if len(auths) != 2 || auths[0] == auths[1] {
		t.Fatalf("两次请求应使用不同 key, got %v", auths)
	}
}

func TestSessionAffinityInvalidHeaderFallsBack(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 1, []string{"gpt-x"}, nil, "sk-only")
	_, plain := env.addToken(t, "affinity-invalid", nil)
	gateway := env.authGateway(t)

	for name, session := range map[string]string{
		"empty":      "",
		"whitespace": "   ",
		"overlong":   strings.Repeat("s", 129),
	} {
		t.Run(name, func(t *testing.T) {
			if resp, body := postWithSession(t, gateway, "/v1/chat/completions", plain, session, `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
				t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
			}
			if env.opt.Affinity.Len() != 0 {
				t.Fatalf("非法 session 不应写亲和, len = %d", env.opt.Affinity.Len())
			}
		})
	}
}

func TestSessionAffinityExpiry(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, chatOKHandler("ok"))
	env.addChannel(t, "only", up.server.URL, 1, []string{"gpt-x"}, nil, "sk-only")
	token, plain := env.addToken(t, "affinity-ttl", nil)
	gateway := env.authGateway(t)

	if resp, _ := postWithSession(t, gateway, "/v1/chat/completions", plain, "sess-ttl", `{"model":"gpt-x","messages":[]}`); resp.StatusCode != 200 {
		t.Fatalf("first status = %d", resp.StatusCode)
	}
	if _, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-ttl"); !ok {
		t.Fatal("TTL 内应命中")
	}
	env.clock.Advance(11 * time.Minute)
	if _, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-ttl"); ok {
		t.Fatal("TTL 过期后应回退普通调度")
	}
}

func TestSessionAffinityResponsesEndpoint(t *testing.T) {
	env := newEnv(t)
	nativeA := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_a","object":"response","output":[]}`))
	})
	nativeB := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_b","object":"response","output":[]}`))
	})
	env.addResponsesChannel(t, "ra", nativeA.server.URL, 1, []string{"gpt-x"}, nil, "sk-ra")
	env.addResponsesChannel(t, "rb", nativeB.server.URL, 1, []string{"gpt-x"}, nil, "sk-rb")
	token, plain := env.addToken(t, "affinity-responses", nil)
	gateway := env.authGateway(t)

	if resp, body := postWithSession(t, gateway, "/v1/responses", plain, "sess-resp", `{"model":"gpt-x","input":"hi"}`); resp.StatusCode != 200 {
		t.Fatalf("first status = %d, body = %s", resp.StatusCode, body)
	}
	entry, ok := env.opt.Affinity.Lookup(token.ID, "gpt-x", "sess-resp")
	if !ok {
		t.Fatal("Responses 入口成功也应写入亲和")
	}
	for i := 0; i < 2; i++ {
		if resp, body := postWithSession(t, gateway, "/v1/responses", plain, "sess-resp", `{"model":"gpt-x","input":"hi"}`); resp.StatusCode != 200 {
			t.Fatalf("request %d status = %d, body = %s", i+2, resp.StatusCode, body)
		}
	}
	hitsA, _, _ := nativeA.snapshot()
	hitsB, _, _ := nativeB.snapshot()
	if hitsA+hitsB != 3 {
		t.Fatalf("总命中应为 3, got a=%d b=%d", hitsA, hitsB)
	}
	switch entry.ChannelID {
	case channelIDByName(t, env, "ra"):
		if hitsA != 3 {
			t.Fatalf("粘性渠道 ra 应命中 3 次, got %d", hitsA)
		}
	case channelIDByName(t, env, "rb"):
		if hitsB != 3 {
			t.Fatalf("粘性渠道 rb 应命中 3 次, got %d", hitsB)
		}
	default:
		t.Fatalf("亲和记录指向未知渠道 %d", entry.ChannelID)
	}
}

func TestSessionAffinityConcurrentSameSession(t *testing.T) {
	env := newEnv(t)
	upA := newUpstream(t, chatOKHandler("a"))
	upB := newUpstream(t, chatOKHandler("b"))
	env.addChannel(t, "a", upA.server.URL, 1, []string{"gpt-x"}, nil, "sk-a")
	env.addChannel(t, "b", upB.server.URL, 1, []string{"gpt-x"}, nil, "sk-b")
	_, plain := env.addToken(t, "affinity-concurrent", nil)
	gateway := env.authGateway(t)

	var wg sync.WaitGroup
	var failures atomic.Int64
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions",
					strings.NewReader(`{"model":"gpt-x","messages":[]}`))
				if err != nil {
					failures.Add(1)
					continue
				}
				req.Header.Set("Authorization", "Bearer "+plain)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Session-Id", "sess-concurrent")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					failures.Add(1)
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					failures.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("并发同 session 请求失败 %d 次", failures.Load())
	}
	hitsA, _, _ := upA.snapshot()
	hitsB, _, _ := upB.snapshot()
	if hitsA+hitsB != 20 {
		t.Fatalf("应完成 20 次上游请求, got a=%d b=%d", hitsA, hitsB)
	}
	if env.opt.Affinity.Len() != 1 {
		t.Fatalf("同一 session 只应有一条亲和记录, len = %d", env.opt.Affinity.Len())
	}
}

func channelIDByName(t *testing.T, env *testEnv, name string) int64 {
	t.Helper()
	var ch store.Channel
	if err := env.db.Where("name = ?", name).First(&ch).Error; err != nil {
		t.Fatalf("find channel %s: %v", name, err)
	}
	return ch.ID
}

func TestPromoteStickyRespectsPriorityGroups(t *testing.T) {
	candidates := []router.Candidate{
		{Channel: &store.Channel{ID: 1, Priority: 0}},
		{Channel: &store.Channel{ID: 2, Priority: 0}},
		{Channel: &store.Channel{ID: 3, Priority: 1}},
	}
	got := promoteSticky(candidates, affinity.Entry{ChannelID: 2})
	if got[0].Channel.ID != 2 {
		t.Fatalf("最高优先级组内应提升粘性渠道, got %d", got[0].Channel.ID)
	}
	for _, cand := range got[1:] {
		if cand.Channel.ID == 2 {
			t.Fatal("候选列表不应重复")
		}
	}
	got = promoteSticky(candidates, affinity.Entry{ChannelID: 3})
	if got[0].Channel.ID != 1 {
		t.Fatalf("低优先级粘性渠道不得越过 priority, got %d", got[0].Channel.ID)
	}
	got = promoteSticky(candidates, affinity.Entry{ChannelID: 99})
	if got[0].Channel.ID != 1 || len(got) != len(candidates) {
		t.Fatalf("未知粘性渠道应原样返回, got %+v", got)
	}
}
