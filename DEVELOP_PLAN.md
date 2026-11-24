# 开发计划与验收

> 需求与技术设计全文见 [docs/AI网关项目-开发任务书.md](./docs/AI网关项目-开发任务书.md)。本文件只管任务、状态与验收，架构与技术选型见 [README.md](./README.md)，改码规则见 [AGENTS.md](./AGENTS.md)。

## 状态枚举

| 状态 | 含义 |
|---|---|
| planned | 已建卡确认，未动手 |
| in-progress | 执行中 |
| done | 验收标准全部满足且有验证证据 |
| blocked | 受阻；最新进度行注明原因与可否绕行 |
| canceled | 放弃；最新进度行注明原因 |
| deferred | 延后；最新进度行注明重启条件 |

## 卡面约定

- **WHEN/THEN 场景写法**（接口 / 交互类任务可选）：
  - WHEN <操作或输入>
  - THEN <可观察的预期结果>
  - 例：WHEN 查询 32 天范围 THEN 返回 400 并提示超出
- **done 卡不删**：完成的任务保留在任务清单中——进度即项目史；**归档规则**（T73）：主文件只保留非 done 卡，done 卡按阶段原样迁入 `docs/history/P0–P3.md`，"只追加"在归档内继续生效
- **非 done 卡必填治理四字段**：`阻塞项 | 类型(外部|待复验|技术) | 重启条件 | 复查日期`；代码已发布仅差人工复验的 blocked 卡应转 done 并拆出 `verify` 卡，而不是长期挂 blocked
- **并发规则**：一个 AI 会话同一时间只持一张 in-progress 卡；修改 DEVELOP_PLAN.md 前先 `git pull --rebase`；决策落盘先于任何代码改动

## 里程碑

| 里程碑 | 内容 | 完成判据 |
|---|---|---|
| P0（功能完成，1 项性能待实测） | T0–T7 功能项全部 done | 「验收清单」1–10 全部通过；体积、内存、冷启动、三平台编译与优雅停机已有实测证据；`首 token 额外延迟 <5ms` 尚待真实上游环境实测，因此 P0 不宣称全部非功能项完成 |
| P1（已完成） | T10 Codex 接入；T11 启动打开管理台；T12 管理 UI 与更新检查；T18 Responses 原生上游透传与渠道协议能力；T19 系统设置；T20 健康探测；T21 令牌治理强制；T22 用量成本报表；T23 会话亲和；T24 请求级配置覆盖；T25 Playground；T26 Prometheus + OTel；T29 npm 平台包发布流水线；T30 一键自动更新；T32 全页面排版审计；T33 用量图表组件化 | T10–T12、T18–T26、T29–T30、T32–T33 全部按任务卡验收通过；T13 canceled 并已由 T37 吸收 |
| P1+（已完成增量） | T35a 自更新下载链 npmmirror 优先（done）；T35b 版本检查改用 npm dist-tags 优先（done） | 两张卡均已验收，随 v0.1.27 发布 |
| P2（已完成） | T28 渠道预设供应商模板（done）；T36 按预算自动模型降级（done）；T37 Anthropic 原生协议打通（吸收 T13，done）；T38 Gemini 原生协议打通（done）；插件钩子（暂定，有需要再建卡） | 已建卡项按任务卡验收；多用户与语义缓存已移除出计划（2026-11-16 用户决策）；T36 → T37 → T38 已全部交付 |
| P3（已完成，4 卡待复验/延后） | 已交付：T42 渠道探测协议化、T43 渠道模型目录发现与令牌模型选择器、T40 Windows 安装器、T44 Windows 无控制台启动与系统托盘、T41 Playground 四协议调试、T39 全弹性布局与间距系统重构、T46 版本发布门禁与预发布通道、T47 发布 v0.2.0、T51 发布 v0.3.0 Windows/Linux、T53 发布 v0.3.1 Windows/Linux、T54 更新网络容错、T55 npm 镜像更新资产包、T56 发布 v0.4.0 Windows/Linux、T57 JetBrains AI Assistant 流式等待/日志刷新/OpenCode 预设与 v0.5.0 发布、T58 OpenCode Go 会话头/模型目录/日志刷新稳定性与 v0.5.1 发布、T60 JetBrains 空增量保活修复与 v0.5.2 发布、T62 JetBrains + ZAI reasoning 流兼容修复与 v0.5.3 发布、T63–T68 质量审查修复批次、T70–T72 成本统计修复与 v0.5.4/v0.5.5；实现完成待人工复验：T49 Windows 托盘 DPI 截图矩阵、T50 安装包更新真实升级冒烟、T52 托盘 Logo 多 DPI 截图矩阵；延后：T48 macOS 安装器（需 Apple Developer Program） | 按任务卡验收；macOS 安装器需 Developer ID/notarization，后续通过 `ENABLE_MACOS_INSTALLER=true` 启用；Windows 固定路径安装、镜像保留数据、自启动、单实例、日志定位、Linux 归档、npm 五包与托盘 Logo 资源均已验证 |
| P4（当前阶段） | 三视角审视改进批次：T73 流程治理与文档体系、T74 CI 门禁补齐、T75 后端补齐、T76 前端设计系统、T77 前端页面体验、T78 README 与用户文档、T79 i18n | 排除有支出项（Windows 代码签名、Apple 签名公证）；各卡按验收标准通过；全部完成后 `go test -race ./...`、`npm test`、`npm run build`、`make build` 通过并记录证据 |

P0 功能完成定义（1–10 全部通过）：

1. `make build` 产出单二进制；`./evergate` 启动后 `:8787/v1/*`（端口经 evergate.yml 可配）与管理台同时可用
2. 故障切换（非流式）：配置双渠道，mock 主渠道返回 500 → 请求自动落到次渠道，客户端收到正常响应且无感知
3. 故障切换（流式，首 token 前）：主渠道建连后挂起不返回 → 首 token 超时 → 自动切换次渠道，客户端正常收到完整流式输出
4. 首 token 后失败：上游流中途断开 → 客户端收到明确 SSE 错误，不出现重复内容
5. 429 处理：mock 上游 429 + `Retry-After: 60` → 该 key cooldown_until = now+60s，期间请求自动用其他 key
6. 401 处理：某 key 返回 401 → 该 key 自动 disabled，其余 key 正常服务
7. 全部渠道耗尽：返回 503 + OpenAI 格式错误体，管理台渠道状态显示 down
8. 重启进程：渠道/密钥/令牌/日志完整保留，cooldown 状态从 DB 恢复
9. 上游 key 在 DB 中为密文；管理台任何接口不回显明文
10. 调度器（状态机+错误分类）关键状态/分支场景均有表驱动断言，标准 Go 语句覆盖率 ≥ 90%；提供基于 httptest mock 上游的集成测试。覆盖率报告必须标注为语句覆盖率，不得冒充分支覆盖率

非功能验收项：二进制体积 < 30MB；空闲内存 < 50MB；冷启动 < 1s（到 /healthz 200）；网关自身首 token 额外延迟 < 5ms（与直连对比）；三平台交叉编译通过；优雅停机（SIGINT/SIGTERM 后停止接新请求，在途请求最长 30s 排空）。

（技术背景见 [README.md](./README.md)，改码规则见 [AGENTS.md](./AGENTS.md)，勿在此重复）

## 任务清单

### P4 批次：三视角审视改进（2026-11-24 用户指令）

> 来源：产品经理 / 项目经理 / UI 设计师三视角审视报告。用户指令：**除涉及支出的项目外全部实施**。
> 排除（有支出）：Windows 代码签名证书、Apple Developer Program 签名与公证。
> 延后（非支出但本轮不可闭环）：内置 LiteLLM 价格表（需核实数据许可证与体积，建卡另议）；应用内自更新真实旧→新 E2E（需真实历史版本环境，写入发版检查清单由下次发版执行）；i18n 全量字典（T79，排在批次末尾，未完成则如实标注）。

### T48: 统一发版产物与桌面安装体验（P3，用户指令）

- 目标：统一 Windows/macOS/Linux 的公开交付形态和安装体验；Windows/macOS 各只提供一个安装器，安装后均为托盘/菜单栏桌面应用；Linux/Docker 提供一个无托盘 tar.gz；公开产物采用统一机器友好命名
- 已确认决策：
  1. 公开命名统一为 `evergate_<version>_<os>_<arch>[_setup].<ext>`；版本不带 `v`，系统使用 `windows/macos/linux`，架构使用 `amd64/arm64`；Windows 为 `evergate_X.Y.Z_windows_amd64_setup.exe`，macOS 为 `evergate_X.Y.Z_macos_arm64_setup.<pkg|dmg>`，Linux 为 `evergate_X.Y.Z_linux_amd64.tar.gz`，校验文件为 `evergate_X.Y.Z_checksums.txt`
  2. Windows/macOS 安装流程一致：欢迎/许可 → 检测新装或升级 → 可选登录自启 → 安装进度 → 可选立即启动；不提供程序目录、存储模式、配置目录或数据目录选择
  3. 程序位置固定：Windows `%LOCALAPPDATA%\Programs\EverGate`；macOS `~/Applications/EverGate.app`
  4. 配置与数据固定同一根目录：Windows `%LOCALAPPDATA%\EverGate`；macOS `~/Library/Application Support/EverGate`，内部为 `evergate.yml` 与 `data/evergate.db`
  5. 不兼容旧安装器及其自定义配置/数据路径；标准安装器只使用固定路径，安装或升级不得覆盖、删除已有配置与数据库
  6. 高级用户仍可通过 CLI `-config` 或 `EVERGATE_CONFIG` 覆盖，不进入安装器 UI
  7. 首批标准安装器之前的开发版本不作为自动更新兼容目标；允许用户手动安装一次标准版本完成迁移，不为旧更新器永久保留裸二进制资产
- 验收标准：
  1. Windows/macOS 安装器均只有上述五个用户步骤，无目录选择页；两端自启动默认关闭、完成后启动默认开启
  2. macOS 产出带原生菜单栏状态图标的 `EverGate.app`，不显示 Dock 图标；菜单提供打开管理台、打开日志、重启和退出；保持单实例
  3. Windows 安装器继续提供现有托盘功能；macOS 安装器完成后方可删除公开裸 macOS 二进制与 Windows console 的推荐下载入口
  4. Linux 只提供无托盘 `evergate_X.Y.Z_linux_amd64.tar.gz`，解压后包含可执行 `evergate`、LICENSE 与 README，可直接运行或放入 Docker 镜像
  5. 公开 GitHub Release 只提供 Windows/macOS 安装器、Linux tar.gz 与 checksums，不再发布平台裸二进制；npm 四包仍作为独立安装渠道发布，但不作为 GitHub 更新回退资产
  6. checksums 覆盖所有公开资产；发布门禁校验命名、包内版本、安装流程文档和四类公开产物，不再同时推荐旧命名
  7. macOS 安装器和 `.app` 必须完成 Developer ID 签名与 notarization；无签名/公证凭据时如实标记 blocked，不得宣称可直接分发
  8. 发布前完成 Windows/macOS 安装、升级保留数据、自启动、托盘交互、单实例、Linux tar.gz 解压运行与 Docker 使用验证
- 边界：本轮不改变业务 API/DB/schema；不回改已发布 v0.2.0 资产；不提供 Windows/macOS 便携版或跨平台二进制；macOS 首版仅支持 arm64，不含 Intel/Universal；不静默迁移旧安装器数据
- 状态：deferred
- 进度（只追加新行，不修改旧行）：
  - 2026-11-17 计划 用户确认统一公开产物与 Windows/macOS 安装流程；建卡待排期
  - 2026-11-17 决策 程序安装位置、配置位置和数据位置均固定，不在安装器提供路径选择；仅保留升级兼容和 CLI 高级覆盖
  - 2026-11-17 决策 用户确认当前仅自用，不需要兼容既有安装版本的自动更新链路；标准安装器发布后允许手动安装一次迁移，公开 Release 删除裸二进制
  - 2026-11-17 修正 用户确认忽略本机旧安装版本，按标准方案直接切换固定路径；卡面移除旧自定义路径升级保留要求，仅保留数据不覆盖
  - 2026-11-17 执行 开始实现标准产物、Windows/macOS 安装流程与 Linux tar.gz
  - 2026-11-17 执行 公开产物改为版本化 `evergate_<version>_<os>_<arch>[_setup].<ext>`；Release 只产出 Windows setup、macOS setup.dmg、Linux tar.gz 与 checksums；npm 四包继续独立发布，但不再作为 GitHub 裸二进制回退
  - 2026-11-17 执行 Windows 安装器改为固定程序/配置/数据路径，五段式流程包含安装状态、自启动、进度与完成后启动；macOS 新增菜单栏 App、自定义安装器 App、`.dmg` 打包、Developer ID/notarization 门禁与 LaunchAgent
  - 2026-11-17 验证 Windows 安装器已在临时隔离路径完成首次安装、升级保留 config/database、自启动默认关闭/按需启用、单实例、启动失败提示、静默卸载保留数据；GUI 子系统=2 且含 Per-Monitor V2 manifest
  - 2026-11-17 验证 Linux tar.gz 包含 `evergate`、`LICENSE`、`README.md`；Go 单元测试、前端构建、三平台构建、actionlint 与 shell/PowerShell 语法检查通过
  - 2026-11-17 阻塞 macOS App/安装器尚未在 macOS 主机构建、签名、公证和安装冒烟；仓库缺少 Developer ID 证书与 Apple notarization secrets，无法在本机 Windows 环境完成该验收
  - 2026-11-24 决策 转 deferred。阻塞项：macOS App/安装器签名与公证；类型：外部（需 Apple Developer Program 与 6 个签名/公证 Secrets）；重启条件：凭据可用并设置仓库变量 ENABLE_MACOS_INSTALLER=true；复查日期 2027-01-15。Windows/Linux 产物已随 v0.3.0+ 发布，不受本卡阻塞

### T73: 流程治理与文档体系重构（P1，质量审查）

- 目标：DEVELOP_PLAN 归档 done 卡、blocked 卡补治理字段并纠正状态；AGENTS.md 瘦身到 ≤60 行，功能专属约束迁入 `docs/design/`；补 `docs/RISK.md`、`docs/README.md`、`CONTRIBUTING.md`、`SECURITY.md`、Issue 模板、发版检查清单、回滚策略
- 验收标准：
  1. DEVELOP_PLAN.md 主文件 ≤300 行：仅保留 open/blocked/deferred 卡、状态枚举、卡面约定、里程碑与验收清单；done 卡按阶段原样迁入 `docs/history/P0.md … P3.md`（进度行不改写）
  2. T48 转 deferred（重启条件：Apple Developer Program 可用），T49/T50/T52 拆为 done + 验证卡 `verify`（待人工复验，写明复查日期）；所有非 done 卡含「阻塞项 / 类型 / 重启条件 / 复查日期」
  3. AGENTS.md ≤60 行且保留：通用规范五条、目录约定、红线、流程要求；迁出的约束在 `docs/design/约束-协议.md`、`docs/design/约束-管理台.md` 可索引
  4. `docs/RISK.md` 含 ≥8 条风险（可能性/影响/缓解/复查日期），第一条为 NPM_TOKEN 到期；`docs/README.md` 为文档索引；`CONTRIBUTING.md`、`SECURITY.md`、`.github/ISSUE_TEMPLATE/` 存在且引用的命令可用
  5. `docs/发版检查清单.md` 含：CHANGELOG 段、门禁、tag、下载校验、**真实升级一次**、回写证据；`docs/回滚与备份.md` 含 DB 备份位置、降级兼容说明
  6. README 删除可预见期内无法兑现的 macOS 安装器承诺，改为如实描述开关状态
- 边界：不改代码；不重写历史进度行；不删除任何 done 卡内容（仅移动）
- 状态：done
- 进度：
  - 2026-11-24 计划 用户确认三视角审视改进批次；本卡与 T74–T79 同时建卡
  - 2026-11-24 收尾 上一会话已落地文档体系，暂停 in-progress；由 T80 统一验收后流转
  - 2026-11-24 验证 T80 收尾验收：DEVELOP_PLAN 250 行、AGENTS 48 行；P0–P3 done 卡归档、风险/贡献/安全/Issue 模板/发版/回滚文档齐全；24 个 Markdown 文件相对链接全部可解析

### T74: CI 门禁补齐（P0，质量审查）

- 目标：新增 `ci.yml` 在 main push / PR 运行完整质量门禁；`release.yml` quality job 补 `go test -race`、relay 覆盖率、gitleaks、actionlint；macOS runner 编译 darwin cgo 变体（不发布）；npm 发布支持 OIDC Trusted Publishing 可选开关
- 验收标准：
  1. `ci.yml` 触发于 push main 与 pull_request；步骤含 `npm ci`、`npm test`、`npm run lint`、`go vet`、`go test -race -p 1 ./...`、`make coverage-scheduler`、`make coverage-relay`（阈值按当前基线设定并写入 Makefile）、gitleaks、actionlint、macOS job `CGO_ENABLED=1 GOOS=darwin go build ./internal/desktop/ ./cmd/evergate/`
  2. `release.yml` quality job 同步上述 Go 侧步骤（race、relay 覆盖率、gitleaks、actionlint）
  3. release.yml npm 发布：当仓库变量 `NPM_TRUSTED_PUBLISHING=true` 时使用 `permissions.id-token: write` + `npm publish --provenance`，否则沿用 `NPM_TOKEN`；`docs/发版检查清单.md` 写明 npm 侧配置步骤
  4. `actionlint` 本地或 CI 通过；workflow YAML 语法有效
- 边界：不改业务代码；relay 覆盖率阈值不得高于当前实测值（如实记录基线）
- 状态：done
- 进度：
  - 2026-11-24 计划 建卡
  - 2026-11-24 收尾 上一会话已落地 CI 与覆盖率门禁，暂停 in-progress；由 T80 统一验收后流转
  - 2026-11-24 验证 `ci.yml` 与 `release.yml` 的 race、relay 覆盖率、gitleaks、actionlint、macOS cgo/OIDC 步骤齐全；actionlint 通过；gitleaks 扫描 146 commits，无泄漏（测试夹具使用最小路径 allowlist）；races 与远端 CI 在 T80 推送后执行
  - 2026-11-24 验证 远端 CI 的 Quality（go -race/覆盖率/前端）与 Security（gitleaks/actionlint）已通过；macOS hosted runner 在分配前被 GitHub 取消，拆分 T74v 跟踪，workflow 改为不阻塞主门禁

### T75: 后端补齐：统计聚合、导出、审计、随机初始密码、迁移备份（P1）

- 目标：补齐前端与文档所需的后端能力：`GET /admin/api/logs/stats` 聚合、`GET /admin/api/logs/export`（CSV）、`GET/POST /admin/api/config/export|import`（不含任何明文密钥/哈希）、管理操作审计表与 `GET /admin/api/audit-logs`、`POST /admin/api/channels/test-all`、首启随机管理员密码、AutoMigrate 前自动备份 SQLite
- 验收标准：
  1. `logs/stats` 接受与 `logs` 相同筛选参数，返回 `total / success / success_rate / p50 / p95 / avg_total_ms`，基于全量匹配行计算（非最新 100 条）；单测覆盖
  2. `logs/export` 以 `text/csv` 返回当前筛选结果（上限 10,000 行），列含 id/时间/模型/渠道/令牌/状态/首 token/总耗时/tokens/成本/错误类型；不含请求体与任何 key
  3. `config/export` 返回 JSON：渠道（含 model_mapping、超时字段，**不含 key**）、模型降级链、系统设置、价格覆盖、令牌（**不含哈希**）；`config/import` 为幂等 upsert（按名称匹配），返回 created/updated/skipped 计数；导出/导入均写审计
  4. `audit_logs` 表（id/created_at/username/action/target/summary/ip），中间件在 channels/keys/tokens/settings/fallbacks/prices/config 的写操作成功后记录；`audit-logs` 分页查询接口；AutoMigrate 建表
  5. `channels/test-all` 对全部启用渠道并发（≤4）执行现有探测逻辑，返回每渠道结果，单次 ≤30s
  6. 首次启动若需创建管理员，生成 16 位随机密码（含大小写/数字/符号中至少三类）替代 `admin123`，以 WARN 级别同时写入 stdout 与文件日志，并打印「仅首次启动、首登强制改密」提示；`evergate.yml.example`/README 同步；`EVERGATE_INITIAL_ADMIN_PASSWORD` 环境变量可覆盖（用于自动化）；现有 admin 测试改为注入密码
  7. `store.Open` 在执行 AutoMigrate 前，若检测到 `schema_version` 变化或表结构变更，复制 `evergate.db` 为 `backups/evergate-<utc>.db`（保留最近 5 份），失败仅告警不阻断
  8. `go test -race ./...`、`go vet`、`make build` 通过；新增 handler 有表驱动测试；所有新接口经 sessionMiddleware 且不回显任何密钥
- 边界：不引入新依赖；不改 relay 调度逻辑；不做多管理员
- 状态：done
- 进度：
  - 2026-11-24 计划 建卡
  - 2026-11-24 收尾 上一会话已落地后端能力与测试，暂停 in-progress；由 T80 统一验收后流转
  - 2026-11-24 验证 日志 stats/CSV、配置导入导出、审计、test-all、随机初始密码、迁移备份的后端测试通过；secret 导出/审计回显检查通过，新增路由均经 sessionMiddleware

### T76: 前端设计系统：token、暗色模式、组件与可访问性（P1，UI 审查）

- 目标：建立颜色 token 层并迁移全部硬编码调色板类；新增 `StatusBadge`、`ConfirmDialog`、`SettingRow`、`Skeleton`、`Pagination`、`usePageTitle`；修复 Dialog/Drawer 可访问性；Field 自动关联控件；统一导航中文；移除 `alert/confirm` 与 emoji 图标
- 验收标准：
  1. `index.css` 通过 Tailwind 4 `@theme` 定义 `--color-success/-warning/-danger/-info/-muted` 各 ≥3 阶（subtle/text/solid/border），并定义 `dark` 自定义变体；提供 `.dark` 类切换与侧栏开关，持久化到 localStorage，默认跟随 `prefers-color-scheme`
  2. `web/src` 内 grep `(text|bg|border)-(red|emerald|green|amber|yellow|rose|sky|blue|violet|purple|teal|orange)-\d{2,3}` 命中为 0（indigo 品牌色除外，需集中为 `--color-brand-*` token）
  3. `StatusBadge` 收口 `format.ts stateTone`、`Tokens.tsx tokenStatusTone`、`Playground.tsx statusPill`、`format.ts statusTone` 四处映射；`Badge tone` 改为枚举 `success|info|warning|danger|neutral|brand`
  4. `ConfirmDialog` 替代全部 `alert()/confirm()`（Channels.tsx:130/185/252、Tokens.tsx:94）；危险操作主按钮为 danger 样式且文案说明不可逆后果
  5. Dialog/Drawer：`role="dialog"`、`aria-modal="true"`、`aria-labelledby`、关闭按钮有 `aria-label`、Esc 关闭、打开时焦点移入、关闭后焦点归还、Tab 焦点循环；vitest 覆盖 Esc 与焦点归还
  6. `Field` 通过 `useId` 生成 id 并透传给首个控件（Input/Textarea/Select 接受 id），裸 `<label>` 全部加 `htmlFor`；`SettingRow` 替代 Channels×3 + Tokens×1 重复块且整块可点击切换
  7. `Skeleton` 用于 Dashboard/Tokens/Logs/Channels 首次加载；`usePageTitle` 在每个页面设置 `document.title` 为「<页面名> · EverGate」；导航统一中文（运行概览/调试台），👤 替换为 lucide `UserRound`
  8. `npm run lint`、`npm test`、`npm run build` 通过；`npm run audit:layout` 通过
- 边界：不改页面业务逻辑（T77 负责）；不引入 UI 库依赖
- 状态：done
- 进度：
  - 2026-11-24 计划 建卡
  - 2026-11-24 收尾 上一会话已落地设计系统与组件测试，暂停 in-progress；由 T80 统一验收后流转
  - 2026-11-24 验证 语义色硬编码扫描为 0、浏览器 `alert/confirm` 调用为 0；Dialog/Drawer、ConfirmDialog、Field、Pagination 组件测试通过；前端 lint/test/build 通过

### T77: 前端页面体验补齐（P1，产品/UI 审查）

- 目标：渠道表单内联 key 输入、类型下拉说明、模型映射表格编辑、渠道级超时字段、「测试全部」与延迟排序；日志筛选改下拉 + 时间范围 + 页码分页 + CSV 导出；令牌创建成功给出客户端配置片段 + 剪贴板失败兜底；Dashboard 使用后端聚合、引导状态机、额度 80% 横幅；Settings 增配置导出/导入与审计日志入口；Playground 标注工具调用边界
- 验收标准：
  1. 新建渠道表单含「API key（可多行，可选）」，创建成功后一并导入；编辑态不显示该字段
  2. 「类型」下拉每项带一行说明；预设选中时自动隐藏不相关项；三个原生能力开关文案保留
  3. 模型映射从 JSON textarea 改为两列（对外模型 → 上游模型）表格，支持增删行，保留「切换 JSON」高级模式
  4. 渠道表单暴露连接/首 token/整体超时三个可空字段（单位秒，留空用全局）
  5. 渠道页「测试全部」按钮调用 `channels/test-all`，结果按延迟升序可排序展示；单渠道测试保留
  6. 日志页：渠道/令牌为 Select（数据来自 channels/tokens），新增日期范围（起/止）筛选；分页器显示页码与每页条数（20/50/100）；「导出 CSV」按钮下载当前筛选
  7. 令牌创建成功对话框显示可复制的 `OPENAI_BASE_URL` / `OPENAI_API_KEY` / `ANTHROPIC_BASE_URL` 等环境变量片段；`navigator.clipboard` 不可用或失败时回退到 `document.execCommand('copy')` 并提示手动复制
  8. Dashboard 成功率/P95 来自 `logs/stats`；空状态改为四段状态机（零渠道→有渠道零 key→零令牌→首次 200 前持续显示「复制 base_url 与令牌」卡片）；任一令牌剩余额度 ≤20% 且 quota>0 时显示横幅
  9. Settings 页新增「配置导出 / 导入」与「审计日志」区块（审计列表分页、显示时间/用户/动作/目标/IP）
  10. Playground 顶部说明：仅支持 prompt/system/model/stream，工具调用请用客户端；文案与后端边界一致
  11. `npm run lint`、`npm test`、`npm run build` 通过；新增交互有 vitest 覆盖（确认对话框、映射表格、剪贴板兜底）
- 边界：不改 relay；不新增页面路由（审计日志放 Settings 内）
- 状态：done
- 进度：
  - 2026-11-24 计划 建卡
  - 2026-11-24 收尾 上一会话已落地页面体验与交互测试，暂停 in-progress；由 T80 统一验收后流转
  - 2026-11-24 验证 渠道内联 key/映射/超时/test-all、日志筛选分页导出、令牌配置片段与剪贴板兜底、Dashboard 聚合引导、Settings 配置与审计均有实现；`t77` 6 个交互测试与全量前端测试通过

### T78: README 与用户文档补齐（P1，产品审查）

- 目标：README 明确定位与接入指南（Codex / Claude Code / Cline / Cursor / OpenCode / OpenAI SDK / curl）、FAQ、错误码表、HTTPS/反代指引、备份说明；新增 `docs/openapi.yaml`（下游 `/v1` 四协议）
- 验收标准：
  1. README「定位」一句话明确「个人 / 小团队的本地多供应商聚合器」；「接入」节含 ≥6 个客户端可直接复制的配置片段，base_url 与令牌来源准确（含 `server.base_path` 情况）
  2. `docs/FAQ.md` 覆盖：401/403/429/503 含义、cooldown 恢复、成本显示「未计价/无 usage」原因、Windows SmartScreen 提示、如何找到初始密码、如何备份
  3. `docs/HTTPS与反向代理.md` 给出 Caddy 与 Nginx 最小配置、`admin_loopback_only` 与 `base_path` 注意事项
  4. `docs/openapi.yaml` 覆盖 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1/messages/count_tokens`、`/v1beta/models/{model}:generateContent|streamGenerateContent|countTokens`、`/v1/models`，鉴权方式与错误体格式准确；通过 `node --check` 或 swagger-cli 校验（无则 YAML 解析）
  5. README 与 docs 内所有相对链接可解析（glob 校验文件存在）
- 边界：不复制任何 AGPL 项目内容；不承诺 macOS 签名安装器
- 状态：done
- 进度：
  - 2026-11-24 计划 建卡
  - 2026-11-24 收尾 上一会话已落地 README 与用户文档，暂停 in-progress；由 T80 统一验收后流转
  - 2026-11-24 验证 README 覆盖 6 类客户端接入；FAQ/HTTPS/回滚文档存在；OpenAPI 经 YAML 解析并核对四协议路径；README/docs 共 24 个 Markdown 文件相对链接可解析

### T79: i18n 基础设施与英文字典（P2，产品审查）

- 目标：前端引入零依赖 `t()` 字典与语言切换（zh-CN / en），导航、PageHeader、按钮、空状态、错误文案全部走字典；默认 zh-CN
- 验收标准：侧栏语言切换持久化；`web/src` 内 PageHeader title/description 与 nav 不再含硬编码中文字面量；`npm run build` 通过；字典有单测确保 zh/en 键集合一致
- 边界：页面内深层文案允许分批迁移并在本卡进度中如实标注剩余量
- 状态：done
- 进度：
  - 2026-11-24 计划 建卡，排在 T76–T78 之后
  - 2026-11-24 收尾 上一会话已落地 i18n 基础设施、导航和 PageHeader 部分迁移；由 T80 核对剩余范围
  - 2026-11-24 验证 侧栏语言切换持久化、zh/en 键集合一致；PageHeader 与 Empty 的硬编码属性命中均为 0，主要按钮、空状态、登录/改密与本地错误提示已迁移；剩余 507 个非字典/非注释中文字面量行按卡面边界留待后续深层文案批次

### T80: P4 批次统一收尾与验收（P0，用户指令）

- 目标：将 T73–T79 的未提交实现按验收标准逐项核对，修正计划状态和文档结构，补齐 T79 可见文案覆盖，完成 P4 全量门禁并交付可提交的工作树
- 验收标准：
  1. `DEVELOP_PLAN.md` 只保留一张 in-progress 卡；T73–T79 按真实结果转为 done 或有重启条件的 deferred，非 done 卡具备治理字段
  2. T79 的侧栏语言切换、PageHeader 标题/描述、主要按钮、空状态和错误提示走字典；深层页面文案剩余项在卡内如实记录
  3. `docs/openapi.yaml` 可解析，README/docs 相对链接可解析；AGENTS ≤60 行、DEVELOP_PLAN ≤300 行
  4. `go test -race -p 1 ./...`、`go vet ./...`、调度/relay 覆盖率、前端测试/lint/build、actionlint、`make build`、`make build-all` 通过
  5. 工作树只包含 P4 收尾相关改动；提交前 `git diff --check` 通过，完成后提交并推送 main，等待 CI 全绿
- 边界：不新增业务功能；不做有支出的签名/公证；不重发已发布 tag；深层低频文案允许作为 T79 后续批次记录，但不影响导航、页面标题、主要操作与错误反馈的语言切换
- 阻塞项：本机 CGO 已关闭且无 gcc，`go test -race` 只能在 Ubuntu CI 执行；类型：待复验；重启条件：收尾提交推送 main 后 CI 完成；复查日期：2026-11-24
- 状态：in-progress
- 进度：
  - 2026-11-24 计划 用户指令完成收尾；建立唯一收尾卡，统一验收 P4 已有实现并修正计划状态
  - 2026-11-24 验证 本地 Go/vet/覆盖率/前端/OpenAPI/链接/actionlint/gitleaks/build/build-all 通过；远端 Quality 与 Security 通过；首个 CI 发现的两处 SSE keepalive recorder race 已修复

### P3 待复验任务（由 T73 拆分）

### T74v: 复验 macOS cgo CI（P0，待外部环境）

- 目标：在可用 GitHub hosted macOS runner 上执行 `CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build ./internal/desktop/ ./cmd/evergate/`
- 验收标准：CI `macOS cgo build (no release)` job 实际分配到 runner 并全绿；若失败，记录编译日志并修复
- 阻塞项：当前私有仓库的 `macos-14` job 在 runner 分配前被 GitHub 取消；类型：外部（hosted macOS runner/额度）；重启条件：Actions 账单或配额恢复后设置仓库变量 `ENABLE_MACOS_CI=true`；复查日期：2026-12-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T74 拆出；Linux race、安全与构建已通过，仅 macOS cgo 环境待复验

### T49v: 复验 Windows 托盘 DPI 截图矩阵（P3，待人工复验）

- 目标：补齐 T49 剩余的人工复验项
- 验收标准：在 Windows 100% / 125% / 150% / 200% 缩放及两台不同 DPI 显示器间移动/重启后，托盘与 TrackPopupMenu 菜单文字、行高、圆角保持原生清晰；记录截图尺寸与实际 DPI
- 阻塞项：需实际切换系统缩放与显示器拓扑；类型：待复验；重启条件：具备多 DPI 环境；复查日期 2027-01-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T49 拆出

### T50v: 复验安装包级真实升级（P3，待人工复验）

- 目标：用真实历史版本 → 新版本完成一次管理台内更新
- 验收标准：安装 v0.5.x 后在管理台触发更新到下一正式版，经历下载/校验/安装/重启四阶段，配置、数据库与自启动选择保留；失败路径（校验失败、取消）保留旧版本可运行；回写证据到发版检查清单
- 阻塞项：需等待下一个正式版发布；类型：待复验；重启条件：v0.6.0 tag 可用；复查日期 2026-12-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T50 拆出，并入 docs/发版检查清单.md

### T52v: 复验 Windows 托盘 Logo 多 DPI 矩阵（P3，待人工复验）

- 目标：补齐 T52 剩余的人工复验项
- 验收标准：100% / 125% / 150% / 200% 与混合 DPI 下托盘 Logo 大小清晰、不模糊；右键菜单仍为原生 TrackPopupMenu；单实例与退出/重启行为不变
- 阻塞项：需实际切换系统缩放与显示器拓扑；类型：待复验；重启条件：具备多 DPI 环境；复查日期 2027-01-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T52 拆出
