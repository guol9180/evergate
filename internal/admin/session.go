package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SessionManager 无状态 HMAC 签名 Cookie 会话（本地单管理员场景，不引入第三方 session 库）。
// Cookie 值：base64(username|expiryUnix).base64(HMAC-SHA256(username|expiryUnix, secret))
// 签名密钥 = 配置 secret + 落库的密钥版本：改密时轮换版本即可吊销所有旧会话，重启后仍生效。
type SessionManager struct {
	base   []byte
	ttl    time.Duration
	mu     sync.RWMutex
	secret []byte
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
	return &SessionManager{base: []byte(secret), secret: []byte(secret), ttl: ttl}
}

// Issue 为用户签发会话令牌。
func (s *SessionManager) Issue(username string, now time.Time) string {
	payload := username + "|" + strconv.FormatInt(now.Add(s.ttl).Unix(), 10)
	mac := hmac.New(sha256.New, s.key())
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
	mac := hmac.New(sha256.New, s.key())
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

// Rotate 切换会话密钥版本：此前签发的全部 Cookie 立即失效，当前会话由调用方重签。
func (s *SessionManager) Rotate(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secret = append(append([]byte{}, s.base...), []byte(":"+version)...)
}

func (s *SessionManager) key() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secret
}
