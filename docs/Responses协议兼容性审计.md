# Responses 协议兼容性审计（T15）

- 审计日期：2026-11-10
- 触发问题：Codex 声明 `apply_patch` custom tool grammar 时，EverGate v0.1.6 返回 `Responses custom tool "apply_patch" grammar format is not supported`
- 许可证边界：仅阅读 gpt-load（MIT）、Bifrost（Apache-2.0）、LiteLLM 核心（MIT）、OpenAI Codex（Apache-2.0）与 OpenAI 官方文档；未阅读或复制 new-api / OmniRoute 代码。

## 结论

1. **Chat Completions 上游不能原生强制执行 Responses grammar**。Bifrost 的做法是把 grammar 放进降级后 function 参数的 `description`，让模型仍能看到约束；是否严格满足交给客户端工具解析器验证。
2. **custom tool 降级不能只处理声明，还必须完整处理往返**：请求里 custom tool → function tool；历史里 `custom_tool_call` → assistant `tool_calls`；`custom_tool_call_output` → tool message；响应里 function call → `custom_tool_call`。
3. **流式 custom tool 参数不能把 JSON 增量直接透出**。Bifrost 会先累积 function arguments，完成后解出原始文本，先发一次完整 `response.custom_tool_call_input.delta`，再发 `.done`，保证序列号递增且客户端可见完整输入。
4. **模型未按 schema 返回 `{"input":"..."}` 时不应丢弃结果**。Bifrost 在 arguments 没有 `input` 字段时把原始 arguments 作为 custom input 返回；这比直接报 conversion error 更符合“不丢模型输出”的网关原则。
5. **原生 Responses 透传是长期解**。在渠道不支持 Responses 或需要跨协议调度时，仍应保留 Chat Completions 枢轴与降级转换。

## 官方 schema 与 Codex 事实

OpenAI Responses Create Response 参考：

- custom tool：`{type:"custom", name, description?, format?}`
- grammar format：`{type:"grammar", syntax:"lark"|"regex", definition:string}`
- model output：`{type:"custom_tool_call", call_id, name, input}`
- client 回传：`{type:"custom_tool_call_output", call_id, output}`
- SSE 事件：`response.custom_tool_call_input.delta` / `response.custom_tool_call_input.done`

OpenAI Codex 0.167.0-alpha.3.2 源码事实：

- `codex-rs/core/src/tools/handlers/apply_patch_spec.rs` 构造名为 `apply_patch` 的 custom tool。
- `codex-rs/core/assets/tools/apply_patch.lark` 是完整 Lark grammar，约束 `*** Begin Patch` / hunk / `*** End Patch`。
- handler 只接受 `ToolPayload::Custom { input }`；也就是说 EverGate 最终必须给 Codex 返回 `custom_tool_call.input` 的原始 patch 文本，而不是 JSON wrapper。
- `ResponsesApiRequest` 可能发送 `text.verbosity`；本机真实捕获值为 `{"verbosity":"low"}`。

本机隔离捕获（假下游令牌，Authorization 未记录到文档）：

- Codex 0.167.0-alpha.3.2 对未知模型发送的顶层字段：`model/stream/input/tools/tool_choice/parallel_tool_calls/reasoning/store/include/prompt_cache_key/client_metadata`，均在 EverGate 白名单内。
- 常规工具声明包含 `function`、`namespace`、`web_search{external_web_access:false}`，当前 EverGate 均可处理。
- 未知模型走 fallback metadata，未带 `apply_patch`；用户真实会话使用模型目录中的 freeform patch 模型，因此发送 grammar custom tool 并触发本次错误。
- 使用 `model_catalog_json` 指向本机无凭据模型目录并选择 `deepseek-flash` 后，成功捕获完整 `apply_patch` Lark grammar 与 `text.verbosity`。

## 参考项目策略

| 项目 / 版本 | 关键行为 | 对 EverGate 的启示 |
|---|---|---|
| Bifrost `57218cb` / `core/providers/openai/customtools.go` | custom tool → 单参数 `input` function；grammar 写入参数 description；`additionalProperties:false`；历史 input 包装为 JSON，响应 arguments 解包；无 `input` 字段时原样返回；流式累积后合成 custom delta + done | 直接采用该成熟降级策略，并补齐流式 delta 与 raw fallback |
| gpt-load `243874f` / `internal/execution/bifrost` | Responses 有 native / converted 双路由；Codex mixed tools 的 best-effort 回归同时覆盖 function、namespace、web_search、custom；对会改变语义的 custom history 与不可保留 tool choice 给 critical semantic loss | EverGate 当前无渠道级 Responses 能力声明，先修转换；后续再拆“原生 Responses 透传”任务卡 |
| LiteLLM `433163a8` / `litellm_responses_transformation` | 使用 OpenAI SDK typed model 保留 custom tool / custom_tool_call / grammar；在 Chat↔Responses 双向转换中保留 custom 形状与流式 tool call | 字段必须按官方 schema 建 typed struct，不能靠零散 map 猜字段；未知能力显式诊断 |
| OpenAI Codex `a7a9d82` / `apply_patch_spec.rs` | apply_patch 是 grammar custom tool；工具执行器期待原始 patch 字符串 | EverGate 的 custom tool 输出必须恢复原始文本，且 grammar 不能被静默丢弃 |

## EverGate 支持矩阵（T15 修复后）

| 能力 | 状态 | 说明 |
|---|---|---|
| Codex 顶层请求字段 | 支持（T15 修复后） | fallback 模型捕获的 11 个字段原本已在白名单；已知模型另发送 `text.verbosity`，本卡补齐 |
| function tool | 支持 | 直转 Chat function |
| namespace tool | 支持（T15 修复语义） | 递归扁平化；输出恢复原 `namespace`，历史继续按扁平名回传 |
| Responses Lite `additional_tools` input item | 支持（T15 修复） | hoist 到 Chat 上游 `tools`，原 item 不进入 messages |
| `configuration_update` input item | 支持（T15 修复） | 不进入 Chat messages；最新 `reasoning.effort` 覆盖上游 `reasoning_effort` |
| `text.verbosity` | 支持（T15 修复） | 转为 Chat Completions `verbosity` |
| web_search `external_web_access:false` | 支持 | 不转发给 Chat 上游 |
| custom tool `format:text` | 支持 | 降级为单字段 function |
| custom tool grammar | 支持（T15 修复） | v0.1.6 曾显式 400；现降级为 function 参数描述并在响应恢复 custom call |
| custom tool choice | 支持 | 转为 Chat function choice |
| `custom_tool_call` 历史 | 支持 | 包装为 `{"input":...}` arguments |
| `custom_tool_call_output` 历史 | 支持 | 转 tool message |
| custom 非流式输出 | 支持 | function call 恢复为 custom_tool_call |
| custom 流式输出 | 支持（T15 修复） | 完整输入先发 `.delta` 再发 `.done` |
| async / deferred custom tool | 显式不支持 | 保持 400 |
| programmatic `allowed_callers` | 显式不支持 | 保持 400 |
| Responses WebSocket | 不支持 | Codex 配置必须 `supports_websockets=false` |

## 修复策略与风险

- 请求侧：接受 `format.type="grammar"` 且 `syntax` 只允许 `lark` / `regex`；把 `syntax + definition` 写入 `input` 参数 description，并保留原 tool description。
- 响应侧：优先解 `arguments.input`；没有该字段时返回原始 arguments，避免丢失模型输出。
- 流式侧：累积完 arguments 后发完整 `response.custom_tool_call_input.delta` 与 `.done`，再关闭 output item。
- 风险：Chat 上游只能“提示”grammar，不能硬约束。模型生成非法 patch 时，Codex `apply_patch` 解析器会返回错误并让模型重试；这是当前无原生 Responses 上游能力下的可接受降级。

## 真实 Codex 回归

2026-11-10 使用 Codex `0.167.0-alpha.3.2`、隔离 `CODEX_HOME`、EverGate 当前源码构建与 mock 上游完成真实链路：

1. Codex 发送 `apply_patch` Lark grammar 与 `text.verbosity=low`；
2. EverGate 转给 Chat 上游的 function schema 中保留完整 grammar 描述；
3. mock 上游返回 `apply_patch` function call；
4. EverGate 返回 `custom_tool_call.input` 原始 patch；
5. Codex 成功执行 `apply_patch` 并创建 `t15-real.txt`；
6. Codex 将 `custom_tool_call_output` 回传给 EverGate；
7. EverGate 转为 Chat tool message 并获得最终文本。

最终文件内容：

```text
real apply_patch ok
```
