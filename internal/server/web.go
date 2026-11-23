package server

import (
	"bytes"
	"html"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// spaHandler serves the embedded frontend and rewrites the runtime base path
// into index.html. Only resolved API paths receive JSON 404s; frontend routes
// continue to fall back to index.html.
func spaHandler(webFS fs.FS, basePath string) gin.HandlerFunc {
	index, err := fs.ReadFile(webFS, "index.html")
	if err != nil {
		return func(c *gin.Context) {
			http.Error(c.Writer, "embedded index.html is unavailable", http.StatusInternalServerError)
		}
	}
	baseHref := "/"
	if basePath != "" {
		baseHref = html.EscapeString(basePath) + "/"
	}
	index = bytes.Replace(index, []byte(`<base href="./" />`), []byte(`<base href="`+baseHref+`" />`), 1)
	fileServer := http.FileServerFS(webFS)

	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.Header("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(c.Writer, c.Request, "index.html", time.Time{}, bytes.NewReader(index))
	}
	notFound := func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{"message": "not found", "type": "invalid_request_error", "code": "not_found"},
		})
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if basePath != "" {
			switch {
			case path == basePath:
				http.Redirect(c.Writer, c.Request, basePath+"/", http.StatusTemporaryRedirect)
				return
			case path == basePath+"/":
				path = "/"
			case strings.HasPrefix(path, basePath+"/"):
				path = strings.TrimPrefix(path, basePath)
			case path == "/":
				http.Redirect(c.Writer, c.Request, basePath+"/", http.StatusTemporaryRedirect)
				return
			default:
				notFound(c)
				return
			}
		}

		if isAPIPath(path) {
			notFound(c)
			return
		}
		if path == "/" {
			serveIndex(c)
			return
		}
		if strings.HasPrefix(path, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache")
		}
		rel := strings.TrimPrefix(path, "/")
		if _, statErr := fs.Stat(webFS, rel); statErr != nil {
			if strings.HasPrefix(path, "/assets/") {
				c.Status(http.StatusNotFound)
				return
			}
			serveIndex(c)
			return
		}
		c.Request.URL.Path = path
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func isAPIPath(path string) bool {
	return path == "/v1" || strings.HasPrefix(path, "/v1/") ||
		path == "/admin" || strings.HasPrefix(path, "/admin/") ||
		path == "/healthz" || path == "/metrics"
}
