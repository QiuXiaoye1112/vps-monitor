# 完整部署流程

本文是从零搭建 `vps-monitor` 的完整步骤。

## 1. 准备条件

- 一台中心 VPS + 一台或多台被监控 VPS
- 域名（可选，没有域名直接使用 IP）
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

# 部署（域名自动 HTTPS，IP 走 HTTP）
sudo bash deploy_panel.sh monitor.example.com $(openssl rand -hex 24)
# 或
sudo bash deploy_panel.sh 1.2.3.4 $(openssl rand -hex 24)

# 检查
systemctl status vps-monitor-api
curl http://127.0.0.1:8000/api/health
```

访问：`https://monitor.example.com` 或 `http://1.2.3.4`

## 4. HTTPS

域名部署时 `deploy_panel.sh` 自动申请 Let's Encrypt 证书。
如失败可手动：

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
```

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

写入 `/etc/vps-monitor.env` 持久化。

## 11. 卸载

```bash
sudo vm → 完整卸载
```
