# 管理台设计参考（T5a 补课产出 → T6 输入）

> 日期：2026-11-09。来源：gpt-load web/ 源码精读（MIT，可借鉴实现）+ new-api README 功能清单与截图描述（AGPL 红线：未阅读任何代码/组件/样式，仅产品层）。

## 一、gpt-load 前端学到的交互模式

| 模式 | 出处 | T6 采纳 |
|---|---|---|
| 首页事实行「X 分组 · Y/Z 可用凭据 · N 模型」+ 版本/运行时长/更新时间戳 | HomeSummary.vue | Dashboard 顶部事实行：渠道数 · 可用 key/总 key · 模型数 |
| 凭据行可展开（详情折叠）+ 状态徽标 + 行内权重编辑 + 近期成功/失败计数 | GroupCredentialRecord.vue | key 列表：状态徽标（active 绿 / cooldown 橙+剩余时间 / disabled 红 / half_open 蓝）+ 展开显示连续失败数、退避档位、上次错误 |
| 连通测试三态色调：passed→success / failed→danger / inconclusive→warning | GroupCredentialRecord / CredentialTestDialog | 渠道测试按钮结果徽标同款三态 |
| 失败类别人话映射表（rate_limited/model_unavailable/invalid_key/upstream_host_error…） | credential-failure-presenter.ts | 与后端 retry.ErrorKind 一一对应（T4a 分类学已对齐）：限流/模型不可用/凭据无效/服务器故障/内容策略 |
| 凭据批量操作条（选中后批量测试/启停/删除） | GroupCredentialBatchBar.vue | P0 简化为批量导入多行 key（任务书核心），批量操作 P1 |
| 日志筛选表单 + 高级筛选抽屉 + **日志详情抽屉 LogDetailDrawer** | monitor/LogsTab + LogDetailDrawer.vue | 日志详情抽屉 + **重试轨迹可视化**（任务书 4.6 核心：每次尝试的渠道/key/错误/耗时时间线） |
| 健康摘要条（问题渠道汇总横条） | HealthSummaryStrip.vue | Dashboard 渠道健康列表（active/degraded/down 徽标） |

## 二、new-api 产品设计要点（仅 README 功能清单 + 截图描述）

| 功能域 | new-api 做法 | T6 取舍 |
|---|---|---|
| 渠道配置 | 上游 key + 可用模型 + 分组 + 渠道测试 | ✅ 全采纳（P0 无分组维度，以渠道为单位） |
| 路由 | 模型映射 / 渠道优先级权重 / 重试 / 渠道亲和 / 多上游 key | ✅ P0：映射/优先级/权重/多 key；亲和 P1 |
| 用量日志 | tokens、成本、响应时间列 | ✅ P0：状态/耗时/token 数列；成本报表 P1 |
| 令牌 | 控制台创建 API key、配额、模型限制 | ✅ P0：创建时**明文仅显示一次+一键复制**（new-api 同款） |
| Playground / 模型广场 / 插件市场 / 多语言 | README 截图功能 | ❌ P1/P2（任务书明确分期） |
| 快速上手流程 | 建渠道→测连通→建 key→设 base_url | ✅ Dashboard 空状态引导同款四步 |

## 三、T6 五页面设计规格

### 1. 登录 `/login`
- 居中卡片：用户名/密码 + 登录；错误提示内联
- 登录成功且 `must_change_password=true` → 强制跳改密页（后端已 403 拦截其余 API，前端同步约束）

### 2. Dashboard `/`
- 事实行：渠道数 · 可用 key/总 key · 去重模型数（gpt-load 模式）
- 今日卡片：请求量 / 成功率 / P95 延迟 / 活跃渠道（数据源：/admin/api/logs 聚合，P0 前端现算）
- 渠道健康列表：名称 + 状态徽标 + key 可用性 + 实时成功率；异常渠道排前
- 空状态：四步引导（建渠道→导 key→建令牌→复制 base_url）

### 3. 渠道管理 `/channels`
- 列表：状态徽标 / 名称 / 类型 / 优先级 / 权重 / 模型数 / key 可用数 / 启停开关 / 测试 / 编辑
- 测试按钮：loading→三态结果徽标（ok/latency / 错误信息）
- 新建/编辑抽屉或弹窗：类型、base_url（提示含 /v1）、模型多行 tag、模型映射 JSON 编辑、优先级/权重、超时覆盖
- 渠道详情：key 列表（状态徽标+cooldown 倒计时+连续失败/档位/最近错误，展开行）+ 批量导入（多行 textarea，返回 added 数）+ 单 key 启停/删除/权重

### 4. 令牌管理 `/tokens`
- 列表：名称 / 状态 / 额度 / 已用 / 允许模型 / 过期时间 / 创建时间 / 操作
- 创建成功弹窗：**明文 key 仅此一次** + 大号等宽字体 + 一键复制（new-api 模式）+「关闭后无法再次查看」警示

### 5. 请求日志 `/logs`
- 筛选栏：时间范围 / 渠道 / 模型 / 状态码 / 令牌（对应后端 query 参数）
- 表格：时间 / 模型 / 渠道 / 状态码（色彩：2xx 绿 4xx 橙 5xx 红）/ 首 token / 总耗时 / tokens / 令牌 / 重试次数徽标
- **详情抽屉**（核心调试体验）：基本信息 + 重试轨迹时间线（每次尝试：渠道→key→错误类型（人话映射）→耗时→箭头串联，失败红/成功绿）

## 四、技术决策（任务书 3.2 落地）

- React 19 + TypeScript + Vite；TanStack Query（服务端状态）+ lucide-react 图标
- Tailwind CSS 4；组件按 shadcn/ui 风格手写最小集（按钮/输入/表格/徽标/抽屉/弹窗），**不跑 shadcn CLI**（避免 P0 生成大量样板；AGENTS 反过度设计）
- 路由：react-router（P0 无深路由嵌套，TanStack Router 收益小、迁移成本高；记录为 P1 可升级项，偏离任务书 3.2 的 Router 选型，理由：任务书允许"二选一后不得混用"精神的务实落地）
- 开发模式：Vite devServer 代理 /admin/api 与 /v1 到 :8787；生产 go:embed 嵌入（T7）
- 全部时间展示：本地时区（存储 UTC，展示层转换——AGENTS 红线）
