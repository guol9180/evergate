package relay

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"evergate/internal/affinity"
)

const openCodeSessionHeader = "x-opencode-session"

// openCodeSessionForRequest returns the routing session required by OpenCode Go.
// It preserves an existing conversation identifier and creates an opaque fallback
// for clients that do not send one.
func openCodeSessionForRequest(baseURL string, incoming http.Header) string {
	if !isOpenCodeGoBaseURL(baseURL) {
		return ""
	}
	for _, name := range []string{openCodeSessionHeader, "X-Session-Id"} {
		if session := affinity.NormalizeSession(incoming.Get(name)); session != "" {
			return session
		}
	}
	return randomOpenCodeSession()
}

func isOpenCodeGoBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(u.Path), "/inference/go/")
}

func randomOpenCodeSession() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return "evergate-" + hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("evergate-%d", time.Now().UnixNano())
}
