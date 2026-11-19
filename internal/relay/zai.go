package relay

import (
	"encoding/json"
	"net/url"
	"strings"
)

var zaiDisabledThinking = json.RawMessage(`{"type":"disabled"}`)

// applyKtorZAIThinkingPolicy avoids reasoning-only streams on JetBrains clients.
// Ktor waits for visible content and may cancel before ZAI emits the first
// content delta. Explicit client reasoning options always take precedence.
func applyKtorZAIThinkingPolicy(userAgent, baseURL string, stream bool, body []byte) []byte {
	if !stream ||
		!strings.Contains(strings.ToLower(userAgent), "ktor-client") ||
		!isZAICodingBaseURL(baseURL) {
		return body
	}

	var wire map[string]json.RawMessage
	if err := json.Unmarshal(body, &wire); err != nil {
		return body
	}
	for _, field := range []string{"thinking", "reasoning", "reasoning_effort"} {
		if raw, ok := wire[field]; ok && len(raw) > 0 && string(raw) != "null" {
			return body
		}
	}
	var model string
	if err := json.Unmarshal(wire["model"], &model); err != nil ||
		!strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "glm-") {
		return body
	}

	wire["thinking"] = zaiDisabledThinking
	out, err := json.Marshal(wire)
	if err != nil {
		return body
	}
	return out
}

func isZAICodingBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(u.Path), "/api/coding/")
}
