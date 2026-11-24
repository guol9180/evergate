# 贡献指南

EverGate 是个人维护的开源项目，欢迎 issue 与 PR。开始前请读 [AGENTS.md](./AGENTS.md)（开发规范）与 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)（当前任务卡）。

## 开发流程

建卡、认领、分支、提交、文档同步与提交前验证的唯一规范是 [AGENTS.md](./AGENTS.md)；当前执行顺序与验收标准见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)。开始修改前先按这两份文档完成建卡与验证准备。

## 不接受的 PR

- 引入 [AGENTS.md · 禁止事项](./AGENTS.md#禁止事项) 列出的依赖（Redis、消息队列、微服务、多租户计费等）
- 复制 new-api（AGPL）或 OmniRoute 的任何代码 / 样式
- 手写 SQL 迁移脚本
- 改动与任务卡无关的相邻代码（见"精准修改"）

## 许可证

贡献内容按 [MIT](./LICENSE) 授权；可借鉴的参考项目范围见 AGENTS.md 许可证红线。
