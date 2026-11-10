package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// spaHandler 服务嵌入的前端构建产物；未知非 API 路径回退 index.html（SPA 路由）。
func spaHandler(webFS fs.FS) gin.HandlerFunc {
	fileServer := http.FileServerFS(webFS) // Go 1.22+
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// API 未知路径返回 404 JSON，不落入 SPA
		if strings.HasPrefix(path, "/v1") || strings.HasPrefix(path, "/admin") || strings.HasPrefix(path, "/healthz") {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{"message": "not found", "type": "invalid_request_error", "code": "not_found"},
			})
			return
		}
		// 资源文件带内容哈希：长缓存；其余（含 index.html）不缓存
		if strings.HasPrefix(path, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache")
		}
		// 静态文件不存在 → 回退 index.html（前端路由由 react-router 接管）
		rel := strings.TrimPrefix(path, "/")
		if rel != "" {
			if _, err := fs.Stat(webFS, rel); err != nil {
				c.Request.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}
