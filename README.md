# EverGate

**本地自托管 AI 网关**：把多家 LLM 供应商（OpenAI / DeepSeek / Kimi / 智谱 / Anthropic / Gemini / 本地 vLLM / 任意 OpenAI 兼容端点）统一为一个 OpenAI 兼容 API；上游或 key 故障时自动切换，客户端无感。*One gate, every model, zero interruption.*

> Self-hosted OpenAI-compatible LLM gateway with key-pool failover, multi-provider routing and zero-buffer SSE streaming.

[![Release](https://img.shields.io/github/v/release/guol9180/evergate?include_prereleases&style=flat-square)](https://github.com/guol9180/evergate/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white&style=flat-square)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey?style=flat-square)](https://github.com/guol9180/evergate/releases/latest)

**下载**：[GitHub Releases](https://github.com/guol9180/evergate/releases/latest) 提供三平台单二进制（Windows x64 / Linux x64 / macOS Apple Silicon），推送 `v*` tag 自动构建。版本变更见 [CHANGELOG.md](./CHANGELOG.md)。

## 功能亮点

- **统一入口**：OpenAI Chat Completions `/v1/chat/completions`（SSE 流式 + tool calls 透传）、OpenAI Responses `/v1/responses`、`/v1/models`；Anthropic Messages 延后支持（见路线图）
- **不中断**（本项目存在的第一理由）：key 池调度 + cooldown 状态机 + 错误分类重试（重试 → 换 key → 换渠道）；SSE 场景下「首个 chunk 写给客户端之前」可无感切换，之后绝不重试
- **双协议上游**：Chat Completions 全渠道支持；渠道级声明 Responses 能力后 `/v1/responses` 原生透传保真，未声明渠道自动 R→CC 桥接兜底
- **Web 管理台**：登录 / Dashboard / 渠道与 key 池管理 / 下游令牌 / 请求日志（重试轨迹可视化）/ 系统设置热更新，内置新版本检查
- **安全**：上游 key AES-GCM 加密落库（主密钥不进 DB）；管理台密码 bcrypt；任何接口不回显明文；`/metrics` 仅 loopback
- **可观测**：每请求一条日志（含 retry_trace 与统一口径 first_token_ms），异步批量落库不阻塞请求路径
- **零依赖交付**：`1 个可执行文件 + 1 个 SQLite 数据文件`，`./evergate` 一条命令同时启动 API 与管理台，无需 Docker / Redis / 数据库服务

## 协议路由矩阵

| 下游入口 | 上游声明 Responses 能力 | 未声明 Responses 能力 |
|---|---|---|
| `POST /v1/chat/completions` | CC→CC 透传 | CC→CC 透传 |
| `POST /v1/responses` | R→R 原生透传（含非流式、SSE、工具与 usage） | R→CC 桥接转换 |

- 渠道编辑页开启「原生支持 Responses API」后，`/v1/responses` 优先路由到该渠道，仅改写鉴权、base_url 与模型映射；请求/响应 JSON 与 SSE 事件保持上游语义，`store` / `previous_response_id` 等有状态字段保真。
- 桥接模式把 Responses 请求转换为上游 Chat Completions：支持 `function`、自由文本/`lark`/`regex` grammar `custom` 工具、namespace 工具、Responses Lite `additional_tools`、`reasoning.effort` 与 `text.verbosity`；不可桥接的有状态字段显式 400，绝不静默丢弃。
- 不同 namespace 同名工具映射为 `<namespace>__<tool>` 唯一函数名，响应与历史回传恢复原始 namespace/name。

## 快速开始

```bash
# 方式一：直接下载 Release 单二进制后运行
./evergate

# 方式二：源码构建（web/dist 已入库，后端构建无需 Node）
make build              # → bin/evergate(.exe)，React 管理台已嵌入
make build-all          # 三平台交叉编译：windows/amd64、linux/amd64、darwin/arm64
go run ./cmd/evergate   # 开发态直接运行
```

1. 复制配置模板：`cp evergate.yml.example evergate.yml`（密钥留空则首次启动自动生成并写回）
2. 启动后访问管理台 `http://localhost:8787/`（默认 `admin/admin123`，首登强制改密）；默认启动自动打开浏览器，服务器部署可设 `server.open_browser: false`
3. 管理台添加渠道（Base URL + 上游 key，支持多行批量导入）并创建下游令牌
4. 客户端只需改 base_url 为 `http://localhost:8787/v1` 并使用下游令牌

存活检查 `GET /healthz`；指标 `GET /metrics`（仅 loopback）。默认端口 `8787`，避开常用 8080，修改只需改 `evergate.yml` 的 `server.port`。

### 运行时设置与 YAML 边界

首次启动时 `evergate.yml` 的 `defaults` 与日志保留天数会初始化 SQLite `runtime_settings` 表；此后这些运行时业务设置以 SQLite 为唯一事实源，管理台「系统设置」保存即热生效：

- 上游连接 / 首 token / 整体超时；重试渠道数、每渠道 key 数、重试总时长
- cooldown 失败阈值、基础时长与上限；请求日志保留天数；健康探测参数

端口、base path、SQLite 路径、master key、session secret 属启动引导配置，修改需重启。

## 安全模型

- 上游 key 以 AES-GCM 加密存储，主密钥仅在 `evergate.yml`（已 gitignore）；DB 文件泄露 ≠ key 泄露
- 下游令牌仅存 SHA-256 哈希；管理台接口永不回显任何 key 明文
- 面向**本地 / 内网可信环境**设计：管理台与 API 同端口，公网暴露前请自行加反向代理 TLS 与访问控制

## 架构概览

单进程 Go 服务，自上而下：

1. **`/v1/*` OpenAI 兼容入口**：下游令牌鉴权 → 路由器（模型 → 候选渠道，priority 升序 + 同级 weight 加权随机）→ key 池调度（渠道内加权轮询，跳过 cooldown/disabled）→ 错误分类重试控制器 → relay（SSE 零缓冲 flush 透传，首 token 超时属可重试错误）
2. **`/admin/api/*` 管理台 API**（session 鉴权）：渠道 / 凭据 / 令牌 / 日志 / 用量 / 设置 CRUD
3. **SQLite（WAL 模式）**：channels / channel_keys / tokens / request_logs / usage_daily / admins / runtime_settings，业务数据唯一事实源；后台 goroutine 负责健康探测、冷却恢复、日志清理
4. **React 管理台**：构建产物经 go:embed 嵌入同一二进制

核心技术栈：Go 1.24+ / Gin / GORM / glebarez 纯 Go SQLite；React 19 / TypeScript / Vite / TanStack Query/Router/Table / Tailwind 4 / shadcn/ui。

## 路线图

P1 进行中：npm 平台包分发、一键自动更新、渠道健康自动探测与状态徽标、令牌额度与白名单、用量成本报表、会话亲和调度、请求级配置覆盖、Playground、Prometheus / OTel 指标；Anthropic Messages 入口延后。完整任务卡与验收标准见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)。

## 参与开发

```bash
go test ./...                # 后端全量测试
npm --prefix web run build   # 前端构建（改前端后执行并提交 dist）
make build                   # 提交前完整构建
```

- 开发规范：[AGENTS.md](./AGENTS.md)；需求与设计：[docs/AI网关项目-开发任务书.md](./docs/AI网关项目-开发任务书.md)；协议调研：[docs/协议生态调研-ChatCompletions与Responses.md](./docs/协议生态调研-ChatCompletions与Responses.md)
- 参考项目边界：gpt-load（MIT）/ bifrost（Apache-2.0）/ LiteLLM 核心（MIT）仅作设计借鉴；new-api（AGPL-3.0）不复制任何代码

## License

[MIT](./LICENSE) © 2026 guol9180

---

**关键词 / Keywords**：AI 网关 · LLM Gateway · OpenAI Compatible · OpenAI Proxy · Chat Completions · Responses API · Codex · API 聚合 · 多渠道负载均衡 · 故障切换 failover · key 池调度 · 限流冷却 · SSE 流式透传 · 自托管 self-hosted · Go · Gin · React · SQLite
