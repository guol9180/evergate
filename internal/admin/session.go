package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SessionManager 无状态 HMAC 签名 Cookie 会话（本地单管理员场景，不引入第三方 session 库）。
// Cookie 值：base64(username|expiryUnix).base64(HMAC-SHA256(username|expiryUnix, secret))
type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

// CookieName 会话 Cookie 名。
const CookieName = "evergate_session"

// NewSessionManager secret 为空将 panic（启动时应由 config 保证已生成）。
func NewSessionManager(secret string, ttl time.Duration) *SessionManager {
	if secret == "" {
		panic("admin: session secret must not be empty")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &SessionManager{secret: []byte(secret), ttl: ttl}
}

// Issue 为用户签发会话令牌。
func (s *SessionManager) Issue(username string, now time.Time) string {
	payload := username + "|" + strconv.FormatInt(now.Add(s.ttl).Unix(), 10)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// Verify 校验会话令牌，返回用户名。
func (s *SessionManager) Verify(token string, now time.Time) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed session token")
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode payload: %w", err)
	}
	sigRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode signature: %w", err)
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(payloadRaw)
	if !hmac.Equal(sigRaw, mac.Sum(nil)) {
		return "", fmt.Errorf("signature mismatch")
	}
	payload := string(payloadRaw)
	idx := strings.LastIndex(payload, "|")
	if idx < 0 {
		return "", fmt.Errorf("malformed payload")
	}
	username := payload[:idx]
	expiry, err := strconv.ParseInt(payload[idx+1:], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse expiry: %w", err)
	}
	if now.Unix() > expiry {
		return "", fmt.Errorf("session expired")
	}
	return username, nil
}
