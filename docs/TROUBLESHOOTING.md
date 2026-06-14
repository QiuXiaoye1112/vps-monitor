# 常见问题排查

## Dashboard 打不开

1. 确认 DNS 解析到 VPS IP
2. VPS 上检查：`curl http://127.0.0.1:8000/api/health`
3. 检查服务：`systemctl status vps-monitor-api`

## 节点 offline

在对应节点检查 Agent：

```bash
systemctl status vps-monitor-agent
journalctl -u vps-monitor-agent -f
```

手动测试上报：

```bash
/opt/vps-monitor/.venv/bin/python /opt/vps-monitor/agent.py --config /etc/vps-monitor-agent.toml --once
```

检查 token 是否与中心一致：

```bash
cat /etc/vps-monitor-agent.toml | grep token
sudo cat /etc/vps-monitor.env | grep TOKEN
```

## Agent 401

token 不一致。确保中心和 Agent 的 token 相同。改完后重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

## Agent 404 / 无法连接

检查 `server_url` 是否正确。远程 Agent 应使用：

```toml
server_url = "http://中心VPS公网IP:8080"
```

而不是域名（域名可能走了 HTTPS/CDN）。

测试连通性：

```bash
curl -i http://中心VPS公网IP:8080/api/health
```

## 验证入口卡住

旧 iptables DROP 规则拦截了本地回路。手动删除后重新放行：

```bash
iptables -D INPUT -p tcp --dport 8080 -j DROP
```

然后用菜单重新配置白名单：`sudo vm` → 监控主机 → 选主机 → 允许访问。

## 更新不生效

更新是写到磁盘的，需要退出 `sudo vm` 再重新打开。

如果更新本身失败（网络问题），手动执行：

```bash
cd /opt/vps-monitor && git fetch origin && git reset --hard origin/master
```

## 节点 ID 冲突

两台机器用了相同的 `node_id`：

```bash
grep node_id /etc/vps-monitor-agent.toml
```

每台必须不同。改完重启 Agent。
