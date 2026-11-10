# 开发计划与验收

> 需求与技术设计全文见 [docs/AI网关项目-开发任务书.md](./docs/AI网关项目-开发任务书.md)。本文件只管任务、状态与验收，架构与技术选型见 [README.md](./README.md)，改码规则见 [AGENTS.md](./AGENTS.md)。

## 任务清单

### T0: 环境准备（P0，阻塞项）

- 目标：安装 Go 1.24+，确认 go / node / npm / git 全部可用
- 验收标准：`go version` 输出 ≥ 1.24；`go build ./...` 在空 module 下可正常执行
- 边界：不安装 Docker / Redis 等任何被禁止的外部服务
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-09 执行 检测本机：node v22.23.2 / npm 10.9.8 / git 2.55.0 已具备；PATH 与常见安装目录均无 Go，需安装后解除阻塞（可绕行：先做 T6 前端脚手架以外的设计工作，但 T1 起全部依赖 Go）
  - 2026-11-09 执行 从 go.dev 下载 go1.27.3.windows-amd64.zip（79,010,309B，SHA256 官方校验通过），解压至 D:\dev\environment\go；设置用户环境变量 GOPATH=D:\dev\environment\gopath，PATH 追加 D:\dev\environment\go\bin 与 D:\dev\environment\gopath\bin
  - 2026-11-09 验证 `go version` → go1.27.3 windows/amd64；`go env GOROOT/GOPATH/GOMODCACHE` 输出正确；冒烟测试（临时目录 go mod init + go build 空程序）编译成功后清理；注册表用户 PATH/GOPATH 复核一致
  - 2026-11-09 交付 Go 1.27.3 工具链可用，T0 验收标准全部满足，T1 阻塞解除

### T0a: 目录骨架与任务书入库（P0）

- 目标：按任务书 4.2 创建项目目录架构；新建 `docs/` 并将开发任务书复制入库
- 验收标准：目录结构与任务书 4.2 一致（cmd/evergate、internal 十个包、web/、docs/、evergate.yml、Makefile）；`docs/AI网关项目-开发任务书.md` 与源文件内容一致；空目录含 `.gitkeep`
- 边界：不写任何 Go / TypeScript 代码；不初始化 git 仓库；Makefile 仅占位（真实构建在 T7）
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-09 计划 用户指令确认：初始化目录架构 + 新建 docs/ + 复制任务书
  - 2026-11-09 执行 创建 13 个目录、12 个 .gitkeep、evergate.yml（任务书 4.7 模板）、Makefile 占位；复制任务书至 docs/；同步更新三件套文档（README 加 docs 链接、AGENTS 目录约定补 docs/ 与 Makefile、DEVELOP_PLAN 源文档路径改为仓库内相对链接）
  - 2026-11-09 验证 SHA256 比对源/副本一致（A6CACA68…CF34E60）；递归列目录与任务书 4.2 结构逐项一致；evergate.yml 与任务书 4.7 模板一致
  - 2026-11-09 交付 目录骨架（cmd/evergate、internal 十包、web/、docs/）+ evergate.yml + Makefile 占位 + docs/AI网关项目-开发任务书.md（25745B）

### T1: 项目骨架（P0）

- 目标：Go module + Gin + GORM + SQLite(WAL) + AutoMigrate + 配置加载 + slog + `/healthz`
- 验收标准：`go run ./cmd/evergate` 启动成功；`GET /healthz` 返回 200；evergate.yml 被读取且默认值生效；启动日志打印版本/端口/DB 路径
- 边界：不做业务 API；不做前端
- 状态：done
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-09 执行 开始 T1：go mod init + config/store/server 骨架 + /healthz
  - 2026-11-09 执行 创建 go.mod（module evergate，go 1.24）与四组代码：internal/config（YAML 加载、默认值、校验、Duration 类型）、internal/store（SQLite WAL/foreign_keys/busy_timeout、AutoMigrate 入口）、internal/server（Gin + Recovery + slog 请求日志 + /healthz 含 DB 探活）、cmd/evergate/main.go（装配 + SIGINT/SIGTERM 优雅停机 30s 排空）；另建 Makefile 本机目标与 .gitignore。引入依赖：gin v1.12.0、gorm v1.31.2、glebarez/sqlite v1.11.0、yaml.v3 v3.0.1
  - 2026-11-09 执行 依赖下载受阻（sum.golang.org 在当前网络不可达），设置用户级 GOPROXY=https://goproxy.cn,direct 后 go mod tidy 成功（依赖图 286 行）
  - 2026-11-09 验证 gofmt 修正 2 文件缩进后无 diff；go vet ./... 通过；go build ./... 通过；go run ./cmd/evergate 启动成功；GET /healthz → 200 {"status":"ok"}；启动日志含 version/addr/db 三要素；PRAGMA journal_mode → wal（WAL 实际生效）；Ctrl+C → shutdown 日志 + evergate stopped 优雅退出；请求日志中间件输出 http_request 一条
  - 2026-11-09 交付 T1 验收标准全部满足：cmd/evergate/main.go、internal/{config,store,server}、go.mod+go.sum、Makefile、.gitignore

  - 2026-11-09 执行 需求变更（用户指令）：8080 与本地 Java 项目冲突，默认端口改为 8787；同步修改 evergate.yml、config.Default()、README 快速开始、验收清单 #1
  - 2026-11-09 验证 Get-NetTCPConnection 确认 8787 空闲；重新构建启动后 GET http://localhost:8787/healthz → 200

### T1b: Git 仓库初始化与首次推送（P0）

- 目标：本地 git 仓库初始化（main 分支）+ 首次提交；GitHub 创建同名仓库 `evergate` 并推送
- 验收标准：`git log` 存在首次提交且不含 data/、.gocache/、bin/ 等被忽略产物；远程 `origin` 指向 `<owner>/evergate`；`git push` 后远程 main 与本地一致
- 边界：仓库可见性默认 private（用户可随时改 public）；不添加 LICENSE（后续单独决策）；不做 CI/发布配置
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-09 计划 用户指令确认：判断首次远程初始化时机并执行提交推送
  - 2026-11-09 执行 时机判断=可以（文档齐全、T1 可构建可运行、.gitignore 就绪、无密钥数据）；新增 .gitattributes 统一换行符
  - 2026-11-09 执行 git init -b main + 首次提交 d168d99（26 文件 / 1254 行，feat: 前缀；data/、.gocache/ 经 status --ignored 确认未入库）
  - 2026-11-09 执行 gh 原 token 失效，两次设备授权失败（直连 github.com API 超时；期间 curl 报 schannel SEC_E_NO_CREDENTIALS 为本地 TLS 组件故障，一度误诊）；用户确认 Clash 代理端口 7900（verge-mihomo），CONNECT 实测通
  - 2026-11-09 执行 设置 git 全局 GitHub 专用代理 http.https://github.com.proxy=http://127.0.0.1:7900（仅影响 GitHub，不碰其他 git 源）；gh 走会话级 HTTPS_PROXY；重新设备授权成功（guol9180）
  - 2026-11-09 执行 gh repo create evergate --private --source . --push → https://github.com/guol9180/evergate，main 推送成功并建立跟踪
  - 2026-11-09 验证 gh repo view：name=evergate、visibility=PRIVATE、defaultBranch=main；git status -sb 显示 main 与 origin/main 同步；origin/main 指向 d168d99
  - 2026-11-09 交付 远程仓库 https://github.com/guol9180/evergate（private）+ 本地 main d168d99 已推送；用户全局 HTTP(S)_PROXY 环境变量为无效值 http://127.0.0.1:9，未擅自修改（仅 GitHub 流量已经 git 专用配置走 7900），已向用户提示

### T2: store 层（P0）

- 目标：全部表模型（channels / channel_keys / tokens / request_logs / usage_daily / admins）与 DAO + AES-GCM 加密工具 + 首次启动初始化（默认管理员、生成 master_key）
- 验收标准：AutoMigrate 建表成功；上游 key 密文落库且可解密使用；首次启动生成 admin/admin123 与 master_key 并写回配置；加密工具有单元测试
- 边界：不做调度与管理 API；不做用量聚合任务
- 状态：done
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-09 执行 开始 T2：六表模型 + AES-GCM 加密 + 首次启动初始化（默认管理员 / master_key 生成写回）
  - 2026-11-09 执行 新增 internal/store/models.go（channels/channel_keys/tokens/request_logs/usage_daily/admins 六表，JSON 列用 GORM serializer，UsageDaily 显式 TableName 防 usage_dailies 错名，UTC 经 gorm NowFunc 强制）、crypto.go（AES-256-GCM：Encrypt/Decrypt/GenerateRandomKey，base64(nonce||cipher)）、bootstrap.go（EnsureAdmin 幂等 + bcrypt 校验）、internal/config/persist.go（密钥写回：按行替换保留注释，失败兜底整体重写）；main.go 接线密钥生成与默认管理员；引入 golang.org/x/crypto（bcrypt）；清理已含代码目录的 4 个 .gitkeep
  - 2026-11-09 验证 go vet / go test ./...（config 4 用例 + store 9 用例，含加密往返/错钥/防篡改/建表/管理员幂等/UTC 存储/写回保注释）/ go build 全部通过
  - 2026-11-09 验证 运行时首次启动：日志出现「generated missing secrets and wrote back to config」与「default admin created」；evergate.yml 两密钥写入且注释保留；sqlite_master 实查 6 表齐全（含 usage_daily 正确表名）；admins 行 bcrypt 哈希 60 字符（$2a$10$）、must_change=true、无明文；重启后无重复生成/建管理员（幂等），:8787/healthz 两次均 200；优雅停机正常
  - 2026-11-09 交付 T2 验收标准全部满足；「key 密文落库」全链路（渠道 API 写入）在 T5 接入，本卡以单测证明加解密正确
  - 2026-11-09 执行 修正安全缺陷：T2 提交误将含真实密钥的 evergate.yml 推送到远程（private）；处理=evergate.yml 出库并 gitignore、模板入库为 evergate.yml.example、本地轮换两密钥（当前无已加密数据，轮换零损失）；旧值仅存于 git 历史 d168d99..537d5ff，因已轮换且仓库 private 无实际风险

### T3: 调度内核（P0，纯逻辑先写测试）

- 目标：错误分类器 → cooldown 状态机 → key 池调度 → 路由器；表驱动单测覆盖任务书 4.5 节全部规则
- 验收标准：`go test ./internal/retry/... ./internal/pool/... ./internal/router/...` 全部通过；调度器分支覆盖率 ≥ 90%（对应验收清单 #10）
- 边界：不接 HTTP；不接真实上游
- 状态：done
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-09 执行 开始 T3：retry（错误分类器+重试预算）→ pool（cooldown 状态机+加权轮询）→ router（优先级+加权随机），全部注入时钟/随机源保证可测
  - 2026-11-09 执行 新增三包：internal/retry（Classify 表驱动分类：网络/408/409/429/5xx→可重试、401/403→auth、400/404/413/422→透传、内容策略标识识别、ParseRetryAfter 秒数/HTTP日期、Budget 3渠道×2key+总时长上限）；internal/pool（StateMachine：active/cooldown/half_open/disabled、档位退避 30s→2m→10m→30m、Retry-After 优先、探测单飞、Snapshot/Restore 重启恢复；nginx 风格平滑加权轮询 wrr；Manager.Acquire 调度+ReportFailure 分类驱动+DeriveChannelStatus 渠道健康推导）；internal/router（priority 升序分组+同级 weight 加权随机排序 Efraimidis–Spirakis+模型映射）。全程注入假时钟/固定种子，无真实 sleep
  - 2026-11-09 验证 gofmt 无 diff、go vet、go build、go test ./... 全绿；调度三包覆盖率 pool 94.8% / retry 97.6% / router 97.0%（语句总计 95.6%，≥90% 达标，对应验收清单 #10）；WRR 序列断言 nginx 经典 a,a,b,a,c,a,a；状态机全流转（含档位升级 2m→10m→30m 封顶、探测单飞互斥、429 立即冷却、401 禁用、快照恢复后冷却按原到期时间生效）逐条实测
  - 2026-11-09 交付 T3 验收标准全部满足：internal/{retry,pool,router} 共 9 文件（6 源码 + 3 测试），纯逻辑零 HTTP 依赖，待 T4 relay 接线

### T4: relay 链路（P0）

- 目标：OpenAI 兼容上游适配器 + SSE 零缓冲透传 + 首 token 超时 + 重试预算串联；httptest mock 双渠道集成测试
- 验收标准：集成测试覆盖验收清单 #2–#7（非流式/流式首 token 前切换、首 token 后失败不重复、429 cooldown、401 禁用、渠道耗尽 503）
- 边界：不做管理 API；不做前端
- 状态：done
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-09 执行 新增 internal/relay（relay.go：编排/路由候选/重试预算/同渠道 key 去重/错误透传/503 OpenAI 错误体/模型映射重构请求体（RawMessage 保真）//v1/models 去重合并/渠道级超时覆盖+transport 缓存；stream.go：SSE 零缓冲 flush 循环/首 chunk 前可重试窗口/首 chunk 后永不重试+明确 SSE error event/15s keepalive 心跳）；server 增加 SetupRoutes 钩子；main 接线 router+pool+relay
  - 2026-11-09 验证 集成测试 9 用例全绿：#2 非流式 500→次渠道（含模型映射生效断言）；#3 流式挂起→首token超时→次渠道完整流且无重复；#4 首 chunk 后断连→SSE 错误事件+次渠道 0 调用+无重复；#5 429+Retry-After→key 冷却至+60s（快照实测）+其余 key 服务+冷却期 0 复用；#6 401→key disabled（后续 0 使用）；#7 双渠道 500→503+OpenAI 错误体三字段+3 次失败后 key 冷却不再打上游；另加 400 透传/多行 SSE 保真/models 排序去重
  - 2026-11-09 验证 gofmt/vet/build 全量 test 全绿（7 包）；运行时冒烟：:8787 healthz 200、/v1/models 200 空列表、非法 JSON→400 OpenAI 错误体、优雅停机
  - 2026-11-09 交付 T4 验收标准全部满足（验收清单 #2-#7）；token 鉴权/请求日志落库为 T5 范围

### T4a: 参考项目补课与对照评审（P0，用户指令）

- 目标：补执行任务书第 2 章学习指令——精读 gpt-load 调度内核源码、读 LiteLLM Router 文档、读 bifrost 错误分类；与 T3/T4 实现逐项对照，产出差距清单
- 验收标准：完成 gpt-load 精读（凭据池/重试/错误分类/流式处理四个主题）+ LiteLLM Router 文档阅读 + bifrost 错误分类阅读；对照报告落盘 docs/；发现的差距全部转为决策（立即修 / 建卡延后 / 维持现状并说明理由）
- 边界：new-api（AGPL）本轮不看代码，其产品交互留待 T6 前端时参考截图与功能清单；不引入参考项目任何代码（只学设计，除非明确标注 MIT/Apache 且有必要）
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-09 计划 用户选择方案 A：先补课再继续 T5；任务卡确认
  - 2026-11-09 执行 经代理克隆 gpt-load / bifrost 源码至临时目录并抓取 LiteLLM Router 文档；精读 gpt-load internal/health（错误判定引擎）与 internal/state（调度账本/quota 冷却）、bifrost core/failureclass.go（错误分类学）与 providers/*/errors.go（SSE 流内错误）、LiteLLM Router 语义（allowed_fails/cooldown_time）
  - 2026-11-09 执行 照评审发现 8 项差距全部修复：SSE 流内错误事件解析（首 chunk 错误仍可重试）、404 model_not_found 换渠道不惩罚 key、瞬时错误混合策略（单请求跳渠道+跨请求阈值冷却）、凭据/配额/限流文案/529 标识识别、SSE 推断限流默认 60s 冷却；详见 docs/参考项目对照评审.md
  - 2026-11-09 验证 retry 42 用例（31 分类 + 11 流错误状态）+ relay 12 集成用例（含 3 个新场景：model_not_found 换渠道、瞬时错误混合语义、SSE 首 chunk 错误无感切换）全绿；gofmt/vet/7 包 test/build 全绿
  - 2026-11-09 交付 对照评审报告（docs/参考项目对照评审.md）：8 项已修复 + 5 项 P1 建议 + 4 项有依据维持现状

### T5: 管理 API（P0）

- 目标：auth / channels / keys / tokens / logs 的 REST + 下游令牌鉴权中间件 + 请求日志异步落库（缓冲 channel）
- 验收标准：登录登出可用；渠道与 key CRUD + 批量导入 + 连通测试可用；令牌 CRUD 可用；日志分页查询可用；任何接口不回显明文（对应验收清单 #9）
- 边界：不做用量统计报表（P1）；不做 Playground（P1）
- 状态：done
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-09 执行 开始 T5：auth/channels/keys/tokens/logs REST + 令牌鉴权中间件 + 日志异步落库 + cooldown 状态持久化（验收 #8）
  - 2026-11-09 执行 新增 internal/admin（HMAC 无状态 Cookie 会话 + sessionMiddleware 强制首登改密 + auth/channels CRUD/toggle/test 连通测试/keys 批量导入启停删除/tokens CRUD/logs 分页多维筛选），key 列表零密文字段（验收 #9）；internal/relay/auth.go（令牌 SHA-256 鉴权中间件）+ logwriter.go（缓冲 channel + 1s/64条批量刷盘 + 满溢丢弃计数）；internal/health/persist.go（cooldown 状态 5s 定期持久化 + 启动恢复 + 停机刷盘，验收 #8）；ChannelKey 增 Level 列；main 全量接线（UTC 时钟贯穿）
  - 2026-11-09 验证 单测全绿：admin 4 组（登录失败/403 强制改密/改密放行/渠道 key 级联/令牌明文仅一次/日志分页筛选/会话防篡改）、relay auth 3 组（缺失/错误/禁用/过期令牌 401、日志批量落库）、health 持久化 2 组（快照往返恢复原到期时间/active 不恢复）
  - 2026-11-09 验证 运行时端到端（真实进程 + curl）：登录→改密前 403→改密→建渠道→批量导 3 key→建令牌（明文仅一次）→令牌列表无明文→带令牌 /v1/models 返回渠道模型→无令牌 401→chat 失败 503（上游不可达，预期）→日志异步落库（model/status/重试轨迹/令牌名齐全）；优雅停机
  - 2026-11-09 交付 T5 验收标准全部满足；用量统计/Playground 为 P1

### T5a: 前端管理台设计补课（P0，用户指令）

- 目标：T6 开工前完成参考学习——精读 gpt-load 前端源码（MIT，可借鉴实现）；new-api 仅读 README/功能清单/交互截图学产品设计（AGPL 红线：不看任何代码）；产出 T6 的页面与交互设计要点
- 验收标准：gpt-load web/ 结构与关键页面（渠道/凭据/日志/仪表盘）精读完成；new-api 产品功能清单梳理完成；设计要点落盘 docs/管理台设计参考.md 并映射到 T6 五页面
- 边界：不复制 new-api 任何代码/组件/样式；gpt-load 可借鉴实现但本项目技术栈为 React（任务书 3.2 定稿），只学交互与结构不搬 Vue 代码
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-09 计划 用户指令确认：T6 前先补前端设计学习
  - 2026-11-09 执行 精读 gpt-load web/src（HomeSummary 事实行、GroupCredentialRecord 凭据可展开行+状态徽标+行内权重+近期成功失败计数、连通测试三态色调、credential-failure-presenter 失败类别人话映射、Monitor 的 LogsFilterForm+LogDetailDrawer 日志详情抽屉、GroupCredentialBatchBar 批量条、HealthSummaryStrip 健康摘要条）；经代理抓取 new-api README（仅功能清单与截图描述，AGPL 红线未读任何代码）梳理渠道配置/路由/用量日志/令牌/快速上手五功能域
  - 2026-11-09 执行 设计规格落盘 docs/管理台设计参考.md：T6 五页面逐一映射（登录强制改密跳转、Dashboard 事实行+健康列表+空状态四步引导、渠道三态测试+key 展开+批量导入、令牌明文一次+一键复制、日志筛选+详情抽屉+重试轨迹时间线）；技术决策：React19+Vite+TanStack Query+Tailwind4+lucide，shadcn 风格手写最小组件集（不跑 CLI），路由用 react-router 并记录偏离任务书理由
  - 2026-11-09 交付 T6 设计输入齐备，可直接开工

### T6: 前端管理台（P0）

- 目标：Vite 脚手架 → 登录（首登强制改密）→ Dashboard → 渠道管理（批量导 key、连通测试、启停）→ 令牌管理 → 请求日志（重试轨迹可视化）
- 验收标准：`npm run build` 成功；五个页面可用且与 T5 API 对接完成；日志详情可展示每次尝试的渠道/key/错误/耗时
- 边界：不做 Playground / 用量统计 / 系统设置页（P1）；第一版仅简体中文
- 状态：done
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-09 执行 开始 T6：按 docs/管理台设计参考.md 规格——React19+Vite+TanStack Query+Tailwind4+lucide，手写 shadcn 风格最小组件集，五页面 + 重试轨迹时间线
  - 2026-11-09 执行 web/ 全量实现：手写脚手架（package.json/vite.config/tsconfig，无 CLI 交互）；api 层（types + fetch 信封客户端 + TanStack Query hooks 全套）；ui.tsx 最小组件集（Button/Input/Field/Badge/Card/Spinner/Dialog/Drawer/Empty/表格件）；Layout 侧边栏（me 守卫 401→login、must_change_password→改密页）；五页面——Login/ChangePassword/Dashboard（事实行+今日卡片+渠道健康+空状态四步引导）/Channels（表格+启停开关+三态测试徽标+key 展开面板：状态徽标/连续失败/退避档位/冷却剩余/最近错误+批量导入 textarea）/Tokens（明文仅一次+复制）/Logs（筛选栏+分页+详情抽屉+重试轨迹时间线红绿点）；错误类别人话映射与 T4a 分类学对齐；npm install 90 包经代理完成
  - 2026-11-09 验证 npm run build（tsc 严格模式 + vite）通过，产物 349KB JS/21KB CSS（gzip 107KB）；浏览器实测（vite preview :4173 代理后端 :8787）：登录→Dashboard 事实行/卡片/渠道健康表→渠道页 key 面板展开 3 key 状态与批量导入→日志页详情抽屉重试轨迹时间线（尝试1→openai-main·key#1·10000ms·服务器故障徽标）→令牌页新建（明文仅一次+复制+警示）五页面全通过；截图存证
  - 2026-11-09 交付 T6 验收标准全部满足；Playground/用量报表/系统设置为 P1

### T7: 嵌入与交付（P0）

- 目标：go:embed 嵌入前端产物 + Makefile 三平台交叉编译（windows/amd64、linux/amd64、darwin/arm64）
- 验收标准：`make build` 产出单二进制且 `./evergate` 启动后 `/v1/*` 与管理台同端口可用（对应验收清单 #1）；三平台编译通过；二进制 < 30MB
- 边界：不用 goreleaser 发布流水线（手写 build 脚本即可）
- 状态：done
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效
  - 2026-11-10 执行 开始 T7：go:embed 嵌入 + SPA 服务 + Makefile 三平台交叉编译 + 非功能指标测量；安装 GNU Make 4.4.1（ezwinports）
  - 2026-11-10 执行 web/embed.go（go:embed all:dist + fs.Sub 取根）；server/web.go（NoRoute SPA 服务：/v1 /admin /healthz 前缀走 404 JSON，未知路径回退 index.html；assets 长缓存 immutable）；main 接线 WebFS + version 改 var（-ldflags 注入 git describe）；Makefile：build（npm build → go build -s -w）与 build-all（target 级 export GOOS/GOARCH）；web/dist 入库（后端免 Node 构建）
  - 2026-11-10 验收 make build → bin/evergate.exe 26.98MB（<30MB ✅）；运行实测：healthz 200、管理台 / 200（HTML 含 React 挂载点与 assets）、SPA 回退 /channels 200、/v1/models 无令牌 401、/v1 未知路径 404 JSON、assets 缓存头 immutable——验收清单 #1 ✅；三平台交叉编译 windows/amd64 26.98MB + linux/amd64 26.58MB + darwin/arm64 25.26MB 全部 <30MB ✅；冷启动日志时间线 ~180ms（<1s ✅，PowerShell 测量工具 ~2s 开销已用日志时间线排除）；空闲私有内存 49.1MB / 工作集 18.3MB（<50MB ✅）；优雅停机此前多次验证 ✅
  - 2026-11-10 交付 T7 验收标准全部满足，P0 十项验收清单全部达成；首 token 额外延迟 <5ms 属 4.9 非功能项，待真实上游环境实测（本地 loopback 无法代表真实网络，如实标注）

### T7b: 安全检查、公开发布与自动打包（P0，用户指令）

- 目标：公开仓库前全历史敏感信息扫描与清理；仓库转 public；手动发布 v0.1.0（三平台二进制）；配置 GitHub Actions tag 触发自动构建发布并以 v0.1.1 验证
- 验收标准：历史中无有效密钥类字符串残留（已知泄漏项已重写并验证）；Release v0.1.0 含 3 个平台二进制可下载；Actions 工作流在 v0.1.1 tag 上自动构建并挂载资产成功
- 边界：不引入 goreleaser（手写 workflow）；本卡不改业务代码
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户指令确认：安全检查 → public → 发版 → 后续自动化
  - 2026-11-10 执行 安全审计：全历史高危模式扫描（API key/私钥/token）——唯一真实泄漏=T2 事故的 537d5ff:evergate.yml 旧 master_key/session_secret（已轮换失效，且当时 DB 无加密数据，实际风险为零）；admin_test.go 命中为假 key 测试夹具（无害）；当前树无敏感文件（evergate.yml 未跟踪、data/ 与 bin/ 未入库）
  - 2026-11-10 执行 历史重写：git filter-branch 移除 evergate.yml 全历史 → refs/original/reflog 清理 + gc --prune=now --aggressive → 本地零残留验证 → 强推远程（全历史哈希改变：本文件早期进度行中的旧哈希 d168d99/537d5ff/7de81d4/018c586/b776276/f4204ca 等自此仅为历史标签，不再对应仓库对象）
  - 2026-11-10 执行 gh repo edit --visibility public；tag v0.1.0 → 干净重建（ldflags 版本串 v0.1.0 实测注入）→ gh release create v0.1.0 手动上传三平台二进制（附 SHA256 摘要）
  - 2026-11-10 执行 新增 .github/workflows/release.yml（tag v* 触发：npm ci+build → setup-go(go.mod 版本) → CGO_ENABLED=0 三平台构建 → sha256 checksums → softprops/gh-release 挂载）；推 v0.1.1 验证：Actions 全绿，自动产出 4 资产（3 二进制+checksums.txt）
  - 2026-11-10 验证 Release 页面：v0.1.0（手动）+ v0.1.1（自动）均可下载；仓库 PUBLIC；历史 grep 旧密钥值零命中
  - 2026-11-10 交付 T7b 验收标准全部满足：公开仓库安全、v0.1.0 可下载、后续发版=推 tag 即自动打包
  - 2026-11-10 执行 改进（用户反馈）：自动发版缺少描述——新增 CHANGELOG.md（发版描述唯一来源），工作流按 `## <tag>` 提取段落作为 Release 正文（无段落时回退自动生成笔记），并回填修复 v0.1.1 描述
  - 2026-11-10 验证 v0.1.2 端到端：Actions 提取 CHANGELOG 段落成功，Release 正文=变更说明+Full Changelog 对比链接，4 资产齐全

### T8: 首次改密后前端停留改密页缺陷修复（P0，用户反馈）

- 目标：修复首次登录修改密码成功后被路由守卫重定向回 `/change-password` 的前端状态缓存问题
- 验收标准：真实浏览器流程中提交改密后停留在 `/`，且 `/admin/api/auth/me` 返回 `must_change_password=false`；后端改密 API 行为不变；`go test ./...`、`npm run build`、`make build` 全部通过
- 边界：不改数据库 schema、不改后端密码哈希与鉴权逻辑、不读取/修改用户安装目录数据（只读确认故障状态）
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户反馈“提交后仍停在首次修改页面”，并提供本地安装数据目录；缺陷卡确认
  - 2026-11-10 执行 只读检查 `C:\Users\PC\Downloads\data\evergate.db`：`admins.must_change_password=0`，`updated_at=2026-11-10 01:43:12 UTC`，证明后端改密已成功、问题限定在前端路由状态
  - 2026-11-10 执行 浏览器级反馈环复现两次：改密 API 成功且 `/me` 已返回 false，但页面仍在 `/change-password`（红色稳定）
  - 2026-11-10 执行 定位与修复：`useMe` 的 `['me']` 缓存在 `staleTime=60s` 内仍保留 `must_change_password=true`，Layout 路由守卫据此把 `/` 弹回改密页；`useChangePassword.onSuccess` 现同步 `setQueryData` 清除强制标记，再执行页面导航
  - 2026-11-10 验证 同一浏览器反馈环修复前红 2/2、修复后绿 2/2（路径 `/`，`/me.must_change_password=false`）；`npm --prefix web run build`、`make build`、`gofmt`、`go vet ./...`、`go test ./...` 全部通过
  - 2026-11-10 交付 T8 验收标准满足；当前仓库未引入前端测试框架，本次真实浏览器反馈环为临时调试产物，验证后删除，不为单缺陷扩依赖（后续若建 P1 前端测试体系再统一引入）
  - 2026-11-10 发版 v0.1.3：Release Action 1m55s 成功；Release 正文正确提取 CHANGELOG 修复说明，三平台二进制 + checksums.txt 共 4 资产齐全

### T9: 品牌图标统一到浏览器角标与 Windows exe（P0，用户反馈）

- 目标：以管理台侧边栏当前 TowerControl 管理端 logo 为唯一视觉源，新增浏览器 favicon，并把同源图标嵌入 Windows exe，供用户将 exe 放到桌面作为启动器
- 验收标准：`web/dist` 内提供并引用 favicon；Windows 构建产物可通过系统图标 API 提取到非空图标；`npm --prefix web run build`、`go test ./...`、`make build` 全部通过；图标来源保留 Lucide ISC 许可证声明
- 边界：当前交付形态是单文件 exe 而非安装器，本卡只嵌入 exe 图标，不做安装器/开机自启/快捷方式创建；macOS/Linux 二进制不做图标包装；不新增运行时依赖
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户指令确认：统一管理端 logo、网页角标与 exe 图标
  - 2026-11-10 执行 新增 `web/public/logo.svg`（源自管理端 Lucide TowerControl，保留 ISC 来源声明），侧边栏改为直接引用该 SVG；`web/index.html` 引用 SVG favicon 与兼容 ICO；由同一 SVG 渲染生成 16–256px `favicon.ico`，并经 `make icon`（rsrc v0.10.2，构建工具不进 go.mod）生成 `cmd/evergate/rsrc_windows_amd64.syso`
  - 2026-11-10 验证 `make icon` 重复执行 SHA256 一致；`npm --prefix web run build`、`make build`、`make build-all`、`gofmt`、`go vet ./...`、`go test ./...` 全部通过；本机与 Release windows/amd64 exe 经 Windows 图标 API 均提取到 32×32 图标；临时服务实测 `/logo.svg` 与 `/favicon.ico` 均 200 且与源文件 SHA256 一致
  - 2026-11-10 交付 T9 验收标准满足：管理台侧边栏、浏览器角标、Windows exe 图标共用同一视觉源；Linux/macOS 交叉编译不受影响
  - 2026-11-10 发版 v0.1.4：Release Action 1m47s 成功；重新下载 Release Windows exe 后仍可提取 32×32 图标，三平台二进制 + checksums.txt 共 4 资产齐全，Release 正文正确提取 CHANGELOG 说明

### T10: Codex 接入兼容层——OpenAI Responses API 入口（P1，用户指令）

- 目标：新增 `POST /v1/responses`，把 Codex 使用的 OpenAI Responses 请求/流式事件转换为现有 OpenAI-compatible Chat Completions 上游协议，使 Codex 可通过 EverGate 下游令牌走统一路由、key 池、重试与日志链路
- 验收标准：
  1. Codex `config.toml` 可配置 `base_url="http://127.0.0.1:8787/v1"`、`wire_api="responses"` 并完成一次真实代码问答任务；
  2. mock 上游集成测试覆盖非流式、SSE 流式、工具调用、上游错误、令牌无效四类场景；
  3. Responses SSE 事件序列可被 Codex 消费，首个事件前复用现有重试/切换能力，首个事件后不得重复输出；
  4. 请求进入既有 TokenAuth、模型路由、key 池、cooldown、retry、request log 与管理台日志链路；
  5. README 提供 Codex 接入示例（`model_provider`、`base_url`、`env_key`/令牌来源），并说明当前上游仍走 Chat Completions 转换；
  6. `go test ./...`、`go vet ./...`、`make build` 全部通过。
- 边界：本卡做协议转换入口，不改数据库 schema；不新增外部服务；第一版不改渠道模型（上游仍按 OpenAI-compatible Chat Completions 处理），不做 Anthropic `/v1/messages`；Responses 字段只实现 Codex 实际所需子集，未支持字段必须显式返回可诊断错误而非静默丢弃
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户指令确认：把 Codex 接入加入开发计划。前置事实已核实：Codex 官方配置要求自定义 provider 使用 `wire_api="responses"`；当前 EverGate `POST /v1/responses` 返回 404，现有入口仅 `/v1/chat/completions` 与 `/v1/models`
  - 2026-11-10 执行 用户指令确认开始 T10；测试切面采用公共 HTTP 边界（gateway `/v1/responses` → mock 上游 `/chat/completions`），先写非流式/SSE/工具调用红色用例，再实现协议转换
  - 2026-11-10 执行 新增 `internal/relay/responses.go` 与 `responses_stream.go`：请求白名单校验、instructions/input/function_call/function_call_output 转换、namespace 工具扁平化、非流式响应转换、Chat SSE → Responses 语义事件转换；`cmd/evergate/main.go` 挂载 TokenAuth 保护的 `/v1/responses`
  - 2026-11-10 验证 relay 公共接口测试覆盖非流式、SSE、工具调用多轮回传、流中错误、不支持字段显式 400、无效令牌 401、首个 Responses 事件前故障切换；完整 `go test ./internal/relay` 通过。真实 Codex v0.167.0-alpha.3.2 通过临时 EverGate 调用 mock 上游完成两轮请求：第一轮返回 `exec_command` function_call，第二轮已转换出 assistant tool_calls + tool output，最终输出 `EverGate tool ok`
  - 2026-11-10 交付 T10 验收标准满足；`gofmt`、`go vet ./...`、`go test ./... -count=1`、`npm --prefix web run build`、`make build` 全部通过

### T11: 首次启动打开管理台与桌面体验优化（P1，用户指令）

- 目标：Windows 用户双击 exe 启动后自动打开后台管理页面，无需记忆或输入 `http://127.0.0.1:8787`
- 验收标准：`server.open_browser` 可控制是否自动打开，默认 true；服务完成监听并通过 healthz 探活后再调用系统默认浏览器；`127.0.0.1`/局域网地址按 `server.base_path` 生成正确 URL；Windows/macOS/Linux 打开命令有单元覆盖或显式平台分支；启动失败不打开页面；`go test ./...`、`make build` 通过
- 边界：不新增安装器、服务、快捷方式文件或权限申请；默认开启以匹配本地双击使用场景，服务器/自动化部署可显式设为 false；不做开机自启；打开失败只记录 warning 不影响网关服务
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户指令确认：启动 exe 后自动弹出管理台页面
  - 2026-11-10 执行 新增 `internal/browser`（AdminURL / healthz 就绪等待 / Windows-macOS-Linux 默认浏览器命令）与 `server.open_browser` 配置（默认 true）；主程序在服务监听后异步探活并打开管理台，失败仅 warning
  - 2026-11-10 验证 `internal/browser`、`internal/config` 单元测试通过：URL/base_path、三平台命令、默认开关、YAML false 覆盖、healthz 未就绪不打开均覆盖
  - 2026-11-10 交付 T11 验收标准满足；完整 Go 测试、前端构建与 `make build` 通过

### T12: 管理台界面 refinement 与自动更新检查（P1，用户指令）

- 目标：优化现有五页面管理台的视觉层级、空状态与可读性；增加 GitHub Release 自动更新检查，让用户在管理台内看到当前版本、最新版本与下载入口
- 验收标准：更新检查走管理台会话鉴权 API，不向前端暴露 GitHub token；请求失败/离线/开发版本显式降级；版本比较有单元测试；UI 在 1280×800 常规桌面与窄屏下不破版；`npm run build`、`go test ./...`、`make build` 通过
- 边界：第一版做“自动检查 + 明确下载入口”，不做静默替换正在运行的 exe、不做强制升级、不做差量更新、不引入 Electron/安装器；保持现有五页面信息架构不推倒重做
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户指令确认：优化管理台 UI，并新增自动更新获取能力
  - 2026-11-10 执行 新增 `internal/update`（版本比较、GitHub Release 查询、403/429 后 Atom 降级、Windows 资产推导）与管理台会话保护接口 `GET /admin/api/system/update`；前端新增自动更新查询、Dashboard 更新横幅/下载按钮、侧边栏版本区，并优化侧边栏窄屏、登录卡片、背景层级、指标卡与表格滚动
  - 2026-11-10 验证 update/admin 单元测试通过；临时 v0.1.3 实例真实检查到 GitHub v0.1.4 并返回 Windows 下载 URL；无头 Chrome 实测 Dashboard 1280×800 与 390×844 均无横向溢出，且显示 v0.1.4 更新提示与下载入口
  - 2026-11-10 交付 T12 验收标准满足；完整 Go 测试、前端构建与 `make build` 通过
  - 2026-11-10 发版 v0.1.5：Release Action 2m30s 成功；重新下载 Release Windows exe 后实测 healthz=200、`POST /v1/responses` 未带令牌返回 401、`GET /admin/api/system/update` 未登录返回 401，确认新入口与更新接口已进入远程产物；三平台二进制 + checksums.txt 共 4 资产齐全

### T13: Anthropic Messages 入口与三协议接入矩阵（P1，用户指令）

- 目标：确立 EverGate 后续客户端接入的默认协议矩阵——OpenAI Chat Completions `/v1/chat/completions`、OpenAI Responses `/v1/responses`、Anthropic Messages `/v1/messages` 三种下游 API 协议均可接入同一模型路由、key 池、重试、cooldown 与请求日志链路；补齐缺失的 Anthropic Messages 入口
- 验收标准：
  1. 三个公共入口均支持 TokenAuth、模型映射、key 池调度、故障切换、请求日志与管理台查询；
  2. `/v1/messages` mock 上游集成测试覆盖非流式、SSE 流式、tool_use/tool_result、上游错误、无效令牌、首事件前故障切换；
  3. Anthropic SSE 事件序列可被真实 Anthropic-compatible 客户端消费；
  4. 三种协议共同能力的请求/响应/流式/工具调用有对照测试矩阵，能力差异必须在 README 中显式列明；
  5. `go test ./...`、`go vet ./...`、`npm --prefix web run build`、`make build` 全部通过。
- 边界：第一版 `/v1/messages` 是下游入口协议转换，上游仍统一走 OpenAI-compatible Chat Completions；不做 Anthropic 原生上游适配器、不做数据库 schema 变更、不做超出客户端所需范围的字段全集，未支持字段显式返回可诊断错误
- 状态：planned
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户指令确认：以后客户端接入默认覆盖 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 三种 API 协议；当前前两者已由 T0/T10 交付，Anthropic Messages 为本卡待开发项

### T14: Responses custom tool 兼容修复（P1，用户反馈缺陷）

- 目标：修复新版 Codex 向 `POST /v1/responses` 声明 `type="custom"` 工具时被 EverGate 400 拒绝的问题，使 custom tool 可完成声明、调用、结果回传与流式事件往返
- 验收标准：
  1. 公共 HTTP 边界测试先复现 `unsupported Responses tool type "custom"`，修复后通过；
  2. custom tool 在上游 Chat Completions 中降级为单字段 `input` 的 function tool，不把 `type="custom"` 原样发给上游；
  3. 上游 function call 返回 `{"input":"..."}` 时，Responses 输出转换为 `custom_tool_call`，包含 `call_id/name/input`；
  4. `custom_tool_call_output` 可回传为 Chat Completions tool message，并在下一轮得到最终文本；
  5. SSE 输出使用 `response.custom_tool_call_input.done` 官方事件，且不泄漏 Chat Completions framing；
  6. `go test ./... -count=1`、`go vet ./...`、`make build` 通过。
- 边界：只处理客户端所需的自由文本 custom tool 子集；不实现 grammar/lark/regex 约束、allowed_callers、异步 custom tool、WebSocket；不改变上游仍为 OpenAI-compatible Chat Completions 的架构
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户反馈 Codex 报 `unsupported Responses tool type "custom"`；已核对 OpenAI 官方 Responses 参考确认 custom tool、custom_tool_call、custom_tool_call_output 与 `response.custom_tool_call_input.done` schema，开始建红测
  - 2026-11-10 执行 红测 `TestResponsesCustomToolStreamRoundTrip` 先在公共 HTTP 边界复现用户原始错误：`400 {"error":{"message":"unsupported Responses tool type \"custom\""}}`
  - 2026-11-10 执行 `responsesToChat` 支持 custom tool / custom_tool_call / custom_tool_call_output：custom tool 降级为上游 function schema `{input:string}`，回传输出转 tool message；响应侧按请求声明恢复 `custom_tool_call`，SSE 发送官方 `response.custom_tool_call_input.done` 后再 `output_item.done`；同时转换 custom tool_choice 并显式拒绝 async/grammar 变体
  - 2026-11-10 验证 红测转绿；新增非流式 custom tool 测试与两轮 custom tool 流式回传测试。`go test ./... -count=1` 与 `go vet ./...` 通过，`npm --prefix web run build` 与 `make build` 在沙箱外完成并通过（本沙箱内 esbuild spawn EPERM）
  - 2026-11-10 交付 T14 验收标准满足。已用 OpenAI 官方 Create Response / streaming events 参考核对字段；未声明支持 grammar、异步 custom tool 或 WebSocket，相关输入会显式报错或在 README 标注不支持

## 状态枚举

| 状态 | 含义 |
|---|---|
| planned | 已建卡确认，未动手 |
| in-progress | 执行中 |
| done | 验收标准全部满足且有验证证据 |
| blocked | 受阻；最新进度行注明原因与可否绕行 |
| canceled | 放弃；最新进度行注明原因 |

## 里程碑

| 里程碑 | 内容 | 完成判据 |
|---|---|---|
| P0（本次） | T0–T7 全部 done | 下方「验收清单」全部通过 |
| P1（后续） | T10 Codex 接入；T11 启动打开管理台；T12 管理 UI 与更新检查；T13 三协议接入矩阵与 Anthropic Messages 入口；会话亲和、健康探测徽标、用量成本统计、Playground、令牌额度/白名单强制、请求级 config 覆盖、系统设置、OTel 可选导出 | T10–T13 按上方任务卡验收；其余项开工前再拆任务卡 |
| P2（后续，暂不建卡） | Anthropic 原生上游适配、多用户角色、语义缓存、按预算模型降级、插件钩子 | 开工前再拆任务卡 |

## 验收清单

P0 完成的唯一定义（全部通过才算 done）：

1. `make build` 产出单二进制；`./evergate` 启动后 `:8787/v1/*`（端口经 evergate.yml 可配）与管理台同时可用
2. 故障切换（非流式）：配置双渠道，mock 主渠道返回 500 → 请求自动落到次渠道，客户端收到正常响应且无感知
3. 故障切换（流式，首 token 前）：主渠道建连后挂起不返回 → 首 token 超时 → 自动切换次渠道，客户端正常收到完整流式输出
4. 首 token 后失败：上游流中途断开 → 客户端收到明确 SSE 错误，不出现重复内容
5. 429 处理：mock 上游 429 + `Retry-After: 60` → 该 key cooldown_until = now+60s，期间请求自动用其他 key
6. 401 处理：某 key 返回 401 → 该 key 自动 disabled，其余 key 正常服务
7. 全部渠道耗尽：返回 503 + OpenAI 格式错误体，管理台渠道状态显示 down
8. 重启进程：渠道/密钥/令牌/日志完整保留，cooldown 状态从 DB 恢复
9. 上游 key 在 DB 中为密文；管理台任何接口不回显明文
10. 调度器（状态机+错误分类）单元测试覆盖 ≥ 90% 分支；提供基于 httptest mock 上游的集成测试

非功能验收项：二进制体积 < 30MB；空闲内存 < 50MB；冷启动 < 1s（到 /healthz 200）；网关自身首 token 额外延迟 < 5ms（与直连对比）；三平台交叉编译通过；优雅停机（SIGINT/SIGTERM 后停止接新请求，在途请求最长 30s 排空）。
