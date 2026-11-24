# HTTPS 与反向代理

EverGate 默认以明文 HTTP 监听所有网卡；下游令牌与管理台会话 Cookie 都以明文传输。**只在本机或可信局域网直接使用**；跨网络访问请按本文配置 TLS。

## 前提

1. 在 `evergate.yml` 设置 `server.admin_loopback_only: false`（否则反代后管理台 API 会拒绝非 loopback 来源）
2. 反代层**必须**自行增加访问控制（管理台有登录鉴权，但 `/v1` 仅靠令牌；建议再加 IP 白名单或 Basic Auth）
3. 若使用子路径部署，设置 `server.base_path: /eg`，反代规则与之对应

## Caddy（自动 HTTPS，推荐）

`Caddyfile`：

```
gateway.example.com {
    reverse_proxy 127.0.0.1:8787 {
        flush_interval -1
        transport http {
            read_timeout 3600s
        }
    }
}
```

- `flush_interval -1` 让 SSE 流式响应逐段下发，不要缓冲
- `read_timeout` 覆盖长响应（整体超时最长可设 24h，按需调整）

## Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name gateway.example.com;
    ssl_certificate     /etc/ssl/certs/gateway.pem;
    ssl_certificate_key /etc/ssl/private/gateway.key;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE：关闭缓冲，拉长读超时
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

- `proxy_buffering off` 与 `Connection ""` 是流式响应不卡顿的关键；否则首个 chunk 可能被 Nginx 缓冲到响应结束
- 管理台会话 Cookie 在 `c.Request.TLS != nil` 时自动带 `Secure`；经反代终止 TLS 时 EverGate 看到的是 HTTP，Cookie 不带 `Secure`。如需强制，请在反代层注入 `X-Forwarded-Proto` 并信任该头（Gin 默认信任代理时 `c.Request.TLS` 仍为 nil，当前实现不据此加 Secure，属于已知限制）

## 子路径部署（`server.base_path: /eg`）

管理台、静态资源与 API 统一挂载到 `/eg/`，下游端点变为 `/eg/v1/chat/completions` 等。反代规则不需要改写路径，直接透传即可：

```
gateway.example.com {
    reverse_proxy 127.0.0.1:8787
}
```

客户端 base_url 填 `https://gateway.example.com/eg/v1`。

## 不涉及

EverGate 不使用 WebSocket；无需 `Upgrade` 头。Prometheus `/metrics` 仅允许 loopback 抓取，不能经反代暴露。
