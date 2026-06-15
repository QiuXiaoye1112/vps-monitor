# 安全说明

## Token

Agent 写入接口需要 token，通过 Header 传递：

```
Authorization: Bearer <token>
```

也支持 `X-Monitor-Token` Header。为避免 token 出现在代理和访问日志中，不支持 URL 查询参数传递。

生成 token：

```bash
openssl rand -hex 24
```

## 网络隔离

- FastAPI 只监听 `127.0.0.1`，不暴露到公网
- Agent 入口（默认 8080）通过 Nginx 只开放 `/api/`，其余返回 404
- Agent 入口应通过 iptables 白名单限制：`sudo vm` → 监控主机 → 允许访问
- Dashboard 走 Nginx 80/443

## 防火墙

通过 `sudo vm` → 监控主机 → 防火墙操作管理。规则样例：

```
ACCEPT 远程IP  →  允许上报
DROP   所有IP  →  阻止（排除本地回路）
```

规则自动持久化，重启不丢失。

## Cloudflare

- Dashboard 走 Cloudflare：SSL/TLS 建议 `Full` 或 `Full strict`，不推荐 `Flexible`
- Agent 上报绕开 Cloudflare，直接走 `http://中心VPS公网IP:8080`
