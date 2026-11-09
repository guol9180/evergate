// Package server 组装 Gin 路由、中间件与基础端点。
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Options 是服务装配依赖。
type Options struct {
	BasePath string   // 可选子路径，如 "/eg"
	DB       *gorm.DB // healthz 探活用，可为 nil
	Logger   *slog.Logger
	// SetupRoutes 业务路由注册钩子（relay / admin 等），入参为挂载在 BasePath 下的路由组。
	SetupRoutes func(r gin.IRoutes)
}

// New 构建带 recovery 与请求日志的 Gin 引擎。
func New(opts Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode) // 运行日志统一走 slog，关闭 Gin 调试输出
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	r := gin.New()
	r.Use(gin.Recovery()) // 红线：每个 handler 必须有 recovery
	r.Use(requestLogger(logger))

	var root gin.IRoutes = r
	if opts.BasePath != "" {
		root = r.Group(opts.BasePath)
	}
	root.GET("/healthz", healthz(opts.DB))
	if opts.SetupRoutes != nil {
		opts.SetupRoutes(root)
	}
	return r
}

// requestLogger 用 slog 记录每条请求（运行日志，区别于入库的请求日志）。
func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http_request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
		)
	}
}

// healthz 是存活检查：数据库可达时返回 200，否则 503。
func healthz(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
		sqlDB, err := db.DB()
		if err == nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			err = sqlDB.PingContext(ctx)
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
