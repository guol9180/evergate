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
| P3（已完成，4 卡待复验/延后） | 已交付：T42 渠道探测协议化、T43 渠道模型目录发现与令牌模型选择器、T40 Windows 安装器、T44 Windows 无控制台启动与系统托盘、T41 Playground 四协议调试、T39 全弹性布局与间距系统重构、T46 版本发布门禁与预发布通道、T47 发布 v0.2.0、T51 发布 v0.3.0 Windows/Linux、T53 发布 v0.3.1 Windows/Linux、T54 更新网络容错、T55 npm 镜像更新资产包、T56 发布 v0.4.0 Windows/Linux、T57 JetBrains AI Assistant 流式等待/日志刷新/OpenCode 预设与 v0.5.0 发布、T58 OpenCode Go 会话头/模型目录/日志刷新稳定性与 v0.5.1 发布、T60 JetBrains 空增量保活修复与 v0.5.2 发布、T62 JetBrains + ZAI reasoning 流兼容修复与 v0.5.3 发布、T63–T68 质量审查修复批次、T70–T72 成本统计修复与 v0.5.4/v0.5.5；实现完成待人工复验：T49 Windows 托盘 DPI 截图矩阵、T50 安装包更新真实升级冒烟、T52 托盘 Logo 多 DPI 截图矩阵；延后：T48 macOS 安装器（需 Apple Developer Program） | 按任务卡验收；macOS 安装器需 Developer ID/notarization，后续通过 `ENABLE_MACOS_INSTALLER=true` 启用；Windows 固定路径安装、镜像保留数据、自启动、单实例、日志定位、Linux 归档、npm 五包与托盘 Logo 资源均已验证 |
| P4（已完成） | 三视角审视改进批次：T73 流程治理与文档体系、T74 CI 门禁补齐、T75 后端补齐、T76 前端设计系统、T77 前端页面体验、T78 README 与用户文档、T79 i18n | 排除有支出项（Windows 代码签名、Apple 签名公证）；各卡按验收标准通过并整卡归档至 [P4](./docs/archive/P4.md)；`go test -race ./...`、`npm test`、`npm run build`、`make build` 证据已入卡 |
| P5（进行中） | 内置浏览器体验审查修复与预设/价格/目录/首启：T96 单实例互斥按数据目录隔离、T88 全局布局与导航修复、T89 表单反馈与信息表达修复、T90 i18n 字典补全、T91 渠道预设/自定义分离与四协议、T92 预设模型价格、T98 全局控件尺寸与对齐统一、T93 下游模型目录接口、T94 首启管理员设置向导、T97 全页面 UI/UX 逐项验收、T95 发布 v0.7.0 | 各卡按验收标准通过并附验证证据；`go test -p 1 ./...`、`go vet ./...`、`make build`、前端 lint/test/build 通过；T97 矩阵全绿后才进入发版；v0.7.0 按发版检查清单发布 |

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

### 执行中

### 待执行（自上而下即执行顺序）

> 来源：P5 体验审查与预设/价格/目录（2026-11-25 用户指令，内置浏览器真实环境审查 + 三条产品优化）。


#### T94: 首启管理员设置向导（P2，用户决策）

- 目标：首次启动不再随机生成密码打印到日志，而是进入与登录页同款的「初始化管理员账号」界面，强制设置用户名与密码；设置成功后进入登录页，登录后才进入主页。替代 T75 第 6 条的随机密码流程
- 已确认决策：
  1. 启动时 admins 为空且未设置 `EVERGATE_INITIAL_ADMIN_PASSWORD` → 不创建管理员，日志提示"请在管理台完成初始管理员设置"；设置了该环境变量则保留现有自动化路径（自动创建、首登强制改密），供容器 / 无界面部署使用
  2. 新增 `GET /admin/api/auth/setup-status`（返回 `setup_required`）与 `POST /admin/api/auth/setup`（`username` + `password`）；setup 接口仅在 admins 为空时可调用，且**始终仅限 loopback**（不受 `server.admin_loopback_only` 影响），已存在管理员时返回 403；并发调用只允许一个成功
  3. 校验在服务端强制：用户名 3–32 位 `[A-Za-z0-9_.-]`（预填 `admin`），密码沿用现有改密策略（≥8 位、至少三类字符）；创建后 `must_change_password=false`，写审计 `setup_admin`
  4. 设置成功后跳转 `/login` 并预填用户名，由用户登录进入主页（不自动登录）
  5. `/login` 检测到 `setup_required` 自动跳 `/setup`；`/setup` 在已完成设置时跳回 `/login`；桌面版 `open_browser` 打开根路径同样落到 `/setup`
  6. 已有安装（admins 非空）升级后行为不变
- 验收标准：
  1. WHEN 全新数据目录启动且未设环境变量 THEN 控制台与日志不再出现密码；`/` 重定向到 `/setup`，页面与登录页同款（Logo、品牌、卡片），含用户名、密码、确认密码
  2. WHEN 提交弱密码 / 非法用户名 / 两次密码不一致 THEN 表单内联提示，不发请求或服务端 400
  3. WHEN 设置成功 THEN 跳转 `/login` 且用户名已预填；用该账号登录后进入 `/`；审计日志出现 `setup_admin`
  4. WHEN 已存在管理员 THEN `POST /auth/setup` 返回 403；WHEN 非回环地址请求 setup THEN 403
  5. WHEN 启动前设置 `EVERGATE_INITIAL_ADMIN_PASSWORD` THEN 保持旧行为（自动创建 + 首登强制改密），`web/scripts` 布局审计脚本不受影响
  6. README「快速开始」第 2 步、「安全模型」、运维配置「初始管理员密码」小节、FAQ 同步为新流程；Go 单测覆盖空表 / 非空表 / 非回环 / 校验 / 并发；前端测试覆盖设置页渲染、校验、跳转
- 边界：不做多管理员、不做邮箱/找回密码；不改 session 机制；不自动登录
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-25 计划 建卡；来源：用户产品优化 #4（替换随机初始密码流程）

#### T97: 全页面 UI/UX 逐项验收（P1，用户要求）

- 目标：发版前对管理台每个页面按矩阵逐项验收 UI 与 UX，每格附证据（尺寸测量或截图），不过项回到对应卡修复后复验；矩阵全绿才能进入 T95 发版（2026-11-25 用户要求：最后验收要保证每个页面的细节都改好，UI 与 UX 都要）
- 验收矩阵（每页在 907 / 1440 两个宽度、浅色 / 深色、中文 / 英文各过一遍）：
  1. 全局壳层：侧栏两态与底部按钮、文档滚动锁、路由滚动归零、弹窗 / 抽屉动画与 Esc / 焦点归还、焦点环可见、无横向溢出、标题层级一致
  2. 概览：KPI 网格、渠道健康表、空状态引导、英文无中文残留
  3. 渠道：预设 / 自定义表单字段、校验提示、拉取成功 / 失败、测试全部、单测结果、key 表冷却、删除确认
  4. 令牌：目录选择器、创建后明文一次展示与复制、三种协议的调用示例
  5. 日志：筛选即时生效、状态色、详情抽屉、错误解析、分页、导出
  6. 调试台：四协议切换、流式 / 非流式、停止、错误只呈现一次、命中链路
  7. 用量：筛选、横幅、KPI、趋势图刻度、价格表单、Top 榜成本语义
  8. 设置：各分区栅格、保存反馈、审计即时刷新、导入导出
  9. 登录 / 改密 / 首启设置：同款视觉、校验、跳转
- 验收标准：矩阵每格记录"通过"或"不通过 → 修复 → 复验通过"；`npm run audit:layout`（本机 Chrome + 开发实例）在两种宽度无溢出；证据摘要写入卡面，截图留在 gitignore 的 `web/layout-audit/`
- 边界：只验收与修复，不加新功能；超出本批次的缺陷另建卡
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-25 计划 建卡；来源：用户要求最终验收逐页逐项

#### T95: 发布 v0.7.0（P2）

- 目标：T88–T94、T97 验收后按发版检查清单发布 v0.7.0（新增功能 → MINOR）
- 验收标准：CHANGELOG `## v0.7.0` 与 tag 精确对应且在 main 历史；CI 全绿；真实旧→新升级一次并回写证据
- 边界：不包含 macOS 安装器
- 阻塞项：无；类型：—；重启条件：—；复查日期：—
- 状态：planned
- 进度：
  - 2026-11-25 计划 建卡

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

#### T50v: 复验安装包级真实升级（P3，待人工复验）

- 目标：用真实历史版本 → 新版本完成一次管理台内更新
- 验收标准：安装 v0.5.x 后在管理台触发更新到下一正式版，经历下载/校验/安装/重启四阶段，配置、数据库与自启动选择保留；失败路径（校验失败、取消）保留旧版本可运行；回写证据到发版检查清单
- 边界：只执行一次真实升级与失败回退复验；不发布版本，不扩大为全平台更新矩阵，发现实现缺陷另建修复卡
- 阻塞项：需等待下一个正式版发布；类型：待复验；重启条件：v0.6.0 tag 可用；复查日期 2026-12-15
- 状态：planned
- 进度：
  - 2026-11-24 计划 由 T50 拆出，并入 docs/发版检查清单.md

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

#### T84: 改为直接正式版发布并发布 v0.6.0（P0，用户指令）

- 目标：移除标准流程中的预发布环节，后续默认直接发布正式版，并立即将当前候选功能集发布为 `v0.6.0`
- 版本判断：`v0.5.5 → v0.6.0`。P4 新增功能按 0.x 语义提升 MINOR；用户明确取消 RC / 24 小时前置门槛
- 验收标准：
  1. AGENTS 与发版检查清单不再要求 MINOR 先发 `-rc.1`，默认直接发布正式 tag
  2. `CHANGELOG.md` 含唯一非空 `## v0.6.0` 段，正式 tag 指向 `main` 提交
  3. 本地 Go、前端、覆盖率、脚本、构建、actionlint 与文档门禁通过
  4. `main` CI Quality / Security 全绿；macOS cgo 按仓库变量如实跳过或通过
  5. annotated tag `v0.6.0` 指向 `main`；Release workflow 的 quality / release / windows-installer / verify-release 全绿，macOS 安装器按签名开关跳过
  6. GitHub Release 为正式 Latest，资产为 Windows setup、Linux tar.gz 与 checksums；npm 五包 `latest` 均为 `0.6.0`
  7. 下载资产重算 SHA256，并回写 Action URL、资产大小 / 哈希、npm dist-tags 与 npmmirror 证据
- 边界：不发布未签名 DMG、裸二进制或第二个候选 tag；不改写历史 tag；真实旧→新升级仍由 T50v 人工复验
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-25 计划 用户明确取消预发布环节并要求直接发布正式版；建立 T84，开始更新发布规范与正式版门禁
  - 2026-11-25 验证 AGENTS 与发版清单已改为默认直接正式版；本地 Go/vet、前端 11+19、lint/build、脚本 10 项、scheduler 93.5%、relay 81%、actionlint、`make build`、`make build-all`、Markdown/OpenAPI 门禁通过
  - 2026-11-25 验证 main CI [45524001137](https://github.com/guol9180/evergate/actions/runs/45524001137) Quality/Security 成功，macOS cgo 按变量跳过；annotated tag `v0.6.0` 指向 `9e6cc0f` 并推送
  - 2026-11-25 交付 Release [45525049079](https://github.com/guol9180/evergate/actions/runs/45525049079)：首次 verify-release 因 updates 包 registry 元数据等待超时失败，但五包已实际发布；同一 tag 重放失败 job 后 attempt 2 全成功，macOS-installer 按开关跳过
  - 2026-11-25 验证 GitHub Release 为正式 Latest，资产 Windows setup 9,841,266B（SHA256 `da20e2c7427609a281ee44a7ed77bab7c12fb0c987019b04939f9d0d22ad9ccf`）、Linux tar.gz 10,883,933B（SHA256 `1f0087a31b10f1a59c7351ffa7869ab991f94c6619446d05b0b1f35a5111e240`）、checksums 205B；独立下载重算一致
  - 2026-11-25 验证 npm 官方与 npmmirror 五包均 `latest=0.6.0` 且版本存在；npmmirror 主包通过公开同步任务 `6b064029a8442cc48b9fca64` 刷新成功；历史 `next=0.6.0-rc.1` 仅为旧记录，后续标准流程不再产生候选通道

#### T83: 发布 v0.6.0-rc.1（P0，用户指令）

- 目标：将 P4 三视角改进批次与新版文档分层按 MINOR 预发布通道发布为 `v0.6.0-rc.1`，验证 GitHub prerelease、Windows/Linux 资产、npm 五包 `next` 标签与发布门禁
- 版本判断：`v0.5.5 → v0.6.0-rc.1`。本批次新增管理台能力、后端统计/配置/审计接口与安全/可靠性改进，按 0.x 语义提升 MINOR；先发布候选版，正式 `v0.6.0` 待候选验证后再发
- 验收标准：
  1. `CHANGELOG.md` 含唯一非空 `## v0.6.0-rc.1` 段，发版提交位于 `main` 历史
  2. 本地 Go、前端、覆盖率、脚本、构建、actionlint 与文档门禁通过
  3. `main` 推送后的 CI Quality / Security 全绿；macOS cgo job 按仓库变量如实跳过或通过
  4. annotated tag `v0.6.0-rc.1` 指向 `main` 提交；Release workflow 的 quality / release / windows-installer / verify-release 全绿，macOS 安装器按签名开关跳过
  5. GitHub Release 为 prerelease，仅含 Windows setup、Linux tar.gz 与 checksums，不含裸二进制或未签名 DMG
  6. npm 主包、三个平台子包与 updates 包均为 `0.6.0-rc.1`，`next` 指向该版本，`latest` 保持 `0.5.5`
  7. 回写 Action URL、资产大小与 SHA256、npm dist-tags；真实旧→新升级继续由 T50v 在正式版发布后复验
- 边界：不发布正式 `v0.6.0`、不移动 `latest`、不发布未签名 DMG 或裸二进制；不夹带业务代码；不改写历史 tag
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-24 计划 用户确认推送并发布；按 MINOR 预发布规则确定 `v0.6.0-rc.1`，建立发版卡并开始本地门禁
  - 2026-11-24 验证 本地 `go test -p 1 ./...`、vet、前端 11+19 测试、lint/build、发布脚本 10 项、scheduler 93.5%、relay 81%、actionlint、`make build`、`make build-all` 与文档门禁通过；main CI [45366058083](https://github.com/guol9180/evergate/actions/runs/45366058083) Quality/Security 成功，macOS cgo 按变量跳过
  - 2026-11-24 交付 annotated tag `v0.6.0-rc.1` 指向 `98b35c4` 并推送；Release [45367176889](https://github.com/guol9180/evergate/actions/runs/45367176889) 的 quality/release/windows-installer/verify-release 全成功，macOS-installer 按开关跳过
  - 2026-11-24 验证 Release 为 prerelease，资产为 Windows setup 9,840,014B（SHA256 `7f8eea3b2e9e9e87db4090ce435bac0509022c0c456270ee5ccea2e5ef22e044`）、Linux tar.gz 10,884,033B（SHA256 `4eed8b59f10ae697a11e4f894e021f482c8f292af64be5ab7c7171a5464d869f`）、checksums 215B；独立下载重算一致，无裸二进制或 DMG，GitHub Latest 仍为 v0.5.5
  - 2026-11-24 验证 npm 官方与 npmmirror 五个包均为 `next=0.6.0-rc.1`、`latest=0.5.5`；真实旧→新升级按边界继续由 T50v 在正式版发布后复验

#### T82: 按新版 init-project-docs 补齐调研与设计分层（P1，用户指令）

- 目标：按新版 `init-project-docs` 复审文档目录与内容归属，移除 `docs/` 根目录散文件，将已结题调研 / 审计 / 历史需求原文归档，并确保结论只由 README、AGENTS、现行 `docs/agents/` 文档与任务卡承载
- 验收标准：
  1. `docs/` 根目录只保留子目录；文档索引迁入 `docs/agents/README.md`，仓库内所有入口链接同步更新
  2. 已结题的参考评审、管理台设计调研、协议生态调研、Responses 审计、三项 P2 调研与最初任务书迁入 `docs/archive/`；只搬不改，索引明确标注 `concluded`、日期与权威结论入口
  3. `docs/agents/` 只保留现行领域规范、用户 / 运维文档、风险评估与 API 契约；不创建空的 `research/`、`design/` 或 `adr/`，同一决策不同时写 design 与 ADR
  4. README、AGENTS、DEVELOP_PLAN 的引用单向且准确：当前事实不依赖旧调研，历史原文只作为追溯材料；无旧路径残留
  5. 七条自检通过：五条规范完整、内容归属唯一、无重复正文、进度只追加、预算内、分区与状态一致、调研 / 设计状态与结案去向明确；全部 Markdown 链接与锚点可解析
- 边界：只调整文档索引、目录与引用；不改写结题原文和历史任务卡，不重新论证已有技术结论，不改业务代码、版本号或发布产物
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-24 计划 新版 skill 复审确认 `docs/README.md` 为根目录散文件、7 份结题调研 / 审计仍混在 agents、最初任务书被误作现时权威入口；建立本卡并开始整理
  - 2026-11-24 验证 文档索引迁入 `docs/agents/README.md`；`docs/` 根目录无散文件，仅保留非空 `agents/`、`archive/`；7 份结题调研 / 审计与最初任务书逐字迁入归档，并在索引记录 `concluded`、日期和权威结论入口
  - 2026-11-24 验证 新版七项自检通过：README 134 行、AGENTS 57 行、DEVELOP_PLAN 179 行；36 个 Markdown 文件链接与锚点可解析；现行文档无重复长正文；`docs/agents/openapi.yaml` 解析通过；`git diff --check` 通过
  - 2026-11-24 修正 行数按实际内容行为 README 133、AGENTS 56、DEVELOP_PLAN 179；前一行 134 / 57 为脚本包含尾随空元素的计数，三份文档均低于预算
  - 2026-11-24 交付 调研 / 设计分层与单向引用已补齐；未创建空的 research/design/adr，未改业务代码、版本号或发布产物

#### T81: 按 init-project-docs 复审并整理项目文档（P1，用户指令）

- 目标：按 `init-project-docs` 的既有项目维护流程复审 README、AGENTS、DEVELOP_PLAN 及 `docs/` 索引，修正内容归属、目录命名、生命周期分区、归档和交叉引用
- 验收标准：
  1. AGENTS.md 原样保留 skill 规定的必建五条标准条目，专题规范只留摘要与链接，全文 ≤150 行
  2. 专题文档归入 `docs/agents/`，已结束任务卡归入 `docs/archive/`；迁移只改位置和链接，不改写历史进度
  3. DEVELOP_PLAN.md 按执行中 / 待执行 / 受阻或延后 / 最近完成分区；P4 done 卡整卡迁入 `docs/archive/P4.md` 并保留归档索引，主文件 ≤300 行
  4. README.md ≤300 行，继续回答“这是什么 / 能干什么 / 怎么跑”，详细接入与排障通过文档索引定位且无正文复制
  5. 七条自检全部通过：五条规范完整、内容归属唯一、无重复副本、进度只追加、预算内、分区与状态一致、双视角可检索；全部 Markdown 相对链接可解析
- 边界：只调整文档、索引与文档链接；不改业务代码、测试、版本号或发布产物，不改写任何历史进度行，不顺手润色既有正文
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-24 计划 用户指定使用 init-project-docs 重新审核；只读审计确认 AGENTS 标准条目漂移、专题/历史目录命名不符、计划分区缺失且 P4 done 卡未归档，建立本卡并开始整理
  - 2026-11-24 验证 五条标准与 skill 原文逐字一致；README 133 行、AGENTS 56 行（单节最多 9 行）、DEVELOP_PLAN 161 行；36 个 Markdown 文件相对链接与标题锚点全部可解析；31 个活动文档无重复长正文
  - 2026-11-24 验证 P0–P4 归档与迁移前原文逐字一致，P4 的 T73–T80 整卡迁入 `docs/archive/P4.md`；docs 仅保留 `agents/`、`archive/` 两个子目录；`git diff --check` 通过
  - 2026-11-24 交付 文档职责、目录、索引、生命周期分区和归档链已按 init-project-docs 七项自检完成；无业务代码、版本号或发布产物变更

## 归档索引

- [P0 归档任务卡](./docs/archive/P0.md) · [P1](./docs/archive/P1.md) · [P2](./docs/archive/P2.md) · [P3](./docs/archive/P3.md)
- [P4：T73–T80 三视角改进批次](./docs/archive/P4.md)（done）
- [P5：T88–T97 体验审查修复与预设/价格/目录/首启](./docs/archive/P5.md)（进行中，done 卡逐张迁入）
