# EverGate

**本地自托管 AI 网关**：把多家 LLM 供应商（OpenAI / DeepSeek / Kimi / 智谱 / Anthropic / Gemini / 本地 vLLM / 任意 OpenAI 兼容端点）统一为一个 OpenAI 兼容 API；上游或 key 故障时自动切换，客户端无感。*One gate, every model, zero interruption.*

> Self-hosted OpenAI-compatible LLM gateway with key-pool failover, multi-provider routing and zero-buffer SSE streaming.

[![Release](https://img.shields.io/github/v/release/guol9180/evergate?include_prereleases&style=flat-square)](https://github.com/guol9180/evergate/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white&style=flat-square)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey?style=flat-square)](https://github.com/guol9180/evergate/releases/latest)

**下载**：[GitHub Releases](https://github.com/guol9180/evergate/releases/latest) 提供三平台单二进制（Windows x64 / Linux x64 / macOS Apple Silicon）。版本变更见 [CHANGELOG.md](./CHANGELOG.md)。

## 能力概览

- **统一入口**：OpenAI 兼容的 Chat Completions 与 Responses API
- **高可用**：多渠道与 key 池调度，故障自动切换
- **管理台**：渠道、下游令牌、请求日志、调试与用量
- **轻量交付**：单二进制 + SQLite，无需 Docker / Redis / 外部数据库
- **可运维**：指标观测与一键更新

能力边界、协议矩阵与验收细节见 [DEVELOP_PLAN.md](./DEVELOP_PLAN.md) 与 [AGENTS.md](./AGENTS.md)；配置与运维详见 [docs/运维配置.md](./docs/运维配置.md)。

## 快速开始

```bash
# 方式一：直接下载 Release 单二进制后运行
./evergate

# 方式二：npm 全局安装（Windows x64 / Linux x64 / macOS Apple Silicon）
npm i -g @heguoliang/evergate
evergate

# 方式三：源码构建（web/dist 已入库，后端构建无需 Node）
make build              # → bin/evergate(.exe)，React 管理台已嵌入
make build-all          # 三平台交叉编译：windows/amd64、linux/amd64、darwin/arm64
go run ./cmd/evergate   # 开发态直接运行
```

npm 安装会自动选择本平台子包，实际运行仍是原生单二进制。国内网络可用 `npm i -g @heguoliang/evergate --registry=https://registry.npmmirror.com`。

1. 复制配置模板：`cp evergate.yml.example evergate.yml`（密钥留空则首次启动自动生成并写回）
2. 启动后访问管理台 `http://localhost:8787/`（初始账号 `admin/admin123`，仅用于本机初始化，首登强制改密）；默认启动自动打开浏览器，服务器部署可设 `server.open_browser: false`
3. 在管理台添加渠道并创建下游令牌
4. 客户端只需改 base_url 为 `http://localhost:8787/v1` 并使用下游令牌

存活检查 `GET /healthz`。默认端口 `8787`，通过 `evergate.yml` 的 `server.port` 修改。

## 安全模型

- 上游 key 加密存储（主密钥仅在 `evergate.yml`，已 gitignore），管理台接口不回显明文；下游令牌仅存哈希
- 管理台与 API 同端口，面向本地 / 内网可信环境；公网暴露前请自行加 TLS 与访问控制
- 初始 `admin/admin123` 仅用于首次登录并会强制改密

## 架构概览

单进程 Go 服务：

1. `/v1/*`：下游令牌鉴权 → 路由调度 → relay/SSE 透传
2. `/admin/api/*`：管理台 API（session 鉴权）
3. SQLite（WAL）：业务数据与运行时设置的唯一事实源
4. React 管理台构建产物 go:embed 进同一二进制

技术栈：Go 1.24+ / Gin / GORM / 纯 Go SQLite；React 19 / TypeScript / Vite / TanStack Query / Tailwind 4。

## 参与开发

```bash
go test ./...                # 后端全量测试
npm --prefix web run build   # 前端构建（改前端后执行并提交 dist）
make build                   # 提交前完整构建
```

提交前验证流程与代码规范见 [AGENTS.md](./AGENTS.md)；当前阶段与后续任务见 [DEVELOP_PLAN.md · 里程碑](./DEVELOP_PLAN.md#里程碑)；需求与协议调研见 [docs/AI网关项目-开发任务书.md](./docs/AI网关项目-开发任务书.md)、[docs/协议生态调研-ChatCompletions与Responses.md](./docs/协议生态调研-ChatCompletions与Responses.md)。

## License

[MIT](./LICENSE) © 2026 guol9180

---

**关键词 / Keywords**：AI 网关 · LLM Gateway · OpenAI 兼容 · Responses API · 故障切换 failover · key 池 · SSE 流式 · 自托管 self-hosted · Go · React · SQLite
