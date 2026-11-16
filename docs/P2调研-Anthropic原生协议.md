# P2 调研：Anthropic 原生协议打通（T37 前置）

- 调研日期：2026-11-16
- 调研方法：以 Anthropic 官方 Python/TypeScript SDK 的 api.md、message_create_params.py、streaming/_messages.py 为 schema 权威源；DeepSeek / Kimi 官方 API 文档核对兼容层；Claude Code 官方文档（code.claude.com）核对网关接入要求；LiteLLM 文档补充客户端发现行为。
- 目的：为 T37 确认 Messages API 请求/SSE/鉴权细节、可用上游生态、Claude Code 接入要求与 M→CC 桥接的不可桥接清单。

## 结论先行

1. **三家上游可统一到一条拼接规则**：EverGate 渠道 Base URL + `/v1/messages` 可同时覆盖 Anthropic 官方（`https://api.anthropic.com`）、DeepSeek（`https://api.deepseek.com/anthropic`）、Kimi（`https://api.moonshot.ai/anthropic`）——与现有 CC 渠道「Base URL + /chat/completions」模式完全同构。
2. **SSE 事件流三家一致且与 CC/R 根本不同**：`message_start → (content_block_start → delta… → content_block_stop)… → message_delta → message_stop`，事件名 = data.type，**没有 [DONE] 终止帧**——flush 循环必须靠 EOF 终止（T18 已建立同款规则，可直接复用抽象）。
3. **Claude Code 接入可行**，但有两条硬性注意：模型命名需含 claude/anthropic（客户端只自动发现这类名字）；Anthropic 官方立场是不支持经网关路由到非 Claude 模型——EverGate 属第三方网关，跨模型路由的兼容性自担（这正是 M→CC 桥接的价值，也是风险面）。

## 一、官方 Messages API schema（SDK 权威源）

### 请求（POST /v1/messages）

必填三项：`model`、`max_tokens`（绝对上限，模型可提前停止）、`messages`（user/assistant 交替，上限 100,000 条）。主要可选：

- `system`：顶层字符串或 content block 数组（**不是** messages 内的 system role）。
- `tools: [{name, description, input_schema}]` + `tool_choice: {type: auto|any|tool}`。
- `thinking: {type, budget_tokens}`：budget_tokens ≥ 1,024，**thinking token 计入 max_tokens**。
- `temperature` / `top_p` / `stop_sequences` / `metadata` / `stream`。

### 内容块（messages 与响应共用）

`text`（可带 `cache_control`）、`tool_use`（id/name/input）、`tool_result`（tool_use_id/content）、`thinking`（含 signature）。`cache_control` 挂在 content block 上，用于 prompt caching 计费。

### 响应

`{id, type:"message", role:"assistant", content:[块…], model, stop_reason, stop_sequence, usage:{input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens}}`；stop_reason 含 end_turn / max_tokens / stop_sequence / tool_use 等。

### SSE 事件流（官方 SDK + DeepSeek/Kimi 文档三源一致）

```text
message_start
  → (content_block_start → content_block_delta* → content_block_stop)*   # delta: text_delta / input_json_delta / thinking_delta / signature_delta
  → message_delta（含最终 usage 与 stop_reason）
  → message_stop
# 穿插：ping 事件与 keep-alive 注释；错误可表现为流中 error 事件
# 终止：无 data: [DONE]；正常结束以 message_stop 事件 + 连接关闭
```

DeepSeek 文档补充两条对 relay 实现关键的事实：

- ping / keep-alive 可能出现在任意两个事件之间，必须原样转发（客户端 keep-alive）。
- **流开始后的错误可能直接断连而不发 Anthropic error 事件**——「EOF 且未见 message_stop」必须归类为流中断错误（EverGate 现有 SSE 中断语义可对齐）。

## 二、鉴权与请求头

- Anthropic 官方：`x-api-key: <key>` + `anthropic-version: <date>`（SDK 自动发送；本网络抓取 SDK `_client.py` 超时，版本默认值实现时以 SDK 常量实测为准，本报告不写死）。
- DeepSeek 兼容层：`x-api-key` **或** `Authorization: Bearer` 二选一，两者同给时 **Bearer 优先**（官方明示）。
- Kimi：同 Anthropic 形态（官方示例用 `Authorization: Bearer` + `anthropic-version`）。
- **EverGate M→M 透传的鉴权改写规则**：按渠道注入 `x-api-key`（三家通吃），剥离开游 `Authorization`，注入渠道 key。

## 三、可用上游生态与 Base URL 对齐

| 上游 | Base URL（EverGate 渠道填写） | 拼接后 | 备注 |
|---|---|---|---|
| Anthropic 官方 | `https://api.anthropic.com` | `/v1/messages` | 官方 |
| DeepSeek | `https://api.deepseek.com/anthropic` | `/v1/messages` | 官方文档：SDK base_url 填此值，SDK 自动追加 /v1/messages；直连也可用 /anthropic/messages |
| Kimi / Moonshot | `https://api.moonshot.ai/anthropic` | `/v1/messages` | 官方示例端点 /anthropic/v1/messages |

另有一个官方端点 `POST /v1/messages/count_tokens`（SDK 提供 messages.count_tokens）——M→M 透传需要一并路由（Claude Code 是否实际调用待接入后抓包确认，不影响透传实现）。

## 四、Claude Code 接入要求（官方文档实证）

- 环境变量：`ANTHROPIC_BASE_URL` 指向 EverGate（如 `http://localhost:8787`）；凭证按场景配 `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY`——EverGate 下游鉴权需同时接受这两种头（实现卡加验收）。
- 企业管控：managed settings 可设 `allowedProviders: ["customEndpoint"]` 锁定只允许走该网关。
- **命名发现**（LiteLLM 官方文档记录的实测观察）：Claude Code / Claude Desktop 只自动发现网关模型名中**含 claude 或 anthropic** 的模型——EverGate 模型映射的对外名应满足此约定，管理台提示需写入。
- 官方立场：Anthropic 不支持经网关路由 Claude Code 到非 Claude 模型——经 EverGate M→CC 桥接用非 Claude 模型属用户自担的兼容路径，文档需如实标注支持边界。

## 五、M→CC 桥接建议（第一版边界）

| Messages 字段 | 桥接策略 |
|---|---|
| `system`（顶层） | → CC system/developer message |
| `messages` user/assistant text | → 同角色 message，content 直传 |
| `tool_use` 块 | → assistant `tool_calls`（id/name/arguments） |
| `tool_result` 块 | → `role:"tool"` + `tool_call_id` |
| `stop_reason` | end_turn→stop、max_tokens→length、tool_use→tool_calls |
| `usage` | input_tokens→prompt_tokens、output_tokens→completion_tokens |
| `thinking` | **不可桥接（无 CC 等价物），显式 400** |
| `cache_control` | **不可桥接（剥离会改变 prompt caching 计费语义），显式 400** |

错误映射：Anthropic `error.type`（invalid_request_error / authentication_error / rate_limit_error / api_error / overloaded_error）→ EverGate 统一 OpenAI 错误体；overloaded_error 归入可重试/换渠道分类。

## 六、局限与实现期待实测项

- `anthropic-version` 默认值：本网络抓 SDK client 源码超时，实现时以官方 SDK 常量为准。
- Claude Code 是否调用 `/v1/messages/count_tokens`：接入后抓包确认（透传路径已覆盖，不构成风险）。
- 流中 error 事件各家差异：以官方 SDK + DeepSeek + Kimi 三方文档对齐的事件序为准，回归测试同时覆盖「流中 error 事件」与「断连无 error」两种失败形态。
