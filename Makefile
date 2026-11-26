.PHONY: build build-web build-all icon run test coverage-scheduler coverage-relay fmt gofmt-check staticcheck govulncheck audit-web vet clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
WINDOWS_GUI_LDFLAGS := $(LDFLAGS) -H windowsgui
ifeq ($(OS),Windows_NT)
  EXT := .exe
endif

# 单命令交付：构建前端 → 产出本平台单二进制（验收清单 #1）
build: build-web
	go build -ldflags "$(LDFLAGS)" -o bin/evergate$(EXT) ./cmd/evergate
	@echo "==> bin/evergate$(EXT) ($(shell du -m bin/evergate$(EXT) 2>/dev/null | cut -f1)MB, version=$(VERSION))"

build-web:
	npm --prefix web run build

# 由 public/favicon.ico 重新生成 Windows 资源对象；常规构建直接使用入库的 .syso
icon:
	go run github.com/akavel/rsrc@v0.10.2 \
		-ico web/public/favicon.ico \
		-manifest internal/winres/gui.manifest \
		-o internal/winres/resources_windows_amd64.syso

# 三平台交叉编译（任务书 4.9）；target 级 export 保证 Windows make 兼容
build-all: build-web
build-all: bin/evergate-windows-amd64.exe bin/evergate-windows-amd64-console.exe bin/evergate-linux-amd64 bin/evergate-darwin-arm64

bin/evergate-windows-amd64.exe: export GOOS := windows
bin/evergate-windows-amd64.exe: export GOARCH := amd64
bin/evergate-windows-amd64.exe: build-web
	go build -tags windowsgui -ldflags "$(WINDOWS_GUI_LDFLAGS)" -o $@ ./cmd/evergate

bin/evergate-windows-amd64-console.exe: export GOOS := windows
bin/evergate-windows-amd64-console.exe: export GOARCH := amd64
bin/evergate-windows-amd64-console.exe: build-web
	go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/evergate

bin/evergate-linux-amd64: export GOOS := linux
bin/evergate-linux-amd64: export GOARCH := amd64
bin/evergate-linux-amd64: build-web
	go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/evergate

bin/evergate-darwin-arm64: export GOOS := darwin
bin/evergate-darwin-arm64: export GOARCH := arm64
bin/evergate-darwin-arm64: build-web
	go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/evergate

run:
	go run ./cmd/evergate

test:
	go test ./...

coverage-scheduler:
	node -e "require('node:fs').mkdirSync('.gocache',{recursive:true})"
	go test ./internal/pool ./internal/retry ./internal/router -coverprofile=.gocache/scheduler-cover.out
	go tool cover -func=.gocache/scheduler-cover.out > .gocache/scheduler-cover.txt
	node scripts/check-scheduler-coverage.mjs .gocache/scheduler-cover.txt

# relay 语句覆盖率门禁（T74）：基线 81.0%（2026-11-24 实测），阈值 81 不得高于基线
coverage-relay:
	node -e "require('node:fs').mkdirSync('.gocache',{recursive:true})"
	go test ./internal/relay -coverprofile=.gocache/relay-cover.out
	go tool cover -func=.gocache/relay-cover.out > .gocache/relay-cover.txt
	node scripts/check-relay-coverage.mjs .gocache/relay-cover.txt

fmt:
	gofmt -l -w cmd internal scripts

# gofmt 门禁（T106）：只检查项目 Go 源码，避免把 web/node_modules 的第三方 Go 文件算进来
gofmt-check:
	node scripts/check-gofmt.mjs

# staticcheck 门禁（T106）：固定版本，本地与 CI 同口径
staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./cmd/... ./internal/...

# govulncheck 门禁（T106）：只阻断代码实际可达的漏洞
govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./cmd/... ./internal/...

# 前端依赖漏洞门禁（T106）：高危及以上阻断
audit-web:
	npm --prefix web audit --audit-level=high

vet:
	go vet ./...

clean:
	rm -rf bin
