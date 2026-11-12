# 更新日志

发版描述自动取自本文件：工作流按 `## <tag>` 提取对应段落作为 GitHub Release 正文（T7b 约定）。

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
