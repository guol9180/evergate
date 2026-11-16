# P2 调研：按预算自动模型降级（T36 前置）

- 调研日期：2026-11-16
- 调研方法：抓取并核对 TensorZero 官方指南原文与 LICENSE、LiteLLM Router / Budget Routing 文档、gpt-load README；结论只基于已抓取原文，未抓到的部分显式标注。
- 目的：为 T36「按预算自动模型降级」确认业界语义、触发条件与观测模式，校准 EverGate 的设计。

## 结论先行

1. **预算触发降级是 EverGate 的组合设计，不是现成功能的复制**：TensorZero 的降级链由「错误」触发；LiteLLM 的预算由「超额」触发但语义是**拦截请求**（官方示例明确第二个请求直接失败），不是切换便宜模型。T36 的「令牌余额低于阈值 → 自动切低成本模型继续服务」= TensorZero 的有序链 + LiteLLM 的预算触发条件 + EverGate 自己的继续服务语义。
2. **双模型记录模式有业界先例**：LiteLLM 的 spend log 同时记录 model_group（请求组名）与实际服务的 member deployment——对应 T36 的「原始模型 / 实际命中模型」双字段，方向正确。
3. **降级链配置形态确认**：静态有序列表（TensorZero `fallback_variants = [...]` 顺序尝试），不做自动按价格排序——与 T36 卡片边界一致。

## 参考项目语义对比

| 项目 | 降级触发 | 降级目标 | 预算能力 | 对 T36 的启示 |
|---|---|---|---|---|
| TensorZero（Apache-2.0，LICENSE 已核实） | 错误（variant 失败） | 候选 variants 均匀采样后，fallback_variants 按序逐个尝试 | 有 usage/cost 追踪与自定义 rate limits，无预算触发降级 | 有序链结构、三层组合（provider 路由 / variant 重试 / variant 降级） |
| LiteLLM（MIT 核心） | 错误（fallbacks 按名字键控）；预算为独立拦截机制 | model_group 内其他 deployment（同模型不同供应商），不跨模型降级 | provider / model / tag 三种预算：budget_limit（USD）+ time_period（1d/30d/1mo…），超预算拒绝请求；多实例需 Redis | 预算触发条件、双记录（group + 实际 deployment）；也证明「预算自动降级」在 LiteLLM 并不存在 |
| gpt-load（MIT） | 渠道/凭据层 failover | 同模型其他渠道/key | — | README 无模型级降级链；T36 属自研增强 |
| EverGate T36 设计 | ① 令牌剩余额度 < 阈值（请求开始时快照）② 目标模型渠道耗尽/冷却 | 管理台配置的有序降级链（跨模型） | T21 令牌额度 + T22 价格快照/成本落库 | 组合上述两者，语义为「继续服务」而非「拦截」 |

## TensorZero 三层结构（指南原文要点）

Retries & Fallbacks 指南定义三层，可组合：

1. **Model Provider Routing**：同一模型下按序尝试多个 provider（示例：gpt-5-mini 先 OpenAI 再 Azure）；对 best-of-N 这类多推理变体，路由对每次推理独立生效。
2. **Variant Retries**：单 variant 失败后重试，截断指数退避 + 抖动（示例：最多重试 4 次、单次延迟上限 10s）。
3. **Variant Fallbacks**：候选 variants（均匀采样）全部失败后，fallback_variants 按序逐个尝试（示例：先 grok_4 再 gemini_2_5_flash）。

许可证核实：TensorZero LICENSE 原文为 Apache-2.0。但 AGENTS 允许复制清单目前仅 gpt-load / bifrost / LiteLLM 核心——**T36 只学设计不复制代码**，若未来要引用其实现需先修订允许清单。

## LiteLLM 预算路由（Budget Routing）实证

- 三种预算粒度：Provider（如 OpenAI $100/day）、Model（按 deployment base）、Tag（如 product:chat-bot）。
- 配置形态：`provider_budget_config: {openai: {budget_limit: 100.0, time_period: "1d"}}`；model 级用 `max_budget` + `budget_duration`（litellm_params 内）；tag 级用 `tag_budget_config`。
- 行为：官方示例明确「第一个请求成功，第二个请求因超预算**失败**」——预算是硬拦截，不切换模型。
- 多实例一致性依赖 Redis（EverGate 单机 SQLite 无此需求，T21 令牌额度扣减已解决）。
- 另一关键观察（来自 Router 文档）：请求日志保留 model_group 名称，同时记录实际服务的 member deployment——T36 的 original_model / final_model 双记录与此同构。

## 对 T36 实现设计的校准建议

1. **数据模型**：model → 有序候选列表存 DB、管理台热更新（走 runtime_settings 同款机制），不进 YAML。
2. **触发顺序**：先在同模型渠道/key 池内用完现有重试预算（T4 语义），确认渠道耗尽/冷却后才进入降级链——与 TensorZero「provider 路由穷尽后才 variant 降级」同构；预算阈值触发则在路由前判定（请求开始的额度快照）。
3. **白名单交互**：降级候选必须在令牌 allowed_models 白名单内，白名单外候选跳过；全被跳过 = 维持 503。执行顺序：TokenAuth → 令牌治理/白名单 → 降级链解析 → 路由。
4. **观测**：retry_trace 新增 reason ∈ {budget_threshold, channel_exhausted}；请求日志记录 model（实际命中）+ original_model（原始请求）；成本按实际模型价格快照（沿用 T22「不追溯重算」红线）。
5. **不引入的东西**（对照参考确认合理）：不做自动价格排序（LiteLLM/TensorZero 均为显式配置）、不做质量感知降级、不做跨请求预算重估（以请求开始快照为准，避免与 T21 实际 usage 扣减竞态）。

## 局限与未决项

- 无阻塞项：降级语义可全部用 mock 上游在公共 HTTP 边界覆盖，不依赖真实供应商。
- 唯一实现期待办：管理台降级链 UI 的字段命名与 T21/T22 现有页面对齐。
