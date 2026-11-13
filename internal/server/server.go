// Package server 组装 Gin 路由、中间件与基础端点。
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/metrics"
	"evergate/internal/telemetry"
)

// Options 是服务装配依赖。
type Options struct {
	BasePath string   // 可选子路径，如 "/eg"
	DB       *gorm.DB // healthz 探活用，可为 nil
	Logger   *slog.Logger
	// SetupRoutes 业务路由注册钩子（relay / admin 等），入参为挂载在 BasePath 下的路由组。
	SetupRoutes func(r gin.IRoutes)
	// WebFS 嵌入的前端构建产物（nil 表示不服务管理台）。
	WebFS fs.FS
	// Metrics 为 /metrics 指标端点；nil 表示不挂载。
	Metrics *metrics.Registry
	// Tracer 为可选 OTLP 链路导出；nil 或禁用态不产生 span。
	Tracer *telemetry.Tracer
}

// New 构建带 recovery 与请求日志的 Gin 引擎。
func New(opts Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode) // 运行日志统一走 slog，关闭 Gin 调试输出
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	r := gin.New()
	// IP 白名单必须基于直连地址；未提供受信代理配置前不信任 X-Forwarded-For。
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Recovery()) // 红线：每个 handler 必须有 recovery
	r.Use(requestLogger(logger))
	r.Use(traceMiddleware(opts.Tracer))

	var root gin.IRoutes = r
	if opts.BasePath != "" {
		root = r.Group(opts.BasePath)
	}
	root.GET("/healthz", healthz(opts.DB))
	if opts.Metrics != nil {
		// 仅 loopback 可抓取（T26 验收 #1）；路由本身仍受全局 recovery 保护。
		root.GET("/metrics", gin.WrapH(opts.Metrics))
	}
	if opts.SetupRoutes != nil {
		opts.SetupRoutes(root)
	}
	if opts.WebFS != nil {
		r.NoRoute(spaHandler(opts.WebFS))
	}
	return r
}

// traceMiddleware 为管理台/API 请求生成服务端 span；/healthz 与 /metrics 跳过。
func traceMiddleware(tracer *telemetry.Tracer) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if tracer == nil || !tracer.Enabled() || strings.HasSuffix(path, "/healthz") || strings.HasSuffix(path, "/metrics") {
			c.Next()
			return
		}
		ctx, span := tracer.Start(c.Request.Context(), "http.request", telemetry.KindServer,
			telemetry.String("http.method", c.Request.Method),
		)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		span.SetAttr(telemetry.String("http.route", c.FullPath()))
		status := c.Writer.Status()
		span.SetAttr(telemetry.Int("http.status_code", int64(status)))
		var spanErr error
		if status >= http.StatusInternalServerError {
			spanErr = fmt.Errorf("http %d", status)
		}
		span.End(spanErr)
	}
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
