# 参考项目对照评审（T4a 补课产出）

> 日期：2026-11-09。背景：T1–T4 实现时未按任务书第 2 章要求学习参考项目（如实记录于 DEVELOP_PLAN），本轮按用户指令补课：克隆精读 gpt-load（MIT）与 bifrost（Apache-2.0）源码、抓取 LiteLLM Router 文档，对照审查 T3/T4 调度内核与 relay 实现。仅学习设计，未复制任何代码。

## 一、学习了什么

| 来源 | 精读内容 | 关键收获 |
|---|---|---|
| gpt-load `internal/health/execution_judge.go`、`error_rules.go` | 错误判定引擎 | DispatchState（请求是否已发出）/ DownstreamCommitted（是否已写给客户端）/ ReplaySafety（重放是否安全）三维度判定；自定义错误规则层；空响应换候选不惩罚凭据 |
| gpt-load `internal/state/scheduling.go`、`quota_cooldown.go` | 调度账本 | 128 位加权公平调度账本；**quota 冷却与健康冷却分离**；**按模型的配额冷却表**；恢复需新样本严格晚于最近拒绝 |
| bifrost `core/failureclass.go` | 错误分类学（最有价值） | transient 服务器状态集 {500,502,503,504,**529**}；quotaMarkers（余额耗尽伪装成 400/403/429）；credentialTypes/Codes/Markers（凭据错误不限状态码）；**modelAccessCodes（model_not_found 换渠道不惩罚 key）**；regionMarkers |
| bifrost `core/providers/*/errors.go` | SSE 流内错误 | **错误事件骑在已提交的 200 上**；streamErrorStatus 按错误 type/code 推断等效 HTTP 状态（rate_limit_error→429、invalid_request_error→400、overloaded_error→529）；首 chunk 错误保持可重试 |
| LiteLLM Router 文档 | Router 语义 | allowed_fails（每分钟窗口计数）+ cooldown_time（默认 100s）；num_retries；retry_after 最小重试间隔；weighted pick 在 order 分层内进行 |

## 二、发现的差距与决策

### 已修复（本轮，含测试）

| # | 差距 | 来源依据 | 修复内容 |
|---|---|---|---|
| F1 | SSE 流内错误事件完全未解析：首 chunk 为 `data:{"error":...}` 时被当作成功首 token 透传且不再重试 | bifrost stream_error.go："events ride a committed 200"；首 chunk 失败应保持可重试 | stream.go 增加错误事件检测（含 `"error"` 字节快速路径），首 chunk 错误仍在可重试窗口内，按推断状态分类换候选；流中途错误事件解析语义记日志 |
| F2 | 404 model_not_found 一律透传——多渠道网关中"该渠道无此模型"是常态，其他渠道可能有 | bifrost modelAccessCodes（model_not_found/DeploymentNotFound/unknown_model→ModelAccess 换候选）；gpt-load ModelUnavailable | 新增 KindModelUnavailable：404/403/400 + 模型不可用标识 → 换渠道，不惩罚 key |
| F3 | 5xx/网络错误计入 key 失败且换 key——服务器过载换 key 无益，会烧光整个 key 池 | bifrost："rotating would burn every key on a condition none of them can avoid"；gpt-load UpstreamHostError→SkipGroup | **混合策略**：单请求内跳渠道（不试同渠道第二把 key）；跨请求仍计 key 失败（达阈值冷却，持续故障渠道最终 down——保留任务书 4.5③ 语义） |
| F4 | 凭据错误只认 401/403 状态码 | bifrost credentialMarkers（Bedrock 403、Google 400 带凭据文案） | Classify 增加 invalid_api_key / authentication_error / api key not valid 等标识识别，不限状态码 |
| F5 | 配额耗尽（insufficient_quota/credit balance）与限流不区分 | bifrost quotaMarkers/quotaCodes | 识别为限流类 + 默认 2 分钟冷却（换 key 可救——其他 key 可能有余额） |
| F6 | 非 429 的限流文案不识别 | bifrost rateLimitPatterns（Anthropic 400、Vertex 403 报限流） | 4xx + 限流文案（rate limit/throttled/requests per minute 等）→ 按限流处理 |
| F7 | Anthropic 529 overloaded_error 未识别 | bifrost transientServerStatusCodes | 纳入瞬时故障集合 |
| F8 | SSE 推断的 429 无 Retry-After 头，按阈值计数冷却太弱 | LiteLLM cooldown 语义 | SSE 推断限流 → 默认 60s 冷却 |

### 记录不修（P1+ 建议，按价值排序）

1. **按模型的配额冷却**（gpt-load quota_cooldown）：key 对模型 A 限流但模型 B 可用，当前 key 级冷却粒度偏粗；
2. **空响应换候选**（gpt-load content.empty_response）：200 但空内容值得一试下一候选，不计失败；
3. **区域封禁类**（bifrost regionMarkers）：unsupported countries 等，换 key/渠道有效；
4. **窗口式失败计数**（LiteLLM allowed_fails）：每分钟窗口计数 vs 当前连续计数，抗抖动更好；
5. **quota/健康冷却分离持久化**（gpt-load）：额度恢复时只解除配额冷却。

### 审视后维持现状（有依据）

- **平滑加权轮询**（nginx 风格）：gpt-load 用更复杂的 128 位公平调度账本（服务其多分组/断点恢复场景），个人规模 smooth WRR 精确且可测；
- **首 chunk 后永不重试**：与 gpt-load DownstreamCommitted 约束、bifrost 透传语义一致；
- **401 立即禁用**：静态 API key 场景正确（gpt-load 的 RecordCredentialFailure 阈值设计服务其 OAuth 可刷新凭据场景）；
- **连续失败阈值冷却**：任务书 4.5③ 明确要求，保持。

## 三、验证证据

- `internal/retry`：31 分类用例 + StreamErrorStatus 11 用例全绿
- `internal/relay`：12 集成用例全绿（新增：model_not_found 换渠道、瞬时错误跳 key + 阈值冷却混合语义、SSE 首 chunk 错误事件无感切换且 key 冷却）
- 全仓 gofmt / vet / 7 包 test / build 全绿
