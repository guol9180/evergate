# EverGate

本地运行的 AI 网关：把多家 LLM 供应商（OpenAI / Anthropic / Gemini / DeepSeek / 本地 vLLM / 任意 OpenAI 兼容端点）统一为一个 OpenAI 兼容 API；上游或 key 故障时自动切换，客户端无感知。*One gate, every model, zero interruption.*

> **当前状态**：P0 已完成并发布三平台单二进制；P1 功能按 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md) 里程碑推进。
> 完整需求与技术设计见 [docs/AI网关项目-开发任务书.md](./docs/AI网关项目-开发任务书.md)。

**下载**：[GitHub Releases](https://github.com/guol9180/evergate/releases/latest) 提供三平台单二进制（Windows x64 / Linux x64 / macOS Apple Silicon），推 `v*` tag 自动构建。
版本变更见 [CHANGELOG.md](./CHANGELOG.md)（发版描述即取自该文件）。

## 功能

P0（第一版交付范围）：

- **统一入口**：OpenAI 兼容 `/v1/chat/completions`（含 SSE 流式与 tool calls 透传）、OpenAI Responses `/v1/responses` + `/v1/models`；Anthropic `/v1/messages` 当前阶段不提供（协议范围决策见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)）
- **不中断**（本项目存在的第一理由）：key 池调度 + cooldown 状态机 + 错误分类重试（重试 → 换 key → 换渠道）；SSE 场景下「首个 chunk 写给客户端之前」可无感切换，之后绝不重试
- **渠道与 key 池管理**：增删改查、多行批量导入 key、单渠道连通测试
- **下游令牌**：发放给客户端的 key，SHA-256 哈希校验
- **请求日志**：每请求一条（含重试轨迹 retry_trace），异步批量落库，不阻塞请求路径；流式首 token 统一按“发起上游请求 → 收到首条 SSE data”计时，Chat Completions / Responses 一致
- **Web 管理台**：登录 / Dashboard / 渠道管理 / 令牌管理 / 请求日志（重试轨迹可视化），自动检查 GitHub 新版本并提供 Windows 下载入口；浏览器角标与 Windows exe 使用同一 EverGate logo
- **安全**：上游 key AES-GCM 加密落库；管理台密码 bcrypt；任何接口不回显明文
- **交付形态**：`1 个可执行文件 + 1 个 SQLite 数据文件`，运行 `./evergate` 一条命令同时启动 API 与管理台，无需 Docker / Redis / 任何外部依赖

P1/P2 方向与协议范围决策见 [DEVELOP_PLAN.md · 里程碑](./DEVELOP_PLAN.md#里程碑)。

**非目标**：多租户 SaaS 计费、K8s 部署、MCP/A2A 协议代理、模型训练/微调、语义缓存（P2 再议）。

## 环境要求

| 依赖 | 要求 | 当前机器状态 |
|---|---|---|
| Go | 1.24+ | ✅ go1.27.3 已安装（`D:\dev\environment\go`） |
| Node.js / npm | 20+ / 10+ | ✅ v22.23.2 / 10.9.8（仅改前端时需要；`web/dist` 已入库，后端构建免 Node） |
| GNU Make | 4.x | ✅ 4.4.1 |
| git | 任意新版 | ✅ 2.55.0 |
| 其他 | 无 | 不需要 Docker / Redis / 消息队列 |

## 快速开始

单命令交付：`make build` 自动构建前端并编译单二进制（版本号取自 git describe）。

```bash
make build              # → bin/evergate(.exe)，React 管理台已嵌入（约 27MB）
make build-all          # 三平台：windows/amd64、linux/amd64、darwin/arm64
make icon               # favicon.ico → Windows exe 图标资源（.syso）
./bin/evergate          # 启动：API 与管理台同端口 :8787（可配，见 evergate.yml）
go run ./cmd/evergate   # 开发态直接运行
```

- API 基址：`http://localhost:8787/v1`（客户端只需改 base_url + 用下游令牌）
- 管理台：`http://localhost:8787/`（首次启动自动创建 admin/admin123，首登强制改密）
- 桌面体验：默认启动后自动打开管理台；服务器部署可在 `evergate.yml` 设置 `server.open_browser: false`

常见上游渠道基址（EverGate 会在其后拼接 `/chat/completions`；声明 Responses 能力的渠道还会拼接 `/responses`）：

| 上游 / 套餐 | 管理台 Base URL |
|---|---|
| DeepSeek | `https://api.deepseek.com` |
| 智谱 / Z.ai Coding 国际版 | `https://api.z.ai/api/coding/paas/v4` |
| 智谱 BigModel Coding 中国版 | `https://open.bigmodel.cn/api/coding/paas/v4` |
| 智谱 BigModel 常规 API | `https://open.bigmodel.cn/api/paas/v4` |

> 智谱 / Z.ai Coding 套餐不要把 Base URL 配成 `https://open.bigmodel.cn/api/v1`。实测该地址会让 Coding key 收到空 body 的 HTTP 500，EverGate 最终汇总为 `503 all_channels_exhausted`；请按上表修改渠道 Base URL 并重置 cooldown 中的 key。

### 协议路由矩阵

| 下游入口 | 上游声明 Responses 能力 | 未声明 Responses 能力 |
|---|---|---|
| `POST /v1/chat/completions` | CC→CC 透传 | CC→CC 透传 |
| `POST /v1/responses` | R→R 原生透传（含非流式、SSE、工具与 usage） | R→CC 桥接转换 |

说明：

- 在渠道编辑页开启“原生支持 Responses API”后，`/v1/responses` 会优先路由到该渠道，仅改写鉴权、base_url 与模型映射；请求/响应 JSON 与 SSE 事件保持上游语义。
- 未开启能力的渠道仍使用 Responses→Chat Completions 桥接；`previous_response_id`、`background=true`、`store=true` 这类无法安全桥接的状态字段会在候选渠道均无 Responses 能力时返回 400 `responses_native_required`。
- CC→R 请求转换暂不提供：当前声明 Responses 能力的渠道仍都支持 Chat Completions；若未来出现仅 Responses 上游，再独立建卡。
- 请求日志统一记录 prompt/completion/total/cached/reasoning token；Chat Completions 与 Responses 的 usage 名称差异在网关内归一化。

### 接入 Codex

在 `C:\Users\<用户名>\.codex\config.toml` 中添加：

```toml
model = "你在EverGate渠道里配置的模型名"
model_provider = "evergate"

[model_providers.evergate]
name = "EverGate"
base_url = "http://127.0.0.1:8787/v1"
env_key = "EVERGATE_API_KEY"
wire_api = "responses"
supports_websockets = false
```

然后设置下游令牌并重启 Codex：

```powershell
setx EVERGATE_API_KEY "eg-你的EverGate令牌"
```

若渠道未声明 Responses 能力，EverGate 会把 Codex 的 Responses 请求转换为上游 OpenAI-compatible Chat Completions 请求，并保留路由、key 池、重试、cooldown 与请求日志链路。桥接模式当前支持 `function`、自由文本 `custom`、`lark`/`regex` grammar custom tool、namespace 工具、Responses Lite `additional_tools`、`configuration_update.reasoning.effort` 与 `text.verbosity`；custom tool 会在上游降级为单字段 `input` 的 function schema，grammar 会写入参数描述（Chat 上游无法硬约束），返回时恢复为 `custom_tool_call` / `custom_tool_call_output` 并补回 namespace。不同 namespace 下同名工具（例如多个 MCP server 都暴露 `js`）会映射为 `<namespace>__<tool>` 的上游唯一函数名，响应与历史回传再恢复原始 namespace/name。异步/deferred custom tool、programmatic allowed_callers、图片输入、加密 compaction 与 Responses WebSocket 不在本阶段支持范围内。

> 端口约定：默认 `8787`，避开本地 Java 项目常用的 8080；如需修改只改 `evergate.yml` 的 `server.port`，无需改代码。
- 存活检查：`GET /healthz`；指标：`GET /metrics`（仅允许 loopback 访问）

## 架构概览

单进程 Go 服务，自上而下：

1. **`/v1/*` OpenAI 兼容入口**：下游令牌鉴权 → 路由器（模型 → 候选渠道列表，priority 升序 + 同级 weight 加权随机）→ key 池调度（渠道内加权轮询，跳过 cooldown/disabled）→ 错误分类重试控制器（每请求预算：3 渠道 × 每渠道 2 key，总时长 60s）→ relay（SSE 零缓冲透传，首 token 超时 30s 属可重试错误）
2. **`/admin/api/*` 管理台 API**（session 鉴权）：渠道 / 凭据 / 令牌 / 日志 / 用量 CRUD 与查询
3. **SQLite（WAL 模式）**：channels / channel_keys / tokens / request_logs / usage_daily / admins，全部业务数据唯一事实源；后台 goroutine 负责健康探测、冷却恢复、日志清理
4. **React 管理台**：构建产物经 go:embed 嵌入同一二进制

核心技术决策：

| 决策 | 理由 |
|---|---|
| Go + Gin + GORM + glebarez/go-sqlite | 单二进制、CGO-free 三平台交叉编译、SSE 流式开销低；纯 Go SQLite 驱动免去本机 C 编译依赖 |
| React 19 + TypeScript + Vite + TanStack Query/Router/Table + Tailwind 4 + shadcn/ui | 后台现代观感、组件可控、无重型 UI 库锁定；对标项目共同路线 |
| AES-GCM 加密上游 key，主密钥在 evergate.yml | 本地 DB 文件泄露 ≠ key 泄露 |
| DB 是唯一事实源，evergate.yml 仅做引导 | 渠道变更经管理台保存即生效，无需重启 |
| P0 只实现 OpenAI 兼容上游适配器 | anthropic / gemini 等经其官方 OpenAI 兼容端点接入；Anthropic Messages 下游入口延后、原生上游适配属 P2（范围见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)） |
| Windows 优雅停机以 SIGINT（Ctrl+C）为准 | SIGTERM 在 Windows 语义不可靠；Interrupt 触发后最长 30s 排空在途请求 |

参考项目与许可证边界：gpt-load（MIT，调度内核可直接借鉴）、bifrost（Apache-2.0，性能与错误分类）、litellm（MIT 核心，Router 语义）；**new-api 为 AGPL-3.0，仅学产品设计，严禁复制任何代码**。详见 [AGENTS.md · 禁止事项](./AGENTS.md#禁止事项)。

（开发规范见 [AGENTS.md](./AGENTS.md)，当前计划见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)）
