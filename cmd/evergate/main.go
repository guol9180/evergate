// evergate 是唯一入口：加载配置 → 迁移 DB → 启动 HTTP。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/admin"
	"evergate/internal/config"
	"evergate/internal/health"
	"evergate/internal/pool"
	"evergate/internal/relay"
	"evergate/internal/router"
	"evergate/internal/server"
	"evergate/internal/store"
	"evergate/web"
)

// version 由构建注入（make build 用 -ldflags 覆盖），开发态为 dev。
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("evergate exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "evergate.yml", "引导配置文件路径")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if _, statErr := os.Stat(*cfgPath); errors.Is(statErr, fs.ErrNotExist) {
		logger.Warn("config file not found, using defaults", slog.String("config", *cfgPath))
	}

	// 首次启动：生成空缺的 master_key / session_secret 并写回配置文件（任务书 4.7）。
	secretsGenerated := false
	if cfg.Security.MasterKey == "" {
		key, err := store.GenerateRandomKey()
		if err != nil {
			return fmt.Errorf("generate master key: %w", err)
		}
		cfg.Security.MasterKey = key
		secretsGenerated = true
	}
	if cfg.Security.SessionSecret == "" {
		key, err := store.GenerateRandomKey()
		if err != nil {
			return fmt.Errorf("generate session secret: %w", err)
		}
		cfg.Security.SessionSecret = key
		secretsGenerated = true
	}
	if secretsGenerated {
		if err := cfg.Persist(*cfgPath); err != nil {
			return fmt.Errorf("persist generated secrets: %w", err)
		}
		logger.Warn("generated missing secrets and wrote back to config", slog.String("config", *cfgPath))
	}

	db, err := store.Open(cfg.Storage.SQLitePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db }() // 连接随进程退出关闭；T2 起模型迁移在此前完成
	if err := store.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	created, err := store.EnsureAdmin(db, store.DefaultAdminUsername, store.DefaultAdminPassword)
	if err != nil {
		return fmt.Errorf("ensure default admin: %w", err)
	}
	if created {
		logger.Warn("default admin created, must change password on first login",
			slog.String("username", store.DefaultAdminUsername),
			slog.String("password", store.DefaultAdminPassword),
		)
	}

	// 全局 UTC 时钟（AGENTS 红线：所有时间以 UTC 存储）
	nowFn := func() time.Time { return time.Now().UTC() }
	poolMgr := pool.NewManager(
		cfg.Defaults.Cooldown.FailThreshold,
		pool.ScheduleFrom(cfg.Defaults.Cooldown.Base.Duration, cfg.Defaults.Cooldown.Max.Duration),
		nowFn,
	)
	if err := health.RestoreCooldowns(db, poolMgr); err != nil {
		logger.Warn("restore key cooldown states failed", slog.Any("error", err))
	}
	stopPersist := health.StartCooldownPersister(db, poolMgr, logger, 5*time.Second)
	defer stopPersist()

	logWriter := relay.NewLogWriter(db, 256, logger)
	defer logWriter.Close()

	routerInst := router.New(time.Now().UnixNano())
	relayHandler := relay.New(relay.Options{
		DB:        db,
		Router:    routerInst,
		Pool:      poolMgr,
		MasterKey: cfg.Security.MasterKey,
		Defaults:  cfg.Defaults,
		Logger:    logger,
		Now:       nowFn,
		LogWriter: logWriter,
	})
	adminAPI := admin.New(admin.Options{
		DB:        db,
		MasterKey: cfg.Security.MasterKey,
		Pool:      poolMgr,
		Session:   admin.NewSessionManager(cfg.Security.SessionSecret, 24*time.Hour),
	})

	engine := server.New(server.Options{
		BasePath: cfg.Server.BasePath,
		DB:       db,
		Logger:   logger,
		SetupRoutes: func(r gin.IRoutes) {
			r.POST("/v1/chat/completions", relay.TokenAuth(db), relayHandler.HandleChatCompletions)
			r.GET("/v1/models", relay.TokenAuth(db), relayHandler.HandleModels)
			adminAPI.RegisterRoutes(r)
		},
		WebFS: web.DistFS,
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		// 不设 WriteTimeout：SSE 长流式不能被全局写超时打断（任务书 4.5⑥）。
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	logger.Info("evergate started",
		slog.String("version", version),
		slog.String("addr", addr),
		slog.String("base_path", cfg.Server.BasePath),
		slog.String("db", cfg.Storage.SQLitePath),
	)

	// Windows 下以 Ctrl+C（SIGINT）为主，SIGTERM 兜底；最长 30s 排空（任务书 4.9）。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining requests (max 30s)")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("evergate stopped")
	return nil
}
