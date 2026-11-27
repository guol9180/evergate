package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"evergate/internal/browser"
	"evergate/internal/desktop"
	"evergate/internal/selfupdate"
)

// stopSignals 汇总 OS 信号之外的三类停止来源：自更新重启、托盘重启、托盘退出。
type stopSignals struct {
	update  <-chan struct{}
	restart <-chan struct{}
	exit    <-chan struct{}

	requestUpdate  func()
	requestRestart func()
	requestExit    func()
}

func newStopSignals() *stopSignals {
	updateCh, restartCh, exitCh := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var updateOnce, restartOnce, exitOnce sync.Once
	return &stopSignals{
		update:  updateCh,
		restart: restartCh,
		exit:    exitCh,
		requestUpdate: func() {
			updateOnce.Do(func() { close(updateCh) })
		},
		requestRestart: func() {
			restartOnce.Do(func() { close(restartCh) })
		},
		requestExit: func() {
			exitOnce.Do(func() { close(exitCh) })
		},
	}
}

// serve 启动监听与托盘，等待任一停止来源后优雅排空，并按需拉起新进程。
func serve(b *bootstrapped, rt *runtime) error {
	logger := rt.logger
	cfg := rt.cfg
	errCh := make(chan error, 1)
	go func() {
		if err := rt.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	logger.Info("evergate started",
		slog.String("version", version),
		slog.String("addr", rt.server.Addr),
		slog.String("base_path", cfg.Server.BasePath),
		slog.String("config", rt.paths.ConfigPath),
		slog.String("config_source", rt.paths.Source),
		slog.String("data_dir", rt.paths.DataDir),
		slog.String("db", rt.sqlitePath),
		slog.Bool("autostart", b.autostart),
	)
	adminURL := browser.AdminURL(cfg.Server.Port, cfg.Server.BasePath)
	openAdmin := func() {
		if err := browser.WaitUntilReady(adminURL, browser.Open, 100*time.Millisecond); err != nil {
			logger.Warn("open admin UI failed", slog.Any("error", err), slog.String("url", adminURL))
		}
	}
	if shouldOpenBrowser(cfg.Server, b.autostart) {
		go openAdmin()
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
		Restart: rt.signals.requestRestart,
		Exit:    rt.signals.requestExit,
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
	case <-rt.signals.update:
		logger.Info("self-update restart requested, draining requests (max 30s)")
	case <-rt.signals.restart:
		manualRestartRequested = true
		logger.Info("tray restart requested, draining requests (max 30s)")
	case <-rt.signals.exit:
		logger.Info("tray exit requested, draining requests (max 30s)")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := rt.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	b.releaseInstance()
	if spec, ok := rt.updater.PendingLaunch(); ok {
		if err := selfupdate.Spawn(spec); err != nil {
			logger.Error("launch new version failed", slog.Any("error", err), slog.String("path", spec.Path))
		} else {
			logger.Info("new version launched", slog.String("path", spec.Path))
		}
	} else if manualRestartRequested {
		workingDir, _ := os.Getwd()
		spec := selfupdate.LaunchSpec{
			Path: rt.updater.Executable(), Args: append([]string(nil), os.Args[1:]...), Dir: workingDir,
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
