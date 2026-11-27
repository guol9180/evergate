package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 协议分派表必须每类请求恰好命中一项，且 chat 请求不命中任何桥接。
func TestBridgeSpecForMatchesExactlyOneProtocol(t *testing.T) {
	cases := []struct{ protocol, want string }{
		{"chat", ""},
		{"", ""},
		{responsesProtocol, "responses_bridge"},
		{messagesProtocol, "messages_bridge"},
		{geminiProtocol, "gemini_bridge"},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if tc.protocol != "" {
			c.Set("evergate.protocol", tc.protocol)
		}
		got := ""
		if spec := bridgeSpecFor(c); spec != nil {
			got = spec.convertErrType
		}
		if got != tc.want {
			t.Fatalf("protocol %q: got bridge %q, want %q", tc.protocol, got, tc.want)
		}
	}
}
