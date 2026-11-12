package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

func (e *testEnv) addToken(t *testing.T, name string, mutate func(*store.Token)) (*store.Token, string) {
	t.Helper()
	plain := "eg-" + name
	sum := sha256.Sum256([]byte(plain))
	token := &store.Token{Name: name, KeyHash: hex.EncodeToString(sum[:]), Enabled: true}
	if mutate != nil {
		mutate(token)
	}
	if err := e.db.Create(token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token, plain
}

func (e *testEnv) authGateway(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/chat/completions", TokenAuth(e.db), e.HandleChatCompletions)
	engine.POST("/v1/responses", TokenAuth(e.db), e.HandleResponses)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

func authPost(t *testing.T, gateway *httptest.Server, path, token, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, gateway.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(data)
}

func tokenRow(t *testing.T, e *testEnv, id int64) store.Token {
	t.Helper()
	var token store.Token
	if err := e.db.First(&token, id).Error; err != nil {
		t.Fatalf("read token: %v", err)
	}
	return token
}

func logsForToken(t *testing.T, e *testEnv, tokenID int64) []store.RequestLog {
	t.Helper()
	e.logWriter.Close()
	var logs []store.RequestLog
	if err := e.db.Where("token_id = ?", tokenID).Order("id asc").Find(&logs).Error; err != nil {
		t.Fatalf("read logs: %v", err)
	}
	return logs
}

func TestTokenModelWhitelistBlocksBothProtocolsBeforeUpstream(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})
	env.addChannel(t, "only", up.server.URL, 0, []string{"allowed", "denied"}, nil, "sk")
	_, token := env.addToken(t, "model-whitelist", func(token *store.Token) {
		token.AllowedModels = []string{"allowed"}
	})
	gateway := env.authGateway(t)

	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		resp, body := authPost(t, gateway, path, token, `{"model":"denied","input":"hi"}`)
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, `"code":"model_not_allowed"`) {
			t.Fatalf("%s model whitelist response: %d %s", path, resp.StatusCode, body)
		}
	}
	if hits := up.hits.Load(); hits != 0 {
		t.Fatalf("denied model should not reach upstream, hits = %d", hits)
	}
	logs := logsForToken(t, env, 1)
	if len(logs) != 2 {
		t.Fatalf("governance rejection logs = %d, want 2", len(logs))
	}
	for _, log := range logs {
		if log.ErrorType != "model_not_allowed" || log.StatusCode != http.StatusForbidden {
			t.Fatalf("governance log = %#v", log)
		}
	}
}

func TestTokenQuotaPrefetchAndSuccessfulUsageDeduction(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`))
	})
	env.addChannel(t, "quota", up.server.URL, 0, []string{"m"}, nil, "sk")
	token, plain := env.addToken(t, "quota", func(token *store.Token) {
		token.Quota = 10
	})
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: %d %s", resp.StatusCode, body)
	}
	if row := tokenRow(t, env, token.ID); row.UsedQuota != 10 {
		t.Fatalf("used quota = %d, want 10", row.UsedQuota)
	}
	resp, body = authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, `"code":"quota_exceeded"`) || !strings.Contains(body, `"type":"insufficient_quota"`) {
		t.Fatalf("exhausted response: %d %s", resp.StatusCode, body)
	}
	if hits := up.hits.Load(); hits != 1 {
		t.Fatalf("exhausted request should not reach upstream, hits = %d", hits)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 2 {
		t.Fatalf("quota logs = %d, want 2", len(logs))
	}
	if logs[0].TotalTokens != 10 || logs[0].ErrorType != "" || logs[1].ErrorType != "quota_exceeded" {
		t.Fatalf("quota logs = %#v", logs)
	}
}

func TestResponsesBridgeUsageDeductsQuota(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":6,"completion_tokens":4,"total_tokens":10}}`))
	})
	env.addChannel(t, "bridge", up.server.URL, 0, []string{"m"}, nil, "sk")
	token, plain := env.addToken(t, "responses-bridge", func(token *store.Token) {
		token.Quota = 20
	})
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/responses", plain, `{"model":"m","input":"hi"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("responses request: %d %s", resp.StatusCode, body)
	}
	if row := tokenRow(t, env, token.ID); row.UsedQuota != 10 {
		t.Fatalf("responses used quota = %d, want 10", row.UsedQuota)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].TotalTokens != 10 || logs[0].ErrorType != "" {
		t.Fatalf("responses log = %#v", logs)
	}
}

func TestChatStreamFinalUsageDeductsQuota(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":2,\"total_tokens\":10}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
	env.addChannel(t, "stream", up.server.URL, 0, []string{"m"}, nil, "sk")
	token, plain := env.addToken(t, "chat-stream", func(token *store.Token) {
		token.Quota = 15
	})
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "[DONE]") {
		t.Fatalf("chat stream: %d %s", resp.StatusCode, body)
	}
	if row := tokenRow(t, env, token.ID); row.UsedQuota != 10 {
		t.Fatalf("stream used quota = %d, want 10", row.UsedQuota)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].TotalTokens != 10 || logs[0].ErrorType != "" {
		t.Fatalf("stream log = %#v", logs)
	}
}

func TestNativeResponsesStreamCompletedUsageDeductsQuota(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp\"}}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
	env.addResponsesChannel(t, "native", up.server.URL, 0, []string{"m"}, nil, "sk")
	token, plain := env.addToken(t, "responses-stream", func(token *store.Token) {
		token.Quota = 10
	})
	gateway := env.authGateway(t)

	resp, body := authPost(t, gateway, "/v1/responses", plain, `{"model":"m","input":"hi","stream":true}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "response.completed") {
		t.Fatalf("responses stream: %d %s", resp.StatusCode, body)
	}
	if row := tokenRow(t, env, token.ID); row.UsedQuota != 5 {
		t.Fatalf("responses stream used quota = %d, want 5", row.UsedQuota)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 1 || logs[0].TotalTokens != 5 || logs[0].ErrorType != "" {
		t.Fatalf("responses stream log = %#v", logs)
	}
}

func TestMissingUsageAndUpstreamFailureDoNotDeductQuota(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		if strings.Contains(body, `"missing"`) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream failed"}}`))
	})
	env.addChannel(t, "usage", up.server.URL, 0, []string{"missing", "m"}, nil, "sk")
	token, plain := env.addToken(t, "no-usage", func(token *store.Token) {
		token.Quota = 20
	})
	gateway := env.authGateway(t)

	if resp, body := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"missing","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("missing usage request: %d %s", resp.StatusCode, body)
	}
	if resp, _ := authPost(t, gateway, "/v1/chat/completions", plain, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("upstream failure should be exhausted: %d", resp.StatusCode)
	}
	if row := tokenRow(t, env, token.ID); row.UsedQuota != 0 {
		t.Fatalf("no-usage/failure should not charge quota, used = %d", row.UsedQuota)
	}
	logs := logsForToken(t, env, token.ID)
	if len(logs) != 2 || logs[0].ErrorType != "usage_missing" || logs[1].StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("no-usage logs = %#v", logs)
	}
}

func TestClientDisconnectDoesNotDeductQuota(t *testing.T) {
	env := newEnv(t)
	token, _ := env.addToken(t, "disconnect", func(token *store.Token) { token.Quota = 10 })
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request = c.Request.WithContext(ctx)
	c.Set(CtxToken, *token)
	cancel()

	errType, message := env.settleTokenUsage(c, &hitRecord{usageSeen: true, promptTokens: 6, completionTokens: 4})
	if errType != "client_disconnected_no_usage_charge" || message == "" {
		t.Fatalf("disconnect settlement = %q %q", errType, message)
	}
	if row := tokenRow(t, env, token.ID); row.UsedQuota != 0 {
		t.Fatalf("disconnected client should not charge quota, used = %d", row.UsedQuota)
	}
}

func TestConcurrentUsageCannotExceedQuota(t *testing.T) {
	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":6,"completion_tokens":4,"total_tokens":10}}`))
	})
	env.addChannel(t, "concurrent", up.server.URL, 0, []string{"m"}, nil, "sk")
	token, plain := env.addToken(t, "concurrent", func(token *store.Token) {
		token.Quota = 100
	})
	gateway := env.authGateway(t)

	const requests = 24
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"hi %d"}]}`, i)
			req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", strings.NewReader(body))
			if err != nil {
				t.Errorf("request %d: %v", i, err)
				return
			}
			req.Header.Set("Authorization", "Bearer "+plain)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("request %d: %v", i, err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}(i)
	}
	wg.Wait()

	row := tokenRow(t, env, token.ID)
	if row.UsedQuota != 100 {
		t.Fatalf("concurrent used quota = %d, want exactly quota 100", row.UsedQuota)
	}
}
