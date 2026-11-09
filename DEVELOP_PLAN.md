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
- 状态：planned
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效

### T3: 调度内核（P0，纯逻辑先写测试）

- 目标：错误分类器 → cooldown 状态机 → key 池调度 → 路由器；表驱动单测覆盖任务书 4.5 节全部规则
- 验收标准：`go test ./internal/retry/... ./internal/pool/... ./internal/router/...` 全部通过；调度器分支覆盖率 ≥ 90%（对应验收清单 #10）
- 边界：不接 HTTP；不接真实上游
- 状态：planned
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效

### T4: relay 链路（P0）

- 目标：OpenAI 兼容上游适配器 + SSE 零缓冲透传 + 首 token 超时 + 重试预算串联；httptest mock 双渠道集成测试
- 验收标准：集成测试覆盖验收清单 #2–#7（非流式/流式首 token 前切换、首 token 后失败不重复、429 cooldown、401 禁用、渠道耗尽 503）
- 边界：不做管理 API；不做前端
- 状态：planned
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效

### T5: 管理 API（P0）

- 目标：auth / channels / keys / tokens / logs 的 REST + 下游令牌鉴权中间件 + 请求日志异步落库（缓冲 channel）
- 验收标准：登录登出可用；渠道与 key CRUD + 批量导入 + 连通测试可用；令牌 CRUD 可用；日志分页查询可用；任何接口不回显明文（对应验收清单 #9）
- 边界：不做用量统计报表（P1）；不做 Playground（P1）
- 状态：planned
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效

### T6: 前端管理台（P0）

- 目标：Vite 脚手架 → 登录（首登强制改密）→ Dashboard → 渠道管理（批量导 key、连通测试、启停）→ 令牌管理 → 请求日志（重试轨迹可视化）
- 验收标准：`npm run build` 成功；五个页面可用且与 T5 API 对接完成；日志详情可展示每次尝试的渠道/key/错误/耗时
- 边界：不做 Playground / 用量统计 / 系统设置页（P1）；第一版仅简体中文
- 状态：planned
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效

### T7: 嵌入与交付（P0）

- 目标：go:embed 嵌入前端产物 + Makefile 三平台交叉编译（windows/amd64、linux/amd64、darwin/arm64）
- 验收标准：`make build` 产出单二进制且 `./evergate` 启动后 `/v1/*` 与管理台同端口可用（对应验收清单 #1）；三平台编译通过；二进制 < 30MB
- 边界：不用 goreleaser 发布流水线（手写 build 脚本即可）
- 状态：planned
- 进度：
  - 2026-11-09 计划 任务卡确认，验收标准生效

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
