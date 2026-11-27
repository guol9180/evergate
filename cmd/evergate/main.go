// evergate 是唯一入口：引导配置与数据库 → 装配服务 → 运行 HTTP 生命周期。
package main

import (
	"log/slog"
	"os"

	"evergate/internal/config"
	"evergate/internal/desktop"
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

// run 按引导 → 装配 → 服务生命周期三段启动；每段失败即终止。
func run() error {
	b, err := bootstrap(parseBootstrapFlags())
	if err != nil {
		return err
	}
	defer b.shutdown()
	if !b.primary {
		return nil
	}
	rt, err := assemble(b)
	if err != nil {
		return err
	}
	defer rt.closeRunners()
	return serve(b, rt)
}

func shouldOpenBrowser(server config.Server, autostart bool) bool {
	return !autostart && server.ShouldOpenBrowser()
}
