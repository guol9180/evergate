package relay

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

func TestSSEWriterKeepaliveDoesNotEmitDataFrames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newSSEWriter(c.Writer)
	w.interval = 10 * time.Millisecond
	w.header()

	stop := w.startKeepalive()
	defer stop()

	// stop 同步等待后台 goroutine 退出后，测试才能读取 recorder。
	time.Sleep(60 * time.Millisecond)
	stop()

	body := rec.Body.String()
	if !strings.Contains(body, ": keepalive\n\n") {
		t.Fatalf("keepalive missing from body %q", body)
	}
	if strings.Contains(body, "data:") {
		t.Fatalf("keepalive must not emit data frames: %q", body)
	}
}

func TestChatSSEWriterKeepaliveIsEmptyDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newChatSSEWriter(c.Writer, "m")
	w.interval = 10 * time.Millisecond
	w.header()

	stop := w.startKeepalive()
	defer stop()

	// 等待至少数个 tick；stop 会同步等待后台 goroutine 退出，之后才能安全读取 recorder。
	time.Sleep(60 * time.Millisecond)
	stop()

	body := rec.Body.String()
	if !strings.Contains(body, `"object":"chat.completion.chunk"`) ||
		!strings.Contains(body, `"model":"m"`) ||
		!strings.Contains(body, `"delta":{}`) {
		t.Fatalf("chat keepalive is not a valid empty delta: %q", body)
	}
	for _, forbidden := range []string{`"content"`, `"role"`, `"tool_calls"`, `"finish_reason"`, `"usage"`, `"reasoning_content"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("chat keepalive contains %s: %q", forbidden, body)
		}
	}
}

func TestChatStreamKeepaliveResetsDataOnlyClientTimeout(t *testing.T) {
	oldInterval := keepaliveInterval
	keepaliveInterval = 20 * time.Millisecond
	t.Cleanup(func() { keepaliveInterval = oldInterval })

	env := newEnv(t)
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		time.Sleep(140 * time.Millisecond)
		_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{\"content\":\"late\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"s\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":2,\"total_tokens\":10}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		w.(http.Flusher).Flush()
	})
	env.addChannel(t, "slow-stream", up.server.URL, 0, []string{"m"}, nil, "sk")
	token, plain := env.addToken(t, "ktor-timeout", func(token *store.Token) { token.Quota = 20 })

	client, streamConn := trackedStreamClient(t)
	req, err := http.NewRequest(http.MethodPost, env.authGateway(t).URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ktor-client")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	body := readSSEUntilDone(t, resp, streamConn(), 80*time.Millisecond, func(line string) bool {
		return strings.HasPrefix(line, "data:")
	})
	if !strings.Contains(body, `"content":"late"`) {
		t.Fatalf("late content missing: %s", body)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if row := tokenRow(t, env, token.ID); row.UsedQuota == 10 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("used quota = %d, want 10", tokenRow(t, env, token.ID).UsedQuota)
}

func trackedStreamClient(t *testing.T) (*http.Client, func() net.Conn) {
	t.Helper()
	var (
		connMu sync.Mutex
		conn   net.Conn
	)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, err := (&net.Dialer{}).DialContext(ctx, network, addr)
			if err == nil {
				connMu.Lock()
				conn = c
				connMu.Unlock()
			}
			return c, err
		},
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport}, func() net.Conn {
		connMu.Lock()
		defer connMu.Unlock()
		return conn
	}
}

func readSSEUntilDone(t *testing.T, resp *http.Response, conn net.Conn, timeout time.Duration, reset func(string) bool) string {
	t.Helper()
	if conn == nil {
		t.Fatal("missing client connection")
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadString('\n')
		body.WriteString(line)
		if reset(line) {
			if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
				t.Fatal(err)
			}
		}
		if readErr != nil {
			t.Fatalf("stream timed out before late content: %v\nbody=%s", readErr, body.String())
		}
		if strings.TrimSpace(line) == "data: [DONE]" {
			break
		}
	}
	return body.String()
}
