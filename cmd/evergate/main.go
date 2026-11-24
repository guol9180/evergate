// evergate 是唯一入口：加载配置 → 迁移 DB → 启动 HTTP。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/admin"
	"evergate/internal/appdir"
	"evergate/internal/applog"
	"evergate/internal/browser"
	"evergate/internal/config"
	"evergate/internal/desktop"
	"evergate/internal/health"
	"evergate/internal/metrics"
	"evergate/internal/pool"
	"evergate/internal/relay"
	"evergate/internal/router"
	"evergate/internal/selfupdate"
	"evergate/internal/server"
	"evergate/internal/settings"
	"evergate/internal/store"
	"evergate/internal/telemetry"
	"evergate/internal/update"
	"evergate/internal/usage"
	"evergate/web"
)

// version 由构建注入（make build 用 -ldflags 覆盖），开发态为 dev。
var version = "dev"
var activeLogPath string

func main() {
	if err := run(); err != nil {
		slog.Error("evergate exited with error", slog.Any("error", err))
		message := err.Error()
		if activeLogPath != "" {
			message += "\n\n日志：" + activeLogPath
		}
		desktop.ShowError("EverGate 启动失败", message)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "", "引导配置文件路径；留空时按平台规则发现")
	autostart := flag.Bool("autostart", false, "由安装器的开机启动项拉起")
	flag.Parse()
	configExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configExplicit = true
		}
	})

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	paths, err := appdir.Discover(*cfgPath, configExplicit)
	if err != nil {
		return fmt.Errorf("resolve config/data paths: %w", err)
	}
	cfg, sqlitePath, err := resolveBootstrapConfig(paths)
	if err != nil {
		return err
	}
	paths.DataDir = filepath.Dir(sqlitePath)
	logger, logPath, logCloser, err := applog.Open(paths.DataDir)
	if err != nil {
		return fmt.Errorf("open application log: %w", err)
	}
	activeLogPath = logPath
	slog.SetDefault(logger)
	defer func() { _ = logCloser.Close() }()

	releaseInstance, primary, err := desktop.AcquireSingleInstance()
	if err != nil {
		return fmt.Errorf("acquire single instance: %w", err)
	}
	if !primary {
		logger.Info("another EverGate instance is running", slog.Bool("autostart", *autostart))
		if !*autostart {
			adminURL := browser.AdminURL(cfg.Server.Port, cfg.Server.BasePath)
			if openErr := browser.WaitUntilReady(adminURL, browser.Open, 100*time.Millisecond); openErr != nil {
				logger.Warn("open existing admin UI failed", slog.Any("error", openErr), slog.String("url", adminURL))
			}
		}
		return nil
	}
	defer releaseInstance()

	if !paths.ConfigExists {
		logger.Warn("config file not found, using defaults", slog.String("config", paths.ConfigPath))
	}
	cfg, secretsGenerated, err := ensureBootstrapSecrets(paths, cfg)
	if err != nil {
		return err
	}
	if secretsGenerated {
		logger.Warn("generated missing secrets and wrote back to config", slog.String("config", paths.ConfigPath))
	}

	// T26：OTLP 默认关闭；仅标准环境变量存在时启用，且只支持 OTLP/HTTP JSON。
	otelCfg := telemetry.FromEnv()
	if otelCfg.Endpoint != "" && telemetry.ProtocolUnsupported() {
		logger.Warn("unsupported OTEL protocol, tracing disabled; only http/json is supported")
		otelCfg = telemetry.Config{}
	}
	otelCfg.ServiceVersion = version
	tracer := telemetry.New(otelCfg, logger)
	if tracer.Enabled() {
		logger.Info("otel tracing enabled", slog.String("endpoint", otelCfg.Endpoint))
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := tracer.Shutdown(ctx); err != nil {
				logger.Warn("otel shutdown failed", slog.Any("error", err))
			}
		}()
	}

	db, err := store.Open(sqlitePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if err := store.MigrateWithBackup(db, sqlitePath, version, logger); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	initialPassword, fromEnv := store.InitialAdminPassword(os.Getenv(store.InitialAdminEnv))
	created, err := store.EnsureAdmin(db, store.DefaultAdminUsername, initialPassword)
	if err != nil {
		return fmt.Errorf("ensure default admin: %w", err)
	}
	if created {
		if fromEnv {
			logger.Warn("initial admin created using password from EVERGATE_INITIAL_ADMIN_PASSWORD",
				slog.String("username", store.DefaultAdminUsername),
				slog.String("must_change_password", "true"),
			)
		} else {
			logInitialAdminPassword(logger, store.DefaultAdminUsername, initialPassword)
		}
	}

	// 全局 UTC 时钟（AGENTS 红线：所有时间以 UTC 存储）
	nowFn := func() time.Time { return time.Now().UTC() }
	settingsMgr, err := settings.Load(db, cfg, nowFn)
	if err != nil {
		return fmt.Errorf("load runtime settings: %w", err)
	}
	usageMgr, err := usage.Load(db)
	if err != nil {
		return fmt.Errorf("load usage pricing: %w", err)
	}
	runtimeDefaults := settingsMgr.Snapshot().ConfigDefaults()
	poolMgr := pool.NewManager(
		runtimeDefaults.Cooldown.FailThreshold,
		pool.ScheduleFrom(runtimeDefaults.Cooldown.Base.Duration, runtimeDefaults.Cooldown.Max.Duration),
		nowFn,
	)
	if err := health.RestoreCooldowns(db, poolMgr); err != nil {
		logger.Warn("restore key cooldown states failed", slog.Any("error", err))
	}
	stopPersist := health.StartCooldownPersister(db, poolMgr, logger, 5*time.Second)
	defer stopPersist()
	stopLogCleaner := settings.StartLogRetentionCleaner(db, settingsMgr, logger, time.Minute)
	defer stopLogCleaner()
	prober := health.NewProber(health.ProberOptions{
		DB: db, MasterKey: cfg.Security.MasterKey, Pool: poolMgr, Now: nowFn,
	})
	stopProbes := health.StartProbes(health.SchedulerOptions{
		DB: db, MasterKey: cfg.Security.MasterKey, Pool: poolMgr,
		Settings: settingsMgr, Logger: logger, Now: nowFn,
	})
	defer stopProbes()
	stopUsageAggregator := usage.StartAggregator(db, logger, time.Minute)
	defer stopUsageAggregator()

	logWriter := relay.NewLogWriter(db, 256, logger)
	defer logWriter.Close()

	metricsReg := metrics.New()
	metricsReg.AddCollector(func() []metrics.Sample {
		return health.ChannelMetricSamples(db, poolMgr)
	})

	routerInst := router.New(time.Now().UnixNano())
	updateAPIURL := os.Getenv("EVERGATE_UPDATE_API_URL")
	if updateAPIURL == "" {
		updateAPIURL = update.DefaultAPIURL
	}
	// npm 仅用于版本发现；标准安装包更新仍从 GitHub Release 获取并校验平台资产。
	var npmRegistries []string
	if customRegistry := os.Getenv("EVERGATE_NPM_REGISTRY"); customRegistry != "" {
		npmRegistries = []string{customRegistry}
	}
	updateChecker := update.NewCheckerWithOptions(update.Options{
		CurrentVersion: version,
		APIURL:         updateAPIURL,
		Registries:     npmRegistries,
		Now:            time.Now,
	})
	restartCh := make(chan struct{})
	var restartOnce sync.Once
	manualRestartCh := make(chan struct{})
	var manualRestartOnce sync.Once
	exitCh := make(chan struct{})
	var exitOnce sync.Once
	updater := selfupdate.New(selfupdate.Options{
		CurrentVersion: version,
		Args:           os.Args[1:],
		Registries:     npmRegistries,
		Latest:         updateChecker.Check,
		Logger:         logger,
		Now:            time.Now,
		RequestRestart: func() { restartOnce.Do(func() { close(restartCh) }) },
	})
	selfupdate.CleanupBackups(updater.Executable(), 1)
	relayHandler := relay.New(relay.Options{
		DB:        db,
		Router:    routerInst,
		Pool:      poolMgr,
		MasterKey: cfg.Security.MasterKey,
		Defaults:  runtimeDefaults,
		Logger:    logger,
		Now:       nowFn,
		LogWriter: logWriter,
		Settings:  settingsMgr,
		Usage:     usageMgr,
		Metrics:   metricsReg,
		Tracer:    tracer,
	})
	adminAPI := admin.New(admin.Options{
		DB:           db,
		MasterKey:    cfg.Security.MasterKey,
		Pool:         poolMgr,
		AllowRemote:  !cfg.Server.ShouldRestrictAdminToLoopback(),
		Now:          nowFn,
		Session:      admin.NewSessionManager(cfg.Security.SessionSecret, 24*time.Hour),
		Update:       updateChecker,
		Settings:     settingsMgr,
		Health:       prober,
		Usage:        usageMgr,
		ConfigPath:   paths.ConfigPath,
		DataDir:      paths.DataDir,
		ConfigSource: paths.Source,
		Playground:   relayHandler.HandlePlayground,
		SelfUpdate:   updater,
		Logger:       logger,
	})

	engine := server.New(server.Options{
		BasePath: cfg.Server.BasePath,
		DB:       db,
		Logger:   logger,
		Metrics:  metricsReg,
		Tracer:   tracer,
		SetupRoutes: func(r gin.IRoutes) {
			r.POST("/v1/chat/completions", relay.TokenAuth(db, logWriter), relayHandler.HandleChatCompletions)
			r.POST("/v1/responses", relay.TokenAuth(db, logWriter), relayHandler.HandleResponses)
			r.POST("/v1/messages", relay.TokenAuth(db, logWriter), relayHandler.HandleMessages)
			r.POST("/v1/messages/count_tokens", relay.TokenAuth(db, logWriter), relayHandler.HandleMessagesCountTokens)
			r.POST("/v1beta/models/*action", relay.RejectQueryKey(), relay.TokenAuth(db, logWriter), relayHandler.HandleGemini)
			r.GET("/v1/models", relay.TokenAuth(db, logWriter), relayHandler.HandleModels)
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
		slog.String("config", paths.ConfigPath),
		slog.String("config_source", paths.Source),
		slog.String("data_dir", paths.DataDir),
		slog.String("db", sqlitePath),
		slog.Bool("autostart", *autostart),
	)
	adminURL := browser.AdminURL(cfg.Server.Port, cfg.Server.BasePath)
	openAdmin := func() {
		if err := browser.WaitUntilReady(adminURL, browser.Open, 100*time.Millisecond); err != nil {
			logger.Warn("open admin UI failed", slog.Any("error", err), slog.String("url", adminURL))
		}
	}
	if shouldOpenBrowser(cfg.Server, *autostart) {
		go func() {
			openAdmin()
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	type runtimeStop struct {
		reason string
		err    error
	}
	runtimeStopped := make(chan runtimeStop, 1)
	go func() {
		select {
		case err := <-errCh:
			desktop.StopTray()
			runtimeStopped <- runtimeStop{reason: "http server error", err: err}
		case <-ctx.Done():
			desktop.StopTray()
			runtimeStopped <- runtimeStop{reason: "shutdown signal"}
		}
	}()

	stopTray, err := desktop.StartTray(desktop.TrayActions{
		OpenAdmin: openAdmin,
		OpenLogs: func() {
			if err := desktop.RevealPath(activeLogPath); err != nil {
				logger.Warn("reveal active log file failed",
					slog.Any("error", err), slog.String("path", activeLogPath))
			}
		},
		Restart: func() {
			manualRestartOnce.Do(func() { close(manualRestartCh) })
		},
		Exit: func() {
			exitOnce.Do(func() { close(exitCh) })
		},
	})
	if err != nil {
		return fmt.Errorf("start system tray: %w", err)
	}
	defer stopTray()

	// Windows 下以 Ctrl+C（SIGINT）为主，SIGTERM 兜底；最长 30s 排空（任务书 4.9）。
	manualRestartRequested := false
	select {
	case stopped := <-runtimeStopped:
		if stopped.err != nil {
			return fmt.Errorf("%s: %w", stopped.reason, stopped.err)
		}
		logger.Info("shutdown signal received, draining requests (max 30s)")
	case <-restartCh:
		logger.Info("self-update restart requested, draining requests (max 30s)")
	case <-manualRestartCh:
		manualRestartRequested = true
		logger.Info("tray restart requested, draining requests (max 30s)")
	case <-exitCh:
		logger.Info("tray exit requested, draining requests (max 30s)")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	releaseInstance()
	if spec, ok := updater.PendingLaunch(); ok {
		if err := selfupdate.Spawn(spec); err != nil {
			logger.Error("launch new version failed", slog.Any("error", err), slog.String("path", spec.Path))
		} else {
			logger.Info("new version launched", slog.String("path", spec.Path))
		}
	} else if manualRestartRequested {
		workingDir, _ := os.Getwd()
		spec := selfupdate.LaunchSpec{
			Path: updater.Executable(), Args: append([]string(nil), os.Args[1:]...), Dir: workingDir,
		}
		if err := selfupdate.Spawn(spec); err != nil {
			logger.Error("restart from tray failed", slog.Any("error", err), slog.String("path", spec.Path))
		} else {
			logger.Info("tray restart launched", slog.String("path", spec.Path))
		}
	}
	logger.Info("evergate stopped")
	return nil
}

func shouldOpenBrowser(server config.Server, autostart bool) bool {
	return !autostart && server.ShouldOpenBrowser()
}

// logInitialAdminPassword 把随机初始密码以醒目分隔线写入日志（stdout + 文件双写）。
// GUI 构建无有效 stdout 时用户可在数据目录 logs/evergate.log 中找到；密码只在此处出现一次。
func logInitialAdminPassword(logger *slog.Logger, username, password string) {
	logger.Warn("==========================================================")
	logger.Warn("EverGate initial admin account created",
		slog.String("username", username),
		slog.String("password", password),
	)
	logger.Warn("This password is shown ONCE at first start and must be changed on first login.")
	logger.Warn("If lost: set EVERGATE_INITIAL_ADMIN_PASSWORD and recreate, or delete the admins row in evergate.db and restart.")
	logger.Warn("==========================================================")
}

func loadBootstrapConfig(paths appdir.Paths) (config.Config, string, bool, error) {
	cfg, sqlitePath, err := resolveBootstrapConfig(paths)
	if err != nil {
		return config.Config{}, "", false, err
	}
	cfg, generated, err := ensureBootstrapSecrets(paths, cfg)
	return cfg, sqlitePath, generated, err
}

func resolveBootstrapConfig(paths appdir.Paths) (config.Config, string, error) {
	cfg, err := config.Load(paths.ConfigPath)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("load config: %w", err)
	}
	sqlitePath, err := appdir.SQLitePath(paths.ConfigPath, paths.DataDir, cfg.Storage.SQLitePath)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("resolve sqlite path: %w", err)
	}
	if !paths.ConfigExists {
		cfg.Storage.SQLitePath = sqlitePath
	}
	return cfg, sqlitePath, nil
}

func ensureBootstrapSecrets(paths appdir.Paths, cfg config.Config) (config.Config, bool, error) {
	secretsGenerated := false
	if cfg.Security.MasterKey == "" {
		key, err := store.GenerateRandomKey()
		if err != nil {
			return config.Config{}, false, fmt.Errorf("generate master key: %w", err)
		}
		cfg.Security.MasterKey = key
		secretsGenerated = true
	}
	if cfg.Security.SessionSecret == "" {
		key, err := store.GenerateRandomKey()
		if err != nil {
			return config.Config{}, false, fmt.Errorf("generate session secret: %w", err)
		}
		cfg.Security.SessionSecret = key
		secretsGenerated = true
	}
	if secretsGenerated {
		if err := cfg.Persist(paths.ConfigPath); err != nil {
			return config.Config{}, false, fmt.Errorf("persist generated secrets: %w", err)
		}
	}
	return cfg, secretsGenerated, nil
}
