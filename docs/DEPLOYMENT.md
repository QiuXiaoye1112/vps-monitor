# 完整部署流程

本文是从零搭建 `vps-monitor` 的完整步骤。

## 1. 准备条件

- 一台中心 VPS + 一台或多台被监控 VPS
- 一个已解析到中心 VPS 的域名（Dashboard 不支持使用 IP 部署）
- Python 3.8+

角色：
- 中心 VPS：FastAPI + SQLite + Dashboard + Nginx
- 被监控 VPS：只运行 `agent.py`
- 中心 VPS 也可以监控自己

架构：
- `:80/443` → Nginx → `127.0.0.1:8000`（Dashboard）
- Agent 入口（默认 `:8080`）→ Nginx → `127.0.0.1:8000`（仅 `/api/`，端口可配）

## 2. 一键安装（推荐）

```bash
curl -fsSL https://raw.githubusercontent.com/QiuXiaoye1112/vps-monitor/master/install.sh | bash
```

安装后管理：`sudo vm`

## 3. 中心 VPS 手动部署

```bash
# 安装依赖
sudo apt-get update && sudo apt-get install -y git python3 python3-venv python3-pip nginx curl sqlite3

# clone 项目（deploy_panel.sh 也会自动 clone，可跳过）
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
cd /opt/vps-monitor

# 使用域名部署
sudo bash deploy_panel.sh monitor.example.com $(openssl rand -hex 24)

# 检查
systemctl status vps-monitor-api
curl http://127.0.0.1:8000/api/health
```

访问：`http://monitor.example.com`，随后可通过 `sudo vm` 配置 SSL。

## 4. HTTPS

部署完成后可通过 `sudo vm` → SSL 证书设置配置证书，或手动执行：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d monitor.example.com
```

## 5. 本机 Agent 配置

推荐使用管理面板：`sudo vm` → 安装中心 VPS 本机监控

或手动：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

```toml
server_url = "http://127.0.0.1:8000"
node_id = "center"
token = "你部署面板时设置的 token"
interval = 1
name = "中心 VPS"
os_type = "Linux"
disk_paths = ["/"]
traffic_reset_day = 0
traffic_reset_hour = 0
traffic_limit_gb = 0
traffic_state_path = "/var/lib/vps-monitor-agent/traffic-state.json"
```

```bash
cd /opt/vps-monitor
.venv/bin/python agent.py --config /etc/vps-monitor-agent.toml --once  # 测试
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload && sudo systemctl enable --now vps-monitor-agent
```

## 6. 远程 VPS Agent

推荐：`sudo vm` → 作为监控节点

或手动：

```bash
sudo apt-get update && sudo apt-get install -y git python3 python3-venv python3-pip
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
cd /opt/vps-monitor
python3 -m venv .venv && . .venv/bin/activate && pip install -r requirements-agent.txt

sudo nano /etc/vps-monitor-agent.toml
```

```toml
server_url = "http://中心VPS公网IP:8080"
node_id = "node-01"
token = "你部署面板时设置的 token"
interval = 1
name = "Example Node"
os_type = "Linux"
disk_paths = ["/"]
traffic_reset_day = 0
traffic_reset_hour = 4
traffic_limit_gb = 0
traffic_state_path = "/var/lib/vps-monitor-agent/traffic-state.json"
```

流量账期配置保存在每台 Agent 自己的 TOML 中，因此不同主机可以使用不同的重置日和小时。`traffic_reset_day = 0` 表示持续累计、不自动重置；`traffic_limit_gb = 0` 表示无上限，面板显示已使用流量和固定满格进度条，但不显示百分比。如果不设置重置时间但设置了流量上限，面板仍会显示累计占用、额度百分比和进度条，但不会按月清零。

面板流量统一按 GB 显示。有流量上限时，前端保留 2 GB 余量：达到“上限减 2 GB”之前按实际流量和实际百分比显示，达到阈值后直接显示上限和 `100%`。例如上限 500 GB 时，实际使用达到 498 GB 后显示 `500 GB / 500 GB`。上限不超过 2 GB 时按实际上限计算。上传下载明细及 Agent 保存的实际流量不会被修改。

当前已使用流量不在 Agent 上设置。请在中心 VPS 执行 `sudo vm`，进入“监控主机”选择节点后设置；本机和远程节点都支持，默认值为 0 GB。网页流量等于 Agent 实际上报流量加中心设置值。中心设置值只在当前账期有效，并在该节点下次到达自己的流量重置日和小时时自动归零；未配置月重置的节点不能设置正数。

从旧版升级后，Agent 会丢弃旧格式中可能混有初始流量的状态并从 0 开始统计。需要保留的当月既有用量请在中心 VPS 手动设置。

```bash
.venv/bin/python agent.py --config /etc/vps-monitor-agent.toml --once  # 测试
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload && sudo systemctl enable --now vps-monitor-agent
journalctl -u vps-monitor-agent -f
```

## 7. Agent 入口

中心 VPS，开放 Agent 上报端口（默认 8080）：

```bash
sudo bash /opt/vps-monitor/deploy_agent_ingress.sh
# 自定义端口：sudo AGENT_PORT=9090 bash /opt/vps-monitor/deploy_agent_ingress.sh
```

## 8. IP 白名单

推荐通过菜单管理：`sudo vm` → 监控主机 → 选主机 → 允许访问（自动持久化）

手动（端口根据配置替换）：

```bash
sudo iptables -I INPUT -p tcp -s 远程VPS_IP --dport 8080 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8080 ! -i lo -j DROP
```

## 9. 管理命令

```bash
sudo vm    # 打开管理面板
```

菜单覆盖：运行状态、token 查看、监控主机管理、添加新主机、防火墙配置、重新部署、HTTPS 开关、更新、卸载。

## 10. 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `VPS_MONITOR_API_HOST` | `127.0.0.1` | API 监听地址 |
| `VPS_MONITOR_API_PORT` | `8000` | API 监听端口 |
| `VPS_MONITOR_AGENT_PORT` | `8080` | Agent 入口端口 |
| `VPS_MONITOR_METRIC_RETENTION_DAYS` | `2` | 原始指标保留天数，`0` 表示不自动清理 |
| `VPS_MONITOR_METRIC_CLEANUP_INTERVAL_SECONDS` | `3600` | 指标清理检查间隔 |

写入 `/etc/vps-monitor.env` 持久化。

## 11. 卸载

```bash
sudo vm → 完整卸载
```
