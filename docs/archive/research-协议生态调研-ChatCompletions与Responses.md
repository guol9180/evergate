# 协议生态调研：Chat Completions vs Responses（对接核心决策输入）

- 调研日期：2026-11-10
- 调研方法：逐家抓取官方文档/README 核对支持矩阵与默认行为；OpenAI 官方 guide 页面（platform.openai.com / developers.openai.com）对脚本访问返回 403，官方定位改用 openai-python 官方 SDK README 原文佐证。
- 局限：无公开的各端点调用量份额数据，"市面在用"的判断基于**支持矩阵 + 官方默认值走向**，非流量统计。

> T15 补记（2026-11-10）：后续通过官方 Markdown 端点成功核对了 OpenAI Responses Create Response / streaming events schema，前文“官方 guide 脚本访问 403”只代表当时的 HTML 抓取路径，不代表官方文档不可用。该补记发生时项目仍保留三协议矩阵；随后同日决策记录已将 Anthropic Messages 延后，最新范围以下方决策记录为准。

## TL;DR

1. **Chat Completions 仍是唯一 100% 通用层**：所有被调研的厂商、网关、推理引擎、客户端全部支持；部分生态（Groq、DashScope compatible-mode、one-api）仍**只支持**它。
2. **Responses 是明确的增长方向**：OpenAI 官方 SDK 将其标注为 primary API；Vercel AI SDK 自 v5 起 OpenAI provider 默认走 Responses；OpenRouter 文档两协议并列且 Responses 章节在前；DeepSeek、Kimi、智谱在 2025–2026 间全部补齐。
3. **建议**：维持 **Chat Completions 作为内部枢轴协议（IR）与兼容基准**（即现状），**Responses 作为一等入口**，并在路线图中增加「渠道能力声明 + Responses 原生透传」：上游原生支持 Responses 的渠道（OpenAI、DeepSeek、Kimi、智谱、OpenRouter 等）直接透传保真，不支持的渠道再回落到现有的 Responses→Chat 转换（对齐 LiteLLM 的双向桥接模式）。

## 证据矩阵

| 厂商 / 项目 | Chat Completions | Responses | 关键证据 | 来源 |
|---|---|---|---|---|
| OpenAI 官方 | ✅（沿用支持） | ✅ 官方主 API | openai-python README："The primary API for interacting with OpenAI models is the Responses API"，chat.completions 被称为 previous standard | [openai-python README](https://github.com/openai/openai-python/blob/main/README.md) |
| Vercel AI SDK | ✅ `openai.chat()` | ✅ **默认**（自 v5） | "Since AI SDK 5, the OpenAI responses API is called by default"；"If your custom base URL only supports the Chat Completions API, create chat models with openai.chat instead" | [AI SDK OpenAI Provider](https://ai-sdk.dev/providers/ai-sdk-providers/openai) |
| OpenRouter | ✅ | ✅ | "OpenRouter supports two request shapes: the Responses API (POST /api/v1/responses) and the Chat Completions API"；文档中 Responses 在前 | [OpenRouter API Overview](https://openrouter.ai/docs/api_reference/overview) |
| LiteLLM | ✅ | ✅ | 双向桥接：无 Responses 配置的 openai_like 渠道自动把 /v1/responses 桥接到 /v1/chat/completions；反向亦然 | [LiteLLM /responses](https://docs.litellm.ai/docs/response_api) |
| gpt-load（MIT，本项目参考项） | ✅ | ✅ | 主入口表并列 `POST /v1/chat/completions` 与 `/v1/responses` 及其资源路径 | [gpt-load README](https://github.com/tbphp/gpt-load) |
| one-api（MIT） | ✅ | ❌ | README 全文无 Responses 支持 | [one-api README](https://github.com/songquanpeng/one-api) |
| vLLM | ✅ | ✅ | /v1/chat/completions、/v1/responses（含 batch/render 变体） | [vLLM Online Serving](https://docs.vllm.ai/en/latest/serving/online_serving/) |
| Ollama | ✅ | ✅（无状态） | "The same client supports stateless Responses requests"；云端 API 不支持有状态 Responses | [Ollama OpenAI Compatibility](https://docs.ollama.com/api/openai-compatibility) |
| Groq | ✅ | ❌（文档未见） | 官方 SDK 文档全部示例为 chat.completions.create | [groq-python README](https://github.com/groq/groq-python) |
| DeepSeek | ✅ | ✅ | API Reference 并列 Chat Completions / Responses / Messages 三协议 | [DeepSeek API Docs](https://api-docs.deepseek.com/) |
| Kimi / Moonshot | ✅ | ✅ | 同样三协议：POST Chat Completions / Responses / Messages | [Kimi API](https://platform.kimi.ai/docs/api/chat) |
| 智谱 GLM | ✅ | ✅ | "智谱提供独立的 Response API 协议……与现有对话补全并存"；注意差异：默认 store=false、流式结束不发送 [DONE]、暂无 cancel | [智谱 Response API 兼容](https://docs.bigmodel.cn/cn/guide/develop/responses/introduction) |
| 阿里 DashScope | ✅ compatible-mode | ❌（该兼容层仅 CC） | 各区域 endpoint 均为 compatible-mode/v1/chat/completions | [DashScope OpenAI 兼容](https://help.aliyun.com/zh/model-studio/compatibility-of-openai-with-dashscope) |
| Anthropic | 原生 Messages（另提供 OpenAI SDK 兼容层） | — | llms.txt 收录 "OpenAI SDK compatibility" 页面（本机区域封锁未能打开正文） | [Claude Docs llms.txt](https://platform.claude.com/llms.txt) |

## 生态分层解读

1. **官方与框架层：Responses 是战略主线。** OpenAI 自己的 SDK 示例已以 responses.create 为主；Vercel AI SDK 更是把 Responses 设为 OpenAI provider 默认——这意味着大量 AI SDK 应用只要填了自定义 base URL，打到的就是第三方的 /v1/responses。
2. **网关/聚合层：双支持已成标配，但 CC 仍是最大公约数。** OpenRouter、LiteLLM、gpt-load 均双支持；LiteLLM 的桥接方向（responses→chat 供无 Responses 的 provider 用）恰好证明：兼容层的现实是绝大多数 provider 只保证 chat/completions。
3. **国产厂商：Responses 已从"OpenAI 专属"变成通用协议。** DeepSeek、Kimi、智谱全部提供，且与 Chat Completions 并存而非替代；智谱还明确警告与 OpenAI 存在字段差异——说明"协议相同"≠"实现相同"。
4. **长尾客户端/厂商：chat-only 仍大量存在。** one-api、Groq、DashScope compatible-mode 均无 Responses。放弃 CC 入口等于放弃这部分存量用户。

## 对 EverGate 的决策建议

- **内部枢轴（IR）继续用 Chat Completions**：跨协议转换（Anthropic Messages、无 Responses 渠道）都经它中转，这是生态最大公约数，也是现状架构，无需推翻。
- **Responses 的一等入口地位保留并强化**：市场默认值正在向它迁移（AI SDK 默认、SDK 主示例），且国产直连场景真实存在。
- **路线图增量：渠道级"是否原生支持 Responses"能力声明 + 原生透传**。理由：
  - Responses→Chat 转换有损（reasoning items、内置工具事件、`previous_response_id` 服务端状态无法表达）；
  - 原生支持 Responses 的渠道已足够多（OpenAI/DeepSeek/Kimi/智谱/OpenRouter），透传可保真并规避各家字段差异；
  - 仅当候选渠道无 Responses 能力时回落转换——与 LiteLLM 模式一致，风险已被验证。
- **Anthropic Messages 继续独立入口**（调研阶段建议）：DeepSeek/Kimi 也提供 Messages API，佐证三协议矩阵的长期合理性；最终 v1 范围以决策记录为准，当前延后。
- **用自家数据闭环验证**：请求日志按入口协议统计流量占比，上线后以真实分布复核本决策（这比任何外部调研都可靠）。

## 风险与不确定项

- OpenAI 官方 guide 未能直接抓取（Cloudflare 403），官方定位引自 openai-python README 原文，非二手转述。
- 缺公开流量份额；若目标用户以存量 chat-only 客户端为主，CC 占比会长期高于矩阵表现。
- Responses 各实现存在差异（智谱示例），做"原生透传"时必须按渠道能力路由，不能按协议名想当然。

## 决策记录（2026-11-10）

用户确认 v1 协议范围：**上游与下游均支持 OpenAI Chat Completions 与 OpenAI Responses**；同协议优先原生透传（仅改写鉴权/base_url/模型映射），Responses→Chat Completions 桥接兜底；Anthropic Messages 下游入口延后（T13），重启条件为渠道池出现真实需求（如 Anthropic 官方上游或 Claude 系客户端）。落地任务卡：DEVELOP_PLAN.md **T18**（2026-11-11 建卡；T16/T17 编号已被缺陷修复卡占用）。
