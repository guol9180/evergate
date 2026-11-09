# EverGate 开发任务书（本地 AI 网关项目）

> **文档用途**：本文档是完整的需求 + 技术方案 + 验收标准，将一次性交给 AI 开发代理执行。开发代理应按第 6 章的实施顺序开发，并以第 5 章 P0 验收标准作为"完成"的定义。
> **项目命名**：**EverGate**（仓库名 `evergate`）— *One gate, every model, zero interruption.*（一个网关，所有模型，永不中断）。已核查 GitHub 全站同名仓库仅 12 个且全部 0 星，命名无碰撞。二进制名、Go module、配置文件、数据文件统一使用 `evergate`。
> **调研说明**：第 2 章所有项目数据（星数、技术栈、许可证、活跃度）均于 2026-11-09 通过 GitHub API 实测核实。

---

## 1. 项目定位与目标

构建一个**本地启动运行的 AI 网关服务**，解决的核心痛点：

1. **统一入口**：多家 LLM 供应商（OpenAI / Anthropic / Gemini / DeepSeek / 本地 vLLM / 任意 OpenAI 兼容端点等）统一为一个 OpenAI 兼容 API，客户端（ChatBox、Cline、RiskyCD 等）只改一次 base_url。
2. **不中断**：某供应商/某 API key 故障时自动切换（重试 → 换 key → 换渠道），客户端无感知。这是本项目存在的第一理由。
3. **性能与体验**：网关自身开销亚毫秒级，流式（SSE）透传不整段缓冲，单二进制双击即用。
4. **配套管理后台**：Web 管理台管理渠道、key 池、下游令牌、请求日志、用量统计、Playground。

**交付形态（硬性要求）**：`1 个可执行文件 + 1 个 SQLite 数据文件`，运行 `./evergate` 一条命令同时启动 API 服务与管理台，无需 Docker、无需 Redis、无需任何外部依赖。

**非目标（明确不做）**：多租户 SaaS 计费、K8s 部署、MCP/A2A 协议代理、模型训练/微调、语义缓存（P2 再议）。

---

## 2. 参考项目借鉴总表

### 2.1 核心对标项目（直接学习实现）

| 优先级 | 项目 | 实测数据 | 技术栈（实测） | 许可证 | 本项目要学的部分 | 学习方式 |
|---|---|---|---|---|---|---|
| ★★★★★ | [tbphp/gpt-load](https://github.com/tbphp/gpt-load) | ⭐7.2k，每日发版 | Go + Gin + GORM + SQLite(默认)/MySQL/PG；管理台 Vue3 + Vite + Tailwind4，嵌入单二进制 | **MIT** | ① 多凭据池调度：加权轮询、重试、cooldown、黑名单、会话亲和；② 渠道/key 双层健康管理；③ 请求日志与用量；④ 单二进制交付形态 | **精读源码，可直接借鉴实现**（与本项目的调度内核最接近，代码量小） |
| ★★★★★ | [QuantumNous/new-api](https://github.com/QuantumNous/new-api) | ⭐51.6k，每日发版 | Go + Gin + GORM + SQLite/MySQL/PG/ClickHouse；管理台 React19 + TanStack Router/Query/Table + Tailwind4 + VChart + i18next | **AGPL-3.0** | **管理后台产品功能全集**：渠道管理（连接测试/批量/优先级权重）、令牌（下游key）管理、用量成本报表、Playground、审计日志、模型映射、多语言 UI、渠道亲和 | **只学产品设计与交互，严禁复制代码**（AGPL 传染） |
| ★★★★ | [maximhq/bifrost](https://github.com/maximhq/bifrost) | ⭐9.4k，每周发版 | Go（高性能网关，官方标称 5k RPS 下自身开销 <200µs） | **Apache-2.0** | ① 低开销流式代理的工程实现；② fallback 状态机与错误分类（哪些错误可重试/切换，哪些不能）；③ provider 抽象层设计；④ 虚拟 key 与预算思想 | **源码可借鉴**，重点读性能路径与错误分类 |
| ★★★★ | [BerriAI/litellm](https://github.com/BerriAI/litellm) | ⭐62.9k，每日发版 | Python + Rust core | 核心 MIT（enterprise/ 目录单独授权） | ① **Router 语义标准**：模型→deployment 列表、优先级、权重、cooldown、allowed_fails、fallback 链；② 100+ provider 适配差异的经验字典（读文档为主）；③ 虚拟 key/花费追踪 | **文档必读**（docs.litellm.ai/docs/routing），provider 差异查字典用；代码体量大不必通读 |

### 2.2 思想借鉴（学设计，不抄实现）

| 项目 | 借鉴点 | 说明 |
|---|---|---|
| [Portkey-AI/gateway](https://github.com/Portkey-AI/gateway)（⭐13.3k，MIT，维护放缓） | **config-driven fallback** | 将 fallback / loadbalance / retry 规则做成「每个请求可携带的 JSON config」，无需改网关代码即可覆盖全局策略。本项目 P1 实现请求级覆盖参数时参考其 config 结构 |
| [tensorzero/tensorzero](https://github.com/tensorzero/tensorzero)（⭐11.7k，Apache-2.0，节奏放缓） | LLMOps / 实验思想 | 路由 + 评估 + A/B 实验。本项目 P2 做「按性价比自动选模型」时回头看 |
| [Helicone/helicone](https://github.com/helicone/helicone)（⭐6.3k，Apache-2.0） | 可观测 Dashboard 交互 | 请求日志的筛选、trace 视图、延迟分布图 UI 设计参考 |

### 2.3 明确不对标（及原因）

| 项目/类别 | 排除原因 |
|---|---|
| Kong / Higress / APISIX / Tyk | 面向 K8s 集群与企业插件生态，部署重、心智负担大，违背"本地轻量单二进制"目标 |
| songquanpeng/one-api（⭐37.4k，MIT，2026-01 后维护停滞） | 设计已被 new-api 全面继承；仅当"最简 Go+React 网关源码"入门读物 |
| agentgateway / theagentrouter | 主打 MCP/A2A/Agent 协议代理，与 LLM 供应商 failover 场景不同 |
| cheaper-inference/OmniRoute | 灰色玩法（订阅账号池 + TLS 流量伪装 + "免费"上游），违反上游服务条款，**严禁参考、严禁引入其依赖** |

### 2.4 许可证红线（开发代理必须遵守）

| 项目 | 能否复制代码 |
|---|---|
| gpt-load (MIT) | ✅ 可复制/改写，保留版权声明 |
| bifrost (Apache-2.0) | ✅ 可复制/改写，保留 NOTICE |
| litellm 核心 (MIT) | ✅ 可（仅核心目录；不碰 enterprise/ 目录代码） |
| **new-api (AGPL-3.0)** | ❌ **只能看交互截图与功能清单学产品设计，任何代码/组件/样式不得复制** |
| Portkey gateway (MIT) | ✅ 可参考 config 结构 |

---

## 3. 技术栈选型（定稿）

### 3.1 后端

| 层 | 选型 | 版本/说明 | 理由 |
|---|---|---|---|
| 语言 | **Go** | 1.24+ | SSE 流式代理开销亚毫秒级；空闲内存 <50MB；编译单二进制、毫秒级启动、交叉编译三平台；三个核心对标（gpt-load/new-api/bifrost）全部是 Go，最大化"抄作业"效率 |
| Web 框架 | **Gin** | v1.10+ | gpt-load/new-api 同款，可无缝借鉴；SSE 支持成熟 |
| ORM | **GORM** | v1.25+ | 同款；AutoMigrate 简化本地开发 |
| 数据库 | **glebarez/go-sqlite + SQLite（WAL 模式）**，预留 MySQL/Postgres driver 开关 | 默认零配置 | 本地个人规模 SQLite 足够；请求日志表按保留天数定时清理；WAL 保证读写并发 |
| 结构化日志 | **标准库 log/slog** | JSON 到 stdout | 运行日志与请求日志分离：请求级日志入库供管理台查询 |
| 指标 | **Prometheus client_golang** | /metrics | QPS、每渠道延迟/失败率、cooldown 命中、首 token 延迟直方图 |
| 认证 | 管理台：session（cookie + secure）或 golang-jwt；下游 API key：入库 SHA-256 hash 比对 | bcrypt 存管理台密码 | 简单可靠，不引入 OAuth |
| 加密 | 上游 API key 用 AES-GCM 加密落库（主密钥来自配置文件或环境变量） | — | 本地文件泄露不等于 key 泄露 |
| 后台任务 | goroutine + time.Ticker | — | 健康探测、cooldown 半开恢复、日志清理。**不引入 Redis/MQ/cron 库** |
| 配置热更新 | 管理台保存即生效（DB 是唯一事实源）；可选 SIGHUP 重载 YAML 引导配置 | fsnotify 可选 | 渠道变更不重启进程 |
| 构建 | Makefile + goreleaser（或手写 build 脚本），`go:embed` 嵌入前端产物 | — | 交付单二进制 |

### 3.2 前端管理台

| 项 | 选型 | 理由 |
|---|---|---|
| 框架 | **React 19 + TypeScript + Vite** | new-api/tensorzero 同款路线；若开发代理对 Vue 更熟，可替换为 Vue3+Vite（gpt-load 同款），但二选一后不得混用 |
| 数据层 | **TanStack Query（服务端状态）+ TanStack Router + TanStack Table** | 三个对标项目共同选择；日志表格、轮询、缓存开箱即用 |
| UI | **Tailwind CSS 4 + shadcn/ui（Radix）+ lucide-react** | 后台现代观感、组件可控、无重型 UI 库锁定 |
| 图表 | **recharts** | 用量/成本/延迟报表 |
| 表单 | react-hook-form + zod | 渠道编辑表单校验 |
| 国际化 | 第一版仅简体中文，预留 i18n 键位结构 | 避免 P0 过度设计 |
| 部署 | 构建产物经 **go:embed** 嵌入二进制；开发模式下 Vite devServer 代理到 Go 后端 | 生产单文件，开发热更新 |

### 3.3 明确不引入（第一版）

Redis、消息队列、微服务拆分、K8s/Helm、ClickHouse、GraphQL、微前端、OAuth2/OIDC、多租户计费、语义缓存、向量库。**开发代理不得自行添加任何上述依赖。**

---

## 4. 系统架构设计

### 4.1 架构总览

```
客户端（ChatBox / Cline / 任意 OpenAI SDK）
        │  OpenAI 兼容协议（含 SSE 流式）
        ▼
┌─────────────────────────────────────────────┐
│                 EverGate（单进程）            │
│  ┌───────────────────────────────────────┐  │
│  │ /v1/*  OpenAI 兼容入口                 │  │
│  │   ├─ 鉴权：下游令牌校验                 │  │
│  │   ├─ 模型解析 → 路由器                  │  │
│  │   │    （模型 → 候选渠道列表）           │  │
│  │   ├─ key 池调度（加权轮询 + cooldown）   │  │
│  │   ├─ 重试/降级控制器（错误分类驱动）      │  │
│  │   └─ relay（SSE 零缓冲透传）            │  │
│  ├───────────────────────────────────────┤  │
│  │ /admin/*  管理台 API（session 鉴权）    │  │
│  │ /metrics  Prometheus                  │  │
│  │ /healthz                              │  │
│  ├───────────────────────────────────────┤  │
│  │ SQLite（WAL）：渠道/密钥/令牌/日志/用量   │  │
│  │ 后台 goroutine：探测/冷却恢复/日志清理    │  │
│  │ Web 管理台（go:embed 静态资源）          │  │
│  └───────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
        │            │            │
        ▼            ▼            ▼
    OpenAI      Anthropic     Gemini / DeepSeek / vLLM / 任意兼容端点
```

### 4.2 目录结构（Go 标准布局）

```
evergate/
├── cmd/evergate/main.go          # 唯一入口：加载配置 → 迁移 DB → 启动 HTTP
├── internal/
│   ├── config/                  # YAML 引导配置加载
│   ├── server/                  # Gin 路由注册、中间件（鉴权/日志/recovery）
│   ├── relay/                   # 核心代理：上游请求构造、SSE 透传、超时控制
│   ├── router/                  # 模型 → 渠道候选列表（优先级+权重）
│   ├── pool/                    # key 池调度器 + cooldown 状态机
│   ├── retry/                   # 错误分类器 + 重试预算
│   ├── provider/                # 上游适配器接口 + openai.go / anthropic.go / gemini.go ...
│   ├── store/                   # GORM 模型与数据访问
│   ├── admin/                   # 管理台 API handlers
│   └── health/                  # 渠道探测任务
├── web/                         # React 管理台源码（构建产物嵌入）
├── evergate.yml                  # 引导配置（端口/DB路径/主密钥/默认超时）
├── Makefile
└── README.md
```

### 4.3 数据模型（SQLite 表）

**channels（上游渠道）**
| 字段 | 类型 | 说明 |
|---|---|---|
| id | INTEGER PK | |
| name | TEXT | 展示名 |
| type | TEXT | openai / anthropic / gemini / openai-compatible / deepseek / vllm ... |
| base_url | TEXT | 上游地址 |
| priority | INTEGER | 越小越优先（同组路由排序） |
| weight | INTEGER | 同优先级内加权随机（默认 1） |
| models | TEXT(JSON) | 该渠道支持的模型名列表 |
| model_mapping | TEXT(JSON) | {请求模型: 上游模型} 映射，空则同名透传 |
| enabled | BOOL | 管理开关 |
| status | TEXT | active / degraded / down（由健康任务+实时错误统计推导） |
| timeout_config | TEXT(JSON) | 连接/首token/总时长覆盖 |
| created_at / updated_at | | |

**channel_keys（上游凭据池）**
| 字段 | 说明 |
|---|---|
| id, channel_id | 归属渠道 |
| key_cipher | AES-GCM 加密后的上游 key |
| enabled / weight | 管理开关与权重 |
| state | active / cooldown / half_open / disabled |
| consecutive_fails, success/fail 计数, last_error | 状态机输入 |
| cooldown_until | 时间戳 |
| rpm_limit | 可选，本网关侧的每分钟请求上限 |
| last_used_at | |

**tokens（下游令牌 = 发给客户端的 key）**
| 字段 | 说明 |
|---|---|
| id, name | |
| key_hash | SHA-256（原文仅创建时展示一次） |
| enabled, quota, used_quota, expired_at | 额度与过期 |
| allowed_models(JSON), allowed_ips(JSON) | 细粒度限制（P1 可先留字段） |
| group | 令牌分组（对应渠道分组，P1） |

**request_logs（请求日志，核心调试体验）**
| 字段 | 说明 |
|---|---|
| id, created_at | |
| token_id, token_name | 下游来源 |
| channel_id, channel_name, key_id | **实际命中的渠道与 key** |
| model（客户端请求）, upstream_model | |
| stream | 是否流式 |
| status_code, error_type, error_message | |
| retry_trace(JSON) | **重试轨迹**：[{channel,key,attempt,error,elapsed_ms}...]，管理台详情页可视化 |
| first_token_ms, total_ms | 延迟 |
| prompt_tokens, completion_tokens, cost | 用量 |
| ip, user_agent | |

**usage_daily（聚合表，供报表）**
| date, channel_id, model, token_id, requests, success, tokens_in, tokens_out, cost |

**admins（管理台账号）**：id, username, password_hash(bcrypt), created_at。首次启动若空表，创建默认 admin/admin123 并强制首登改密。

### 4.4 对外 API

**OpenAI 兼容（客户端侧）**
- `POST /v1/chat/completions` — 完整兼容（含 `stream:true` SSE、tool calls 透传）
- `GET /v1/models` — 所有启用渠道模型去重合并
- `GET /healthz` — 存活检查
- 错误格式：与 OpenAI 一致的 `{error:{message,type,code}}`；全部渠道耗尽时返回 `503` + 明确 message

**管理台（/admin/api/*，session 鉴权）**
- `POST /admin/api/auth/login|logout`、`GET me`
- 渠道：`GET/POST/PUT/DELETE /admin/api/channels`、`POST /channels/:id/test`（发起真实轻量请求测试连通）、`POST /channels/:id/toggle`
- 凭据：`POST /channels/:id/keys`（批量粘贴多行 key）、启停、删除
- 令牌 CRUD、日志查询（分页+多维筛选）、用量统计、Playground 代理（`POST /admin/api/playground/chat`，走内部调用并返回命中渠道信息）
- `GET /metrics`（Prometheus 文本格式，无需鉴权，绑本地）

### 4.5 核心机制设计（调度内核 — 本项目的心脏）

#### ① 路由（模型 → 渠道候选列表）
1. 解析请求 `model`，取所有 `enabled=true` 且 models 包含该模型的渠道
2. 按 `priority` 升序分组；同优先级内按 `weight` 加权随机排序
3. 模型映射：若渠道 model_mapping 命中，替换为上游模型名；否则同名透传
4. 会话亲和（P1）：请求头 `X-Session-Id` 命中粘性表（TTL 10 分钟）时优先用上次渠道+key

#### ② key 池调度（渠道内）
- 每渠道 key 按权重轮询；**跳过 state=cooldown/disabled 的 key**
- key 全部不可用 → 视为渠道故障，进入下一候选渠道

#### ③ cooldown 状态机（每 key 维度）
```
active --连续失败 N 次(默认3)--> cooldown(duration 按 30s→2m→10m 指数退避, 上限30m)
cooldown --到期--> half_open（放行 1 个真实请求作探测）
half_open --成功--> active（计数清零）
half_open --失败--> cooldown(duration 翻倍)
```
- 429 且响应带 `Retry-After` → cooldown_until = now + Retry-After（优先于默认退避）
- 401/403（key 无效/被封）→ 直接 `disabled` + 管理台告警标记，不参与退避
- 管理员手动 enable/disable 永远优先于自动状态
- 状态机内存运行 + 定期持久化到 DB（重启后恢复冷却，避免重启瞬间打死坏 key）

#### ④ 错误分类器（决定重试行为）
| 类别 | 判定 | 动作 |
|---|---|---|
| 可切换重试 | 网络错误/TLS/连接超时、408、409、429、5xx、**首 token 前超时** | 标记 key 失败 → 尝试下一个 key/渠道 |
| 不可重试 | 400、404、413、422（请求本身有问题，换渠道也一样） | 直接透传错误给客户端 |
| 凭据问题 | 401、403 | 禁用该 key → 换 key 继续；全部 key 被禁 → 渠道标记 down |
| 内容策略 | 上游返回内容安全拦截类错误 | 透传（换渠道重试同样的违规内容没有意义，P1 可加"策略可重试"开关） |

#### ⑤ 重试预算（防雪崩）
每请求：最多尝试 `3 个渠道 × 每渠道 2 个 key`；总时长上限默认 60s（可配）。超预算返回 503 + 汇总错误信息。

#### ⑥ SSE 流式红线（最重要的一条）
- **可重试窗口 = 从发起上游请求到「上游首个 chunk 写给客户端」之前**。窗口内任何失败 → 无感切换下一个候选
- **首个 chunk 已写给客户端 → 永不重试**，只能通过 SSE error event 明确上报，避免客户端收到重复内容
- 透传实现：`io.Copy` 风格的 flush 循环，**禁止整段缓冲**；上游空闲超过 keepalive 间隔（默认 15s）向客户端发送 SSE 注释 `: keepalive` 防中间层掐断
- 首独 token 超时（默认 30s，渠道可覆盖）：窗口内超时属于可重试错误

#### ⑦ 健康探测（后台任务）
- 每 5 分钟对 cooldown/half_open 渠道执行轻量探测（低成本请求或 models list）
- 渠道 status 汇总推导：全部 key cooldown → down；部分可用 → degraded；管理台列表实时展示

### 4.6 管理后台页面清单（P0 范围加粗）

| # | 页面 | 核心内容 |
|---|---|---|
| 1 | **登录** | 用户名/密码，首登强制改密 |
| 2 | **Dashboard** | 今日请求量/成功率/P95 延迟/成本卡片；渠道健康状态列表（active/degraded/down） |
| 3 | **渠道管理** | 列表（状态徽标、优先级、权重、模型数、实时成功率）；新增/编辑：类型、base_url、key 池（**支持多行批量粘贴**）、模型列表、映射、超时；**单渠道测试按钮**；启停 |
| 4 | **令牌管理** | 下游 key CRUD、额度、过期时间、允许模型（P0 可只做基础字段） |
| 5 | **请求日志** | 时间/渠道/模型/状态/令牌多维筛选；详情抽屉：**重试轨迹可视化**（每次尝试命中哪个渠道哪个 key、失败原因、耗时）——调试体验的核心 |
| 6 | Playground | 选模型对话测试（流式渲染），显示实际命中渠道 —— P1 |
| 7 | 用量统计 | 按天/渠道/模型/令牌聚合图表与成本 —— P1 |
| 8 | 系统设置 | 默认超时/重试参数、日志保留天数、修改密码 —— P1 |

### 4.7 配置（evergate.yml，唯一需要文件的地方）

```yaml
server:
  port: 8080
  base_path: ""            # 可选挂在子路径下
storage:
  sqlite_path: ./data/evergate.db
  log_retention_days: 14
security:
  master_key: ""           # 为空则首次启动自动生成并写回；用于加密上游 key
  session_secret: ""       # 同上
defaults:
  connect_timeout: 10s
  first_token_timeout: 30s
  overall_timeout: 300s
  retry_budget: { channels: 3, keys_per_channel: 2, total: 60s }
  cooldown: { fail_threshold: 3, base: 30s, max: 30m }
```
业务数据（渠道/密钥/令牌）一律存 DB；YAML 仅做引导。管理台修改即时生效，无需重启。

### 4.8 可观测性
- Prometheus 指标：`gateway_requests_total{channel,model,status}`、`gateway_first_token_ms_bucket`、`gateway_upstream_errors_total{channel,type}`、`gateway_key_cooldown_total`、`gateway_active_key_gauge`
- 每请求一条 request_log（含 retry_trace）；异步批量写库（带缓冲 channel，不阻塞请求路径）
- slog JSON 运行日志，启动时打印版本/端口/DB 路径

### 4.9 非功能指标（验收测量项）
| 指标 | 目标 |
|---|---|
| 二进制体积 | < 30MB（未压缩） |
| 空闲内存 | < 50MB |
| 冷启动 | < 1s（到 /healthz 200） |
| 网关自身首 token 额外延迟 | < 5ms（与直连对比） |
| 平台 | windows/amd64、linux/amd64、darwin/arm64 交叉编译通过 |
| 优雅停机 | SIGTERM 后停止接新请求，在途请求最长 30s 排空 |

---

## 5. 分期计划与验收标准

### P0 — 可用（本次开发必须全部完成）

**范围**：OpenAI 兼容 `/v1/chat/completions`（含流式与 tool calls 透传）+ `/v1/models`；渠道与 key 池管理（增删改查、批量导 key、连通测试）；key 调度 + cooldown 状态机 + 错误分类重试；SSE 零缓冲透传；请求日志（含重试轨迹）+ 下游令牌校验；管理台：登录 / Dashboard / 渠道管理 / 令牌管理 / 请求日志；AES 加密存储上游 key；单二进制构建。

**P0 验收清单（全部通过才算完成）**
1. `make build` 产出单二进制；`./evergate` 启动后 `:8080/v1/*` 与管理台同时可用
2. **故障切换（非流式）**：配置双渠道，mock 主渠道返回 500 → 请求自动落到次渠道，客户端收到正常响应且无感知
3. **故障切换（流式，首 token 前）**：主渠道建连后挂起不返回 → 首token超时 → 自动切换次渠道，客户端正常收到完整流式输出
4. **首 token 后失败**：上游流中途断开 → 客户端收到明确 SSE 错误，**不出现重复内容**
5. **429 处理**：mock 上游 429 + `Retry-After: 60` → 该 key cooldown_until = now+60s，期间请求自动用其他 key
6. **401 处理**：某 key 返回 401 → 该 key 自动 disabled，其余 key 正常服务
7. **全部渠道耗尽**：返回 503 + OpenAI 格式错误体，管理台渠道状态显示 down
8. 重启进程：渠道/密钥/令牌/日志完整保留，cooldown 状态从 DB 恢复
9. 上游 key 在 DB 中为密文；管理台任何接口不回显明文
10. 调度器（状态机+错误分类）单元测试覆盖 ≥ 90% 分支；提供基于 httptest mock 上游的集成测试

### P1 — 好用
优先级/权重会话亲和（X-Session-Id 粘性）、渠道健康自动探测与状态徽标、用量成本统计页、Playground（显示命中渠道）、令牌额度/过期/模型白名单强制、请求级 config 覆盖（参考 Portkey config 结构）、系统设置页、OpenTelemetry 可选导出。

### P2 — 进阶
Anthropic 原生入口（/v1/messages）与协议互转、多用户与角色、请求/响应语义缓存、按预算自动模型降级（参考 TensorZero 思路）、插件钩子。

---

## 6. 给开发代理的实施指令

**开发顺序（严格按序，每步可运行可验证再进入下一步）：**
1. 项目骨架：Go module + Gin + GORM + SQLite(WAL) + AutoMigrate + 配置加载 + slog + /healthz
2. store 层：全部表模型与 DAO + AES 加密工具 + 首次启动初始化（默认管理员、生成 master_key）
3. 调度内核（纯逻辑，先写测试）：错误分类器 → cooldown 状态机 → key 池调度 → 路由器；用表驱动单测覆盖第 4.5 节全部规则
4. relay：OpenAI 兼容上游适配器 + SSE 透传 + 首 token 超时 + 重试预算串联；httptest mock 双渠道集成测试（覆盖验收清单 2-7）
5. 管理 API：auth/channels/keys/tokens/logs 的 REST + 令牌鉴权中间件 + 请求日志异步落库
6. 前端管理台：Vite 脚手架 → 登录 → 渠道管理 → 令牌 → 日志 → Dashboard
7. go:embed 嵌入 + Makefile 三平台交叉编译 + README（部署与配置说明）

**工程纪律：**
- 不引入第 3.3 节禁止的任何依赖；新增依赖须有明确理由并在 PR 说明
- 严格区分"可复制源码"（gpt-load/bifrost/litellm核心）与"禁止复制"（new-api）
- 所有时间使用 UTC 存储，展示层转本地时区
- 数据库迁移只用 GORM AutoMigrate，不手写 SQL 迁移脚本（个人项目阶段）
- 每个 HTTP handler 有 recovery；上游 panic 不得拖垮进程
- 提交信息按 `feat/fix/test/docs:` 前缀

---

## 7. 参考资料

| 资料 | 链接 |
|---|---|
| LiteLLM Router 文档（路由语义标准） | https://docs.litellm.ai/docs/routing |
| gpt-load（调度内核参考，MIT） | https://github.com/tbphp/gpt-load |
| bifrost（性能与 fallback 参考，Apache-2.0） | https://github.com/maximhq/bifrost |
| new-api（管理台产品设计参考，AGPL 勿抄码） | https://github.com/QuantumNous/new-api |
| Portkey gateway config 结构（MIT） | https://github.com/Portkey-AI/gateway |

