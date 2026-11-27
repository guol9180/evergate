package main

import (
	"bytes"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.Handler) (*http.Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return srv, "http://" + ln.Addr().String()
}

func TestShutdownServerDrainsIdleServer(t *testing.T) {
	srv, _ := newTestServer(t, http.NotFoundHandler())
	var logs bytes.Buffer
	shutdownServer(srv, 2*time.Second, slog.New(slog.NewTextHandler(&logs, nil)))
	if logs.Len() != 0 {
		t.Fatalf("idle shutdown should not warn, got %q", logs.String())
	}
}

// 停机超时属于可预期情况：记录告警、强制关闭残留连接，且不把错误抛给启动流程。
func TestShutdownServerClosesHungConnectionsOnTimeout(t *testing.T) {
	started := make(chan struct{})
	srv, url := newTestServer(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done() // 模拟永不返回的长连接，必须靠 Close 打断
	}))
	go func() { _, _ = http.Get(url) }()
	<-started

	var logs bytes.Buffer
	begin := time.Now()
	shutdownServer(srv, 200*time.Millisecond, slog.New(slog.NewTextHandler(&logs, nil)))
	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Fatalf("shutdown took %s, hung handler was not closed", elapsed)
	}
	if logs.Len() == 0 {
		t.Fatal("timeout shutdown should log a warning")
	}
}
