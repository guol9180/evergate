// Package httpx 提供管理台与调试接口共用的响应信封和文本截断工具。
// 信封固定为 {"ok":true,"data":...} 或 {"ok":false,"message":...}，可选稳定 code。
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OK 写入成功信封。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

// Fail 写入失败信封。
func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"ok": false, "message": message})
}

// FailCode 写入带稳定错误码的失败信封，供前端按 code 本地化。
func FailCode(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"ok": false, "code": code, "message": message})
}

// TruncateRunes 按字符数截断，避免切断多字节字符。
func TruncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
