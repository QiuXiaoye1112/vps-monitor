# VPS Monitor

轻量 VPS 监控面板，适合 512MB 小内存 VPS。中心 VPS 跑 FastAPI、SQLite、Dashboard、Nginx；被监控 VPS 只跑 `agent.py`。

- 不使用 Streamlit、pandas、paramiko
- Dashboard 每 1 秒刷新，Agent 默认每 1 秒上报
- 对外访问强制走 Nginx
- 没有域名就用中心 VPS 公网 IP

## 架构
```text
浏览器 -> Nginx 80/443 -> FastAPI 127.0.0.1:8000 -> SQLite
远程 Agent -> Nginx 8080 -> FastAPI 127.0.0.1:8000
```
- `127.0.0.1:8000`：中心 VPS 本机 API，不直接暴露公网
- `http://中心VPS公网IP`：没有域名时访问 Dashboard
- `https://monitor.example.com`：有域名和 HTTPS 时访问 Dashboard
- `http://中心VPS公网IP:8080`：远程 VPS Agent 上报

## 快速部署中心 VPS
以下命令都在中心 VPS 执行。直接安装完整依赖，不用先判断有没有。
```bash
sudo apt-get update
```
```bash
sudo apt-get install -y git python3 python3-venv python3-pip nginx curl sqlite3
```
```bash
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
```
```bash
cd /opt/vps-monitor
```
```bash
openssl rand -hex 24
```
有域名：
```bash
sudo bash deploy_panel.sh monitor.example.com change-this-token
```
没有域名就填中心 VPS 公网 IP：
```bash
sudo bash deploy_panel.sh 1.2.3.4 change-this-token
```
检查：
```bash
systemctl status vps-monitor-api
```
```bash
curl http://127.0.0.1:8000/api/health
```
访问：
```text
http://1.2.3.4
```
有域名时，确保已经将域名的 A 记录解析到本机。

## 快速开启本机监控
以下命令都在中心 VPS 执行。本机 Agent 直接上报到 `127.0.0.1:8000`，不走域名、不走 Cloudflare、不走 8080。
```bash
sudo nano /etc/vps-monitor-agent.toml
```
```toml
server_url = "http://127.0.0.1:8000"
node_id = "center"
token = "change-this-token"
interval = 1
name = "中心 VPS"
os_type = "Linux"
disk_paths = ["/"]
```
```bash
.venv/bin/python agent.py --config /etc/vps-monitor-agent.toml --once
```
```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
```
```bash
sudo systemctl daemon-reload
```
```bash
sudo systemctl enable --now vps-monitor-agent
```

## 快速开启 HTTPS
只有绑定域名时才需要 HTTPS。
```bash
sudo apt-get install -y certbot python3-certbot-nginx
```
```bash
sudo certbot --nginx -d monitor.example.com
```
Cloudflare 小黄云建议 SSL/TLS 使用 `Full` 或 `Full strict`，不要用 `Flexible`。

## 快速开启远程 Agent 8080 入口
以下命令都在中心 VPS 执行。
```bash
cd /opt/vps-monitor
```
```bash
sudo bash deploy_agent_ingress.sh
```
```bash
sudo bash allow_agent_ip.sh 远程VPS_IP
```

## 快速添加一台远程 VPS
以下命令都在远程 VPS 执行。
```bash
sudo apt-get update
```
```bash
sudo apt-get install -y git python3 python3-venv python3-pip
```
```bash
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
```
```bash
cd /opt/vps-monitor
```
```bash
python3 -m venv .venv
```
```bash
. .venv/bin/activate
```
```bash
pip install -r requirements-agent.txt
```
```bash
sudo nano /etc/vps-monitor-agent.toml
```
每台 VPS 的 `node_id` 必须不同，`name` 是 Dashboard 显示名：
```toml
server_url = "http://中心VPS公网IP:8080"
node_id = "node-01"
token = "change-this-token"
interval = 1
name = "Example Node 01"
os_type = "Linux"
disk_paths = ["/"]
```
```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
```
```bash
sudo systemctl daemon-reload
```
```bash
sudo systemctl enable --now vps-monitor-agent
```
## 查看token
```bash
sudo grep VPS_MONITOR_TOKEN /etc/vps-monitor.env
```
## 同步 GitHub 最新代码
以下命令会让服务器上的项目文件与 GitHub `master` 保持一致。它会丢弃 `/opt/vps-monitor` 目录里的本地代码改动，但不会覆盖 `/etc/vps-monitor-agent.toml` 和 `/etc/vps-monitor.env`。
```bash
cd /opt/vps-monitor
```
```bash
git fetch --all
```
```bash
git reset --hard origin/master
```
中心 VPS 执行：
```bash
sudo systemctl restart vps-monitor-api
sudo systemctl restart vps-monitor-agent
```
远程 VPS 只需要执行：
```bash
sudo systemctl restart vps-monitor-agent
```
## 常用状态检查
```bash
systemctl status vps-monitor-api
```
```bash
systemctl status vps-monitor-agent
```
```bash
journalctl -u vps-monitor-agent -f
```

## 保活和开机自启
每台需要上报的 VPS 都执行一次。`enable --now` 会立即启动 Agent，并设置重启后自动启动。
```bash
sudo systemctl enable --now vps-monitor-agent
```
确认已经开启自启：
```bash
systemctl is-enabled vps-monitor-agent
```
确认正在运行：
```bash
systemctl status vps-monitor-agent
```

中心 VPS 的 API 服务也建议开启自启：
```bash
sudo systemctl enable --now vps-monitor-api
```

如果你给 8080 设置了 iptables 访问限制，还需要保存防火墙规则，避免重启后丢失：
```bash
sudo apt install -y iptables-persistent
```
```bash
sudo netfilter-persistent save
```
```bash
systemctl status netfilter-persistent
```
```bash
grep 8080 /etc/iptables/rules.v4
```

本项目默认不启用 `ufw`，避免和手动 iptables 规则混用导致 SSH、Nginx 或 8080 入口异常。

## 详细文档
- [完整部署命令清单](docs/FULL_DEPLOY_COMMANDS.md)
- [完整部署流程](docs/DEPLOYMENT.md)
- [常见问题排查](docs/TROUBLESHOOTING.md)
- [安全说明](docs/SECURITY.md)
- [运维命令](docs/OPERATIONS.md)
