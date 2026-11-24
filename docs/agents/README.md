# 文档索引

> 三件套分工：[README](../../README.md) 讲项目定位与用法；[AGENTS.md](../../AGENTS.md) 讲开发规范；[DEVELOP_PLAN.md](../../DEVELOP_PLAN.md) 管任务、状态与验收。`docs/` 根目录不放散文件；当前领域文档放在 `docs/agents/`，历史原文放在 `docs/archive/`。

## 用户与运维

| 文件 | 内容 |
|---|---|
| [FAQ.md](./FAQ.md) | 错误码、cooldown 恢复、成本显示、初始密码、备份迁移、SmartScreen |
| [运维配置.md](./运维配置.md) | 配置与数据路径、协议接入矩阵、健康探测、环境变量、更新 |
| [HTTPS与反向代理.md](./HTTPS与反向代理.md) | Caddy / Nginx 最小配置、SSE 注意事项、子路径部署 |
| [回滚与备份.md](./回滚与备份.md) | 自动备份位置、手动备份、回滚步骤、配置级迁移 |
| [openapi.yaml](./openapi.yaml) | 下游 `/v1` 四协议接口契约 |

## 开发与专题

| 文件 | 内容 |
|---|---|
| [开发环境.md](./开发环境.md) | 作者本机与 AI 沙箱的 Go / Node / 编码事实 |
| [目录约定.md](./目录约定.md) | 完整目录结构与每包职责 |
| [约束-协议与转发.md](./约束-协议与转发.md) | 协议矩阵、流式口径、探测、亲和、请求级覆盖、降级 |
| [约束-管理台与运维.md](./约束-管理台与运维.md) | 管理台组件、图表、日志、指标、发布、一键更新、CI |
| [发版检查清单.md](./发版检查清单.md) | 发版步骤、真实升级验证、npm Trusted Publishing 配置 |
| [RISK.md](./RISK.md) | 风险登记（可能性 / 影响 / 缓解 / 复查日期） |

## 历史归档

| 文件 | 类型 / 状态 | 归档日期 | 当前结论入口 |
|---|---|---|---|
| [P0.md](../archive/P0.md) | 任务卡归档 | 2026-11-17 | [里程碑](../../DEVELOP_PLAN.md#里程碑) |
| [P1.md](../archive/P1.md) | 任务卡归档 | 2026-11-17 | [里程碑](../../DEVELOP_PLAN.md#里程碑) |
| [P2.md](../archive/P2.md) | 任务卡归档 | 2026-11-17 | [里程碑](../../DEVELOP_PLAN.md#里程碑) |
| [P3.md](../archive/P3.md) | 任务卡归档 | 2026-11-24 | [里程碑](../../DEVELOP_PLAN.md#里程碑) |
| [P4.md](../archive/P4.md) | 任务卡归档 | 2026-11-24 | [里程碑](../../DEVELOP_PLAN.md#里程碑) |
| [research-参考项目对照评审.md](../archive/research-参考项目对照评审.md) | 调研 `concluded` | 2026-11-09 | [协议与转发约束](./约束-协议与转发.md) |
| [research-管理台设计参考.md](../archive/research-管理台设计参考.md) | 调研 `concluded` | 2026-11-09 | [管理台约束](./约束-管理台与运维.md) |
| [research-协议生态调研-ChatCompletions与Responses.md](../archive/research-协议生态调研-ChatCompletions与Responses.md) | 调研 `concluded` | 2026-11-10 | [协议与转发约束](./约束-协议与转发.md) |
| [research-Responses协议兼容性审计.md](../archive/research-Responses协议兼容性审计.md) | 审计 `concluded` | 2026-11-10 | [协议与转发约束](./约束-协议与转发.md) |
| [research-P2-按预算模型降级.md](../archive/research-P2-按预算模型降级.md) | 调研 `concluded` | 2026-11-16 | [协议与转发约束](./约束-协议与转发.md) |
| [research-P2-Anthropic原生协议.md](../archive/research-P2-Anthropic原生协议.md) | 调研 `concluded` | 2026-11-16 | [协议与转发约束](./约束-协议与转发.md) |
| [research-P2-Gemini原生协议.md](../archive/research-P2-Gemini原生协议.md) | 调研 `concluded` | 2026-11-16 | [协议与转发约束](./约束-协议与转发.md) |
| [historical-AI网关项目-开发任务书.md](../archive/historical-AI网关项目-开发任务书.md) | 历史需求 | 2026-11-09 | [README](../../README.md) · [DEVELOP_PLAN](../../DEVELOP_PLAN.md) |

归档规则：任务卡与已结题调研原文只搬不改。当前事实以 README、AGENTS、`docs/agents/` 和 [DEVELOP_PLAN](../../DEVELOP_PLAN.md) 为准，历史原文只用于追溯。未结项调研放 `docs/research/` 并在文件头标 `open / concluded / abandoned`；设计或 ADR 按项目实际需要再创建，不预建空目录。
