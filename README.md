# EverGate

**本地自托管 AI 网关**：把多家 LLM 供应商（OpenAI / Anthropic / Google Gemini / DeepSeek / Kimi / 智谱 / 本地 vLLM / 任意 OpenAI 兼容端点）统一为 OpenAI 兼容 API、Anthropic Messages 与 Gemini API；上游或 key 故障时自动切换，客户端无感。*One gate, every model, zero interruption.*

> Self-hosted OpenAI-compatible LLM gateway with key-pool failover, multi-provider routing and zero-buffer SSE streaming.

**适用**：个人或小团队在本机 / 局域网聚合多个上游供应商与 key；单二进制 + SQLite，无需 Docker / Redis / 外部数据库。不适合多租户计费场景。

[![Release](https://img.shields.io/github/v/release/guol9180/evergate?include_prereleases&style=flat-square)](https://github.com/guol9180/evergate/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white&style=flat-square)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey?style=flat-square)](https://github.com/guol9180/evergate/releases/latest)

## 能力概览

- **统一入口**：OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 与 Gemini API 四种协议，见[协议接入](./docs/agents/运维配置.md#协议接入)
- **高可用**：多渠道与 key 池调度，故障自动切换、按预算模型降级
- **管理台**：渠道、下游令牌、请求日志（重试轨迹）、调试台、用量成本、系统设置与审计日志
- **轻量交付**：单二进制 + SQLite，无需 Docker / Redis / 外部数据库
- **可运维**：Prometheus 指标、可选 OTLP 导出、一键更新

## 安装

| 平台 | 方式 | 说明 |
|---|---|---|
| Windows x64 | [GitHub Releases](https://github.com/guol9180/evergate/releases/latest) 的 `evergate_<version>_windows_amd64_setup.exe` | 安装器含系统托盘；**未做代码签名**，首次运行 SmartScreen 会提示，见 [FAQ](./docs/agents/FAQ.md#windows-smartscreen-拦截怎么办) |
| Linux x64 | `evergate_<version>_linux_amd64.tar.gz` | 解压即用，可放入 Docker 镜像 |
| macOS Apple Silicon | `npm i -g @heguoliang/evergate` | **仅 CLI / npm 渠道**；图形安装器需 Apple 开发者签名与公证凭据，尚未提供，无时间承诺 |
| 任意平台 | `npm i -g @heguoliang/evergate` | 自动选择本平台子包，实际运行仍是原生单二进制 |
| 源码 | `make build` | `web/dist` 已入库，后端构建无需 Node；三平台用 `make build-all` |

请同时核对同版本 `checksums.txt`。国内网络可用 `npm i -g @heguoliang/evergate --registry=https://registry.npmmirror.com`。安装版应用内更新优先从 npmmirror / 官方 npm registry 下载完整安装资产，GitHub Release 仅作备用。

## 快速开始

1. 启动后管理台默认自动打开 `http://localhost:8787/`（服务器部署可设 `server.open_browser: false`）
2. **初始管理员密码在首次启动时随机生成**，打印在控制台与数据目录 `logs/evergate.log` 中（WARN 级别、带分隔线）；自动化场景可用环境变量 `EVERGATE_INITIAL_ADMIN_PASSWORD` 预设。首次登录强制改密
3. 在管理台「渠道管理」选择供应商预设、填入上游 base_url 与 API key，保存后「从上游拉取」模型目录
4. 「令牌管理」创建下游令牌，复制生成的客户端配置片段
5. 客户端把 base_url 指向 `http://localhost:8787/v1`，用该令牌作为 API key

默认端口 `8787`，通过 `evergate.yml` 的 `server.port` 修改；存活检查 `GET /healthz`。路径发现顺序、固定安装目录与数据位置见 [运维配置 · 配置与数据路径](./docs/agents/运维配置.md#配置与数据路径)。

## 接入客户端

令牌来源均为管理台「令牌管理」。若设置了 `server.base_path: /eg`，下列所有 URL 需加前缀 `/eg`（如 `http://localhost:8787/eg/v1`）。

**OpenAI SDK（Python / Node）**

```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key="<令牌>")
resp = client.chat.completions.create(model="<渠道中的模型名>", messages=[{"role": "user", "content": "hi"}])
```

```js
import OpenAI from 'openai'
const client = new OpenAI({ baseURL: 'http://localhost:8787/v1', apiKey: '<令牌>' })
```

**Codex** — `~/.codex/config.toml`

```toml
model_provider = "evergate"
[model_providers.evergate]
name = "EverGate"
base_url = "http://127.0.0.1:8787/v1"
wire_api = "responses"
env_key = "EVERGATE_API_KEY"
```

**Claude Code**

```bash
export ANTHROPIC_BASE_URL="http://localhost:8787"
export ANTHROPIC_AUTH_TOKEN="<令牌>"
export ANTHROPIC_MODEL="<渠道中的模型名>"
```

**Cline（VS Code）**：API Provider 选 `OpenAI Compatible`，Base URL `http://localhost:8787/v1`，API Key 填令牌，Model ID 填渠道中的模型名。

**Cursor**：Settings → Models → 取消 override → OpenAI API Key 填令牌；在 `~/.cursor/config.json` 设置 `"openaiBaseUrl": "http://localhost:8787/v1"`（具体路径随 Cursor 版本变化）。

**OpenCode**：在 `opencode.json` 的 `provider` 中添加 `{"type":"openai","baseURL":"http://localhost:8787/v1","apiKey":"<令牌>"}`，模型写作 `provider/model`。

**curl**

```bash
curl http://localhost:8787/v1/chat/completions \
  -H "Authorization: Bearer <令牌>" -H "Content-Type: application/json" \
  -d '{"model":"<模型>","messages":[{"role":"user","content":"hi"}]}'

curl http://localhost:8787/v1/messages \
  -H "x-api-key: <令牌>" -H "anthropic-version: 2023-06-01" -H "Content-Type: application/json" \
  -d '{"model":"<模型>","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}'
```

## 安全模型

- 上游 key 加密存储（主密钥仅在 `evergate.yml`，已 gitignore），管理台接口不回显明文；下游令牌仅存哈希，明文只在创建时展示一次
- 管理台 API 默认仅允许 loopback 访问；确需远程管理时显式设置 `server.admin_loopback_only: false`，并自行增加 TLS 与访问控制
- **下游 `/v1` 默认监听所有网卡且无 TLS，令牌以明文传输**：请仅在本机或可信局域网使用；跨网络访问按 [HTTPS 与反向代理](./docs/agents/HTTPS与反向代理.md) 配置
- 初始管理员密码随机生成（或由环境变量预设），首登强制修改；管理台写操作记录在审计日志中
- 设置 `server.base_path: /eg` 后，管理台、静态资源与 API 统一挂载到 `http://localhost:8787/eg/`

## 架构概览

单进程 Go 服务：

1. `/v1/*`：下游令牌鉴权 → 路由调度 → relay/SSE 透传
2. `/admin/api/*`：管理台 API（session 鉴权）
3. SQLite（WAL）：业务数据与运行时设置的唯一事实源；升级前自动备份到 `data/backups/`（保留 5 份）
4. React 管理台构建产物 go:embed 进同一二进制

技术栈：Go 1.25+ / Gin / GORM / 纯 Go SQLite；React 19 / TypeScript / Vite / TanStack Query / Tailwind 4。

## 文档

- [FAQ 与故障排查](./docs/agents/FAQ.md)：错误码含义、cooldown 恢复、成本显示、初始密码、备份迁移
- [运维与配置](./docs/agents/运维配置.md)：路径、协议接入矩阵、健康探测、环境变量
- [HTTPS 与反向代理](./docs/agents/HTTPS与反向代理.md)：Caddy / Nginx 最小配置
- [回滚与备份](./docs/agents/回滚与备份.md)
- [OpenAPI（下游 /v1）](./docs/agents/openapi.yaml)
- [发版检查清单](./docs/agents/发版检查清单.md) · [风险登记](./docs/agents/RISK.md) · [文档索引](./docs/agents/README.md)
- [开发规范](./AGENTS.md) · [开发计划](./DEVELOP_PLAN.md) · [贡献指南](./CONTRIBUTING.md) · [安全披露](./SECURITY.md)

## License

[MIT](./LICENSE) © 2026 guol9180

---

**关键词 / Keywords**：AI 网关 · LLM Gateway · OpenAI 兼容 · Responses API · 故障切换 failover · key 池 · SSE 流式 · 自托管 self-hosted · Go · React · SQLite
