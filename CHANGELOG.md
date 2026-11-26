# 更新日志

发版描述自动取自本文件：工作流按 `## <tag>` 提取对应段落作为 GitHub Release 正文（T7b 约定）。

## v0.7.0

- 本版本为 `v0.7.0` 正式版（新增功能 → MINOR），发布到 npm `latest` 与 GitHub Latest
- 渠道预设：供应商预设与自定义彻底分离；预设只开放模型列表、API 地址与 API 密钥，名称/协议/调度字段仅在「切换为自定义」后出现；自定义模式协议收敛为 Chat Completions、Responses、Anthropic Messages、Gemini API 四项（Chat Completions 固定开启）
- 模型价格：内置预设价格目录（OpenAI / DeepSeek / Anthropic / Gemini，含来源 URL 与日期），计价回退链「渠道+模型覆盖 → 全局默认 → 预设目录」；渠道编辑弹窗展示生效价格与来源；用量页新增「按预设查看」与「一键写入为渠道覆盖」；「从上游拉取」带 OpenRouter 风格价格时可一并写入覆盖；日志详情显示价格来源
- 下游模型目录：`GET /v1/models` 按令牌「允许模型」过滤，携带 `anthropic-version` 时返回 Anthropic 结构；新增 `GET /v1beta/models`（Gemini 结构）；令牌创建弹窗与 README 提供三协议 curl 示例
- 首启管理员：默认不再随机生成密码打印到日志，首次启动引导到 `/setup` 设置用户名与密码（仅本机可访问），设置后回到登录页登录；容器 / 自动化仍可用 `EVERGATE_INITIAL_ADMIN_PASSWORD` 自动创建（首登强制改密）；新增 `GET /admin/api/auth/setup-status`、`POST /admin/api/auth/setup`
- 管理台体验：修复文档层双滚动、侧栏两态与底部按钮同轴、页头按钮尺寸不一致、窄屏表格列被裁、1280/1440 横向溢出、弹窗嵌套滚动与入场动画、表单静默失败、探测错误解析、冷却剩余与「未探测」状态；英文界面补齐全部文案（上游原始错误文本按数据展示）；趋势图颜色跟随主题
- 反馈体系与组件原语：提示按场景三分——字段错误内联在字段下方并聚焦第一个出错项，表单与弹窗级错误固定在正文顶部，操作结果统一为右上角 Toast；弹窗、下拉、复选、气泡提示改用 Radix UI 原语（关闭后焦点归还、Esc 先关下拉再关弹窗），Toast 改用 sonner；分段控件改用 Radix RadioGroup 重做为单条轨道（凸起选中段），下拉补齐展开态、箭头旋转与长列表滚动按钮；测试全部、分页器、模型映射编辑器收敛到共用控件
- 登录与侧栏：登录空表单在浏览器端即拦截并提示「请输入用户名 / 密码」；运行概览的渠道 / 模型 / key / 令牌数量改为分段事实条，与下方指标卡片对齐；侧栏账号块图标在左且不再重复显示版本号，语言 / 主题 / 退出登录改为竖向操作行并显示当前语言与主题
- 单实例互斥按数据目录派生，允许开发实例与已安装实例并行运行

## v0.6.0

- 本版本为 `v0.6.0` 正式版，直接发布到 npm `latest` 与 GitHub Latest
- 安全：首次启动的管理员密码改为随机生成（打印到控制台与 `logs/evergate.log`，可用 `EVERGATE_INITIAL_ADMIN_PASSWORD` 预设），不再使用固定 `admin123`；新增管理台写操作审计日志（`GET /admin/api/audit-logs`）
- 可靠性：检测到应用版本变化时在数据库迁移前自动备份 `evergate.db` 到 `data/backups/`（保留 5 份）
- 管理台：颜色语义 token 与暗色模式（侧栏切换、跟随系统）、中英文切换、Dialog/Drawer 焦点管理与 Esc、骨架屏、页码分页器、危险操作确认对话框替代浏览器原生弹窗、页面标题随路由变化
- 管理台：新建渠道可内联粘贴上游 key、类型逐项说明、模型映射表格编辑、渠道级超时字段、「测试全部」并按延迟排序；日志筛选改为渠道/令牌下拉 + 日期范围并支持导出 CSV；令牌创建成功直接给出可复制的客户端环境变量片段；Dashboard 成功率/延迟改为后端全量聚合、四段引导与额度不足 20% 预警；系统设置新增配置导出/导入与审计日志
- 新接口：`GET /admin/api/logs/stats`、`GET /admin/api/logs/export`、`GET /admin/api/config/export`、`POST /admin/api/config/import`、`POST /admin/api/channels/test-all`
- CI：新增 `ci.yml` 在 main 推送与 PR 运行 `go test -race`、vet、前端 lint/test/build、调度与 relay 覆盖率门禁（relay ≥81%）、gitleaks、actionlint 与 macOS cgo 编译；npm 发布支持可选 OIDC Trusted Publishing（仓库变量 `NPM_TRUSTED_PUBLISHING`）
- 文档：README 新增七类客户端接入指南与如实描述 macOS 现状；新增 FAQ、HTTPS 与反向代理、回滚与备份、OpenAPI、发版检查清单、风险登记、贡献指南与安全策略；按 `init-project-docs` 复审三件套，专题文档迁入 `docs/agents/`，已完成卡与已结题调研原文归档至 `docs/archive/`

## v0.5.5

- 修复 Chat Completions 兼容上游流式请求未主动返回 usage 的问题：网关自动设置 `stream_options.include_usage=true`，为额度结算与成本计算取得真实输入/输出 token
- 修复成本已落库却不回显的问题：请求日志列表与详情、Playground 命中摘要即时展示单请求成本；同一下游 token 跨多个供应商时，成本按实际命中的渠道与上游模型逐笔计算后汇总
- 用量成本页明确区分已计价、未配置价格、无 usage 与历史未记录；未知成本显示为 `—`，不再用 `$0` 冒充已知成本，并提示先配置全局默认价或渠道+模型覆盖价
- UTC 日聚合新增 `no_usage_requests` 与 `unknown_cost_requests`；价格变更仍不追溯历史请求，现有 `unpriced` 记录不会被后来价格重算
- 新增 Chat、Responses、Anthropic 与 Gemini 非流式/流式成本 HTTP 边界回归测试；本版本继续发布 Windows 安装器与 Linux tar.gz，macOS 安装器仍受 Developer ID 签名与 notarization 开关约束

## v0.5.4

- 修复同优先级渠道加权随机排序错误，避免后续候选长期得不到首位机会；补强 SSE 首个语义事件判定，注释、空帧和 Messages `ping` 不再阻断首 token 前故障切换
- 修复 `server.base_path` 管理台链路：静态资源、React Router、管理 API、图标与根路径重定向统一适配子路径
- 所有下游协议增加 16MiB 请求体上限；上游错误中的当前 key、Bearer、`sk-/eg-/AIza` 凭证在响应、重试轨迹、请求日志和 Playground 元数据前脱敏
- OpenCode 自动生成的会话在同一请求重试中保持稳定；配置写回改为原子 fsync + 平台安全替换，管理登录增加限流，管理 API 默认仅允许 loopback 访问
- 渠道 URL、权重、模型映射和超时配置补齐创建/更新校验；修复首次改密、登录提交和路由守卫的前端测试门禁
- 发布流水线新增 Go 测试、vet、前端测试、lint 和调度语句覆盖率门禁；本版本继续发布 Windows 安装器与 Linux tar.gz，macOS 安装器仍受 Developer ID 签名与 notarization 开关约束

## v0.5.3

- 修复 JetBrains AI Assistant 调用 ZAI coding + GLM Chat 流时，在可见正文出现前长期只接收 `reasoning_content` 并在约 10 秒后取消的问题
- Ktor/JetBrains 请求未显式声明 `thinking`、`reasoning` 或 `reasoning_effort` 时，ZAI 上游请求自动关闭 thinking；显式思考请求、非 Ktor 客户端、非 ZAI 渠道和非 GLM 模型保持原行为
- 不把 `reasoning_content` 伪造成正文，不发送伪内容，也不改变 Responses、Messages 或 Gemini 原生事件协议
- 本版本为只包含该流式兼容修复的 PATCH 发布；继续发布 Windows 安装器与 Linux tar.gz，macOS 安装器仍受签名与 notarization 开关约束

## v0.5.2

- 修复 JetBrains AI Assistant 生成 Git Commit Message 时约 10 秒后取消请求、正文不回显的问题：Chat Completions 的空闲保活从 SSE 注释帧改为不携带正文、角色、工具调用或 usage 的协议内空增量 `data:` 帧
- 空增量帧不会伪装 `reasoning_content` 为正文，也不会改变真实上游事件、最终 usage 或额度结算；Responses、Messages 与 Gemini 的事件协议保持不变
- 新增只识别 `data:` 事件的 Ktor 超时边界回归测试，覆盖“首角色帧 → 长静默 → 正文/usage”场景；本版本为只包含该流式修复的 PATCH 发布
- 继续发布 Windows 安装器与 Linux tar.gz；macOS 安装器仍受 Developer ID 签名与 notarization 开关约束

## v0.5.1

- 修复 OpenCode Go 渠道请求缺少 `x-opencode-session` 被上游拒绝的问题：优先转发下游同名会话头，其次复用 `X-Session-Id`，均缺失时生成稳定的单请求会话 ID；Playground 在当前页面内复用同一会话
- 修复 OpenCode Go 模型目录无法获取：推理基址 `/inference/go/openai/v1` 会自动映射模型列表与健康探测到 `/inference/go/v1/models`
- 修复请求日志手动刷新按钮点击时发生布局跳动：按钮保持固定宽度和固定文案，刷新期间仅图标旋转并暂时禁用
- 本版本为只包含上述三个问题修复的 PATCH 发布；继续发布 Windows 安装器与 Linux tar.gz，macOS 安装器仍受签名开关约束

## v0.5.0

- 修复 JetBrains AI Assistant 生成 Git Commit Message 时，上游在首个角色帧后短暂没有文本增量导致客户端提前断开的问题：流式响应空闲期间改为每 5 秒发送标准 SSE 注释心跳，真实内容帧、usage 与首 token 语义保持不变
- 请求日志页新增手动刷新按钮；刷新沿用当前筛选条件与页码，并显示刷新状态
- 渠道预设新增 OpenCode Go，自动填充官方 OpenAI 兼容端点 `https://opencode.ai/inference/go/openai/v1` 与代表性模型
- 本版本按 0.x 新增功能规则提升 MINOR；继续发布 Windows 安装器与 Linux tar.gz，macOS 安装器仍因缺少 Developer ID 签名/公证凭据而不随本版本发布

## v0.4.0

- 应用内更新新增 `@heguoliang/evergate-updates` 完整资产包，优先从 npmmirror → 官方 npm registry 下载 Windows 安装器或 Linux 归档，GitHub Release 仅作为备用
- npm 更新包校验 `dist.integrity`（sha512）与包内 checksums（SHA256）；Windows 仍运行完整 setup，Linux 仍安全提取 tar.gz，不使用裸二进制替换安装形态
- checksums 与安装资产 GET 对瞬态连接错误、429 与 5xx 执行三次有界重试，404、大小超限和完整性失败不重试
- 更新弹窗恢复自动/npm/GitHub 三种通道选择，自动模式优先 npm 镜像；程序不写入任何本机代理地址，也不要求用户必须配置代理
- 本版本继续发布 Windows 安装器与 Linux tar.gz；macOS 安装器仍因缺少 Developer ID 签名/公证凭据而不随本版本发布

## v0.3.1

- 修复 Windows GUI 托盘 Logo 不显示：图标组资源实际 ID 为 2，改为按系统小图标尺寸使用 `LoadImageW` 加载，并移除静默回退系统空图标
- 托盘图标加载失败现在会明确阻止托盘初始化并写入错误日志；新增资源 ID、GUI 构建产物和图标可提取性回归门禁
- 保持 Per-Monitor V2 DPI、原生右键菜单、单实例与日志定位行为不变；console 产物不链接 GUI 图标资源
- 本补丁版继续发布 Windows 安装器与 Linux tar.gz；macOS 安装器仍因缺少 Developer ID 签名/公证凭据而不随本版本发布

## v0.3.0

- 破坏性分发调整：公开 GitHub Release 不再提供裸二进制，统一为 `evergate_<version>_<os>_<arch>[_setup].<ext>`；v0.3.0 发布 Windows 安装器与 Linux tar.gz，macOS 安装器因暂缺 Developer ID 签名凭据不随本版本发布，v0.2.0 及更早安装版本允许手动安装一次完成迁移
- Windows 安装器改为固定程序、配置与数据路径，移除目录选择；安装流程统一为许可、新装/升级状态、可选登录自启、安装进度与可选立即启动，升级保留已有数据
- macOS 安装器代码已实现 `EverGate.app` 菜单栏应用与单个 `.dmg` 流程：不显示 Dock 图标，支持打开管理台、打开日志目录、重启、退出和单实例；正式发布继续强制 Developer ID 签名与 notarization，未启用签名凭据时 Release 不包含未签名 DMG
- Linux 公开产物统一为 `evergate_<version>_linux_amd64.tar.gz`，包内含 `evergate`、`LICENSE` 与 `README.md`，保持无托盘运行并可用于 Docker
- Windows GUI 嵌入 Per-Monitor V2 DPI awareness manifest，改善高 DPI 与混合缩放显示器下的托盘原生菜单清晰度；托盘“查看日志”改为打开目录并选中当前 `evergate.log`
- 一键更新改为安装包级链路：Windows/macOS 校验后退出并启动完整安装器，Linux 校验 tar.gz 后安全提取并原子替换，macOS 额外校验 notarization 与 Developer ID
- 更新资产按版本、OS、架构和 checksums 精确匹配；npm 四包继续作为独立安装渠道，但不再作为 GitHub 更新回退资产，npm 安装用户通过 npm 更新
- 更新弹窗新增安装阶段和下载/校验取消；发布门禁校验四类公开资产命名、版本与 checksums，完整资产齐备后才移动 GitHub Latest

## v0.2.0

- Windows 分发升级：新增 Inno Setup 安装器，支持统一或分离的配置/数据目录、静默卸载保留数据、可选开机自启；配置路径统一由显式参数、环境变量、注册表、平台默认和便携目录发现
- Windows 桌面运行升级：提供无控制台 GUI 与 console 双产物，GUI 支持系统托盘、单实例、打开管理台/日志、重启与退出；日志同时写入数据目录并按大小滚动
- 渠道健康探测按协议适配 OpenAI 兼容、Anthropic 与 Gemini，不再假设所有上游都支持同一个 `/models` 路径
- 新增渠道模型目录发现与令牌模型选择器：支持 Anthropic/Gemini 分页、模型映射场景下的对外名称选择，并保持无推理副作用
- Playground 扩展为 Chat Completions / Responses / Anthropic Messages / Gemini 四协议调试，统一流式提取、停止与命中链路展示
- 管理台完成全弹性布局与间距系统重构：桌面、平板和窄屏统一卡片内滚动，修复页面级横向溢出、图表空白与操作按钮可访问性
- 发布流程加固：tag、CHANGELOG、main 历史与 npm 四包执行强一致门禁；RC/Beta 使用 npm `next` 与 GitHub prerelease，正式版才移动 `latest`；开发构建不参与更新比较
- 真实上游流式测试完成：ZAI 与 DeepSeek 在无代理环境下各完成 50 个正式样本，网关首 token 额外延迟中位数均低于 5ms，均无重试或非 200 响应

## v0.1.29

- 新增 Gemini 原生入口 `POST /v1beta/models/{model}:generateContent`、`:streamGenerateContent` 与 `:countTokens`；模型从 URL 路径解析，下游支持 `x-goog-api-key` 鉴权，`?key=` query 鉴权会被拒绝
- 渠道新增「原生支持 Gemini API」能力声明：声明渠道仅改写路径模型与鉴权，原样透传 `systemInstruction`、`safetySettings`、`tools`、`generationConfig` 与 data-only SSE
- 未声明能力的渠道自动桥接 Chat Completions，支持文本、`functionCall` / `functionResponse`、工具声明与基础生成参数；`safetySettings`、`thinkingConfig`、`inlineData` / `fileData`、`topK`、`responseSchema` 显式返回 400
- Gemini usage 按输入 `promptTokenCount + toolUsePromptTokenCount`、输出 `candidatesTokenCount + thoughtsTokenCount` 归一计价，流式与非流式一致
- 渠道预设新增 Google Gemini 官方模板；渠道列表以紧凑徽标同时展示 Responses / Messages / Gemini 能力

## v0.1.28

- 新增 Anthropic Messages 原生入口 `POST /v1/messages` 与 `POST /v1/messages/count_tokens`；下游鉴权同时接受 `Authorization: Bearer` 与 `x-api-key`，可直接承接 Claude Code 的 `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY`
- 渠道新增「原生支持 Anthropic Messages」能力声明：声明渠道仅改写鉴权、Base URL 与模型映射并透传 JSON/SSE；未声明渠道自动桥接 Chat Completions，支持文本、工具调用与 usage，`thinking` / `cache_control` 等无法等价表达的字段返回 400
- Messages SSE 按 Anthropic 事件序透传：无 `[DONE]`、`ping` 原样转发、以 `message_stop` + EOF 结束；首事件前可故障切换，首事件后不重试
- 渠道预设新增 Anthropic 官方、DeepSeek Anthropic 兼容、Kimi Anthropic 兼容三种模板；Base URL 按厂商填写后网关统一追加 `/v1/messages`
- 新增按预算自动模型降级：令牌剩余额度低于阈值或原模型渠道耗尽时，按可配置降级链切换低成本模型；请求日志保留原始/实际模型与降级原因，成本按实际命中模型价格快照计算
- 模型降级可在令牌级关闭，候选模型必须通过令牌模型白名单；首 chunk 写出后不再因降级重试

## v0.1.27

- 一键更新下载链改为 npmmirror → 官方 npm registry → GitHub Releases；每个源独立超时（元数据 8 秒、tarball 90 秒），镜像故障会快速切换而不是占满任务预算
- 版本检查改为 npm `dist-tags` 优先（npmmirror → 官方 registry），两源均不可用才回退 GitHub Releases；国内无代理环境也能发现新版本
- GitHub 可达时补充更新说明链接与资产列表，不可达时仍以 npm 版本为准，「立即更新」不会因缺少 GitHub 元数据而不可用
- 管理台更新弹窗新增「检查来源」，下载通道徽标显示实际命中的 registry；缺少更新说明链接时不再渲染空链接
- `EVERGATE_NPM_REGISTRY` 现在同时收窄版本检查与下载链为指定 npm registry

## v0.1.26

- 新建渠道支持供应商预设：OpenAI / DeepSeek / Kimi / 智谱 BigModel 常规 / BigModel Coding / Z.ai Coding 六种模板，选中后自动填充类型、Base URL、原生 Responses 能力与默认渠道名
- Coding 套餐（智谱 / Z.ai）显示专用地址避坑提示，避免误填常规 `/api/paas/v4` 导致 503；预设不包含任何 key 或账号信息
- 「自定义」保持原有手动填写行为；编辑已有渠道不显示预设选择器，避免覆盖用户配置
- 厂商基址知识收敛为产品内唯一维护点，README 不再维护基址清单；渠道表单提示补充 Responses 透传开关的适用场景

## v0.1.25

- 管理台新增一键更新：Dashboard 提示新版本后可直接下载、校验、替换并优雅重启，界面展示下载/校验/替换/重启四阶段进度与失败原因
- 下载优先 npm 平台子包并校验 `dist.integrity`（sha512），失败自动回退 GitHub Releases 并用 `checksums.txt` 校验 SHA256；`EVERGATE_NPM_REGISTRY` / `EVERGATE_UPDATE_API_URL` 可切换镜像与 API
- Windows 使用运行中 exe 改名交换（`<exe>.bak-<version>-<timestamp>`），Unix 同目录原子替换；失败自动恢复旧文件，启动时仅保留最近一份备份并清理 `.new-*` 残留
- 重启沿用 30 秒优雅排空，新进程以相同参数/工作目录启动；新版本健康后版本号生效，旧版本备份保留供手动回滚
- 开发版本（dev/dirty）、`go run` 临时文件或不可写安装目录会拒绝自更新并提示原因

## v0.1.24

- 新增 npm 安装渠道：`npm i -g @heguoliang/evergate` 自动按 `os`/`cpu` 选择 `@heguoliang/evergate-win32-x64` / `@heguoliang/evergate-linux-amd64` / `@heguoliang/evergate-darwin-arm64` 平台子包
- 主包仅含 KB 级启动垫片，不打包二进制；实际运行仍是原生单二进制，Node 仅用于解析平台包并转发参数
- 发版流水线在 GitHub Release 之外同步发布 4 个 npm 包：平台子包先行、主包最后，已发布版本幂等跳过；支持 npmmirror 自动同步
- 使用 npm scoped 包名避开未占用包名的垃圾包名风控；`NPM_TOKEN` 仅存 GitHub Actions secret

## v0.1.22

- 用量页日趋势从手写 div 柱状图改为 Recharts 组合图：柱=请求数、线=成功率，双 Y 轴 + 网格 + hover tooltip（请求/成功/成功率/成本）
- 图表与卡片宽高自适应：窄屏按视口高度 `clamp(224px,32vw,360px)`，桌面填满与价格卡同高的卡片，修复卡片底部大块空白；390/768/1152/1280/1600 宽度截图与 DOM 测量通过
- 补齐所选日期区间内缺失的聚合日，避免时间轴被压缩；零请求日成功率断线而非显示 0%
- Usage 路由改为懒加载，Recharts 不进入首屏 bundle（首屏 JS gzip 117.21KB，略低于改动前）；新增依赖 Recharts 3.10.1 + react-is 19

## v0.1.21

- 新增 Prometheus `/metrics`（仅 loopback 直连可抓取，非本机 403）：暴露请求/错误/并发数、首 token 延迟直方图、上游尝试与错误分类，以及渠道健康与 key 状态
- relay 成功/失败路径统一记录首 token 与上游尝试指标；渠道、模型、错误类型标签带基数上限，指标不含 key、令牌或请求正文
- 新增可选 OTLP/HTTP JSON 链路导出：设置 `OTEL_EXPORTER_OTLP_ENDPOINT`（或 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`）后生成 `http.request` / `relay.request` / `relay.attempt` 父子 span；默认完全关闭，不创建后台 goroutine 或外部请求
- 导出失败只记录 warning 不影响请求；进程优雅退出时排空待导出 span；未引入 Prometheus/OTel SDK，保持单二进制体积约束

## v0.1.20

- 管理台全页面排版统一：Dashboard / 渠道 / 令牌 / 请求日志 / Playground / 用量成本 / 系统设置统一使用 PageHeader 与一致的标题、描述字号
- 渠道、令牌、请求日志、渠道 key 详情与用量 Top 表全部改为卡片内横向滚动并设置合理 min-width，1280 桌面不再出现无谓滚动条，390 窄屏无页面级横向溢出
- 行内操作统一为图标按钮（渠道测试/编辑/删除、令牌编辑/删除、key 删除）并补充 aria-label/title；共享 Switch 替换手写开关
- 日志详情 Drawer 窄屏占满宽度；Dialog 表单在窄屏单列；设置页来源信息移动端自动换行，折叠侧栏退出按钮只保留图标
- 表单弹窗改为固定底部操作栏（内容区独立滚动），长表单无需滚到底部才能保存；表头 nowrap、操作列右对齐，Dashboard 指标按「数值+标签」成组换行，表格与表单间距统一

## v0.1.19

- 修复侧栏品牌 logo 与菜单图标不在同一对齐线：折叠态 logo 与菜单图标统一到同一垂直中线（36px），展开态统一左边缘（24px）
- 同步收紧侧栏水平内边距，展开态品牌文字间距保持自然；Playground 页面不再受影响

## v0.1.18

- Playground 布局精修：响应正文与命中链路合并为单卡，新增协议/HTTP 状态栏、usage 指标 chips 与 Playground 日志提示，减少浮动卡片割裂感
- 流式输出增加光标与自动滚动，长内容限制在独立滚动区；表单计数器右对齐、协议切换改为紧凑分段控件，移动端徽标与控件不再拉伸换行
- 新增共享 `Select` 与 `Switch` 表单组件用于 Playground，统一焦点态、禁用态与无障碍 `role=switch`；重试时间线末项去除多余留白

## v0.1.17

- 管理台新增 Playground：选择模型与 Chat Completions / Responses 协议，编辑 system/user 消息并切换流式，直接验证渠道与协议兼容性，无需创建或复制下游令牌
- 请求复用网关调度内核（含 retry/cooldown/亲和规则），流式响应逐段渲染，可中途停止；完成后展示命中渠道、上游模型、首 token、总耗时、usage 与重试轨迹
- Playground 请求写入 request_logs 并标记 `source=playground`，不暴露任何上游或下游 key；消息长度与请求体有后端上限，前端同步限制
- 修复 Playground 包装 writer 与 admin 响应共用 header 导致 Content-Length 污染、非流式响应为空的问题

## v0.1.16

- 新增请求级 `X-EverGate-Config` 覆盖：支持超时（connect/first_token/overall）、重试预算（渠道数/每渠道 key 数/总时长）、候选渠道 id 列表与 `session_affinity` 开关，覆盖只对当前请求生效
- 严格白名单解析：未知字段、类型错误、敏感字段（api_key/model/allowed_models 等）与越界值返回 400 并给出字段路径；小于安全下限的时长自动收敛，超大值拒绝
- 覆盖在 TokenAuth 与令牌治理之后应用，不能绕过模型白名单、额度、IP 限制或上游 key 加密；候选渠道必须存在、启用且支持请求模型
- 请求日志新增 `config_override`：仅保存脱敏后的生效摘要（≤512B），不保存原始 header

## v0.1.15

- 新增 `X-Session-Id` 会话亲和：同一下游令牌 + 模型 + 会话在 10 分钟 TTL 内优先复用上次成功的渠道与 key，每次成功复用自动续期并改写记录
- 亲和只在最高优先级组内提升候选顺序，且粘性 key 仍需通过 enabled/cooldown/half-open 探测名额校验；停用、冷却、删除或失败时回退现有重试与健康调度
- 亲和表为有界 LRU（4096 条），只保存 token/model/session 的 SHA-256 指纹，不保存 session 原文；仅进程内存，不持久化、不跨实例
- 修复路由器共享 `math/rand` 在并发请求下缺少互斥的数据竞争

## v0.1.14

- 新增全局默认价与渠道+模型覆盖价（每百万 token，micro-USD 固定精度）；请求完成时按当时价格快照计算输入/输出成本，价格变更不追溯历史请求
- 请求日志新增 `cost_micros` / `cost_status`，无 usage 与无匹配价格分别标记 `no_usage` / `unpriced`，不伪造零成本
- 后台按 UTC 日从 request_logs 事务内先删后插重建 usage_daily，幂等聚合请求、成功、输入/输出 token、成本与已计价/未计价数量；启动、运行中和停机均会刷新
- 管理台新增「用量成本」页：日期范围（默认近 7 天）、渠道/模型/令牌筛选，总览、轻量日趋势、Top 渠道/模型/令牌，以及全局价格和渠道+模型覆盖维护

## v0.1.13

- 下游令牌强制停用、过期、模型白名单与 IP 白名单（精确 IP / CIDR），两条 OpenAI 协议入口均在上游调用前拒绝并返回统一错误码
- Token 额度按成功响应实际输入 + 输出 token 数原子扣减；quota=0 不限，客户端断开、上游失败或响应无 usage 不扣减，并把 usage 缺失原因写入请求日志
- 管理台令牌列表显示额度、已用、剩余与超限状态，创建/编辑支持额度、有效期、模型白名单与 IP 白名单；IP 白名单默认基于 TCP 直连地址，不信任伪造转发头

## v0.1.12

- 新增渠道健康自动探测：按系统设置周期、并发与超时调用 `GET /models`，不触发推理请求；成功恢复 half-open/cooldown key，失败记录状态码、错误分类、摘要、延迟与下次探测时间
- 404/405 探测结果标记 inconclusive 并释放 half-open 探测名额，不把“探测路径不支持”误判为渠道故障；渠道停用后不再进入探测队列，设置变更会按新间隔重建等待
- Dashboard 与渠道列表新增 active/degraded/down/inconclusive 徽标、可用 key 数、最近探测时间与错误摘要，异常渠道优先排序；手动「测试」复用公共探测路径并同步落库
- 请求转发路径持久化 key 成功/失败统计，渠道详情的健康视图可结合实时 key 状态与请求失败记录

## v0.1.11

- 新增 SQLite 运行时设置单表与管理台系统设置页：超时、重试预算、cooldown、日志保留与健康探测参数保存后热生效，无需重启
- `evergate.yml` 仅在首次启动时初始化安全运行时设置；端口、Base Path、SQLite 路径、master key 与 session secret 仍保留在 YAML，不会进入管理台 API/UI
- 请求路径通过原子配置快照读取最新值；key cooldown 状态机支持阈值与退避表热更新；日志保留任务会按最新配置即时/周期清理过期请求

## v0.1.10

- 新增渠道级“原生支持 Responses API”能力声明：`/v1/responses` 优先路由到该类渠道并原样透传 JSON/SSE，仅改写鉴权、base_url 与模型映射
- Responses 原生 SSE 支持无 `data: [DONE]` 的 EOF 结束、`response.failed` 首事件前失败切换、reasoning/内置事件与工具调用保真；chat-only 渠道继续使用 Responses→Chat 桥接
- `previous_response_id`、`background=true`、`store=true` 等无法安全桥接的状态字段在候选渠道均无 Responses 能力时显式返回 400 `responses_native_required`
- 请求日志新增 total/cached/reasoning token，并统一 Chat Completions 与 Responses 两套 usage 字段口径

## v0.1.9

- 修复流式请求首 token 计时口径：现在从发起上游请求开始计算，到首条 SSE data 为止；此前 ZAI 这类“响应头与首条 data 几乎同时到达”的渠道会被记录为 `first_token_ms=0`
- Chat Completions 与 Responses 流式路径统一使用同一首 token 口径，成功尝试的 retry trace 耗时也包含等待首 token 的时间

## v0.1.8

- 修复 Codex 多个 namespace 下同名工具（如多个 MCP 均暴露 `js`）触发 `Responses flattened tool name "js" maps to multiple declarations` 的问题：上游使用确定性唯一函数名，返回与历史回传恢复原始 namespace/name
- 请求日志重试轨迹新增每次尝试的上游 HTTP 状态码；空 error body 的上游错误会在 503 汇总与管理台中显示 `http <status> (empty error body)`，不再只有 all_channels_exhausted
- 文档补充智谱 / Z.ai Coding 正确 Base URL：国际版 `https://api.z.ai/api/coding/paas/v4`，中国版 `https://open.bigmodel.cn/api/coding/paas/v4`

## v0.1.7

- 修复 Codex `apply_patch` grammar custom tool 被误拒绝的问题：grammar 降级为上游 function 参数描述，返回时恢复原始 patch 文本
- 补齐 Responses custom 流式 `input.delta`、namespace 输出恢复、Responses Lite `additional_tools` hoist、`configuration_update.reasoning.effort` 覆盖与 `text.verbosity` 转换

## v0.1.6

- 修复新版 Codex 声明 Responses `custom` tool 时返回 `unsupported Responses tool type "custom"` 的问题；支持 custom tool 声明、调用、结果回传与 `response.custom_tool_call_input.done` 流式事件

## v0.1.5

- 新增 Codex 兼容入口 `POST /v1/responses`：支持非流式、SSE、工具调用与多轮 function_call 输出回传
- 桌面体验优化：启动后自动打开管理台，管理台 UI 层级与窄屏适配 refined
- 新增 GitHub Release 自动更新检查：显示当前/最新版本、更新说明与 Windows 下载入口；GitHub API 限流时自动降级 Releases Atom

## v0.1.4

- 统一 EverGate 品牌图标：管理台侧边栏、浏览器角标与 Windows exe 桌面图标使用同一 logo

## v0.1.3

- 修复首次登录修改密码成功后仍被旧前端缓存弹回强制改密页的问题；后端密码更新本身已确认成功

## v0.1.2

- 发版描述机制升级：Release 正文自动取自 CHANGELOG.md 对应版本段落（本条即验证）

## v0.1.1

- CI 自动发布就绪：推 v* tag 自动构建三平台二进制并附 SHA256 校验文件
- 修复：清理误提交文件；T7b 安全审计与历史清理记录补全

## v0.1.0 — P0 首个可用版本

本地 AI 网关：一个网关，所有模型，永不中断。单二进制 + SQLite，双击即用。

### 功能
- OpenAI 兼容 `/v1/chat/completions`（SSE 流式 + tool calls 透传）与 `/v1/models`
- 多渠道多 key 池调度：优先级/权重/平滑加权轮询、cooldown 状态机（30s→2m→10m→30m 退避、half_open 单飞探测）
- 错误分类驱动的无感故障切换（吸收 bifrost/gpt-load/LiteLLM 生产经验：SSE 流内错误事件、model_not_found 换渠道、quota/限流/凭据标识识别）
- SSE 零缓冲透传，首 chunk 前可重试、首 chunk 后绝不重复
- Web 管理台（登录/渠道/凭据/令牌/日志，重试轨迹可视化）已嵌入同一二进制
- 上游 key AES-GCM 加密存储；请求日志异步落库

### 快速开始
```
./evergate          # 启动 :8787，管理台 http://localhost:8787/
```
默认管理员 admin/admin123（首登强制改密）。

### 验收
P0 十项验收清单全部通过；调度内核测试覆盖率 95.6%；二进制 27MB；冷启动 <200ms。
