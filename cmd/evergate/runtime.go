package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/admin"
	"evergate/internal/appdir"
	"evergate/internal/config"
	"evergate/internal/health"
	"evergate/internal/metrics"
	"evergate/internal/pool"
	"evergate/internal/relay"
	"evergate/internal/router"
	"evergate/internal/selfupdate"
	"evergate/internal/server"
	"evergate/internal/settings"

	"evergate/internal/update"
	"evergate/internal/usage"
	"evergate/web"
)

// runtime 是装配阶段的产物：已就绪的 HTTP 服务、自更新器与需要随进程退出的后台任务。
type runtime struct {
	cfg        config.Config
	paths      appdir.Paths
	sqlitePath string
	logger     *slog.Logger
	server     *http.Server
	updater    *selfupdate.Manager
	signals    *stopSignals
	// closeRunners 按注册逆序停止后台任务并冲刷日志。
	closeRunners func()
}

// assemble 把 manager、handler 与后台任务组装为可服务实例；失败不可恢复的依赖在此一次性建好。
func assemble(b *bootstrapped) (*runtime, error) {
	db, logger, cfg := b.db, b.logger, b.cfg

	var runners []func()
	closeRunners := func() {
		for i := len(runners) - 1; i >= 0; i-- {
			runners[i]()
		}
	}

	// 全局 UTC 时钟（AGENTS 红线：所有时间以 UTC 存储）
	nowFn := func() time.Time { return time.Now().UTC() }
	settingsMgr, err := settings.Load(db, cfg, nowFn)
	if err != nil {
		return nil, fmt.Errorf("load runtime settings: %w", err)
	}
	usageMgr, err := usage.Load(db)
	if err != nil {
		return nil, fmt.Errorf("load usage pricing: %w", err)
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
	runners = append(runners,
		health.StartCooldownPersister(db, poolMgr, logger, 5*time.Second),
		settings.StartLogRetentionCleaner(db, settingsMgr, logger, time.Minute),
	)
	prober := health.NewProber(health.ProberOptions{
		DB: db, MasterKey: cfg.Security.MasterKey, Pool: poolMgr, Now: nowFn,
	})
	runners = append(runners,
		health.StartProbes(health.SchedulerOptions{
			DB: db, MasterKey: cfg.Security.MasterKey, Pool: poolMgr,
			Settings: settingsMgr, Logger: logger, Now: nowFn,
		}),
		usage.StartAggregator(db, logger, time.Minute),
	)

	logWriter := relay.NewLogWriter(db, 256, logger)
	runners = append(runners, logWriter.Close)

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
	signals := newStopSignals()
	updater := selfupdate.New(selfupdate.Options{
		CurrentVersion: version,
		Args:           os.Args[1:],
		Registries:     npmRegistries,
		Latest:         updateChecker.Check,
		Logger:         logger,
		Now:            time.Now,
		RequestRestart: signals.requestUpdate,
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
		Tracer:    b.tracer,
	})
	runners = append(runners, relayHandler.Close)
	adminAPI := admin.New(admin.Options{
		DB:             db,
		MasterKey:      cfg.Security.MasterKey,
		Pool:           poolMgr,
		AllowRemote:    !cfg.Server.ShouldRestrictAdminToLoopback(),
		Now:            nowFn,
		Session:        admin.NewSessionManager(cfg.Security.SessionSecret, 24*time.Hour),
		Update:         updateChecker,
		Settings:       settingsMgr,
		Health:         prober,
		Usage:          usageMgr,
		ConfigPath:     b.paths.ConfigPath,
		DataDir:        b.paths.DataDir,
		ConfigSource:   b.paths.Source,
		Playground:     relayHandler.HandlePlayground,
		SelfUpdate:     updater,
		OnConfigChange: relayHandler.InvalidateConfig,
		Logger:         logger,
	})

	engine := server.New(server.Options{
		BasePath: cfg.Server.BasePath,
		DB:       db,
		Logger:   logger,
		Metrics:  metricsReg,
		Tracer:   b.tracer,
		SetupRoutes: func(r gin.IRouter) {
			r.POST("/v1/chat/completions", relay.TokenAuth(db, logWriter), relayHandler.HandleChatCompletions)
			r.POST("/v1/responses", relay.TokenAuth(db, logWriter), relayHandler.HandleResponses)
			r.POST("/v1/messages", relay.TokenAuth(db, logWriter), relayHandler.HandleMessages)
			r.POST("/v1/messages/count_tokens", relay.TokenAuth(db, logWriter), relayHandler.HandleMessagesCountTokens)
			r.POST("/v1beta/models/*action", relay.RejectQueryKey(), relay.TokenAuth(db, logWriter), relayHandler.HandleGemini)
			r.GET("/v1/models", relay.TokenAuth(db, logWriter), relayHandler.HandleModels)
			r.GET("/v1beta/models", relay.RejectQueryKey(), relay.TokenAuth(db, logWriter), relayHandler.HandleGeminiModels)
			adminAPI.RegisterRoutes(r)
		},
		WebFS: web.DistFS,
	})

	addr := cfg.Server.ListenAddr()
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// 不设 WriteTimeout：SSE 长流式不能被全局写超时打断（任务书 4.5⑥）。
	}

	return &runtime{
		cfg: cfg, paths: b.paths, sqlitePath: b.sqlitePath, logger: logger,
		server: srv, updater: updater, signals: signals,
		closeRunners: closeRunners,
	}, nil
}
