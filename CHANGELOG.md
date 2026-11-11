# 更新日志

发版描述自动取自本文件：工作流按 `## <tag>` 提取对应段落作为 GitHub Release 正文（T7b 约定）。

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
