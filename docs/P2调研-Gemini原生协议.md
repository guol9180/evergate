# P2 调研：Gemini 原生协议打通（T38 前置）

- 调研日期：2026-11-16
- 调研方法：Google 官方 python-genai SDK（Apache-2.0）的 types.py / models.py 源码作为 schema 权威源；ai.google.dev 官方文档站在本网络不可达（curl 000，多路径重试），相关差异标注为「实现时实测」。
- 目的：为 T38 确认 Gemini 原生 REST 形态、请求/响应 schema、SSE 模式、G→CC 桥接映射与透传鉴权规则。

## 结论先行

1. **REST 形态与 CC/R 完全不同**：方法名后缀路径（`models/{model}:generateContent`、`:streamGenerateContent?alt=sse`），模型名内嵌在 URL 而非 body——路由与模型映射的改写点从 body 移到路径参数，这是 T38 最大的实现差异。
2. **role 命名与工具形态自成一体**：user/model（非 assistant）；函数调用是 functionCall part（name+args 对象，非 JSON 字符串），回传是 functionResponse part（非 tool role 消息）。
3. **usage 口径最细**：usageMetadata 除 prompt/candidates 外还有 thoughtsTokenCount、toolUsePromptTokenCount，官方注释明确 total = 四项之和——T22 成本计算需要把 Gemini 的 thought token 归入输出侧计价。

## 一、REST 端点（官方 SDK 源码证实）

```text
POST /v1beta/models/{model}:generateContent                 # 非流式
POST /v1beta/models/{model}:streamGenerateContent?alt=sse   # SSE 流式
```

SDK models.py 中路径拼接即此形态；`alt=sse` 为流式必需参数。**模型在 URL 中**：EverGate 模型映射需改写路径段而非 body.model；下游入口同理要从路径解析模型名（Gin 路由参数即可，注意模型名含 `/` 的别名要先归一化）。

## 二、请求 schema（types.py 权威摘录）

```jsonc
{
  "contents": [
    { "role": "user" | "model",
      "parts": [
        { "text": "..." },
        { "functionCall": { "name": "...", "args": {} } },
        { "functionResponse": { "name": "...", "response": {} } },
        { "inlineData": { "mimeType": "...", "data": "..." } }
      ] }
  ],
  "system_instruction": { "parts": [{ "text": "..." }] },
  "tools": [ { "functionDeclarations": [{ "name": "", "description": "", "parameters": {} }] } ],
  "tool_config": { "function_calling_config": { "mode": "AUTO|ANY|NONE" } },
  "generation_config": { "temperature": 0, "max_output_tokens": 0, "top_p": 0 },
  "safety_settings": [ { "category": "HARM_CATEGORY_...", "threshold": "BLOCK_..." } ]
}
```

注意点：

- `system_instruction` 是顶层字段（REST 层亦有 camelCase systemInstruction 别名），对应 Messages 的顶层 system、CC 的 system message。
- safetySettings 按 category×threshold 逐条配置，是 OpenAI 兼容层无法表达的典型字段（原生入口的核心价值之一）。
- `functionCall.args` 是**对象**（CC 的 arguments 是 JSON 字符串），桥接时要做 stringify/parse 往返。
- 新字段 partial_args（流式部分参数）已出现在 FunctionCall——透传天然兼容，桥接第一版不解析、聚合后再转发。

## 三、响应与 usage

```jsonc
{
  "candidates": [
    { "content": { "role": "model", "parts": [] },
      "finish_reason": "STOP|MAX_TOKENS|SAFETY|RECITATION|...",
      "index": 0 }
  ],
  "usage_metadata": {
    "prompt_token_count": 0,
    "candidates_token_count": 0,
    "thoughts_token_count": 0,
    "tool_use_prompt_token_count": 0,
    "total_token_count": 0
  },
  "model_version": "..."
}
```

官方注释明确：total = prompt + candidates + thoughts + tool_use 四项之和。对 T22 成本口径的直接影响：prompt_token_count 计输入，candidates + thoughts（必要时含 tool_use_prompt，按 Google 计费文档实测确认）计输出——实现卡需在价格快照换算处加 Gemini 特例并写测试。

## 四、SSE 模式

`alt=sse` 下响应为 `data: {GenerateContentResponse JSON}` 连续帧，无事件名行（区别于 Anthropic 的 event: 行）、无 [DONE]，流以连接关闭终止——与 T18「EOF 终止、不假设 [DONE]」规则一致。帧内即完整/增量的 GenerateContentResponse 结构；实现时以真实流式响应核对首帧是否含 usageMetadata（文档站不可达，列实测项）。

## 五、鉴权与透传

- Gemini API developer key：`x-goog-api-key: <key>` header（官方 SDK 形态）或 `?key=` query。
- **EverGate 规则：统一 x-goog-api-key header 注入，禁用 query 传 key**——避免 key 进入任何访问日志；透传时剥离开游同名 header。
- 渠道 Base URL：官方 `https://generativelanguage.googleapis.com` + `/v1beta/models/...` 路径拼接。

## 六、G→CC 桥接映射表（第一版）

| Gemini | Chat Completions | 说明 |
|---|---|---|
| contents[].role=user | messages[].role=user | 直传 |
| role=model | role=assistant | 命名映射 |
| system_instruction | system/developer message | 单条前置 |
| parts[].text | content 文本 | 拼接 |
| functionCall part | assistant tool_calls（args 需 stringify） | id 需生成并暂存映射 |
| functionResponse part | role:"tool" message（response 需序列化） | 与生成的 id 配对 |
| finish_reason: STOP/MAX_TOKENS/SAFETY | stop / length / content_filter | SAFETY 在 CC 无原生语义，映射为 content_filter |
| usage_metadata | prompt_tokens = prompt_token_count；completion_tokens = candidates + thoughts | 口径见上节 |
| safety_settings / generation_config.thinking_config | **不可桥接** | 显式 400 或显式降级并记录（与 T37 同规则） |

## 七、OpenAI 兼容层差异（证据受限，实现时补实测）

ai.google.dev 在当前网络不可达（curl 000，多路径重试），本报告未核对官方兼容层文档原文。基于官方 SDK schema 与公共知识的判断（**需实现时实测确认**）：兼容层支持 /v1/chat/completions 参数子集，systemInstruction、safetySettings、thinkingConfig、原生 functionResponse 回传形态等不可用或受限——这正是 T38 原生入口的存在价值。T38 开工时在可达网络环境抓取一次官方文档补录，或直接用真实 key 对比两种端点行为。

## 八、局限与待实测项汇总

1. ai.google.dev 文档与 OpenAI 兼容层差异——网络恢复后补录（不阻塞 schema 实现与 mock 测试）。
2. SSE 首帧是否携带 usageMetadata / partial_args 聚合边界——真实流式响应核对。
3. tool_use_prompt_token_count 的计费归属（输入 or 输出）——按 Google 计费文档实测后写死。
4. Vertex AI 明确不做（T38 边界），仅 Gemini API developer key 形态。
