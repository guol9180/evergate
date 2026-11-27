// Package httpx 提供管理台与调试接口共用的响应信封和文本截断工具。
// 信封固定为 {"ok":true,"data":...} 或 {"ok":false,"code":...,"message":...}：
// 失败必须带稳定 code，前端按 code 本地化，message 只作兜底。
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OK 写入成功信封。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

// FailCode 写入失败信封；code 为稳定错误码，供前端按 code 本地化。
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
