# EverGate

**本地自托管 AI 网关**：把多家 LLM 供应商（OpenAI / DeepSeek / Kimi / 智谱 / Anthropic / Gemini / 本地 vLLM / 任意 OpenAI 兼容端点）统一为一个 OpenAI 兼容 API；上游或 key 故障时自动切换，客户端无感。*One gate, every model, zero interruption.*

> Self-hosted OpenAI-compatible LLM gateway with key-pool failover, multi-provider routing and zero-buffer SSE streaming.

[![Release](https://img.shields.io/github/v/release/guol9180/evergate?include_prereleases&style=flat-square)](https://github.com/guol9180/evergate/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white&style=flat-square)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey?style=flat-square)](https://github.com/guol9180/evergate/releases/latest)

**下载**：[GitHub Releases](https://github.com/guol9180/evergate/releases/latest) 提供三平台单二进制（Windows x64 / Linux x64 / macOS Apple Silicon），推送 `v*` tag 自动构建。版本变更见 [CHANGELOG.md](./CHANGELOG.md)。

## 功能

- **统一入口**：OpenAI Chat Completions / Responses 与 `/v1/models`；上游支持任意 OpenAI 兼容端点，Responses 渠道可原生透传，其余自动桥接
- **故障切换**：渠道 + key 池调度、错误分类重试与 cooldown；SSE 首个 chunk 前可无感切换，之后不重试
- **管理台**：渠道/key、下游令牌、请求日志、Playground、用量成本、系统设置
- **治理与成本**：令牌额度、有效期、模型/IP 白名单；价格快照与 UTC 日聚合
- **单二进制交付**：1 个可执行文件 + 1 个 SQLite 数据文件，无需 Docker / Redis / 外部数据库
- **可观测与更新**：Prometheus `/metrics`、可选 OTLP、一键更新；配置见 [运维配置](./docs/运维配置.md)

协议、调度与验收细节见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)；下游协议调研见 [协议生态调研](./docs/协议生态调研-ChatCompletions与Responses.md)。

## 快速开始

```bash
# 方式一：直接下载 Release 单二进制后运行
./evergate

# 方式二：npm 全局安装（Windows x64 / Linux x64 / macOS Apple Silicon）
npm i -g @heguoliang/evergate
evergate

# 方式三：源码构建（web/dist 已入库，后端构建无需 Node）
make build              # → bin/evergate(.exe)，React 管理台已嵌入
make build-all          # 三平台交叉编译：windows/amd64、linux/amd64、darwin/arm64
go run ./cmd/evergate   # 开发态直接运行
```

npm 安装会自动选择本平台子包，实际运行仍是原生单二进制；Node 仅用于启动垫片。国内网络可用 `npm i -g @heguoliang/evergate --registry=https://registry.npmmirror.com`。

1. 复制配置模板：`cp evergate.yml.example evergate.yml`（密钥留空则首次启动自动生成并写回）
2. 启动后访问管理台 `http://localhost:8787/`（初始账号 `admin/admin123`，仅用于本机初始化，首登强制改密）；默认启动自动打开浏览器，服务器部署可设 `server.open_browser: false`
3. 管理台添加渠道（Base URL + 上游 key，支持多行批量导入）并创建下游令牌；可按需配置 token 额度、有效期、模型白名单与 IP 白名单
4. 客户端只需改 base_url 为 `http://localhost:8787/v1` 并使用下游令牌

存活检查 `GET /healthz`。默认端口 `8787`，避开常用 8080，修改只需改 `evergate.yml` 的 `server.port`。

## 运维与配置

运行时设置、Prometheus/OTLP、一键更新与环境变量见 [docs/运维配置.md](./docs/运维配置.md)。

## 安全模型

- 上游 key 以 AES-GCM 加密存储，主密钥仅在 `evergate.yml`（已 gitignore）；下游令牌仅存 SHA-256
- 管理台与 API 同端口，面向本地 / 内网可信环境；公网暴露前请自行加 TLS 与访问控制
- 初始 `admin/admin123` 仅用于首次登录并会强制改密

## 架构概览

单进程 Go 服务：

1. `/v1/*`：下游令牌鉴权 → 路由 → key 池调度 → 错误分类重试 → relay/SSE
2. `/admin/api/*`：管理台 API（session 鉴权）
3. SQLite（WAL）：渠道、令牌、请求日志、用量与运行时设置的事实源
4. React 管理台构建产物 go:embed 进同一二进制；`/healthz` 与 `/metrics` 为运维端点

技术栈：Go 1.24+ / Gin / GORM / 纯 Go SQLite；React 19 / TypeScript / Vite / TanStack Query / Tailwind 4。

## 路线图

当前阶段、已完成与后续任务见 [DEVELOP_PLAN.md · 里程碑](./DEVELOP_PLAN.md#里程碑)。Anthropic Messages 入口延后。

## 参与开发

```bash
go test ./...                # 后端全量测试
npm --prefix web run build   # 前端构建（改前端后执行并提交 dist）
make build                   # 提交前完整构建
```

- 开发规范：[AGENTS.md](./AGENTS.md)；需求与设计：[docs/AI网关项目-开发任务书.md](./docs/AI网关项目-开发任务书.md)；协议调研：[docs/协议生态调研-ChatCompletions与Responses.md](./docs/协议生态调研-ChatCompletions与Responses.md)
- 运维与配置：[docs/运维配置.md](./docs/运维配置.md)；参考项目与许可证边界见 [AGENTS.md](./AGENTS.md)

## License

[MIT](./LICENSE) © 2026 guol9180

---

**关键词 / Keywords**：AI 网关 · LLM Gateway · OpenAI Compatible · OpenAI Proxy · Chat Completions · Responses API · Codex · API 聚合 · 多渠道负载均衡 · 故障切换 failover · key 池调度 · 限流冷却 · SSE 流式透传 · 自托管 self-hosted · Go · Gin · React · SQLite
