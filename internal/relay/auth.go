package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/store"
)

// CtxTokenID / CtxTokenName 下游令牌信息写入 gin Context 的键。
const (
	CtxTokenID   = "evergate.token_id"
	CtxTokenName = "evergate.token_name"
	CtxToken     = "evergate.token"
)

// TokenAuth 下游令牌鉴权中间件：Bearer <key> → SHA-256 查库比对（任务书 4.3 tokens）。
// 校验启用状态、过期时间与 IP 白名单；通过后把令牌快照放进 Context 供 relay 治理使用。
// logWriter 可选：治理拒绝也会进入请求日志，但永不记录 token 明文。
func TokenAuth(db *gorm.DB, logWriter ...*LogWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(raw, "Bearer ")) == "" {
			abortUnauthorized(c, "missing bearer token")
			return
		}
		key := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
		sum := sha256.Sum256([]byte(key))
		hash := hex.EncodeToString(sum[:])

		var token store.Token
		err := db.Where("key_hash = ?", hash).First(&token).Error
		if err != nil {
			abortUnauthorized(c, "invalid token")
			return
		}
		now := time.Now().UTC()
		if !token.Enabled {
			rejectToken(c, logWriter, token, http.StatusUnauthorized, "token_disabled", "token disabled")
			return
		}
		if token.ExpiredAt != nil && now.After(*token.ExpiredAt) {
			rejectToken(c, logWriter, token, http.StatusUnauthorized, "token_expired", "token expired")
			return
		}
		if !clientIPAllowed(c.ClientIP(), token.AllowedIPs) {
			rejectToken(c, logWriter, token, http.StatusForbidden, "ip_not_allowed", "client ip is not allowed")
			return
		}
		c.Set(CtxToken, token)
		c.Set(CtxTokenID, token.ID)
		c.Set(CtxTokenName, token.Name)
		c.Next()
	}
}

func rejectToken(c *gin.Context, writers []*LogWriter, token store.Token, status int, code, message string) {
	for _, writer := range writers {
		if writer == nil {
			continue
		}
		writer.Submit(&store.RequestLog{
			TokenID: token.ID, TokenName: token.Name, StatusCode: status,
			ErrorType: code, ErrorMessage: message, IP: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		})
	}
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"message": message, "type": "invalid_request_error", "code": code},
	})
}

func clientIPAllowed(clientIP string, rules []string) bool {
	if len(rules) == 0 {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return false
	}
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(rule); err == nil {
			if network.Contains(ip) {
				return true
			}
			continue
		}
		if allowed := net.ParseIP(rule); allowed != nil && allowed.Equal(ip) {
			return true
		}
	}
	return false
}

func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{"message": msg, "type": "invalid_request_error", "code": "invalid_token"},
	})
}

// tokenFromContext 取出令牌身份（未鉴权时为零值）。
func tokenFromContext(c *gin.Context) (int64, string) {
	id, _ := c.Get(CtxTokenID)
	name, _ := c.Get(CtxTokenName)
	idv, _ := id.(int64)
	namev, _ := name.(string)
	return idv, namev
}

func tokenSnapshotFromContext(c *gin.Context) *store.Token {
	value, _ := c.Get(CtxToken)
	token, _ := value.(store.Token)
	if token.ID == 0 {
		return nil
	}
	return &token
}
