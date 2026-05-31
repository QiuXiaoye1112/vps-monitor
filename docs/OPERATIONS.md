# 运维命令

## 查看 API 状态

在中心 VPS 执行：

```bash
systemctl status vps-monitor-api
```

查看 API 日志：

```bash
journalctl -u vps-monitor-api -f
```

健康检查：

```bash
curl http://127.0.0.1:8000/api/health
```

## 查看 Agent 状态

在被监控 VPS 执行：

```bash
systemctl status vps-monitor-agent
```

## 查看 Agent 日志

在被监控 VPS 执行：

```bash
journalctl -u vps-monitor-agent -f
```

## 保活和开机自启

每台需要上报的 VPS 都要启用 Agent 自启。机器宕机或重启后，systemd 会自动拉起 `vps-monitor-agent` 继续上报。

在被监控 VPS 执行：

```bash
sudo systemctl enable --now vps-monitor-agent
```

确认已经设置为开机自启：

```bash
systemctl is-enabled vps-monitor-agent
```

确认当前正在运行：

```bash
systemctl status vps-monitor-agent
```

如果 Agent 配置改过，重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

中心 VPS 的 API 服务也建议保持开机自启：

```bash
sudo systemctl enable --now vps-monitor-api
```

确认中心 API 自启：

```bash
systemctl is-enabled vps-monitor-api
```

## 查看节点数据库

在中心 VPS 执行：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

## 删除脏节点

先备份数据库：

```bash
cp /opt/vps-monitor/vps_monitor.db "/opt/vps-monitor/vps_monitor.db.bak.$(date +%Y%m%d%H%M%S)"
```

只保留指定节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM nodes WHERE id NOT IN ('vmrack','vimss');"
```

清理旧指标：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM metrics WHERE node_id NOT IN ('vmrack','vimss');"
```

## 修改显示名

在对应 VPS 编辑 Agent 配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

修改：

```toml
name = "新的显示名"
```

重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

## 同步 GitHub 最新代码

在中心 VPS 或远程 VPS 执行：

```bash
cd /opt/vps-monitor
```

拉取远端状态：

```bash
git fetch --all
```

强制同步到远端 master：

```bash
git reset --hard origin/master
```

注意：这会丢弃服务器当前目录里的本地代码改动。

中心 VPS 同步后安装中心依赖：

```bash
. .venv/bin/activate
```

```bash
pip install -r requirements.txt
```

远程 VPS 同步后安装 Agent 依赖：

```bash
. .venv/bin/activate
```

```bash
pip install -r requirements-agent.txt
```

## 重启服务

中心 VPS 重启 API：

```bash
sudo systemctl restart vps-monitor-api
```

被监控 VPS 重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

重载 Nginx：

```bash
sudo nginx -t
```

```bash
sudo systemctl reload nginx
```
