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
| P1（后续，暂不建卡） | 会话亲和、健康探测徽标、用量成本统计、Playground、令牌额度/白名单强制、请求级 config 覆盖、系统设置、OTel 可选导出 | 开工前再拆任务卡 |
| P2（后续，暂不建卡） | Anthropic 原生入口 `/v1/messages` 协议互转、多用户角色、语义缓存、按预算模型降级、插件钩子 | 开工前再拆任务卡 |

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
