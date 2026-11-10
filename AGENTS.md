# 开发规范

## 通用规范（必建四条）

1. **复用优先，不过度设计**：同一逻辑或结构出现第二份副本时，立即抽取为公共函数 / 公共样式 / 公共组件；前端页面表现优先组件化；保证代码质量（可读、可测、可维护），不为假想的未来需求预先抽象。
2. **文档同步**：每次开发结束或提交之前，同步更新 README.md、AGENTS.md、DEVELOP_PLAN.md；文档与代码不同步视为任务未完成。
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
internal/health/       # 渠道探测任务
web/                   # React 管理台源码（构建产物 go:embed 嵌入）
docs/                  # 设计文档与需求（开发任务书等）
evergate.yml           # 引导配置（端口/DB路径/主密钥/默认超时）
Makefile               # 构建入口（T7 实现三平台交叉编译）
```

- 业务数据（渠道/密钥/令牌）一律存 DB，不进 YAML；YAML 仅做引导。
- `web/dist` 随仓库提交（约 400KB），保证后端开发者无 Node 环境也能 `go build`；改前端后必须 `make build-web` 并提交更新后的 dist。
- Go 文件与包名小写；测试文件与被测文件同包（白盒）或 `_test` 包（黑盒）。
- React 组件文件 PascalCase，放 `web/src/` 下按页面/组件分目录。

## 代码风格

- Go：提交前 `gofmt` 无 diff、`go vet` 通过；错误必须处理或显式 `//nolint` 注明理由。
- 所有时间以 UTC 存储，仅展示层转本地时区。
- 对外错误响应统一 OpenAI 格式 `{error:{message,type,code}}`；全部渠道耗尽返回 503。
- 管理台 API 返回统一 JSON 结构；任何接口不得回显上游 key 明文。
- 运行日志（slog JSON 到 stdout）与请求日志（入库供管理台查询）分离。

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
