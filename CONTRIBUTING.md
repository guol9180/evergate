# 贡献指南

EverGate 是个人维护的开源项目，欢迎 issue 与 PR。开始前请读 [AGENTS.md](./AGENTS.md)（开发规范）与 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md)（当前任务卡）。

## 开发流程

1. **先建卡后动手**：在 DEVELOP_PLAN.md 新建任务卡（目标 / 验收标准 ≤5 条 / 边界 / 状态 / 进度），或认领已有 `planned` 卡并改为 `in-progress`；一个会话同一时间只持一张 in-progress 卡
2. **分支**：从 `main` 切出 `feat/<简述>` 或 `fix/<简述>`；改 DEVELOP_PLAN.md 前先 `git pull --rebase`
3. **提交信息**：`feat:` / `fix:` / `test:` / `docs:` 前缀，一行说明动机
4. **文档同步**：新增功能只记在 DEVELOP_PLAN.md；README 仅在定位 / 结构 / 用法 / 环境变化时更新；AGENTS.md 仅在规范变化时更新

## 提交前验证

后端：

```bash
gofmt -l .            # 无输出
go vet ./...
go test -p 1 ./... -count=1
make coverage-scheduler   # ≥90%
make coverage-relay       # ≥81%
make build
```

前端（在 `web/`）：

```bash
npm run lint
npm test
npm run build         # 产物 web/dist 需随提交入库
```

发布脚本：`node --test scripts/*.test.mjs`。CI（`.github/workflows/ci.yml`）会在 PR 上重复以上步骤并额外跑 `go test -race`、gitleaks 与 actionlint。

## 不接受的 PR

- 引入 [AGENTS.md · 禁止事项](./AGENTS.md#禁止事项) 列出的依赖（Redis、消息队列、微服务、多租户计费等）
- 复制 new-api（AGPL）或 OmniRoute 的任何代码 / 样式
- 手写 SQL 迁移脚本
- 改动与任务卡无关的相邻代码（见"精准修改"）

## 许可证

贡献内容按 [MIT](./LICENSE) 授权；可借鉴的参考项目范围见 AGENTS.md 许可证红线。
