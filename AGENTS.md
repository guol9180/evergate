# 开发规范

## 通用规范（必建四条）

1. **复用优先，不过度设计**：同一逻辑或结构出现第二份副本时，立即抽取为公共函数 / 公共样式 / 公共组件；前端页面表现优先组件化；保证代码质量（可读、可测、可维护），不为假想的未来需求预先抽象。
2. **文档同步更新**：每次开发结束或提交之前，同步更新三份文档：README.md（结构与用法变化）、AGENTS.md（规范变化）、DEVELOP_PLAN.md（任务状态与验收清单）；**文档与代码不同步视为任务未完成**。
3. **计划执行分离**：动手前先在 DEVELOP_PLAN.md 建任务卡（目标、验收标准、边界）并确认；执行中不得顺手扩范围，需求或范围变化先回任务卡更新，再改代码。
4. **诚实终态**：声明完成必须附实际产出文件与验证证据（执行了什么、结果如何）；未运行的测试或验证不得声称通过；受阻时如实标注 blocked 并写明原因与可否绕行。

## 目录约定

```
cmd/evergate/main.go   # 唯一入口：加载配置 → 迁移 DB → 启动 HTTP
internal/config/       # YAML 引导配置加载
internal/server/       # Gin 路由注册、中间件（鉴权/日志/recovery）
internal/relay/        # 核心代理：上游请求构造、SSE 透传、超时控制
internal/router/       # 模型 → 渠道候选列表（优先级+权重）
internal/pool/         # key 池调度器 + cooldown 状态机
internal/retry/        # 错误分类器 + 重试预算
internal/provider/     # 上游适配器接口 + openai.go / anthropic.go / gemini.go ...
internal/store/        # GORM 模型与数据访问
internal/admin/        # 管理台 API handlers
internal/health/       # 周期 /models 轻量探测、key cooldown 持久化与健康状态合成
internal/settings/     # SQLite 运行时设置、快照热更新与维护任务
internal/usage/        # 模型价格快照、请求成本、UTC 日幂等聚合与报表查询
internal/browser/      # 启动后打开管理台的系统浏览器封装
internal/update/       # GitHub Release 自动更新检查
web/                   # React 管理台源码（构建产物 go:embed 嵌入）
web/public/logo.svg    # EverGate 品牌图标唯一视觉源；favicon.ico 与 Windows .syso 均由它派生
docs/                  # 设计文档与需求（开发任务书等）
evergate.yml           # 引导配置（端口/DB路径/主密钥；运行时业务设置首次种子）
Makefile               # 构建入口（T7 实现三平台交叉编译）
```

- 业务数据（渠道/密钥/令牌）一律存 DB，不进 YAML；YAML 仅做引导。
- `web/dist` 随仓库提交（约 400KB），保证后端开发者无 Node 环境也能 `go build`；改前端后必须 `make build-web` 并提交更新后的 dist。
- 品牌图标以 `web/public/logo.svg` 为唯一视觉源；更换图标后需重生成 `web/public/favicon.ico` 并执行 `make icon`，再提交派生产物。
- Go 文件与包名小写；测试文件与被测文件同包（白盒）或 `_test` 包（黑盒）。
- React 组件文件 PascalCase，放 `web/src/` 下按页面/组件分目录。

## 代码风格

- Go：提交前 `gofmt` 无 diff、`go vet` 通过；错误必须处理或显式 `//nolint` 注明理由。
- 客户端接入协议矩阵当前范围为 OpenAI Chat Completions `/v1/chat/completions` 与 OpenAI Responses `/v1/responses`（上游、下游均为此两种）；Anthropic Messages `/v1/messages` 已明确延后（T13，重启前先回任务卡更新范围）。新增接入能力必须说明两种入口的可用性，不能只验证单一协议。
- Responses 原生能力是渠道级声明：声明渠道的 `/v1/responses` 必须仅改写鉴权、base_url 与模型映射并原样转发 JSON/SSE；未声明渠道才允许 R→CC 桥接。不可桥接的状态字段必须显式 400，不得静默丢弃。
- 所有时间以 UTC 存储，仅展示层转本地时区。
- 对外错误响应统一 OpenAI 格式 `{error:{message,type,code}}`；全部渠道耗尽返回 503。
- Responses 下游的 `function` / `custom` 工具转换必须保留客户端声明的工具语义，custom grammar 只能降级为提示不能声称硬约束；新增 Responses 工具或事件前先对照 OpenAI 官方 schema 与允许参考项目，并用公共 HTTP 边界写回归测试。
- Responses namespace 工具扁平化必须生成确定性且全局唯一的 Chat 函数名；响应输出与历史回传必须恢复原始 `namespace/name`，不得只按局部工具名去重或依赖跨请求内存状态。
- 管理台 API 返回统一 JSON 结构；任何接口不得回显上游 key 明文。
- 前端路由守卫依赖的服务端状态（如 `['me']`）被 mutation 改变时，必须在成功回调同步更新 TanStack Query 缓存；仅重新导航不得依赖旧缓存判断。
- 运行日志（slog JSON 到 stdout）与请求日志（入库供管理台查询）分离。
- 流式请求的 `first_token_ms` 必须从发起上游请求开始计时，到首条 SSE data 为止；Chat Completions 与 Responses 两条流式路径不得各自定义口径。
- 渠道健康探测只能调用 `GET {base_url}/models`，不得触发推理接口；404/405 必须标记 inconclusive 并释放 half-open 探测名额，不得改变 key 调度状态。
- 下游令牌治理（停用/过期/IP/模型/额度）必须在调用上游前由公共中间件与 relay 公共检查点强制；成功扣减额度只能使用实际输入+输出 usage，客户端断开、上游失败或 usage 缺失不得猜测扣减。
- 成本必须在请求完成时按当时价格快照写入 request_logs，价格变更不得追溯重算；usage 缺失或未匹配价格必须显式标记 no_usage/unpriced。usage_daily 一律从 request_logs 按 UTC 日事务内先删后插重建，禁止增量累加造成重复计数。

## 流程要求（分支 / 提交 / 测试 / 验证）

- **开发顺序**：严格按 [DEVELOP_PLAN.md · 任务清单](./DEVELOP_PLAN.md#任务清单) 任务卡顺序执行，每步可运行、可验证后再进入下一步。
- **提交信息**：`feat:` / `fix:` / `test:` / `docs:` 前缀。
- **提交前验证**：后端 `go test ./...` 通过且 `make build` 成功；涉及前端时 `npm run build` 成功；涉及调度内核时分支覆盖率 ≥ 90%。
- **新增依赖**：须有明确理由并在提交说明中记录；先对照下方禁止清单。
- **依赖下载**：`go mod download` / `npm install` 受网络沙箱限制，失败时走审批，不得绕过。本机 Go 已配置用户级 `GOPROXY=https://goproxy.cn,direct`（sum.golang.org 不可达）；沙箱内执行 go 命令时需设 `GOCACHE` 指向仓库内 `.gocache/`（已 gitignore），避免写 C 盘被拒。

## 禁止事项

- **不引入**：Redis、消息队列、微服务拆分、K8s/Helm、ClickHouse、GraphQL、微前端、OAuth2/OIDC、多租户计费、语义缓存、向量库。
- **许可证红线**：new-api（AGPL-3.0）任何代码/组件/样式不得复制，只看交互与功能清单；OmniRoute（灰色玩法）严禁参考、严禁引入其依赖。可复制源码范围仅限 gpt-load（MIT，保留版权声明）、bifrost（Apache-2.0，保留 NOTICE）、litellm 核心（MIT，不碰 enterprise/）。
- **SSE 红线**：首个 chunk 已写给客户端后禁止重试；禁止整段缓冲上游响应（必须 flush 循环透传）。
- **迁移红线**：数据库迁移只用 GORM AutoMigrate，禁止手写 SQL 迁移脚本（个人项目阶段）。
- **稳定性红线**：每个 HTTP handler 必须有 recovery；上游 panic 不得拖垮进程。

（项目背景见 [README.md](./README.md)，勿在此重复）
