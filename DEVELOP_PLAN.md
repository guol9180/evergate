# 开发计划与验收

> 已完成的历史需求原文见 [归档任务书](./docs/archive/historical-AI网关项目-开发任务书.md)，仅用于追溯。本文件只管任务、状态与验收，当前定位与技术选型见 [README.md](./README.md)，改码规则见 [AGENTS.md](./AGENTS.md)。

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
- **done 卡不删**：完成的任务保留在归档中——进度即项目史；**归档规则**：主文件只保留非 done 卡，done 卡按阶段原样迁入 `docs/archive/`，"只追加"在归档内继续生效
- **非 done 卡必填治理四字段**：`阻塞项 | 类型(外部|待复验|技术) | 重启条件 | 复查日期`；代码已发布仅差人工复验的 blocked 卡应转 done 并拆出 `verify` 卡，而不是长期挂 blocked
- **并发与流程规则**：见 [AGENTS.md · 流程要求](./AGENTS.md)

## 里程碑

| 里程碑 | 内容 | 完成判据 |
|---|---|---|
| P0（功能完成，1 项性能待实测） | T0–T7 功能项全部 done | 「验收清单」1–10 全部通过；体积、内存、冷启动、三平台编译与优雅停机已有实测证据；`首 token 额外延迟 <5ms` 尚待真实上游环境实测，因此 P0 不宣称全部非功能项完成 |
| P1（已完成） | T10 Codex 接入；T11 启动打开管理台；T12 管理 UI 与更新检查；T18 Responses 原生上游透传与渠道协议能力；T19 系统设置；T20 健康探测；T21 令牌治理强制；T22 用量成本报表；T23 会话亲和；T24 请求级配置覆盖；T25 Playground；T26 Prometheus + OTel；T29 npm 平台包发布流水线；T30 一键自动更新；T32 全页面排版审计；T33 用量图表组件化 | T10–T12、T18–T26、T29–T30、T32–T33 全部按任务卡验收通过；T13 canceled 并已由 T37 吸收 |
| P1+（已完成增量） | T35a 自更新下载链 npmmirror 优先（done）；T35b 版本检查改用 npm dist-tags 优先（done） | 两张卡均已验收，随 v0.1.27 发布 |
| P2（已完成） | T28 渠道预设供应商模板（done）；T36 按预算自动模型降级（done）；T37 Anthropic 原生协议打通（吸收 T13，done）；T38 Gemini 原生协议打通（done）；插件钩子（暂定，有需要再建卡） | 已建卡项按任务卡验收；多用户与语义缓存已移除出计划（2026-11-16 用户决策）；T36 → T37 → T38 已全部交付 |
| P3（已完成，3 卡待复验/延后） | 已交付：T42 渠道探测协议化、T43 渠道模型目录发现与令牌模型选择器、T40 Windows 安装器、T44 Windows 无控制台启动与系统托盘、T41 Playground 四协议调试、T39 全弹性布局与间距系统重构、T46 版本发布门禁与预发布通道、T47 发布 v0.2.0、T51 发布 v0.3.0 Windows/Linux、T53 发布 v0.3.1 Windows/Linux、T54 更新网络容错、T55 npm 镜像更新资产包、T56 发布 v0.4.0 Windows/Linux、T57 JetBrains AI Assistant 流式等待/日志刷新/OpenCode 预设与 v0.5.0 发布、T58 OpenCode Go 会话头/模型目录/日志刷新稳定性与 v0.5.1 发布、T60 JetBrains 空增量保活修复与 v0.5.2 发布、T62 JetBrains + ZAI reasoning 流兼容修复与 v0.5.3 发布、T63–T68 质量审查修复批次、T70–T72 成本统计修复与 v0.5.4/v0.5.5；实现完成待人工复验：T49 Windows 托盘 DPI 截图矩阵、T52 托盘 Logo 多 DPI 截图矩阵；延后：T48 macOS 安装器（需 Apple Developer Program） | 按任务卡验收；macOS 安装器需 Developer ID/notarization，后续通过 `ENABLE_MACOS_INSTALLER=true` 启用；Windows 固定路径安装、镜像保留数据、自启动、单实例、日志定位、Linux 归档、npm 五包与托盘 Logo 资源均已验证 |
| P4（已完成） | 三视角审视改进批次：T73 流程治理与文档体系、T74 CI 门禁补齐、T75 后端补齐、T76 前端设计系统、T77 前端页面体验、T78 README 与用户文档、T79 i18n | 排除有支出项（Windows 代码签名、Apple 签名公证）；各卡按验收标准通过并整卡归档至 [P4](./docs/archive/P4.md)；`go test -race ./...`、`npm test`、`npm run build`、`make build` 证据已入卡 |
| P5（已完成） | 内置浏览器体验审查修复与预设/价格/目录/首启：T96 单实例互斥按数据目录隔离、T88 全局布局与导航修复、T89 表单反馈与信息表达修复、T90 i18n 字典补全、T91 渠道预设/自定义分离与四协议、T92 预设模型价格、T98 全局控件尺寸与对齐统一、T93 下游模型目录接口、T94 首启管理员设置向导、T97 全页面 UI/UX 逐项验收、T99 统一反馈体系与三方原语替换、T100 登录空表单校验与提示文案、T101 概览事实条与侧栏底栏重排、T102 分段控件与下拉选择重做、T95 发布 v0.7.0 | 各卡按验收标准通过并附验证证据；`go test -p 1 ./...`、`go vet ./...`、`make build`、前端 lint/test/build 通过；T97 矩阵全绿后才进入发版；v0.7.0 按发版检查清单发布 |
| P6（进行中） | 2026-11-26 代码质量审查整改与交互打磨：T103 缓存 token 计价、T104 key 导入修复、T105 接口加固与信封统一、T106 工程门禁补齐、T107 全局交互动效体系、T108 请求日志详情信息表达、T109 会话 / 错误处理 / 国际化收口、T110 表单体系与 UI 细节、T111 可维护性重构、T112 转发热路径与服务生命周期、T113 令牌限流与事件告警、T114 一键备份与恢复、T115 调试台增强与观测补充 | 各卡按验收标准通过并附验证证据；`go test -p 1 ./...`、`go vet ./...`、`make build`、前端 lint / test / build 通过；UI 类卡在 18787 内置浏览器逐页复验且 `audit:layout` 全绿 |

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

### 待执行（自上而下即执行顺序）

> 来源：2026-11-26 全量代码质量审查（规范 / 可维护性 / 性能 / 安全 / 产品 / 工程化六维，结论已向用户汇报）与用户补充的两项优化（请求日志详情信息表达、全局交互动效与按钮反馈）。用户指令：审查项全部排期修复。
> 用户约束不变：测试与验证不得使用真实环境（已安装实例 8787）；有成熟三方组件不手搓；新增依赖须先告知。

#### T110: 表单体系与 UI 细节收尾（P2，代码审查）

- 目标：用成熟表单方案替换手写表单状态，并收掉审查中剩余的 UI 不一致
- 验收标准：① 引入 react-hook-form + zod，渠道 / 令牌 / 价格 / 降级链 / 设置 / 登录表单迁移，校验文案走 i18n，保存入参强类型；② 表单有未保存更改时关闭需确认，Esc 与点遮罩不丢输入；③ Switch 改用 Radix Switch，侧栏收起态导航项改用 Tooltip 与底部操作一致，Dialog / Drawer 仅在有描述时设置 aria-describedby；④ 图表与弹层颜色全部走语义 token，分页器图标统一为 lucide；⑤ 此前两个未决观察项（新建渠道默认选中项、设置页长表单分区）在本卡内定案并落地；⑥ audit:layout 全绿，18787 逐表单复验，单测覆盖脏数据保护与校验
- 边界：新增依赖 react-hook-form、zod、@hookform/resolvers、@radix-ui/react-switch，实施前告知用户；不改表单字段集合
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：in-progress
- 进度：
  - 2026-11-26 计划 建卡；来源：2026-11-26 代码质量审查（可维护性 / UI 细节）
  - 2026-11-26 决策 用户同意新增 react-hook-form、zod、@hookform/resolvers、@radix-ui/react-switch
  - 2026-11-27 执行 开工；依赖已向用户告知后安装，按 ③ Switch / Tooltip / aria → ① 表单迁移（登录 → 设置 → 令牌 → 渠道 → 价格 → 降级链）→ ② 脏数据保护 → ④⑤ 的顺序分步提交
  - 2026-11-27 执行 ③④ 完成：Switch 改 Radix Switch、侧栏收起态导航项 Tooltip、Dialog / Drawer 可选 description；分页器 lucide 图标、图表激活点语义 token 类；① 登录与令牌表单迁移 RHF + zod（校验文案 i18n、入参强类型、失败聚焦首错字段）；② 新增 useUnsavedGuard 并接入令牌弹窗（Esc / 遮罩 / 取消按钮有脏数据先确认）。验证：lint 0 警告、unit 16 + ui 73 通过（Login 按异步 resolver 调整，新增 guard 两例与令牌脏数据 / 校验两例）、audit:layout 907/1440 浅中 + 深英通过、make build 通过
  - 2026-11-27 执行 ② 渠道弹窗接入脏数据保护（字段快照与初始值比较 → useUnsavedGuard，Esc / 遮罩 / 取消先确认）；⑤ 定案并落地：新建渠道默认选中第一个供应商预设（预设自带模型与价格，自定义接入保留为显式选项，编辑沿用原预设）；设置页三个分区改为 Disclosure（上游超时 / 重试与冷却默认展开，维护任务默认折叠）。验证：lint 0 警告、unit 16 + ui 73 通过、make build 通过、audit:layout 907/1440 浅中 + 深英通过，18787 设置页截图复验
  - 2026-11-27 执行 ① 改密与首启表单迁移 RHF + zod：对象级 refine（两次密码一致落到确认字段）、密码强度三类校验、失败聚焦首错字段；对应测试按异步 resolver 调整，脏数据测试加长等待避免并发下的偶发超时。验证：lint 0 警告、unit 16 + ui 73 通过、make build 通过
  - 2026-11-27 备注 剩余：渠道 / 设置 / 价格 / 降级链表单迁移到 RHF + zod（登录 / 令牌 / 改密 / 首启已完成），完成后归档

#### T111: 可维护性重构（P2，代码审查）

- 目标：在测试全绿的前提下做行为不变的结构重构，降低后续改动成本
- 验收标准：① dispatchModel 三段重复的桥接体处理收敛为表驱动，嵌套层级明显下降；② run() 拆为引导 / 装配 / 服务生命周期三段；③ admin.go 按会话、渠道、凭据、令牌、日志、更新拆文件，路由按公开组 / 会话组注册，中间件只声明一次；④ 用量报表的聚合 SELECT 去重（totals 两条合一），审计摘要不再对每个响应做序列化，状态机与轮询器回收已删除条目；⑤ 改名遗留、变参式可选参数、time.Now() 直调等零散项清理；⑥ 重构前后 go test -p 1、-race 与覆盖率门禁结果不变，接口契约与 openapi.yaml 无 diff
- 边界：不改行为、不改接口、不改 schema；每一步可独立提交并保持全绿
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-26 计划 建卡；来源：2026-11-26 代码质量审查（可维护性）

#### T112: 转发热路径与服务生命周期优化（P2，代码审查）

- 目标：把同步写库移出首包关键路径，去掉每请求的重复配置读库与请求体重编码，补齐服务端生命周期细节
- 验收标准：① key 成功 / 失败统计的持久化移到首包写出之后或并入异步刷盘，内存状态机仍即时，压测给出首 token 延迟 P50 / P95 的前后对比；② 渠道、降级规则等低频配置走内存快照 + 变更失效，单请求配置读库由 4 次以上降到不超过 1 次，管理台变更后立即生效；③ 请求体一次解析并按上游模型缓存重写结果，1MB / 8MB 请求体下有 CPU 与分配的前后对比；④ Transport 缓存按时长分桶并设容量上限；⑤ 日志统计的分位数下推到 SQL，不再全量读入内存；⑥ http.Server 设空闲连接超时，新增 server.host 监听地址配置（默认行为不变），优雅停机超时不再按启动失败处理并强制关闭残留连接；⑦ SQLite synchronous=NORMAL 作为可选项并提供实测对比
- 边界：不改调度语义、重试预算与 SSE 红线，不做多进程与外部缓存；决策项：synchronous=NORMAL 能降低写延迟，代价是断电可能丢最后几次提交，默认值由用户确认
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-26 计划 建卡；来源：2026-11-26 代码质量审查（性能）
  - 2026-11-26 决策 用户确认 synchronous=NORMAL 作为可选项并给出实测对比

#### T113: 令牌限流与事件告警（P2，产品能力）

- 目标：补上「一个客户端打满上游没有刹车」与「挂了半天不知道」两个产品空白
- 验收标准：① 令牌支持 RPM 与并发上限（0 = 不限），超限返回 OpenAI 格式 429 rate_limit_exceeded 并写入请求日志；上游 key 支持 RPM 上限（启用现有 rpm_limit 字段），逼近上限的 key 本轮不参与调度；② 令牌表单与列表、key 面板可配置并展示限流值；③ 告警：可配置 Webhook（通用 JSON + 企业微信 / 飞书 / Telegram / Bark 预设），事件覆盖渠道 down 与恢复、key 因 401 / 403 被禁用、令牌额度低于 20% 与耗尽、更新失败；④ 告警异步发送不阻塞请求，有去重与冷却，失败只记日志，负载不含任何密钥；⑤ 设置页可发送测试告警并查看最近发送记录；⑥ 表驱动与 httptest 集成测试覆盖限流边界与告警负载
- 边界：单进程内存计数，不引入 Redis；不做多渠道告警编排与静默时段
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-26 计划 建卡；来源：2026-11-26 代码质量审查（产品视角）

#### T114: 一键备份与恢复（P2，产品能力）

- 目标：管理台可随时备份整库（含加密后的上游 key）并从备份恢复，不再只能手工拷文件
- 验收标准：① 设置页可创建备份、查看列表（时间 / 大小 / 版本）、下载、删除，备份与升级前自动备份同格式并沿用现有保留策略；② 恢复：上传备份 → 校验（可打开、schema 元数据可读）→ 落盘待生效 → 提示重启；恢复前自动备份当前库，失败可回退；③ 备份文件不含明文密钥，界面明确提示恢复需同一份 master_key；④ 审计记录备份 / 恢复操作；⑤ 集成测试：备份 → 改数据 → 恢复 → 数据回到备份时刻
- 边界：不打包配置文件、不做云端远端存储与定时备份（如需另建卡）
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-26 计划 建卡；来源：2026-11-26 代码质量审查（产品视角）

#### T115: 调试台增强与观测补充（P3，产品能力）

- 目标：让调试台能承担多轮与参数调试，并补齐选渠道最关键的延迟维度
- 验收标准：① 调试台支持多轮对话、temperature / max_tokens 等常用参数、查看实际发往上游的请求体（脱敏、有大小上限），离开页面自动中止在途请求；② 用量报表新增首 token 延迟维度（按模型 / 渠道的均值与 P50），趋势图可切换指标；③ 渠道模型列表可定时同步上游目录，上游新增 / 下线模型在渠道页给出提示，不自动改写对外模型列表；④ 中英 / 浅深 / 窄屏通过，单测覆盖请求体脱敏与延迟聚合
- 边界：调试台仍复用 relay 公共 dispatch；不采集下游请求体入库
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-26 计划 建卡；来源：2026-11-26 代码质量审查（产品视角）

> 来源：P3 待复验任务（由 T73 拆分）。

#### T74v: 复验 macOS cgo CI（P0，待外部环境）

- 目标：在可用 GitHub hosted macOS runner 上执行 `CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build ./internal/desktop/ ./cmd/evergate/`
- 验收标准：CI `macOS cgo build (no release)` job 实际分配到 runner 并全绿；若失败，记录编译日志并修复
- 边界：只复验 macOS cgo 编译；不发布 DMG，不新增业务能力；编译缺陷若需改代码则另建修复卡
- 阻塞项：当前私有仓库的 `macos-14` job 在 runner 分配前被 GitHub 取消；类型：外部（hosted macOS runner/额度）；重启条件：Actions 账单或配额恢复后设置仓库变量 `ENABLE_MACOS_CI=true`；复查日期：2026-12-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T74 拆出；Linux race、安全与构建已通过，仅 macOS cgo 环境待复验

#### T49v: 复验 Windows 托盘 DPI 截图矩阵（P3，待人工复验）

- 目标：补齐 T49 剩余的人工复验项
- 验收标准：在 Windows 100% / 125% / 150% / 200% 缩放及两台不同 DPI 显示器间移动/重启后，托盘与 TrackPopupMenu 菜单文字、行高、圆角保持原生清晰；记录截图尺寸与实际 DPI
- 边界：只复验原生托盘菜单 DPI；不检查 Logo 资源矩阵（由 T52v 负责），发现实现缺陷另建修复卡
- 阻塞项：需实际切换系统缩放与显示器拓扑；类型：待复验；重启条件：具备多 DPI 环境；复查日期 2027-01-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T49 拆出

#### T52v: 复验 Windows 托盘 Logo 多 DPI 矩阵（P3，待人工复验）

- 目标：补齐 T52 剩余的人工复验项
- 验收标准：100% / 125% / 150% / 200% 与混合 DPI 下托盘 Logo 大小清晰、不模糊；右键菜单仍为原生 TrackPopupMenu；单实例与退出/重启行为不变
- 边界：只复验托盘 Logo 多 DPI 清晰度与既有行为；不调整菜单文案或业务功能，发现实现缺陷另建修复卡
- 阻塞项：需实际切换系统缩放与显示器拓扑；类型：待复验；重启条件：具备多 DPI 环境；复查日期 2027-01-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T52 拆出

### 受阻 / 延后

#### T48: 统一发版产物与桌面安装体验（P3，用户指令）

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
- 阻塞项：macOS App/安装器签名与公证未就绪；类型：外部；重启条件：凭据可用且设置 `ENABLE_MACOS_INSTALLER=true`；复查日期：2027-01-15
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

### 最近完成 / 已取消

> 本批次已 done 的卡（T103–T109、T116）已归档到 [docs/archive/P6.md](./docs/archive/P6.md)；主文件只保留非 done 卡。

## 归档索引

- [P0 归档任务卡](./docs/archive/P0.md) · [P1](./docs/archive/P1.md) · [P2](./docs/archive/P2.md) · [P3](./docs/archive/P3.md)
- [P4：T73–T80 三视角改进批次](./docs/archive/P4.md)与 [T81–T84 文档分层复审 / v0.6.0 发布收尾](./docs/archive/P4.md)（done）
- [P5：T88–T102 体验审查修复与预设/价格/目录/首启，含 T95 发布 v0.7.0](./docs/archive/P5.md)（done）
- [P6：T103–T108 代码质量审查整改与交互打磨（批次进行中，done 卡随时归档于此）](./docs/archive/P6.md)
