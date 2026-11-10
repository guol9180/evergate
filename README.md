# EverGate

本地运行的 AI 网关：把多家 LLM 供应商（OpenAI / Anthropic / Gemini / DeepSeek / 本地 vLLM / 任意 OpenAI 兼容端点）统一为一个 OpenAI 兼容 API；上游或 key 故障时自动切换，客户端无感知。*One gate, every model, zero interruption.*

> **当前状态**：P0 已完成并发布三平台单二进制；P1 功能按 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md) 里程碑推进。
> 完整需求与技术设计见 [docs/AI网关项目-开发任务书.md](./docs/AI网关项目-开发任务书.md)。

**下载**：[GitHub Releases](https://github.com/guol9180/evergate/releases/latest) 提供三平台单二进制（Windows x64 / Linux x64 / macOS Apple Silicon），推 `v*` tag 自动构建。
版本变更见 [CHANGELOG.md](./CHANGELOG.md)（发版描述即取自该文件）。

## 功能

P0（第一版交付范围）：

- **统一入口**：OpenAI 兼容 `/v1/chat/completions`（含 SSE 流式与 tool calls 透传）+ `/v1/models`
- **不中断**（本项目存在的第一理由）：key 池调度 + cooldown 状态机 + 错误分类重试（重试 → 换 key → 换渠道）；SSE 场景下「首个 chunk 写给客户端之前」可无感切换，之后绝不重试
- **渠道与 key 池管理**：增删改查、多行批量导入 key、单渠道连通测试
- **下游令牌**：发放给客户端的 key，SHA-256 哈希校验
- **请求日志**：每请求一条（含重试轨迹 retry_trace），异步批量落库，不阻塞请求路径
- **Web 管理台**：登录 / Dashboard / 渠道管理 / 令牌管理 / 请求日志（重试轨迹可视化）
- **安全**：上游 key AES-GCM 加密落库；管理台密码 bcrypt；任何接口不回显明文
- **交付形态**：`1 个可执行文件 + 1 个 SQLite 数据文件`，运行 `./evergate` 一条命令同时启动 API 与管理台，无需 Docker / Redis / 任何外部依赖

P1/P2 方向（会话亲和、Playground、用量报表、Anthropic 原生入口等）见 [DEVELOP_PLAN.md · 里程碑](./DEVELOP_PLAN.md#里程碑)。

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
./bin/evergate          # 启动：API 与管理台同端口 :8787（可配，见 evergate.yml）
go run ./cmd/evergate   # 开发态直接运行
```

- API 基址：`http://localhost:8787/v1`（客户端只需改 base_url + 用下游令牌）
- 管理台：`http://localhost:8787/`（首次启动自动创建 admin/admin123，首登强制改密）

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
| P0 只实现 OpenAI 兼容上游适配器 | anthropic / gemini 等经其官方 OpenAI 兼容端点接入；原生协议适配属 P2 |
| Windows 优雅停机以 SIGINT（Ctrl+C）为准 | SIGTERM 在 Windows 语义不可靠；Interrupt 触发后最长 30s 排空在途请求 |

参考项目与许可证边界：gpt-load（MIT，调度内核可直接借鉴）、bifrost（Apache-2.0，性能与错误分类）、litellm（MIT 核心，Router 语义）；**new-api 为 AGPL-3.0，仅学产品设计，严禁复制任何代码**。详见 [AGENTS.md · 禁止事项](./AGENTS.md#禁止事项)。

（开发规范见 [AGENTS.md](./AGENTS.md)，当前计划见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)）
