# 开发规范

## 通用规范（必建五条）

1. **复用优先，不过度设计**：同一逻辑或结构出现第二份副本时，立即抽取为公共函数 / 公共样式 / 公共组件；前端表现层优先组件化；不牺牲可读性、可测性与可维护性，不为假想的未来需求预先抽象。
2. **文档同步**：DEVELOP_PLAN.md 逐任务更新（进度只追加、状态如实流转）；AGENTS.md 仅当开发规范变化时更新；README.md 仅当项目定位、结构、用法或环境变化时更新——新增功能默认只记录在 DEVELOP_PLAN.md，不逐条追加进 README，README 不是变更日志。文档与代码不同步视为任务未完成。
3. **计划执行分离**：动手前先在 DEVELOP_PLAN.md 建任务卡（目标、验收标准、边界）并经确认；执行中不得自行扩大任务范围，需求或范围变化先更新任务卡，再改代码。
4. **诚实终态**：声明完成必须附实际产出文件与验证证据（执行了什么、结果如何）；未运行的测试或验证不得声称通过；受阻时如实标注 blocked 并写明原因与可否绕行。
5. **精准修改**：每一行改动都必须能追溯到当前任务；不顺手改进相邻代码、注释或格式，不重构与当前任务无关的代码，也不动没坏的东西，匹配现有风格；只清理本次改动造成的孤儿代码（本次改动导致失效的导入 / 变量 / 函数）；发现既有死代码提及而不删，除非任务明确要求。注释只写代码装不下的信息（业务约束、非显然决策、坑位规避），不写改动叙事、阶段引用、代码复述。

## 目录约定（压缩版，完整版见 [docs/agents/目录约定.md](./docs/agents/目录约定.md)）

- `cmd/evergate/main.go` 唯一入口；`internal/` 每包一个职责（relay 转发、router/pool/retry/affinity 调度、store/admin/health/settings/usage 数据与管理、desktop/winres/browser 桌面、update/selfupdate/npmregistry 更新发布、metrics/telemetry 可观测、config/appdir/applog/server 基础设施）
- `web/` React 管理台（`web/dist` 随仓库提交，改前端后必须 `make build-web` 并提交）；`web/public/logo.svg` 品牌图标唯一视觉源
- `scripts/` 构建发布脚本；`installer/` 桌面安装器；`npm/` 发布模板；`docs/agents/` 领域文档；`docs/archive/` 归档任务卡
- 业务数据一律存 DB，不进 YAML；配置与数据路径统一由 `internal/appdir` 解析；Go 文件小写、测试同包白盒；React 组件 PascalCase

## 代码风格（跨模块不变量；功能专属约束见 [协议与转发](./docs/agents/约束-协议与转发.md)、[管理台与运维](./docs/agents/约束-管理台与运维.md)）

- Go：提交前 `gofmt` 无 diff、`go vet` 通过；错误必须处理或显式 `//nolint` 注明理由
- 所有时间以 UTC 存储，仅展示层转本地时区；对外错误统一 OpenAI 格式 `{error:{message,type,code}}`，渠道耗尽返回 503；管理台 API 统一 JSON 信封，**任何接口不得回显上游 key 明文**
- 下游令牌治理（停用/过期/IP/模型/额度）在调用上游前由公共中间件强制；扣减额度只用实际 usage，不得猜测
- 成本按请求完成时的价格快照写入，不追溯重算；usage 缺失 / 未计价必须显式标记

## 流程要求（分支 / 提交 / 测试 / 验证）

- **并发规则**：一个 AI 会话同一时间只持一张 in-progress 卡；修改 DEVELOP_PLAN.md 前先 `git pull --rebase`；决策落盘先于任何代码改动
- **分支**：从 `main` 切出 `feat/<简述>`、`fix/<简述>` 或 `codex/<简述>`；不在 `main` 上直接开发
- **开发顺序**：严格按 [DEVELOP_PLAN.md · 里程碑](./DEVELOP_PLAN.md#里程碑) 当前阶段顺序执行；依赖项先完成，每步可运行、可验证后再进入下一步
- **提交信息**：`feat:` / `fix:` / `test:` / `docs:` 前缀
- **提交前验证**：`go test -p 1 ./...`、`go vet ./...`、`make build` 通过；涉及前端时 `npm run lint`、`npm test`、`npm run build` 通过；CI（`ci.yml`）另跑 `go test -race`、调度覆盖率 ≥90%、relay 覆盖率 ≥81%、gitleaks、actionlint、macOS cgo 编译。覆盖率报告不得把语句覆盖率表述为分支覆盖率
- **新增依赖**：须有明确理由并在提交说明中记录；先对照禁止清单。本机 Go/Node 环境事实见 [docs/agents/开发环境.md](./docs/agents/开发环境.md)

## 版本与发布规则

- **单一事实源**：版本号以 git tag `vMAJOR.MINOR.PATCH[-pre.N]` 为准，必须打在 main 上；二进制注入、npm 包版本、CHANGELOG 标题全部由 tag 派生
- **bump 语义（0.x）**：修复 → PATCH；新增功能或破坏性变更 → MINOR；**1.0 之后**破坏性变更 → MAJOR。禁止把功能发布塞进 PATCH
- **预发布通道**：tag `-rc.N` / `-beta.N`，npm `--tag next`，GitHub Release 标记 prerelease；`latest` 只随正式版移动；MINOR 发布前先出 `-rc.1` 自用
- **发布门禁**：tag 必须有精确对应的 `## vX.Y.Z` CHANGELOG 段且在 main 历史上；发版步骤与"真实旧→新升级一次"见 [docs/agents/发版检查清单.md](./docs/agents/发版检查清单.md)；macOS 安装器只在 `ENABLE_MACOS_INSTALLER=true` 且签名/公证 Secrets 齐全时构建，永远不得发布未签名 DMG
- **开发构建**：`dev`、`-dirty`、`-N-g<sha>` 统一视为开发态，不参与更新比较

## 禁止事项

- **不引入**：Redis、消息队列、微服务拆分、K8s/Helm、ClickHouse、GraphQL、微前端、OAuth2/OIDC、多租户计费、语义缓存、向量库
- **许可证红线**：new-api（AGPL-3.0）任何代码/组件/样式不得复制，只看交互与功能清单；OmniRoute（灰色玩法）严禁参考、严禁引入其依赖。可复制源码范围仅限 gpt-load（MIT，保留版权声明）、bifrost（Apache-2.0，保留 NOTICE）、litellm 核心（MIT，不碰 enterprise/）
- **SSE 红线**：首个 chunk 已写给客户端后禁止重试；禁止整段缓冲上游响应（必须 flush 循环透传）
- **迁移红线**：数据库迁移只用 GORM AutoMigrate，禁止手写 SQL 迁移脚本；升级前自动备份见 `store.MigrateWithBackup`
- **稳定性红线**：每个 HTTP handler 必须有 recovery；上游 panic 不得拖垮进程

## 专题文档

- [目录与模块职责](./docs/agents/目录约定.md) · [协议与转发](./docs/agents/约束-协议与转发.md) · [管理台、可观测性与发布](./docs/agents/约束-管理台与运维.md)
- [运维配置](./docs/agents/运维配置.md) · [FAQ](./docs/agents/FAQ.md) · [HTTPS 与反向代理](./docs/agents/HTTPS与反向代理.md) · [回滚与备份](./docs/agents/回滚与备份.md)
- [发版检查清单](./docs/agents/发版检查清单.md) · [风险登记](./docs/agents/RISK.md) · [文档索引](./docs/README.md)

（项目背景见 [README.md](./README.md)，勿在此重复）
