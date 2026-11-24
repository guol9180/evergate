# 文档索引

> 三件套分工：[README](../README.md) 讲项目定位与用法；[AGENTS.md](../AGENTS.md) 讲开发规范；[DEVELOP_PLAN.md](../DEVELOP_PLAN.md) 管任务、状态与验收。领域文档统一放在 [`agents/`](./agents/)，已结束任务卡统一放在 [`archive/`](./archive/)。

## 用户与运维

| 文件 | 内容 |
|---|---|
| [FAQ.md](./agents/FAQ.md) | 错误码、cooldown 恢复、成本显示、初始密码、备份迁移、SmartScreen |
| [运维配置.md](./agents/运维配置.md) | 配置与数据路径、协议接入矩阵、健康探测、环境变量、更新 |
| [HTTPS与反向代理.md](./agents/HTTPS与反向代理.md) | Caddy / Nginx 最小配置、SSE 注意事项、子路径部署 |
| [回滚与备份.md](./agents/回滚与备份.md) | 自动备份位置、手动备份、回滚步骤、配置级迁移 |
| [openapi.yaml](./agents/openapi.yaml) | 下游 `/v1` 四协议接口契约 |

## 开发与专题

| 文件 | 内容 |
|---|---|
| [开发环境.md](./agents/开发环境.md) | 作者本机与 AI 沙箱的 Go / Node / 编码事实 |
| [目录约定.md](./agents/目录约定.md) | 完整目录结构与每包职责 |
| [约束-协议与转发.md](./agents/约束-协议与转发.md) | 协议矩阵、流式口径、探测、亲和、请求级覆盖、降级 |
| [约束-管理台与运维.md](./agents/约束-管理台与运维.md) | 管理台组件、图表、日志、指标、发布、一键更新、CI |
| [发版检查清单.md](./agents/发版检查清单.md) | 发版步骤、真实升级验证、npm Trusted Publishing 配置 |
| [RISK.md](./agents/RISK.md) | 风险登记（可能性 / 影响 / 缓解 / 复查日期） |
| [参考项目对照评审.md](./agents/参考项目对照评审.md) | T4a：gpt-load / LiteLLM / bifrost 对照与差距处置 |
| [管理台设计参考.md](./agents/管理台设计参考.md) | T5a：gpt-load 前端与 new-api 功能清单对照的设计要点 |
| [协议生态调研-ChatCompletions与Responses.md](./agents/协议生态调研-ChatCompletions与Responses.md) | T10 前调研 |
| [Responses协议兼容性审计.md](./agents/Responses协议兼容性审计.md) | T15 审计结论 |
| [P2调研-按预算模型降级.md](./agents/P2调研-按预算模型降级.md) | T36 调研 |
| [P2调研-Anthropic原生协议.md](./agents/P2调研-Anthropic原生协议.md) | T37 调研 |
| [P2调研-Gemini原生协议.md](./agents/P2调研-Gemini原生协议.md) | T38 调研 |
| [AI网关项目-开发任务书.md](./agents/AI网关项目-开发任务书.md) | 最初需求全文（历史输入，部分细节已被后续任务卡更新） |

## 历史归档

| 文件 | 内容 |
|---|---|
| [archive/P0.md](./archive/P0.md) | P0 阶段 done 任务卡（T0–T9） |
| [archive/P1.md](./archive/P1.md) | P1 阶段 done 任务卡 |
| [archive/P2.md](./archive/P2.md) | P2 阶段 done 任务卡 |
| [archive/P3.md](./archive/P3.md) | P3 阶段 done 任务卡（含安装器 / 托盘 / 更新链） |
| [archive/P4.md](./archive/P4.md) | P4 三视角审视改进批次（T73–T80） |

归档规则：DEVELOP_PLAN.md 只保留当前与最近未归档任务；done 卡整卡迁入 `archive/`，只搬不改，"进度只追加"在归档内继续生效。
