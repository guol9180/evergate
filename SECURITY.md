# 安全策略

## 报告漏洞

请使用 GitHub **Private vulnerability reporting**（仓库 Security 标签页 → Report a vulnerability），**不要**在公开 issue 中披露可利用细节。会在 72 小时内确认收到，7 天内给出处置计划。

## 安全模型（评估范围时参考）

- 上游 API key 以 AES-256-GCM 加密存储，主密钥只在 `evergate.yml`（不入库）；管理台任何接口不回显明文
- 下游令牌仅存 SHA-256 哈希，明文只在创建响应中出现一次
- 管理台 API 默认仅 loopback；管理员初始密码随机生成（或由 `EVERGATE_INITIAL_ADMIN_PASSWORD` 预设），首登强制改密，登录失败限流
- 会话亲和只保存 SHA-256 指纹；请求级覆盖只接受白名单字段，摘要脱敏落日志
- 审计日志记录管理台写操作，不含密钥明文
- 已知限制：下游 `/v1` 默认无 TLS（见 [HTTPS 与反向代理](./docs/agents/HTTPS与反向代理.md)）；Windows 安装器未签名；经反代终止 TLS 时会话 Cookie 不带 `Secure`

## 不在范围内

- 需要物理访问本机或 `evergate.yml` 文件的攻击
- 上游供应商自身的安全问题
- 多租户隔离（项目明确定位为个人 / 小团队本地网关）
