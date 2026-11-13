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
- **令牌治理**：下游 token 支持额度、有效期、模型白名单与 IP 白名单（精确 IP / CIDR）；成功响应按实际输入 + 输出 token 扣减额度
- **会话亲和**：请求携带 `X-Session-Id` 时，同一下游令牌 + 模型的后续请求在 10 分钟内优先复用上次成功的渠道与 key；记录仅存 SHA-256 指纹于进程内存，重启后自然重建，故障时仍按原有重试与健康规则切换
- **请求级配置**：`X-EverGate-Config` 白名单覆盖单请求超时、重试预算、候选渠道与亲和开关，不绕过鉴权、令牌治理或上游 key 加密
- **用量成本**：可配置全局与渠道+模型每百万 token 单价；请求完成时按价格快照计价，后台按 UTC 日幂等聚合，管理台提供日期/渠道/模型/令牌筛选和 Top 维度
- **渠道健康**：周期使用启用 key 调用 `GET /models` 轻量探测（不触发推理），展示健康状态、可用 key 数与最近错误；404/405 仅标记“无法判定”
- **安全**：上游 key AES-GCM 加密落库（主密钥不进 DB）；管理台密码 bcrypt；任何接口不回显明文
- **可观测**：每请求一条日志（含 retry_trace 与统一口径 first_token_ms），异步批量落库不阻塞请求路径；Prometheus `/metrics` 在路线图中，当前未提供
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
2. 启动后访问管理台 `http://localhost:8787/`（初始账号 `admin/admin123`，仅用于本机初始化，首登强制改密）；默认启动自动打开浏览器，服务器部署可设 `server.open_browser: false`
3. 管理台添加渠道（Base URL + 上游 key，支持多行批量导入）并创建下游令牌；可按需配置 token 额度、有效期、模型白名单与 IP 白名单
4. 客户端只需改 base_url 为 `http://localhost:8787/v1` 并使用下游令牌

添加上游 key 后，后台会按「系统设置」中的周期、并发与超时自动调用上游 `/models` 做健康探测；该探测不触发推理请求。

存活检查 `GET /healthz`。默认端口 `8787`，避开常用 8080，修改只需改 `evergate.yml` 的 `server.port`。

### 运行时设置与 YAML 边界

首次启动时 `evergate.yml` 的 `defaults` 与日志保留天数会初始化 SQLite `runtime_settings` 表；此后这些运行时业务设置以 SQLite 为唯一事实源，管理台「系统设置」保存即热生效：

- 上游连接 / 首 token / 整体超时；重试渠道数、每渠道 key 数、重试总时长
- cooldown 失败阈值、基础时长与上限；请求日志保留天数；健康探测参数

端口、base path、SQLite 路径、master key、session secret 属启动引导配置，修改需重启。

### 请求级配置覆盖（`X-EverGate-Config`）

单个请求可通过 header 覆盖超时、重试预算、候选渠道与亲和开关，仅接受以下白名单字段：

```json
{"timeouts":{"connect":"10s","first_token":"60s","overall":"180s"},
 "retry":{"channels":2,"keys_per_channel":1,"total":"45s"},
 "channels":[1,3],
 "session_affinity":false}
```

- header 必须是 ≤4096 字节的 JSON 对象，时长使用 `30s` / `5m`；`channels` 中的 id 必须存在、已启用且支持当前模型。
- 未知字段、类型错误、越界值或试图覆盖 `api_key` / `model` / `allowed_models` 等字段一律返回 400，并指出字段路径。
- 覆盖不能绕过令牌鉴权、模型白名单、额度与 IP 限制；生效字段以脱敏摘要写入请求日志。

## 安全模型

- 上游 key 以 AES-GCM 加密存储，主密钥仅在 `evergate.yml`（已 gitignore）；DB 文件泄露 ≠ key 泄露
- 下游令牌仅存 SHA-256 哈希；管理台接口永不回显任何 key 明文
- 下游令牌的 IP 白名单默认基于 TCP 直连地址判断；当前版本不信任 `X-Forwarded-For`
- 面向**本地 / 内网可信环境**设计：管理台与 API 同端口，公网暴露前请自行加反向代理 TLS 与访问控制
- 初始 `admin/admin123` 只用于首次登录并会强制改密；请不要在未加访问控制的情况下把管理端口暴露到公网

## 架构概览

单进程 Go 服务，自上而下：

1. **`/v1/*` OpenAI 兼容入口**：下游令牌鉴权 → 令牌治理 → `X-EverGate-Config` 白名单覆盖 → 路由器（模型 → 候选渠道，priority 升序 + 同级 weight 加权随机；`X-Session-Id` 命中亲和时在最高优先级组内提前粘性渠道）→ key 池调度（渠道内加权轮询，亲和优先同 key，跳过 cooldown/disabled）→ 错误分类重试控制器 → relay（SSE 零缓冲 flush 透传，首 token 超时属可重试错误）
2. **`/admin/api/*` 管理台 API**（session 鉴权）：渠道 / 凭据 / 令牌 / 请求日志 / 系统设置管理
3. **SQLite（WAL 模式）**：channels / channel_keys / tokens / request_logs / usage_daily / model_prices / admins / runtime_settings，业务数据唯一事实源；后台 goroutine 负责健康探测、冷却恢复、成本聚合与日志清理
4. **React 管理台**：构建产物经 go:embed 嵌入同一二进制

核心技术栈：Go 1.24+ / Gin / GORM / glebarez 纯 Go SQLite；React 19 / TypeScript / Vite / TanStack Query/Router/Table / Tailwind 4 / shadcn/ui。

## 路线图

当前阶段、已完成与后续任务见 [DEVELOP_PLAN.md · 里程碑](./DEVELOP_PLAN.md#里程碑)。Anthropic Messages 入口延后。

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
