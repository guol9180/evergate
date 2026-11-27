package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/gorm"

	"evergate/internal/appdir"
	"evergate/internal/applog"
	"evergate/internal/browser"
	"evergate/internal/config"
	"evergate/internal/desktop"
	"evergate/internal/store"
	"evergate/internal/telemetry"
)

// bootstrapFlags 是引导阶段需要的命令行输入。
type bootstrapFlags struct {
	cfgPath        string
	configExplicit bool
	autostart      bool
}

func parseBootstrapFlags() bootstrapFlags {
	cfgPath := flag.String("config", "", "引导配置文件路径；留空时按平台规则发现")
	autostart := flag.Bool("autostart", false, "由安装器的开机启动项拉起")
	flag.Parse()
	configExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configExplicit = true
		}
	})
	return bootstrapFlags{cfgPath: *cfgPath, configExplicit: configExplicit, autostart: *autostart}
}

// bootstrapped 是引导阶段产出的、装配与生命周期阶段需要共享的状态。
type bootstrapped struct {
	cfg        config.Config
	paths      appdir.Paths
	sqlitePath string
	db         *gorm.DB
	logger     *slog.Logger
	tracer     *telemetry.Tracer
	autostart  bool
	// primary=false 表示已有实例在跑，本次启动只负责唤起管理台后退出。
	primary         bool
	releaseInstance func()
	shutdown        func()
}

// bootstrap 完成日志、配置、单实例、密钥、追踪与数据库准备，并给出统一退出清理。
func bootstrap(f bootstrapFlags) (*bootstrapped, error) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	var cleanups []func()
	var once sync.Once
	shutdown := func() {
		once.Do(func() {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		})
	}
	abort := func(err error) (*bootstrapped, error) {
		shutdown()
		return nil, err
	}

	paths, err := appdir.Discover(f.cfgPath, f.configExplicit)
	if err != nil {
		return nil, fmt.Errorf("resolve config/data paths: %w", err)
	}
	cfg, sqlitePath, err := resolveBootstrapConfig(paths)
	if err != nil {
		return nil, err
	}
	paths.DataDir = filepath.Dir(sqlitePath)
	logger, logPath, logCloser, err := applog.Open(paths.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open application log: %w", err)
	}
	activeLogPath = logPath
	slog.SetDefault(logger)
	cleanups = append(cleanups, func() { _ = logCloser.Close() })

	releaseInstance, primary, err := desktop.AcquireSingleInstance(paths.DataDir)
	if err != nil {
		return abort(fmt.Errorf("acquire single instance: %w", err))
	}
	cleanups = append(cleanups, releaseInstance)
	if !primary {
		logger.Info("another EverGate instance is running", slog.Bool("autostart", f.autostart))
		if !f.autostart {
			adminURL := browser.AdminURL(cfg.Server.Port, cfg.Server.BasePath)
			if openErr := browser.WaitUntilReady(adminURL, browser.Open, 100*time.Millisecond); openErr != nil {
				logger.Warn("open existing admin UI failed", slog.Any("error", openErr), slog.String("url", adminURL))
			}
		}
		shutdown()
		return &bootstrapped{primary: false, shutdown: func() {}}, nil
	}

	if !paths.ConfigExists {
		logger.Warn("config file not found, using defaults", slog.String("config", paths.ConfigPath))
	}
	cfg, secretsGenerated, err := ensureBootstrapSecrets(paths, cfg)
	if err != nil {
		return abort(err)
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
		cleanups = append(cleanups, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := tracer.Shutdown(ctx); err != nil {
				logger.Warn("otel shutdown failed", slog.Any("error", err))
			}
		})
	}

	db, err := store.OpenWithOptions(sqlitePath, store.Options{Synchronous: cfg.Storage.SQLiteSynchronous})
	if err != nil {
		return abort(fmt.Errorf("open database: %w", err))
	}
	if err := store.MigrateWithBackup(db, sqlitePath, version, logger); err != nil {
		return abort(fmt.Errorf("migrate database: %w", err))
	}
	initialPassword, fromEnv := store.InitialAdminPassword(os.Getenv(store.InitialAdminEnv))
	// 设置了环境变量时保持旧的自动创建流程（容器 / 自动化）；否则留空由管理台首启向导设置，
	// 避免随机密码只存在于日志里。
	if fromEnv {
		created, err := store.EnsureAdmin(db, store.DefaultAdminUsername, initialPassword)
		if err != nil {
			return abort(fmt.Errorf("ensure default admin: %w", err))
		}
		if created {
			logger.Warn("initial admin created using password from EVERGATE_INITIAL_ADMIN_PASSWORD",
				slog.String("username", store.DefaultAdminUsername),
				slog.String("must_change_password", "true"),
			)
		}
	} else if required, err := store.SetupRequired(db); err != nil {
		return abort(fmt.Errorf("check admin setup: %w", err))
	} else if required {
		logger.Warn("no admin account yet; complete initial setup in the admin UI (open /setup)")
	}

	return &bootstrapped{
		cfg: cfg, paths: paths, sqlitePath: sqlitePath, db: db,
		logger: logger, tracer: tracer, autostart: f.autostart,
		primary: true, releaseInstance: releaseInstance, shutdown: shutdown,
	}, nil
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
