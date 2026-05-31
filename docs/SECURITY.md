# 安全说明

## token 的作用

Agent 写入接口需要 token。中心服务和每台 Agent 必须使用同一个 token。

Agent 请求会带：

```text
Authorization: Bearer <token>
```

token 不一致会返回 `401`。Dashboard 读取 `/api/nodes`，浏览器端不保存 token。

生成 token：

```bash
openssl rand -hex 24
```

## 为什么 8080 不应全网开放

`8080` 是远程 VPS Agent 上报入口。它只应该给你的远程 VPS 访问。

虽然写入接口仍然需要 token，但 8080 全网开放会增加暴露面，也更容易被扫描和打流量。

推荐做法：

- Dashboard 走 `https://monitor.example.com`
- Agent 上报走 `http://中心VPS公网IP:8080`
- 8080 只允许指定远程 VPS IP

## iptables 放行远程 VPS IP

在远程 VPS 查看公网 IP：

```bash
curl -4 ifconfig.me
```

在中心 VPS 放行这个 IP：

```bash
sudo iptables -I INPUT -p tcp -s 远程VPS_IP --dport 8080 -j ACCEPT
```

拒绝其他 IP：

```bash
sudo iptables -A INPUT -p tcp --dport 8080 -j DROP
```

也可以使用脚本：

```bash
sudo bash allow_agent_ip.sh 远程VPS_IP
```

查看规则：

```bash
sudo iptables -S INPUT | grep -- '--dport 8080'
```

## iptables 重启丢失

本项目不会自动安装 `iptables-persistent`，也不会自动启用 ufw。

原因是不同 VPS 镜像的防火墙方案不一样，自动持久化可能影响你现有规则。

确认规则稳定后，再按你的系统习惯持久化。

## Cloudflare 使用建议

Cloudflare 只建议用于 Dashboard：

```text
https://monitor.example.com
```

Agent 推荐走中心 VPS 公网 IP 的 8080：

```toml
server_url = "http://中心VPS公网IP:8080"
```

如果使用 Cloudflare 小黄云，SSL/TLS 推荐：

```text
Full 或 Full strict
```

不推荐：

```text
Flexible
```

`Flexible` 容易导致跳转、协议判断和反代异常。
