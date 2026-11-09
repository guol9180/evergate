package relay

import (
	"crypto/sha256"
	"encoding/hex"
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
)

// TokenAuth 下游令牌鉴权中间件：Bearer <key> → SHA-256 查库比对（任务书 4.3 tokens）。
// 校验启用状态与过期时间；通过后把令牌身份放进 Context 供 relay 与日志使用。
func TokenAuth(db *gorm.DB) gin.HandlerFunc {
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
		err := db.Where("key_hash = ? AND enabled = ?", hash, true).First(&token).Error
		if err != nil {
			abortUnauthorized(c, "invalid token")
			return
		}
		if token.ExpiredAt != nil && time.Now().After(*token.ExpiredAt) {
			abortUnauthorized(c, "token expired")
			return
		}
		c.Set(CtxTokenID, token.ID)
		c.Set(CtxTokenName, token.Name)
		c.Next()
	}
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
