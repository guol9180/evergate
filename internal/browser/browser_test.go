package browser

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminURL(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		basePath string
		want     string
	}{
		{name: "root", port: 8787, basePath: "", want: "http://127.0.0.1:8787/"},
		{name: "base path", port: 8787, basePath: "/eg", want: "http://127.0.0.1:8787/eg/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AdminURL(tt.port, tt.basePath); got != tt.want {
				t.Fatalf("AdminURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenCommandByPlatform(t *testing.T) {
	tests := []struct {
		goos string
		url  string
		name string
		args []string
	}{
		{goos: "windows", url: "http://127.0.0.1:8787/", name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", "http://127.0.0.1:8787/"}},
		{goos: "darwin", url: "http://127.0.0.1:8787/", name: "open", args: []string{"http://127.0.0.1:8787/"}},
		{goos: "linux", url: "http://127.0.0.1:8787/", name: "xdg-open", args: []string{"http://127.0.0.1:8787/"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			cmd := openCommand(tt.goos, tt.url)
			if cmd.name != tt.name || len(cmd.args) != len(tt.args) {
				t.Fatalf("command = %q %v, want %q %v", cmd.name, cmd.args, tt.name, tt.args)
			}
			for i, arg := range tt.args {
				if cmd.args[i] != arg {
					t.Fatalf("arg[%d] = %q, want %q", i, cmd.args[i], arg)
				}
			}
		})
	}
}

func TestWaitUntilReadyOpensAfterHealthz(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("probe path = %q, want /healthz", r.URL.Path)
		}
		if calls == 0 {
			calls++
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	opened := ""
	if err := WaitUntilReady(server.URL, func(url string) error { opened = url; return nil }, 50*time.Millisecond); err != nil {
		t.Fatalf("WaitUntilReady: %v", err)
	}
	if opened != server.URL+"/" && opened != server.URL {
		t.Fatalf("opened = %q", opened)
	}
}
