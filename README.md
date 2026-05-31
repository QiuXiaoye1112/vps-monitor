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
没有域名：
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
确保已经将域名的A记录解析到本主机

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
node_id = "vmrack"
token = "change-this-token"
interval = 1
name = "Vmrack 洛杉矶"
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

## 详细文档
- [完整部署流程](docs/DEPLOYMENT.md)
- [常见问题排查](docs/TROUBLESHOOTING.md)
- [安全说明](docs/SECURITY.md)
- [运维命令](docs/OPERATIONS.md)
