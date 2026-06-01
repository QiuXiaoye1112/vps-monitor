# 常见问题排查

## Dashboard 能打开，但节点 offline

在中心 VPS 检查 API：

```bash
curl http://127.0.0.1:8000/api/health
```

在中心 VPS 查看节点数据：

```bash
curl http://127.0.0.1:8000/api/nodes
```

在被监控 VPS 查看 Agent 状态：

```bash
systemctl status vps-monitor-agent
```

在被监控 VPS 查看 Agent 日志：

```bash
journalctl -u vps-monitor-agent -f
```

手动上报一次：

```bash
python /opt/vps-monitor/agent.py --config /etc/vps-monitor-agent.toml --once
```

在中心 VPS 检查 8080 放行规则：

```bash
sudo iptables -S INPUT | grep -- '--dport 8080'
```

## Agent 日志 404

通常是 `server_url` 写错了，或者远程 Agent 打到了 Dashboard 域名的非 API 入口。

查看配置：

```bash
cat /etc/vps-monitor-agent.toml
```

远程 VPS 推荐：

```toml
server_url = "http://中心VPS公网IP:8080"
```

测试 8080 API：

```bash
curl -i http://中心VPS公网IP:8080/api/health
```

在中心 VPS 测试根路径应返回 404：

```bash
curl -i http://127.0.0.1:8080/
```

## Agent 日志 401

`401` 表示 token 不一致。

在中心 VPS 查看 token：

```bash
sudo cat /etc/vps-monitor.env
```

在被监控 VPS 查看 token：

```bash
sudo cat /etc/vps-monitor-agent.toml
```

改完 token 后重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

## HTTPS 后 Agent 无法上报

远程 Agent 不建议默认走 HTTPS 域名。

查看配置：

```bash
cat /etc/vps-monitor-agent.toml
```

推荐改成：

```toml
server_url = "http://中心VPS公网IP:8080"
```

测试：

```bash
curl -i http://中心VPS公网IP:8080/api/health
```

重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

## Cloudflare 小黄云异常

Dashboard 可以通过 Cloudflare 访问，但 SSL/TLS 不建议用 Flexible。

检查 Cloudflare SSL/TLS：

```text
SSL/TLS -> Overview -> Full 或 Full strict
```

在中心 VPS 检查 Nginx：

```bash
nginx -t
```

检查 HTTPS：

```bash
curl -I https://monitor.example.com
```

如果 Agent 通过域名上报异常，把 Agent 改回中心 VPS 公网 IP 的 8080：

```toml
server_url = "http://中心VPS公网IP:8080"
```

## 同一个节点名字来回跳

这是两台机器用了同一个 `node_id`。

在中心 VPS 查看节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

在每台 VPS 查看自己的 `node_id`：

```bash
grep node_id /etc/vps-monitor-agent.toml
```

每台 VPS 必须使用不同 `node_id`。改完重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

## 出现脏节点

脏节点通常是测试时留下的旧 `node_id`。

查看节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

只保留指定节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM nodes WHERE id NOT IN ('vmrack','vimss');"
```

清理旧指标：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM metrics WHERE node_id NOT IN ('vmrack','vimss');"
```

## git pull 后本地修改丢失

先看本地改动：

```bash
git status
```

如果确认服务器上的本地改动不要了，强制同步 GitHub：

```bash
git fetch --all
```

```bash
git reset --hard origin/master
```

注意：这会丢弃 `/opt/vps-monitor` 目录里的本地代码改动。正常部署时，配置文件在 `/etc/vps-monitor-agent.toml` 和 `/etc/vps-monitor.env`，不会被这条命令覆盖。
