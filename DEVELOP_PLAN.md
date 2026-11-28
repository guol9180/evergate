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
| P6（已完成） | 2026-11-26 代码质量审查整改与交互打磨：T103 缓存 token 计价、T104 key 导入修复、T105 接口加固与信封统一、T106 工程门禁补齐、T107 全局交互动效体系、T108 请求日志详情信息表达、T109 会话 / 错误处理 / 国际化收口、T110 表单体系与 UI 细节、T111 可维护性重构、T112 转发热路径与服务生命周期、T113 令牌限流与事件告警、T114 一键备份与恢复、T115 调试台增强与观测补充 | T103–T116 全部按验收标准通过并附验证证据；`go test -p 1 ./...`、`go vet ./...`、`make build`、前端 lint / test / build 通过；UI 类卡在 18787 内置浏览器逐页复验且 `audit:layout` 全绿；整卡归档至 [P6](./docs/archive/P6.md) |
| P7（进行中） | 2026-11-27 第二次全量代码质量审查整改批次：T117–T124、T130、T131、T137 已 done；T130–T133 组件化基座与迁移（shadcn/ui Radix 变体、TanStack Table、cmdk、React Aria、react-day-picker、vaul、streamdown）；T121 / T125–T127 体验收口；T128–T129 工程与性能；T134–T136 组件增强、工程基建与 i18n | 按任务卡验收；done 卡归档至 `docs/archive/P7.md` |

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

> 来源：2026-11-27 第二次全量代码质量审查（用户报告「获取上游模型报错时弹出两个重叠窗口」触发），结论见 [审查报告](./docs/research/2026-11-27-全量代码质量审查.md)。T117 已 done 并归档；用户指令：审查发现全部排期修复 → T118–T129（批次一至五，不依赖新增依赖）；用户 2026-11-27 确认候选清单（除第 21 项外全部采纳）→ T130–T136；与组件迁移重叠的 UI 缺陷卡（T121 / T125–T127）排到迁移卡之后，避免在同一份手搓实现上修两遍。

#### 批次一 · 后端与接口加固

#### 批次二 · 组件化基座（shadcn/ui · Radix 变体）

#### 批次三 · 组件迁移

#### T132: 数据表格（P1，G1 #1；吸收 T121④ / T125② / E11）

- 目标：11 处手写表格迁到 TanStack Table（长列表接 TanStack Virtual）：统一列定义、排序、列显隐、行展开、分页
- 验收标准：① 新增 `DataTable` 原语，渠道 / 令牌 / 日志 / 审计 / 备份 / 测试全部 / 价格 / 映射 / 概览健康 / 用量 Top 榜全部迁移，无手写 `<table>`；② 表头点击排序、分页切换回到表格顶部、日志行有键盘可达的「详情」入口；③ 窄屏列折叠规则与现状等价（`audit:layout` 全绿）；④ UI 单测覆盖排序 / 分页 / 空态 / 失败态；⑤ 18787 逐页复验
- 边界：不改接口与数据结构
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### T133: 选择器 / 数字 / 日期（P1，G1 #2 #4 #5 #19；吸收 T121③ / E4 / E8 / A8 前端部分）

- 目标：cmdk 可搜索多选替换 `ModelPicker`；React Aria `NumberField` 替换全部数字输入；react-day-picker + date-fns 替换日期范围与时间格式化
- 验收标准：① 模型选择器具备键盘导航、全选 / 反选，预选随目录到达正确重算，「替换」不丢已有模型；② 全站数字输入支持步进、上下限、整数步长，提交不再做字符串换算；③ 日志 / 用量日期范围与令牌过期时间用日期选择器，开始晚于结束被拦下并有字段提示；④ 时间列今日只显时间、其余显日期（date-fns 本地化）；⑤ UI 单测 + `audit:layout` 全绿 + 18787 复验
- 边界：调试台 SSE 解析保持现状（第 21 项未采纳）
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### 批次四 · 体验收口（在新组件上完成；已随迁移落地的子项以复验记录为准）

#### T121: 交互状态缺陷（P2，A4 / A5 / A6 / A7）

- 目标：修复差异徽标 Tooltip 不生效、告警测试丢草稿、模型选择器预选失效、分页后不回表头
- 验收标准：① `Badge` / `StatusBadge` 透传 ref 与 rest props，差异徽标悬停出现差异明细（UI 单测 + 18787 实测）；② 告警「测试发送」使用当前草稿配置且不清空未保存编辑（后端 `/alerts/test` 接受待测配置，记录标注 draft）；③ `ModelPicker` 在 options 晚于挂载到达时按 `initialModels` 重算预选，「替换」不丢已有模型；④ 列表分页切换后滚动到表格顶部（18787 实测 `scrollTop` 归零）
- 边界：不改告警负载格式与分页组件 API
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### T125: 反馈与导航细节（P2，E1 / E2 / E7 / E8 / E11）

- 目标：Toast 不遮挡页头操作、日志行有可见详情入口、控件说明统一 Tooltip、时间列更易读、空状态统一
- 验收标准：① 错误 Toast 出现时页头操作按钮仍可点击（18787 截图验证）；② 日志行提供键盘可达的「详情」IconButton，整行点击保留；③ 自动刷新 / 导出 / 从上游拉取禁用原因 / 映射新增行的说明全部走 Tooltip，禁用态可触发；④ 时间列今日只显时间、其余显日期，`title` 给完整本地时间；⑤ 降级链、key 导入等手写空状态改用 `Empty`；`audit:layout` 全绿
- 边界：只用现有原语；表格与 Tooltip 库替换留到批次六
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### T126: 未保存保护与筛选一致（P2，E3 / E4）

- 目标：设置页与降级链不再静默丢失未保存修改；用量页模型筛选与日志页一致
- 验收标准：① 设置页 / 降级链有脏数据时出现吸底「未保存的更改」条（保存 / 放弃），切换路由与关闭页签前确认；② 保存成功后脏态清除；③ 用量页模型筛选改为候选下拉（复用日志页候选口径）；④ UI 单测 + 18787 实测
- 边界：不引入 URL 状态库（留到批次六评估）
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### T127: 错误表达（P3，E5 / A12 / E9）

- 目标：错误信息说人话、Playground 中止判定可靠、模型目录获取不因首把 key 失效而失败
- 验收标准：① 上游错误文案为人话摘要（无法连接 / 超时 / 鉴权失败 / 限流）+ 可展开原始详情，不再多层前缀嵌套；② Playground 依据 `signal.aborted` / `AbortError` 判定中止，错误按稳定 code 本地化；③ 预览 / 同步目录按池顺序逐把尝试启用 key，全部失败才报错并汇总原因；④ Go 单测 + 18787 实测（首把 key 失效的渠道）
- 边界：不改对外错误信封结构
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### 批次五 · 工程与性能（不依赖新增依赖）

#### T128: 工程卫生（P3，B3 / B9 / B10 / 文档漂移）

- 目标：开发代理不再指向真实环境；清理死代码与文档漂移
- 验收标准：① `vite.config.ts` 代理默认指向 18787，可用环境变量覆盖，任何情况下不默认指向 8787；② 删除 `TableWrap` / `focusFirstInvalid` / `.eg-table-responsive` / 无效 `data-*` / 空回调等死代码；③ `web/tsconfig.node.tsbuildinfo` 入 `.gitignore`，Makefile 体积输出兼容 Windows，删除重复的 `initTheme()` 调用；④ 修正 DEVELOP_PLAN 归档索引 P6 行；`npm run lint` / `npm test` / `make build` 通过
- 边界：不做格式化与测试重命名（见批次六）
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### T129: 性能（P3，C1 / C2 / C3 / C4）

- 目标：首屏包体下降、调试台流式渲染不整页重绘、Tooltip 单 Provider、去掉额度预检多余查询
- 验收标准：① 全部页面路由级懒加载并拆分 vendor，首屏 JS（gzip）较基线 266KB 下降 ≥30%，Vite 无 chunk 超限警告；② Playground 增量按帧合并、对话区拆为独立组件，流式下无长任务（Performance 实测）；③ Tooltip Provider 提升到根部；④ 额度预检使用鉴权快照，结算仍用条件更新兜底；⑤ 产物对比与 18787 实测写入卡内
- 边界：不改业务逻辑
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### 批次六 · 组件增强与工程基建

> 来源：2026-11-27 用户确认候选组件清单（[审查报告 G 节](./docs/research/2026-11-27-全量代码质量审查.md#g-候选组件清单待用户确认)）：除第 21 项外全部采纳；设计见 [组件化替换方案](./docs/design/P7-组件化替换方案.md)。

#### T134: 抽屉 / Markdown / 侧栏 / 动效（P2，G1 #3 #6 #14 #7）

- 目标：vaul 响应式抽屉（桌面侧边、窄屏底部）；streamdown 渲染调试台输出；shadcn Sidebar 重做侧栏；motion / number-flow 统一进出场与数字动效
- 验收标准：① 日志详情在 ≥1024px 为侧边 Sheet、窄屏为可下拉关闭的底部抽屉；② 调试台输出支持 Markdown、代码高亮、复制消息，流式渲染不闪烁；③ 侧栏支持折叠，收起 / 展开两态与现状等价；④ `prefers-reduced-motion` 下退化为不透明度；⑤ `audit:layout` 全绿 + 18787 复验
- 边界：不改调试台协议逻辑
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### T135: 可选增强与工程基建（P2，G1 #15 #16 #18 · G2 #22 #23 #24；吸收 B4–B6）

- 目标：dnd-kit 拖拽排序、CodeMirror JSON 编辑、nuqs URL 状态；prettier、jsx-a11y、user-event 接入，测试按被测对象重命名
- 验收标准：① 降级链与渠道优先级可拖拽排序；② 模型映射 JSON 与上游请求体用代码编辑器；③ 日志筛选写入 URL，可分享、可前进后退；④ Prettier 全量格式化并接入 CI，`jsx-a11y` 零告警，交互测试改用 `user-event`，测试文件按被测对象命名；⑤ 门禁全绿
- 边界：不引入 ESLint 全家桶
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

#### T136: 国际化迁移（P2，G2 #20）

- 目标：用手写 1822 行字典换成成熟 i18n 方案
- 验收标准：① 先出选型对比（i18next vs Lingui：包体、类型安全、提取流程、Vite 集成）写入设计文档；② 全站文案迁移，中英键集一致由工具保证，语言切换即时生效；③ `<html lang>` 随语言更新；④ 包体变化与首屏影响写入卡内；⑤ UI 单测与 `audit:layout` 中英全绿
- 边界：只做中英两种语言
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned

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

> 本批次已 done 的卡（T103–T116）已归档到 [docs/archive/P6.md](./docs/archive/P6.md)；主文件只保留非 done 卡。
> P7：T117 已 done，归档到 [docs/archive/P7.md](./docs/archive/P7.md)；主文件只保留非 done 卡。

## 归档索引

- [P0 归档任务卡](./docs/archive/P0.md) · [P1](./docs/archive/P1.md) · [P2](./docs/archive/P2.md) · [P3](./docs/archive/P3.md)
- [P4：T73–T80 三视角改进批次](./docs/archive/P4.md)与 [T81–T84 文档分层复审 / v0.6.0 发布收尾](./docs/archive/P4.md)（done）
- [P5：T88–T102 体验审查修复与预设/价格/目录/首启，含 T95 发布 v0.7.0](./docs/archive/P5.md)（done）
- [P6：T103–T108 代码质量审查整改与交互打磨（批次进行中，done 卡随时归档于此）](./docs/archive/P6.md)
- [P7：T117 操作失败反馈重复弹出修复（第二次审查整改批次，进行中）](./docs/archive/P7.md)
