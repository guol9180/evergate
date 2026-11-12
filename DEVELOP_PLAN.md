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

### T13: Anthropic Messages 入口与三协议接入矩阵（延后，原 P1 用户指令）

- 目标：确立 EverGate 后续客户端接入的默认协议矩阵——OpenAI Chat Completions `/v1/chat/completions`、OpenAI Responses `/v1/responses`、Anthropic Messages `/v1/messages` 三种下游 API 协议均可接入同一模型路由、key 池、重试、cooldown 与请求日志链路；补齐缺失的 Anthropic Messages 入口
- 验收标准：
  1. 三个公共入口均支持 TokenAuth、模型映射、key 池调度、故障切换、请求日志与管理台查询；
  2. `/v1/messages` mock 上游集成测试覆盖非流式、SSE 流式、tool_use/tool_result、上游错误、无效令牌、首事件前故障切换；
  3. Anthropic SSE 事件序列可被真实 Anthropic-compatible 客户端消费；
  4. 三种协议共同能力的请求/响应/流式/工具调用有对照测试矩阵，能力差异必须在 README 中显式列明；
  5. `go test ./...`、`go vet ./...`、`npm --prefix web run build`、`make build` 全部通过。
- 边界：第一版 `/v1/messages` 是下游入口协议转换，上游仍统一走 OpenAI-compatible Chat Completions；不做 Anthropic 原生上游适配器、不做数据库 schema 变更、不做超出客户端所需范围的字段全集，未支持字段显式返回可诊断错误
- 状态：deferred
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户指令确认：以后客户端接入默认覆盖 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 三种 API 协议；当前前两者已由 T0/T10 交付，Anthropic Messages 为本卡待开发项
  - 2026-11-11 计划 用户决策（2026-11-10 作出，因并发会话还原文档而延迟落盘）延后本卡：v1 协议范围收敛为上游/下游均支持 Chat Completions 与 Responses、同协议透传优先（新增 T18 承接）；决策依据见 docs/协议生态调研-ChatCompletions与Responses.md。重启条件：渠道池出现真实需求（如 Anthropic 官方上游或 Claude 系客户端），重启前先回本卡更新范围

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
  - 2026-11-10 发版 准备发布 v0.1.6：将修复送入 GitHub Actions 自动打包，供已安装用户通过更新入口下载
  - 2026-11-10 发版 v0.1.6：Release Action 42781290657 成功；Release 正文正确提取 custom tool 修复说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（27,983,872 bytes，启动显示 version=v0.1.6）后在 18900/18901 隔离实例 + mock 上游完成端到端烟测：第一轮 custom tool 返回 200 且包含 `custom_tool_call`、`input:"ping"` 与 `response.custom_tool_call_input.done`；第二轮 `custom_tool_call_output` 回传返回 200 与最终文本 `remote custom ok`

### T15: Codex apply_patch grammar 与 Responses 兼容性审计（P1，用户反馈缺陷）

- 目标：修复 Codex 声明 `apply_patch` Responses custom tool grammar 时被 EverGate 400 拒绝的问题，并系统学习允许参考项目的 Responses↔Chat Completions 桥接实现，审计 EverGate 当前 Responses 子集，避免继续逐字段踩坑
- 验收标准：
  1. 公共 HTTP 边界红测先复现 `Responses custom tool "apply_patch" grammar format is not supported`，修复后通过；
  2. 官方 schema 与至少三个允许参考项目（gpt-load / bifrost / LiteLLM 核心，或 OpenAI Codex 开源实现）的实际桥接策略完成核对，结论落入 docs；
  3. 尽可能捕获本机 Codex 真实 `/v1/responses` 请求形状（令牌脱敏），对照 EverGate 支持矩阵列出已支持 / 显式不支持 / 误拒绝项；
  4. `apply_patch` grammar 在 Chat Completions 上游获得可执行的降级策略，不能静默丢失约束；custom tool 调用与回传仍保持 T14 语义；
  5. 回归测试覆盖 grammar 声明、非流式、SSE、`custom_tool_call_output` 与错误诊断；
  6. Responses namespace 工具 flattened 后的输出必须恢复原 `namespace`，历史回传仍使用上游扁平名；用 mock 上游覆盖 function 与 custom 两类 namespace call；
  7. Responses Lite `additional_tools` 被 hoist 到上游 tools，`configuration_update.reasoning.effort` 能覆盖上游 reasoning effort，`text.verbosity` 转为 Chat `verbosity`；
  8. 真实 Codex 0.167 alpha 完成一轮 `apply_patch` 创建文件与 `custom_tool_call_output` 回传；
  9. `go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 全部通过。
- 边界：本卡仍不把 EverGate 改成 Responses 原生上游透传，不改渠道模型与数据库 schema；只修 Codex 所需 Responses 子集与明显误拒绝项；参考项目严格遵守 AGENTS 许可证红线，不复制 new-api / OmniRoute 代码
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-10 计划 用户反馈新版错误为 `Responses custom tool "apply_patch" grammar format is not supported`，并要求学习其他项目的 Responses 对接方式；T14 只支持自由文本 custom tool，grammar 变体被显式拒绝，需重新审计而非继续局部补丁
  - 2026-11-10 执行 已完成 OpenAI 官方 schema、OpenAI Codex、Bifrost、gpt-load、LiteLLM 源码审计并新增 `docs/Responses协议兼容性审计.md`；额外发现 EverGate 当前只扁平化 namespace 工具、输出不恢复 namespace，属于下一个高概率 Codex 断点，纳入本卡修复范围
  - 2026-11-10 执行 红测 `TestResponsesApplyPatchCustomGrammarStream` 复现 `Responses custom tool "apply_patch" grammar format is not supported`；随后实现 grammar→function 参数描述降级、raw arguments fallback、custom input `.delta + .done` 双事件、namespace 恢复、`additional_tools` hoist 与 `configuration_update` effort 覆盖
  - 2026-11-10 验证 捕获真实 Codex 请求发现 `text.verbosity=low` 亦会被白名单误拒绝，已补 Chat `verbosity` 转换；真实 Codex 0.167.0-alpha.3.2 通过当前源码构建 + mock 上游完整执行 `apply_patch`：grammar 进入 function 参数描述、Codex 创建 `t15-real.txt`、`custom_tool_call_output` 正确回传并获得最终文本
  - 2026-11-10 交付 T15 验收标准满足：`gofmt`、`go vet ./...`、`go test ./... -count=1`、`npm --prefix web run build`、`make build` 全部通过；`npm` / `make` 因沙箱内 esbuild spawn EPERM 在沙箱外执行
  - 2026-11-10 发版 准备发布 v0.1.7：将 apply_patch grammar 修复与 Responses 兼容补齐送入 GitHub Actions 自动打包
  - 2026-11-10 发版 v0.1.7：Release Action 42793297209 成功；Release 正文正确提取修复说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（27,991,040 bytes，启动显示 version=v0.1.7）后，用真实 Codex 0.167.0-alpha.3.2 + 18905/18906 隔离实例 + mock 上游复验：上游 function schema 保留 `grammar syntax: lark`，`text.verbosity=low` 与 reasoning effort 正确转换；Codex 执行 apply_patch 创建 `v017-real.txt`，第二轮 custom output 回传成功，最终输出 `v0.1.7 apply patch ok`

### T16: v0.1.7 Responses namespace 重名与 ZAI 渠道 503 回归修复（P0，用户反馈缺陷）

- 目标：修复 Codex 在不同 Responses namespace 下声明同名工具（如两个 MCP 均暴露 `js`）时被 EverGate 400 拒绝的问题；定位 DeepSeek Harness 访问 ZAI 模型得到 `503 all_channels_exhausted` 的真实原因，并给出代码修复或明确配置结论
- 验收标准：
  1. 公共 HTTP 边界红测先复现 `Responses flattened tool name "js" maps to multiple declarations`；
  2. 多 namespace 同名 function/custom 工具可同时发送给 Chat 上游，且上游工具名唯一、合法、可往返；Responses 输出恢复原始 `namespace/name`，历史 `function_call` / `custom_tool_call` 回传使用同一上游映射；
  3. 从本机只读运行数据定位 ZAI 503 的 retry trace / 渠道配置事实，若属 EverGate 缺陷则补回归测试并修复，若属环境/渠道配置则形成可执行结论且不误改业务数据；
  4. `gofmt`、`go test ./... -count=1`、`go vet ./...`、涉及前端时 `npm --prefix web run build`、`make build` 全部通过。
- 边界：T13 保持 planned 且不在本卡顺手实现；不读取/输出上游 key 明文；不修改用户安装目录数据；不引入代理配置以外的禁止依赖；若网络代理支持需要改配置结构，先在任务卡更新范围
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 用户反馈两个缺陷：Codex 触发 `Responses flattened tool name "js" maps to multiple declarations`；DeepSeek Harness 访问 ZAI 模型触发 `503 {"code":"all_channels_exhausted"}`。暂停 T13，优先做回归修复
  - 2026-11-11 执行 只读定位安装数据实际在 `D:\software\evergate\data`（用户曾提供的 `C:\Users\PC\Downloads\data` 已不存在）。渠道：ZAI base_url `https://open.bigmodel.cn/api/v1`、模型 `glm-5.3` / `glm-5.3-flash`；相关 503 日志共 7 条，前 3 条 trace 为 ZAI key 网络瞬时错误，后 4 条因 key 连续失败进入 cooldown 后无候选渠道，trace 为空；DeepSeek 渠道当日另有成功请求。继续用隔离副本复现 ZAI 网络错误细节并建 Responses 红测
  - 2026-11-11 执行 红测 `TestResponsesDuplicateNamespacedToolNamesRoundTrip` 在公共 HTTP 边界复现用户原始 400；随后将 Responses namespace 工具从“仅按局部名去重”改为确定性唯一映射 `<namespace>__<tool>`，响应恢复原始 namespace/name，历史 function/custom 调用不依赖跨请求状态按同一映射回放，超长名称用确定性哈希截断
  - 2026-11-11 执行 用安装数据 SQLite 快照 + 已知临时下游令牌隔离复现 ZAI：原 `https://open.bigmodel.cn/api/v1` 使用真实 Coding key 返回空 body HTTP 500，连续失败后 key cooldown 导致后续 503；只把快照 Base URL 改为 `https://open.bigmodel.cn/api/coding/paas/v4` 后，同一模型 `glm-5.3-flash` 流式请求返回 200 与 SSE 内容。结论：该问题是渠道 Base URL 配置错误，不是调度器缺陷；未修改用户安装目录数据
  - 2026-11-11 执行 诊断体验修复：retry_trace 每次尝试记录上游 HTTP status_code；空 error body 的 4xx/5xx 显示 `http <status> (empty error body)`，503 汇总消息携带最后一次真实原因，管理台日志详情展示 HTTP 状态；README 增加智谱 / Z.ai Coding 正确基址表
  - 2026-11-11 验证 `gofmt` 无 diff；`go test ./... -count=1` 通过；`go vet ./...` 通过；`npm --prefix web run build` 通过（沙箱内 esbuild spawn EPERM，按既有例外在沙箱外执行）；`make build` 通过并产出 `bin/evergate.exe`
  - 2026-11-11 发版 v0.1.8：Release Action 42941863047 成功（2m27s）；Release 正文正确提取三项修复说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（28,000,768 bytes），本地 SHA256 `ea40729d…cf959` 与 checksums.txt 一致；启动显示 version=v0.1.8，`/healthz` 返回 200，未带令牌访问 `/v1/responses` 返回 401
  - 2026-11-11 交付 T16 验收标准满足：Codex 同名 namespace 工具由红测转绿；ZAI 503 定位为 Base URL 配置错误并在隔离副本验证正确 Coding 端点可用；耗尽诊断与管理台展示增强；未修改用户安装目录数据，待用户在管理台更新 ZAI 渠道 Base URL 并重置 key

### T17: 流式请求首 token 计时口径修复（P0，用户反馈缺陷）

- 目标：修复 ZAI 等上游流式请求成功但请求日志 `first_token_ms=0` 的问题，使首 token 统一从 EverGate 发起上游请求开始计算，而不是从收到上游响应头后重新开始
- 验收标准：
  1. 公共/真实调用链测试先复现“上游等待一段时间后返回首条 SSE data，但 `hit.firstTokenMS` / retry trace elapsed 记为 0”的错误；
  2. Chat Completions 与 Responses 两条流式路径均从上游请求发起时间计算首 token；
  3. 保留 SSE 零缓冲透传与首 chunk 后禁止重试红线；
  4. `gofmt`、`go test ./... -count=1`、`go vet ./...`、涉及前端时 `npm --prefix web run build`、`make build` 全部通过。
- 边界：不改变客户端响应格式、不整段缓冲上游响应、不修改数据库 schema；非流式响应的现有 first token 口径不在本卡扩大调整
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 用户确认 ZAI 已通，但日志没有像 DeepSeek 一样记录首 token。只读检查安装库：成功日志 #15 为 `stream=true status=200 total_ms=14832 first_token_ms=0`；DeepSeek #8 为 `first_token_ms=4`。直连 ZAI 原始 SSE 抓包显示请求到第一条 data 实际 5053ms，证明不是 ZAI 未返回首 token，而是 EverGate 计时口径错误
  - 2026-11-11 执行 红测 `TestStreamFirstTokenMeasuredFromUpstreamRequestStart` 在 chat / responses 两个子用例均复现 `first_token_ms=0`（期望 >=80ms）。根因是 `attemptOne` 在 `client.Do` 前已有计时点，但 `streamResponse` / `streamResponsesResponse` 进入后重新以收到响应头之后的时间作为 start；ZAI 首条 data 已随响应头进入传输缓冲，因此读到首行耗时取整为 0
  - 2026-11-11 执行 将上游请求开始时间传入 Chat Completions 与 Responses 流式转换路径，`recordHit` 与成功 retry trace 均沿用该时间；测试环境同步把 fake clock 注入 Relay，保证计时断言确定性
  - 2026-11-11 验证 红测修复后 chat / responses 两个子用例通过；`gofmt` 无 diff；`go test ./... -count=1` 通过；`go vet ./...` 通过；`make build` 通过（沙箱内 esbuild spawn EPERM，按既有例外在沙箱外执行）
  - 2026-11-11 验证 使用安装库 SQLite 快照 + 当前源码构建（18909 隔离实例）真实调用 ZAI `glm-5.3-flash`：客户端 `time_starttransfer=1.003s`，请求日志记录 `first_token_ms=985 total_ms=3803`；同库旧日志对照仍为 #15 `first_token_ms=0`，确认只影响新请求且口径正确
  - 2026-11-11 发版 v0.1.9：Release Action 42946034821 成功（1m47s）；Release 正文正确提取首 token 修复说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（28,001,280 bytes），本地 SHA256 `4addf370…c504b7` 与 checksums.txt 一致；启动显示 version=v0.1.9，`/healthz` 返回 200
  - 2026-11-11 交付 T17 验收标准满足：两条流式路径首 token 均从发起上游请求开始计时；SSE 零缓冲与首 chunk 后不重试红线未改变；无数据库 schema 变更

### T18: Responses 原生上游透传与渠道协议能力（P1，用户指令）

- 目标：v1 协议范围定为上游/下游均支持 OpenAI Chat Completions 与 OpenAI Responses 两种协议；渠道模型新增 Responses 能力声明，`/v1/responses` 请求对声明该能力的渠道走原生透传（含 SSE 事件流），未声明渠道回落现有 Responses→Chat Completions 桥接；同协议链路保真，跨协议链路语义损失显式化
- 动机：Responses→Chat 桥接无法表达 reasoning items、内置工具与 `previous_response_id` 服务端状态；OpenAI / DeepSeek / Kimi / 智谱 / OpenRouter 等渠道已原生支持 Responses，透传可保真（调研见 docs/协议生态调研-ChatCompletions与Responses.md）
- 验收标准：
  1. WHEN 管理台创建/编辑渠道并声明 Responses 能力 THEN 字段经 AutoMigrate 增量落库、默认关闭，接口不回显上游 key 明文；
  2. WHEN `/v1/responses` 请求路由到声明 Responses 能力的渠道 THEN 非流式与 SSE 仅改写鉴权、base_url 与模型映射，请求/响应体原样转发，`store`/`previous_response_id` 等有状态字段保真；
  3. WHEN Responses 原生上游 SSE 不发送 `data: [DONE]` THEN flush 循环靠连接 EOF 正常终止；首个事件前复用重试/切换，首个事件写给客户端后禁止重试；first_token_ms 沿用 T17 统一口径；
  4. WHEN `/v1/responses` 请求携带 `previous_response_id`、background 等有状态字段且候选渠道均无 Responses 能力 THEN 显式返回可诊断 400，不静默丢弃；
  5. 回归矩阵 2×2 公共 HTTP 边界测试：CC→CC 透传、R→R 透传（非流式/SSE/工具）、R→CC 桥接（非流式/SSE/工具）全绿；CC→R 转换不适用需在测试注释与 README 说明理由；
  6. usage 归一化：CC（prompt/completion）与 Responses（input/output + cached/reasoning 明细）两套口径统一入请求日志；错误分类器识别 Responses 原生错误形状（含流式 `response.failed`）；
  7. `go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过；README 协议矩阵与支持差异同步更新。
- 边界：不做 Anthropic `/v1/messages`（T13 已延后）；不做 `/v1/responses/{id}` 查询/cancel 子路径与 background 模式；不做 CC→R 请求转换（具备 Responses 能力的渠道均支持 CC，如遇仅 Responses 上游再立卡）；不做 Anthropic/Gemini 原生上游适配；调度内核保持协议无关
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 用户确认协议范围决策（决策与调研于 2026-11-10 完成，因并发会话还原文档延迟建卡）：上游/下游均为 CC+Responses、同协议透传优先、R→CC 桥接兜底、Anthropic Messages 延后（T13）
  - 2026-11-11 执行 用户确认开始 T18；先审读 channel/relay/admin/web 当前链路与测试，随后按公共 HTTP 边界建立协议能力与透传红测，不启动 T19
  - 2026-11-11 执行 红测先复现原生 Responses 请求被旧桥接逻辑拒绝 / 错误事件无法切换；随后新增渠道 `supports_responses` 字段（AutoMigrate，默认关闭）、管理台创建/编辑与列表徽标、Responses 能力候选优先排序、R→R `/responses` 原生透传、chat-only 渠道 R→CC 桥接兜底
  - 2026-11-11 执行 原生 Responses SSE 改为按完整 event 分组读取：首事件前识别 `response.failed` 并复用 retry/failover，首事件后透传不重试；支持无 `data: [DONE]`、以 EOF 结束；CC/Responses usage 统一写 prompt/completion/total/cached/reasoning；状态字段无原生候选时显式 400 `responses_native_required`
  - 2026-11-11 验证 公共 HTTP 边界测试覆盖 CC→CC 透传、R→R 非流式/SSE/function tool/状态字段保真、R→R 首事件失败切换、R→CC 既有桥接、状态字段全桥接 400、CC/Responses usage 归一化与请求日志；`go test -p 1 ./... -count=1`、`go vet ./...`、`npm --prefix web run build` 通过
  - 2026-11-11 验证 使用安装库 SQLite 快照 + 当前源码构建（18910 隔离实例）真实调用 DeepSeek Responses `deepseek-flash`：R→R SSE 返回 200，保留 `response.created` / `response.reasoning_text.delta` / `response.completed` 与 reasoning item，无 `[DONE]`，最终文本 `pong`；请求日志记录 `first_token_ms=170 total_ms=802 tokens=34/35/69 cached=0 reasoning=32`
  - 2026-11-11 交付 `make build` 通过并产出当前源码单二进制；T18 验收标准满足，未实现 Anthropic `/v1/messages`、Responses 子路径/background、CC→R 转换或 Anthropic/Gemini 原生上游
  - 2026-11-11 发版 v0.1.10：Release Action 42971414495 成功（2m24s）；Release 正文正确提取 Responses 原生透传说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（28,026,880 bytes），本地 SHA256 `31dab2bf…ccdfb` 与 checksums.txt 一致；启动显示 version=v0.1.10，`/healthz` 返回 200，未带令牌访问 `/v1/responses` 返回 401

### T19: 系统设置与运行时配置热更新（P1）

- 目标：把可安全运行时调整的网关业务配置从 YAML 引导值升级为 SQLite 唯一事实源，并提供管理台系统设置页；relay、重试、cooldown、日志清理与健康探测读取最新配置且无需重启
- 验收标准：
  1. WHEN 首次升级启动 THEN 安全运行时设置由 `evergate.yml` 初始化入库；`master_key`、`session_secret`、端口与 DB 路径仍仅保留在 YAML，不进入管理台 API/UI；
  2. WHEN 管理员修改默认超时、重试预算、cooldown、日志保留天数、健康探测间隔 THEN 保存后新请求与新后台任务生效，无需重启进程；
  3. WHEN 提交非法配置（超时 <=0、重试预算为 0、保留天数 <1、探测间隔过短等）THEN 返回 400 且不产生部分更新；
  4. 设置读写仅限管理台会话鉴权；并发读采用安全快照/缓存，设置更新与请求路径无数据竞争；
  5. 管理台提供系统设置页，展示当前值、默认值来源、保存结果与校验错误；README 同步说明 YAML 与 DB 设置的边界；
  6. `go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过。
- 边界：不迁移安全密钥；不做多管理员/角色权限；不提供任意 YAML 注入；不做请求级覆盖（T24）；不把渠道模型与上游 key 移入设置页
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：作为健康探测、请求覆盖与运行时可观测配置的公共基础，先落地安全设置模型与热更新机制
  - 2026-11-11 执行 用户确认开始 T19；现状核查：YAML 中的 defaults/log_retention_days 仅启动时读取，relay 与 pool 持有静态副本，日志清理任务尚未存在，管理台也没有系统设置页。本卡将新增 SQLite 单例运行时设置、无锁快照读取、管理 API/UI、relay/pool 热应用与日志保留清理；不实现 T20 探测调度
  - 2026-11-11 执行 新增 `internal/settings`：`runtime_settings` SQLite 单例经 AutoMigrate 建表；首次启动用 YAML 的 defaults/log_retention_days 初始化并记录 source=evergate.yml，此后 SQLite 为唯一事实源；管理台只提交数值字段，source 与时间元数据由服务端维护，端口/base_path/DB路径/master_key/session_secret 不进入 API/UI
  - 2026-11-11 执行 设置管理器使用 atomic pointer 发布不可变快照、mutex 串行化保存、完整 Validate 后事务 Updates，非法配置不部分落库/不发布；后台任务可 Subscribe 变更通知。relay 每次请求读取最新快照，pool StateMachine 支持失败阈值与退避表热替换；新增请求日志保留清理任务，收到设置变更立即清理并每分钟兜底
  - 2026-11-11 执行 新增管理台系统设置页 `/settings`，分组编辑上游超时、重试预算、cooldown、日志保留与健康探测参数（健康探测参数供 T20 消费），展示来源、初始化/更新时间与保存结果；README/AGENTS 已同步 YAML 与 SQLite 边界
  - 2026-11-11 验证 settings/pool/admin/relay 测试覆盖：首次 YAML 种子与二次启动不覆盖、非法配置无部分更新、合法更新持久化并发布、订阅通知与取消订阅、日志保留清理、状态机热配置、管理 API 鉴权/敏感字段不泄漏、relay 修改首 token 超时后无需重启即切换备渠道
  - 2026-11-11 验证 隔离实例（18912）完成真实启动/登录/改密流程：首次 GET 显示 source=evergate.yml 与 YAML 种子；PUT 后 source=database 且新值立即生效；重启后仍保持 database 新值。最终 make build 产物 + 无头 Chrome 实测设置页 13 个数值输入、保存 46 秒成功，1280×800 与 390×844 均无横向溢出
  - 2026-11-11 交付 `gofmt` 无 diff；`go test -p 1 ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 全部通过。`go test -race` 因本机缺少 gcc/CGO 编译器无法执行；并发安全由 atomic 快照 + mutex 保存路径与相关单元测试保障
  - 2026-11-12 交付 提交前在最终源码上复验：`gofmt -l .` 无输出、`go vet ./...` 通过、`go test -p 1 ./... -count=1` 全部通过、`make build` 重新生成 web/dist 与 Windows 二进制并通过；`.tmp-t19` 验证目录已按绝对路径核对后删除
  - 2026-11-12 发版 v0.1.11：Release Action 43138295622 成功（2m35s）；Release 正文正确提取系统设置热更新说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（28,078,080 bytes），本地 SHA256 `e86e3794…966c` 与 checksums.txt 一致；隔离启动显示 version=v0.1.11，`/healthz` 返回 200，未登录 GET/PUT `/admin/api/system/settings` 均返回 401

### T20: 渠道健康自动探测与状态徽标（P1）

- 目标：为启用渠道与凭据提供周期性轻量探测，将探测结果、实时 key 状态与请求失败统计合成渠道健康视图，并在 Dashboard / 渠道列表展示可解释徽标
- 验收标准：
  1. WHEN 启用渠道存在可用 key 且到达探测周期 THEN 后台任务使用该 key 发起轻量探测（默认 GET `{base_url}/models`，不发起推理请求），并发受限于设置；
  2. WHEN 探测成功 THEN half-open/cooldown key 按状态机规则恢复，渠道状态与最后探测时间/延迟入库；WHEN 探测失败 THEN 记录状态码、错误分类、摘要与下次探测时间，不直接删除渠道或明文输出 key；
  3. WHEN 上游不支持 models 探测或返回 404/405 THEN 标记 inconclusive 而不是 down，避免把“探测路径不支持”误判为渠道故障；
  4. WHEN 渠道停用、进程退出或修改探测间隔 THEN 探测任务停止/重建，优雅退出不泄漏 goroutine；
  5. Dashboard 与渠道列表显示 active/degraded/down/inconclusive、可用 key 数、最近探测时间与错误摘要；异常渠道排前并提供手动测试入口；
  6. 后台任务使用可控时钟测试：成功恢复、失败保持 cooldown、inconclusive 不改变调度状态、停用渠道不探测、并发上限生效；`go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过。
- 边界：不调用 chat/completions 或 responses 产生推理费用；不引入 Redis/消息队列/外部监控系统；不做按模型健康分片；不做自动删除渠道
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：当前仅有实时请求驱动的 key 状态与手动连通测试，缺少任务书要求的周期探测与健康徽标闭环
  - 2026-11-12 执行 用户确认继续 T20；现状核查：T19 已提供探测间隔/并发/超时的 SQLite 热更新快照，pool 已有 cooldown→half_open 状态机，管理台已有手动 GET `/models` 测试但不更新 key 状态、不落健康结果。本卡将复用 pool.Acquire 作为探测凭据入口，新增可控 HTTP Prober 与周期调度器，并把手动测试改为同一公共探测路径
  - 2026-11-12 执行 新增 `internal/health/probe.go/status.go/scheduler.go`：探测固定 GET `{base_url}/models`，复用 `pool.Acquire` 选择 active/half-open key；成功恢复 key，失败复用 retry 分类并落 key 统计，404/405 返回 inconclusive 且仅释放 half-open 名额；Channel 新增最近探测结果/状态码/延迟/错误分类/摘要与下次探测时间字段
  - 2026-11-12 执行 周期调度器启动即探测，每轮读取 T19 设置快照并发受 `health_probe_concurrency` 限制；设置变更会废弃旧 timer 并按新间隔重建，停用渠道不进入查询队列，stop 取消上下文并等待 goroutine 退出。管理台手动测试复用同一 Prober，渠道列表/Dashboard 返回并展示健康徽标、可用 key 数、最近探测与错误摘要且异常优先排序
  - 2026-11-12 执行 relay 四条请求路径（Chat 非流/流、Responses 原生/桥接流）统一通过公共 helper 上报 key 状态并持久化 success_count/fail_count/last_error/last_used_at，健康视图可结合实时状态与请求失败统计
  - 2026-11-12 验证 health/admin/relay/pool 覆盖：探测成功恢复 cooldown、失败保持 cooldown、404/405 inconclusive 且释放 half-open 名额、停用渠道不探测、并发上限生效、设置变更重建等待间隔；管理 API 手动探测持久化健康视图且不泄漏 key；relay 成功/失败/不可重试统计持久化且不误惩罚无辜 key
  - 2026-11-12 验证 隔离实例（18922 + 假上游 18921）：探测间隔热更新为 10s 后自动请求 `/models` 且实测两次间隔 10.011s；API 返回 status=active、available_key_count=1、last_probe_result=success、HTTP 200、延迟 2ms 与 next_probe_at；手动测试与自动探测同路径。无头 Chrome 登录后渠道页/Dashboard 均显示 health-upstream、正常、1/1、最近探测与摘要，1280×800 横向溢出均为 0
  - 2026-11-12 交付 最终源码复验：`gofmt -l` 无输出、`go test -p 1 ./... -count=1` 全部通过、`go vet ./...` 通过、`npm --prefix web run build` 与 `make build` 成功。`go test -race` 仍受本机缺少 gcc/CGO 限制未执行；调度器停止路径由 context cancel + WaitGroup 测试/实现保障
  - 2026-11-12 发版 v0.1.12：Release Action 43166116456 成功（2m22s）；Release 正文正确提取渠道健康探测说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（28,129,280 bytes），本地 SHA256 `ec8d08f5…ca0e` 与 checksums.txt 一致；隔离启动显示 version=v0.1.12，`/healthz` 返回 200，未登录 GET channels/settings 均返回 401
  - 2026-11-12 交付 开源 README 复审：移除尚未实现的 `/metrics` 已提供表述与未实现的用量 CRUD 描述，明确 Prometheus 在路线图；补充初始 `admin/admin123` 仅用于本机初始化且禁止未加访问控制直接公网暴露；将“渠道健康”独立章节压缩为功能亮点与自动出站探测提示。README/示例配置敏感信息扫描无真实代理、私有路径或密钥；当前本机 bootstrap 密钥经 Git 历史精确检索未泄漏

### T21: 下游令牌额度、有效期、模型白名单与 IP 白名单强制（P1）

- 目标：把 tokens 表中已有但未强制的治理字段接入两条公共请求链路，保证令牌在请求前授权、成功后按实际 token 用量原子扣减，并让管理台清晰展示额度状态
- 验收标准：
  1. WHEN 令牌 disabled、已过期、请求模型不在 `allowed_models`（空列表表示全部允许）或客户端 IP 不匹配 `allowed_ips`（支持精确 IP 与 CIDR）THEN `/v1/chat/completions` 与 `/v1/responses` 均在调用上游前拒绝，并返回统一 OpenAI 错误体与明确 code；
  2. WHEN quota=0 THEN 不限额度；WHEN 请求前剩余额度 <=0 THEN 返回 quota_exceeded；WHEN 成功响应携带 usage THEN 按输入+输出 token 数原子累加 `used_quota`，并发请求不得超额；
  3. WHEN 上游失败、客户端断开或响应无 usage THEN 不扣减额度，且日志可见 usage 缺失原因；两条协议的非流式与流式 usage 提取（含最终 usage chunk / response.completed）均有公共 HTTP 边界测试；
  4. WHEN 配额被扣减 THEN 管理台令牌列表显示额度、已用、剩余、超限状态，额度变化与请求日志可对账；
  5. 白名单/额度错误进入请求日志（不泄漏 token 明文），并复用现有统一错误格式；`go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过。
- 边界：quota 单位定义为 token 数，不做货币余额/充值/多租户计费；不做分组路由；不实现每分钟速率限制（可后续独立卡）；不因额度未知而猜测扣减
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：字段与管理端编辑已存在，但公共链路尚未强制，存在越权与超额风险，优先于用量报表实现
  - 2026-11-12 执行 用户确认继续 T21；现状核查：TokenAuth 仅检查 enabled/expired，allowed_models/allowed_ips/quota 未强制；请求日志已有 usage 字段但无缺因说明，令牌 UI 仅能创建名称。实现采用公共边界：TokenAuth 负责身份/停用/过期/IP，relay 解析 model 后负责模型白名单与额度预检，Chat/Responses 共用 finishRequest 结算与日志
  - 2026-11-12 执行 TokenAuth 查询改为按 hash 获取完整 token 快照，disabled/expired/ip_not_allowed 均返回 OpenAI 错误体并入请求日志；IP 白名单支持精确 IP 与 IPv4/IPv6 CIDR。server 关闭默认受信代理，IP 判断使用 TCP 直连地址，避免伪造 X-Forwarded-For 绕过
  - 2026-11-12 执行 relay 在上游调用前强制 allowed_models 与 quota 预检；成功响应按 prompt+completion tokens 条件原子累加 used_quota，并发请求不得越过 quota；上游失败/无 usage/客户端断开不扣减并记录 usage_missing、usage_zero、client_disconnected_no_usage_charge 等原因
  - 2026-11-12 执行 管理台 token API 返回 remaining/status/allowed_ips 并校验负额度与非法 IP/CIDR；UI 新建/编辑支持额度、过期时间、模型白名单与 IP 白名单，列表显示剩余额度、超限/过期/停用状态
  - 2026-11-12 验证 单元/HTTP 边界测试覆盖：disabled/expired/IP 精确与 CIDR、伪造 X-Forwarded-For 不受信、两条协议模型白名单均不上游、额度预检 429、Chat 非流式与流式 final usage、Responses 桥接非流式与原生 response.completed、无 usage/上游失败/客户端断开不扣减、24 并发请求条件扣减后 used_quota 恰为 100
  - 2026-11-12 验证 隔离实例（18932 + 假上游 18931）：quota=10 且模型/IP 白名单生效；denied Chat/Responses 均 403 model_not_allowed 且上游推理次数 0，Allowed Chat/Responses 各消耗 5 tokens 后 token 状态 exhausted/剩余 0，第 3 次请求 429 insufficient_quota；日志 5 条可与两次 403、两次成功 usage、一次 429 对账。无头 Chrome 令牌页显示 governed-token、额度用尽、10/10、剩余 0、allowed-model 与 127.0.0.1，1280×800 横向溢出 0
  - 2026-11-12 交付 最终源码复验：`gofmt -l` 无输出、`go test -p 1 ./... -count=1` 全部通过、`go vet ./...` 通过、`npm --prefix web run build` 与 `make build` 成功。`go test -race` 仍受本机缺少 gcc/CGO 限制未执行；额度并发安全由 SQLite 条件原子 UPDATE 与并发 HTTP 测试保障
  - 2026-11-12 发版 v0.1.13：Release Action 43178170787 成功（2m40s）；Release 正文正确提取令牌治理说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（28,147,712 bytes），本地 SHA256 `1de791c4…5f4c` 与 checksums.txt 一致；隔离启动显示 version=v0.1.13，`/healthz` 返回 200，无效下游 token 调用 Chat 与未登录访问管理台均返回 401

### T22: 用量成本统计与每日聚合报表（P1）

- 目标：基于请求日志与可配置模型单价生成渠道/模型/令牌维度的每日用量与成本报表，补齐 `usage_daily` 聚合链路，并在管理台提供可筛选统计页
- 验收标准：
  1. 支持全局默认单价与渠道+模型覆盖单价（输入/输出每百万 token，固定小数精度存储），管理台可维护；单价变更不追溯重算历史请求；
  2. WHEN 成功请求记录 usage THEN 按命中渠道与上游模型计算本请求 cost 并写入 request_logs；无单价或无 usage 时明确显示“未计价/未知”，不伪造 0 成本结论；
  3. 后台按 UTC 日聚合 requests/success/tokens/cost 到 `usage_daily`，聚合幂等，启动恢复与优雅停机不重复计数、不丢当日数据；
  4. 管理台统计页支持日期范围（默认近 7 天）、渠道、模型、令牌筛选，展示总请求/成功率/token/成本、日趋势、Top 渠道/模型/令牌；查询超过保留窗口返回 400；
  5. 报表数据可用 SQLite/管理台 API 对账；历史无单价请求保持原值并说明统计口径；`go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过。
- 边界：不做 ClickHouse/外部 OLAP；不引入重量级图表库（优先 CSS/SVG 轻量图表）；不做预算自动降级与告警；不支持多币种账单
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：`usage_daily` 表已预留但未聚合，request_logs 已有 usage/cost 字段，需先由 T18 统一协议 usage，再落地计价与报表
  - 2026-11-12 执行 用户确认继续 T22；现状核查：RequestLog 已有 usage/cost 但成本始终 0，usage_daily 未写入，管理台无统计/价格维护。设计新增 `internal/usage` 公共域：ModelPrice 价格快照与实时计价、request_logs cost_status/cost_micros、usage_daily 幂等按日重建、管理台报表查询与价格维护共用同一口径
  - 2026-11-12 执行 新增 `model_prices`（全局 channel=0/model="" + 渠道+模型覆盖，micro-USD/百万 token）、RequestLog.cost_micros/cost_status、UsageDaily.cost_micros/priced/unpriced；usage.Manager 原子价格快照，CRUD 后立即重载
  - 2026-11-12 执行 relay 在请求完成时按当时渠道+上游模型价格快照计算 prompt+completion 成本；成功无 usage 标记 no_usage，有 usage 无价格标记 unpriced。后台聚合启动全量重建、每分钟刷今天/昨天、停机再刷今天；每日事务内先删后插保证幂等
  - 2026-11-12 执行 新增管理 API：usage report、价格列表/默认价/覆盖 upsert/删除；报表按保留窗口校验日期，返回总览、日趋势、Top 渠道/模型/令牌；管理台新增 `/usage` 页面与侧栏入口
  - 2026-11-12 验证 usage/relay/admin 测试覆盖：默认价/覆盖价优先级、价格变更不追溯历史、成功 usage 计价、无 usage/unpriced 标记、日聚合先删后插两次结果一致、报表筛选/Top/未计价数量、保留窗口 400、价格 CRUD 与负数/不存在渠道校验
  - 2026-11-12 验证 隔离实例（18942 + 假上游 18941）：全局价 $1/$2 与渠道模型覆盖 $3/$4；两次请求各 10 输入+5 输出 token，request_logs 均 cost_status=priced、cost_micros=50。停机重启后报表返回 requests=2、success=2、tokens 20/10、cost_micros=100、priced_requests=2，Top 渠道/模型/令牌与日志对账一致
  - 2026-11-12 验证 无头 Chrome `/usage`：显示近 7 天筛选、请求 2、成功率 100%、Token 30、成本 $0.0001、日趋势与三组 Top、全局价/覆盖价维护；1440×1000 与 390×844 横向溢出均为 0
  - 2026-11-12 交付 最终源码复验：`gofmt -l` 无输出、`go test -p 1 ./... -count=1` 全部通过、`go vet ./...` 通过、`npm --prefix web run build` 与 `make build` 成功。`go test -race` 仍受本机缺少 gcc/CGO 限制未执行；日聚合幂等由事务重建与重复执行测试保障
  - 2026-11-12 发版 v0.1.14：Release Action 43191116439 成功（1m58s）；Release 正文正确提取用量成本说明，三平台二进制 + checksums.txt 共 4 资产齐全。重新下载 Windows exe（28,231,680 bytes），本地 SHA256 `a4fa6c9f…dc92` 与 checksums.txt 一致；隔离启动显示 version=v0.1.14，`/healthz` 返回 200，未登录访问 usage report/prices 均返回 401

### T23: X-Session-Id 会话亲和调度（P1）

- 目标：为携带 `X-Session-Id` 的请求提供 TTL 粘性调度，优先复用同 token+模型最近成功的渠道与 key，提升上游缓存命中与会话稳定性；故障时仍无感切换
- 验收标准：
  1. WHEN 相同启用 token、相同模型与相同非空 `X-Session-Id` 在 TTL 内再次请求，且记录的渠道/key 仍可用 THEN 路由优先命中该渠道/key；
  2. WHEN 粘性渠道/key cooldown、disabled、渠道删除或请求失败 THEN 按现有路由/重试逻辑切换，成功后更新亲和记录；
  3. WHEN 未携带 header、session 为空/超长、TTL 过期或容量满 THEN 退回普通调度；LRU 清理有界且不拖垮请求路径；
  4. 亲和键绑定 token/model，不同下游令牌即使传相同 session ID 也不能复用记录；记录只保存哈希后的 session 指纹，不保存原文；
  5. 并发相同 session 请求测试不出现锁竞争退化；进程重启后允许亲和丢失并自然重建；`go test ./... -count=1`、`go vet ./...`、`make build` 通过。
- 边界：第一版仅进程内存表，不持久化到 SQLite，不引入 Redis；不做跨实例亲和；不保证流式请求中途重试；T24 请求级覆盖可覆盖亲和策略但不绕过健康状态
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：当前路由每次按优先级/权重随机选择，缺少任务书要求的会话亲和能力
  - 2026-11-12 执行 开始 T23：新建进程内 `internal/affinity`（SHA-256 指纹 + TTL 10m + LRU 4096），
    relay 在最高优先级组内提升粘性渠道并为该渠道优先取粘性 key；成功 upsert、失败 forget；
    同时为 router 加权随机补互斥，消除并发请求下的共享 Rand 数据竞争
  - 2026-11-12 验证 `go test -p 1 ./... -count=1` 全包通过；`go vet ./...` 通过；`make build` 成功产出
    `bin/evergate.exe`。新增测试：affinity 单测（token/model 隔离、TTL 过期重建、LRU 淘汰、超长/空 session、
    仅存指纹、ForgetIfMatches、32 goroutine 并发读写）+ relay 公共 HTTP 边界（CC/Responses 粘渠道与粘 key、
    不同 token 隔离、粘性渠道 500 切换并改写亲和、粘性 key 冷却回退同渠道其他 key、非法 header 不写表、
    TTL 过期、4×5 并发同 session 全部 200）。未运行 `-race`：本机无 gcc/CGO，不主张 race 通过
  - 2026-11-12 交付 提交 486fec0、tag v0.1.15；Release Action 43193778665 成功（1m56s），正文取自
    CHANGELOG，`checksums.txt` + 三平台二进制共 4 个 assets 齐全；Windows 产物 SHA256
    `A29758FAB8569CA978217CB778A5B44BE1AE73BFB382F32322989BD8A0A82D81` 与 checksums 一致
  - 2026-11-12 冒烟 下载的 Windows 二进制在隔离端口 18915 启动：日志 version=v0.1.15、`/healthz` 200、
    `/admin/api/auth/me` 401、`/v1/models` 401；进程已停止，未占用 8080

### T24: 请求级路由与重试配置覆盖（P1）

- 目标：参考 Portkey config-driven gateway 思想，为单请求提供严格白名单化的 `X-EverGate-Config` JSON 覆盖，让客户端/管理台可以在不修改全局配置的情况下控制超时、重试预算与候选渠道范围
- 验收标准：
  1. WHEN 请求携带合法 `X-EverGate-Config` THEN 仅允许覆盖白名单字段（connect/first_token/overall 超时、retry channels/keys/total、候选 channel id 列表、session 亲和开关），并应用于本次 CC/Responses 调度；
  2. WHEN JSON 含未知字段、类型错误、channel 不存在/不可用、数值越界或试图覆盖鉴权/密钥/模型白名单 THEN 返回 400 并指出字段路径；合法值必须 clamp 到安全上下限；
  3. 覆盖不能绕过 TokenAuth、令牌模型白名单、额度、IP 限制或上游 key 加密；无效候选全部耗尽仍返回统一 503；
  4. request_logs 记录脱敏后的有效覆盖摘要（如超时/渠道范围/亲和开关），不保存原始 header 中的无关大 JSON，日志大小有上限；
  5. CC 与 Responses 公共 HTTP 边界测试覆盖无 header、合法覆盖、非法覆盖、候选耗尽、与 T21 权限约束叠加；`go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过。
- 边界：不做请求配置持久化/命名配置集；不做 provider hooks/插件系统；不允许请求指定上游 key 明文或索引；不做 CC↔R 协议转换策略覆盖
- 状态：planned
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：将任务书 Portkey config 借鉴收敛为显式白名单，避免任意配置注入与权限绕行

### T25: 管理台 Playground 与命中链路调试（P1）

- 目标：在管理台内置模型调试页，支持选择 CC/Responses 两种协议、流式与非流式请求，并展示实际命中渠道、上游模型、首 token、总耗时与 retry trace，帮助用户验证渠道配置与协议兼容性
- 验收标准：
  1. Playground 仅管理台会话可用，不要求用户复制下游 token，不在响应/UI/日志中暴露任何上游或下游 key 明文；
  2. 支持模型选择、系统/用户消息编辑、stream 开关、协议切换（Chat Completions / Responses），默认请求大小与消息条数有上限；
  3. 流式输出逐段渲染且不整段等待；中断/失败时显示可读错误，非流式显示完整结果与 usage；
  4. 请求完成后展示命中渠道、上游模型、first_token_ms、total_ms、状态码、usage 与重试轨迹；请求进入 request_logs 并标记为 Playground 来源；
  5. 使用 mock 上游覆盖 CC 非流式/流式、Responses 非流式/流式、上游 500 切换、无效模型；前端构建与 1280×800 / 390×844 无横向破版；`go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过。
- 边界：不做多模态上传、对话持久化、Prompt 模板库、自动评测或模型广场；不绕过 T20/T21/T24 的调度与健康规则；第一版不做导出
- 状态：planned
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：补齐任务书“好用”核心管理台体验，同时作为 T18 协议矩阵的真实人工调试入口

### T26: Prometheus 指标与 OpenTelemetry 可选导出（P1）

- 目标：补齐网关运行指标与可选 OTLP 导出，提供本地 Prometheus 抓取端点与标准 OpenTelemetry 接入，帮助定位 QPS、渠道失败率、首 token 延迟与 cooldown 行为
- 验收标准：
  1. `GET /metrics` 输出 Prometheus 文本格式，仅允许 loopback 访问，非本机访问返回 403；指标包含 gateway requests/errors/active requests、每渠道模型状态计数、first_token_ms 直方图、上游错误分类计数、key cooldown/active 指标；
  2. OpenTelemetry 默认关闭；仅当标准 `OTEL_EXPORTER_OTLP_ENDPOINT` 等环境变量存在时启用，不新增 mandatory collector，不影响现有单二进制开箱即用；
  3. relay 关键路径产生请求/重试/上游调用 span 或 metric，标签不包含 key 明文、下游 token 明文、master key、完整请求体；低基数标签（channel/model/status/error kind）有防滥用上限；
  4. 关闭 OTel 时无外部网络请求、无导出 goroutine 泄漏；开启时导出失败只记录 warning，不影响请求成功；优雅停机 flush/退出；
  5. 单元/集成测试覆盖 loopback 限制、核心指标增量、SSE 首 token 指标、OTel 禁用默认值；README 修正 `/metrics` 实际暴露方式；新增依赖理由记录；`go test ./... -count=1`、`go vet ./...`、`make build` 通过。
- 边界：不引入 Jaeger/Collector/监控 SaaS 依赖；不导出请求/响应正文与密钥；不做日志采集管道；Prometheus 与 OTel 配置优先使用标准 env/本地设置，不做多租户隔离
- 状态：planned
- 进度（只追加新行，不修改旧行）：
  - 2026-11-11 计划 P1 建卡：README 当前描述了 `/metrics`，但实现尚未提供该端点；本卡同时补齐任务书 OTel 可选导出，避免继续文档超前

### T27: README 开源优化与仓库公开（P0，用户指令）

- 目标：README 面向公网重写（去个人环境信息、补双语定位/徽章/协议矩阵/安全模型/参与开发/SEO 关键词），添加 MIT LICENSE，GitHub 仓库转 public 并配置描述与 topics 标签
- 动机：项目已具备公开发布条件（v0.1.11，P1 推进中），开源可获取用户反馈与生态曝光
- 验收标准：
  1. 开源安全扫描通过：已跟踪文件无 API key/私钥/密码赋值/个人邮箱/本机路径/IP；`evergate.yml` 与 `data/` 确认被 gitignore；
  2. README 无个人机器状态（本机安装路径/版本列）与内部任务号耦合，含英文定位、徽章、协议矩阵、快速开始、安全模型、License 与关键词块；
  3. 仓库含 MIT LICENSE（版权人 guol9180），README 徽章与 License 小节链接有效；
  4. GitHub 仓库 visibility=public，描述与 ≤20 个 topics 配置成功；
  5. 变更已提交并推送，远程 main 与本地一致。
- 边界：不改代码与构建产物；不发新版本；不迁移仓库；GitHub 描述与 topics 一次配置，不做后续运营承诺
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-12 计划 用户指令确认：优化 README、补充 SEO 标签、将仓库开源
  - 2026-11-12 执行 安全扫描：git grep 密钥模式（sk-/ghp_/gho_/xox/PRIVATE KEY/长密钥赋值）与个人信息（C:\Users、D:\dev、邮箱、IP）均零命中；git ls-files 确认仅 evergate.yml.example 入库；README 重写 + 新增 MIT LICENSE
  - 2026-11-12 验证 README 复核：本机路径/当前机器状态/任务号耦合/邮箱/内网端口零残留，本地链接（LICENSE/CHANGELOG/DEVELOP_PLAN/AGENTS/docs×2）全部存在，英文定位与关键词块在位；提交 34ccc8d 推送 origin/main 成功
  - 2026-11-12 交付 gh repo view 确认 visibility=PUBLIC、双语描述生效、19 个 topics（ai-gateway/llm-gateway/openai-compatible/chat-completions/openai-responses/codex/api-gateway/load-balancer/failover/reverse-proxy/proxy/key-pool/sse/streaming/self-hosted/go/golang/sqlite/react）配置成功；T27 验收标准 1–5 全部满足

### T28: 渠道预设供应商模板（P2，用户指令）

- 目标：管理台新建渠道时提供预设供应商模板（OpenAI / DeepSeek / Kimi / 智谱 BigModel 常规 / BigModel Coding / Z.ai Coding 国际版等），选中后自动填充 Base URL 与推荐默认值，降低接入成本
- 动机：厂商基址知识（含智谱 Coding 套餐配错地址导致 503 的避坑）原维护在 README，转入产品内随渠道表单直接可用
- 验收标准：
  1. WHEN 新建渠道 THEN 可从预设供应商列表选择或选「自定义」；选中预设自动填充 Base URL（如 DeepSeek `https://api.deepseek.com`、Z.ai Coding `https://api.z.ai/api/coding/paas/v4`、BigModel Coding `https://open.bigmodel.cn/api/coding/paas/v4`、BigModel 常规 `https://open.bigmodel.cn/api/paas/v4`）；
  2. WHEN 选择「自定义」 THEN Base URL 手动填写，现有行为完全不变；
  3. 预设条目含显示名、Base URL 与说明（Coding 套餐必须显示避坑提示）；不包含任何 key 或账号信息；
  4. README 不再维护厂商基址清单，知识唯一来源为产品内预设模板；
  5. `go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过，`web/dist` 同步提交。
- 边界：预设清单为内置静态数据，不做在线模板更新与远程模板源；不做预设自动连通探测（沿用现有单渠道连通测试）；不做账号代注册 / OAuth；不涉及 Anthropic/Gemini 原生协议适配
- 状态：planned
- 进度（只追加新行，不修改旧行）：
  - 2026-11-12 计划 用户指令确认建卡并明确暂不执行；README「常见上游基址」小节同步删除，该知识待本卡落地时转入产品预设模板

### T29: npm 平台包发布流水线（P1，用户指令）

- 目标：tag 发版时除 GitHub Release 外同步发布 npm 平台包——主包 `evergate`（命令引导）+ `evergate-win32-x64` / `evergate-linux-amd64` / `evergate-darwin-arm64` 三个平台子包，`optionalDependencies` + `os`/`cpu` 字段按平台自动安装，registry 可被 npmmirror 同步
- 动机：GitHub Releases 在中国网络下载慢/失败；npm registry + 镜像可达，并为一键自动更新（T30）提供内建 integrity 的下载通道。前置实测：npm 包名 `evergate` 未占用、`registry.npmmirror.com` 连通
- 验收标准：
  1. WHEN 推送 `v*` tag THEN CI 自动产出并发布 1 主包 + 3 平台子包，版本号与 git tag 一致，无人工发布步骤；
  2. WHEN 在 Windows / Linux / macOS 分别执行 `npm i -g evergate` THEN 仅安装本平台子包，`evergate` 命令可启动且 `GET /healthz` 返回 200（os/cpu 生效）；
  3. 主包不含平台二进制（KB 级体积）；各包含 LICENSE 与 README 摘要；
  4. npm 账号开启 2FA；NPM_TOKEN 仅存 GitHub Secrets，不落仓库与日志；
  5. 发布后可经 `https://registry.npmmirror.com` 安装（或实测记录同步延迟为可接受）；
  6. `go test ./...`、`make build`、GitHub Release 三平台产物不受影响。
- 边界：不改动 GitHub Releases 现有产物与 changelog 事实源；不做 brew/scoop/winget 等其他包管理器；provenance 为可选加分项不设为验收；不宣传 npx 免安装为长期使用方式
- 状态：planned
- 进度（只追加新行，不修改旧行）：
  - 2026-11-12 计划 用户指令确认建卡；前置准备（npm 账号注册 + 2FA + 仓库 NPM_TOKEN secret）待开工时配置

### T30: 一键自动更新（P1，用户指令）

- 目标：管理台「一键更新」：从 npm registry（默认官方，可配镜像）拉取目标版本 tarball → 解包 → `dist.integrity`（sha512）校验 → 平台替换引擎（Windows 改名交换 / Unix 原子 rename）→ 重启编排（30s 优雅排空）→ 新版本健康自检；GitHub Releases + checksums.txt SHA256 作为 fallback 通道
- 动机：消除每次升级手动下载、手动替换文件的操作；npm 通道（T29）同时解决中国网络下载问题
- 验收标准：
  1. WHEN 管理台点击一键更新 THEN 下载→校验→替换→重启全流程无手工文件操作，UI 显示各阶段进度与结果；
  2. WHEN integrity 校验失败或替换任一步失败 THEN 中止更新，原二进制保持可用，返回可诊断错误；
  3. WHEN Windows 更新 THEN 使用改名交换法（运行中 exe 改名 `.old`，新文件就位，下次启动清理 `.old`），不依赖先停止程序的手工操作；WHEN 重启 THEN 沿用既有 30s 优雅排空；
  4. WHEN 新版本启动后 `/healthz` 通过 THEN 版本号变更为目标版本且旧版本备份保留；WHEN 新版本启动失败 THEN 备份仍在并提示手动回滚路径；
  5. WHEN 开发态（`go run`）或安装目录不可写 THEN 拒绝自更新并给出明确指引；
  6. WHEN npm 通道不可用 THEN 按配置回退 GitHub Releases 下载 + checksums.txt SHA256 校验；
  7. 公共 HTTP 边界回归测试（mock registry / Release）覆盖成功、校验失败、替换失败、重启编排；`go test ./... -count=1`、`go vet ./...`、`npm --prefix web run build`、`make build` 通过。
- 边界：不做强制/后台静默更新（必须用户点击确认）；不做自动回滚 watchdog（仅保留备份与回滚指引）；不做代码签名（已知局限：integrity 防传输损坏与镜像篡改，不防 registry 账号级攻击）；不做差量补丁；不引入 AGPL 实现（禁参考 MinIO selfupdate），替换引擎自研零新增依赖
- 状态：planned
- 进度（只追加新行，不修改旧行）：
  - 2026-11-12 计划 用户指令确认建卡；依赖 T29 平台包就绪后开工；评估结论：下载通道层与替换引擎层正交，npm 不消除 Windows 锁文件问题，替换/重启编排仍需自研

### T31: 三件套文档复审（P0，用户指令）

- 目标：按 init-project-docs 的分类与不冗余规则复审 README / AGENTS / DEVELOP_PLAN，删除 README 中逐条功能说明，恢复「README 项目熟悉、AGENTS 开发规范、DEVELOP_PLAN 计划与验收」的职责边界
- 验收标准：
  1. README 不再包含 `### 会话亲和` 小节，功能能力仅保留一行快照并指向 DEVELOP_PLAN / AGENTS；
  2. README 路线图不再复制 P1 任务清单，改为引用 DEVELOP_PLAN 里程碑；
  3. AGENTS「通用规范」四条与 skill 标准条目一致，目录约定不再声明尚未实现的上游适配器细节；
  4. 三份文档交叉引用有效，无同一信息的整段复制；
  5. 变更提交并推送，工作区干净。
- 边界：只改文档，不碰代码、构建产物与发版；不重写历史任务卡进度
- 状态：done
- 进度（只追加新行，不修改旧行）：
  - 2026-11-12 计划 用户指令确认复审三件套并删除 README 会话亲和小节
  - 2026-11-12 执行 删除 README `### 会话亲和` 详细小节（保留功能亮点一行）；路线图改为引用 DEVELOP_PLAN 里程碑；
    AGENTS 四条恢复 skill 标准文本，provider 目录标注规划中、Makefile 去掉过期 T7 注记
  - 2026-11-12 验证 grep 确认 README 无 `### 会话亲和`/P1 任务清单复制；三文档交叉链接（README↔AGENTS↔DEVELOP_PLAN）均存在
  - 2026-11-12 交付 提交 995a31b 推送 origin/main 成功，工作区干净；变更 3 文件（README/AGENTS/DEVELOP_PLAN），
    未改动代码与构建产物

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
- **done 卡不删**：完成的任务保留在任务清单中——进度即项目史

## 里程碑

| 里程碑 | 内容 | 完成判据 |
|---|---|---|
| P0（本次） | T0–T7 全部 done | 下方「验收清单」全部通过 |
| P1（当前阶段） | T10 Codex 接入；T11 启动打开管理台；T12 管理 UI 与更新检查；T18 Responses 原生上游透传与渠道协议能力；T19 系统设置；T20 健康探测；T21 令牌治理强制；T22 用量成本报表；T23 会话亲和；T24 请求级配置覆盖；T25 Playground；T26 Prometheus + OTel；T29 npm 平台包发布流水线；T30 一键自动更新 | T10–T12、T18–T26、T29–T30 全部按任务卡验收；T13 已延后。默认执行顺序：T18 → T19 → T20 → T21 → T22 → T23 → T24 → T25 → T26 → T29 → T30（T30 依赖 T29），依赖变化时先更新任务卡进度 |
| P2（后续，暂不建卡） | T28 渠道预设供应商模板（已建卡待排期）；Anthropic Messages 下游入口（T13，已延后）、Anthropic 原生上游适配、多用户角色、语义缓存、按预算模型降级、插件钩子 | T28 按上方任务卡验收；其余项开工前再拆任务卡 |

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

（技术背景见 [README.md](./README.md)，改码规则见 [AGENTS.md](./AGENTS.md)，勿在此重复）
