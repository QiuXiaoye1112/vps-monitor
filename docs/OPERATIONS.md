# 运维命令

日常运维通过 `sudo vm` 菜单即可完成，以下为手动命令备查。

## 查看运行状态

中心 VPS：

```bash
systemctl status vps-monitor-api
journalctl -u vps-monitor-api -f
curl http://127.0.0.1:8000/api/health
```

任意节点：

```bash
systemctl status vps-monitor-agent
journalctl -u vps-monitor-agent -f
```

## 开机自启

```bash
sudo systemctl enable --now vps-monitor-api    # 中心
sudo systemctl enable --now vps-monitor-agent  # 所有节点
```

## 修改 Agent 配置

```bash
sudo nano /etc/vps-monitor-agent.toml
# 改完重启
sudo systemctl restart vps-monitor-agent
```

## 数据库

```bash
# 查看所有节点
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"

# 备份
cp /opt/vps-monitor/vps_monitor.db /opt/vps-monitor/vps_monitor.db.bak.$(date +%Y%m%d%H%M%S)

# 删除脏节点
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM nodes WHERE id NOT IN ('center','node-01');"
```

## 更新

```bash
cd /opt/vps-monitor && git fetch origin && git reset --hard origin/master
```

或使用菜单：`sudo vm` → 更新程序

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| VPS_MONITOR_AGENT_PORT | 8080 | Agent 入口端口 |
| VPS_MONITOR_API_HOST | 127.0.0.1 | API 监听地址 |
| VPS_MONITOR_API_PORT | 8000 | API 监听端口 |
| VPS_MONITOR_METRIC_RETENTION_DAYS | 2 | 原始指标保留天数，0 表示不清理 |
| VPS_MONITOR_METRIC_CLEANUP_INTERVAL_SECONDS | 3600 | 指标清理检查间隔 |

持久化到 `/etc/vps-monitor.env` 后 systemd 自动读取。
