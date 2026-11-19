package relay

import (
	"net/http"
	"strings"
	"testing"
)

func TestOpenCodeSessionForRequest(t *testing.T) {
	const baseURL = "https://opencode.ai/inference/go/openai/v1"
	tests := []struct {
		name    string
		baseURL string
		headers http.Header
		want    string
	}{
		{
			name:    "explicit OpenCode session wins",
			baseURL: baseURL,
			headers: http.Header{
				"X-Opencode-Session": {"opencode-direct"},
				"X-Session-Id":       {"generic-session"},
			},
			want: "opencode-direct",
		},
		{
			name:    "generic session is mapped",
			baseURL: baseURL,
			headers: http.Header{"X-Session-Id": {"conversation-42"}},
			want:    "conversation-42",
		},
		{
			name:    "missing session is generated",
			baseURL: baseURL,
			want:    "evergate-",
		},
		{
			name:    "non OpenCode channel is untouched",
			baseURL: "https://api.example.com/v1",
			headers: http.Header{"X-Session-Id": {"conversation-42"}},
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := openCodeSessionForRequest(tt.baseURL, tt.headers)
			if tt.want == "evergate-" {
				if !strings.HasPrefix(got, tt.want) || len(got) != len("evergate-")+32 {
					t.Fatalf("generated session = %q", got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("session = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlaygroundOpenCodeForwardsSessionHeader(t *testing.T) {
	env := newEnv(t)
	var gotSession string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get(openCodeSessionHeader)
		if !strings.HasSuffix(r.URL.Path, "/inference/go/openai/v1/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		chatOKHandler("opencode ok")(auth, body, w, r)
	})
	env.addChannel(t, "opencode", up.server.URL+"/inference/go/openai/v1", 0, []string{"glm-5.3-flash"}, nil, "sk-opencode")

	rec, envelope := env.playgroundWithHeaders(t,
		`{"protocol":"chat","model":"glm-5.3-flash","prompt":"hello","stream":false}`,
		map[string]string{"X-Session-Id": "playground-stable"},
	)
	if rec.Code != http.StatusOK || !envelope.OK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotSession != "playground-stable" {
		t.Fatalf("x-opencode-session = %q", gotSession)
	}
}

func TestOpenCodeSessionIsNotSentToOtherChannels(t *testing.T) {
	env := newEnv(t)
	var gotSession string
	up := newUpstream(t, func(auth, body string, w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get(openCodeSessionHeader)
		chatOKHandler("regular ok")(auth, body, w, r)
	})
	env.addChannel(t, "regular", up.server.URL, 0, []string{"gpt-x"}, nil, "sk-regular")

	rec, envelope := env.playgroundWithHeaders(t,
		`{"protocol":"chat","model":"gpt-x","prompt":"hello","stream":false}`,
		map[string]string{"X-Session-Id": "regular-session"},
	)
	if rec.Code != http.StatusOK || !envelope.OK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotSession != "" {
		t.Fatalf("regular channel received %s=%q", openCodeSessionHeader, gotSession)
	}
}
