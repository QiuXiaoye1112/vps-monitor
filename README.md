# VPS Monitor

轻量 VPS 监控面板，适合 512MB 小内存 VPS。中心 VPS 只跑一个 FastAPI 服务，提供 API、SQLite 存储和极简 HTML Dashboard；被监控 VPS 只跑 `agent.py`。

- 不使用 Streamlit、pandas、paramiko
- Dashboard 每 1 秒刷新
- Agent 默认每 1 秒上报
- Dashboard 走域名 + Nginx + HTTPS
- 远程 Agent 推荐走 `http://中心VPS公网IP:8080`

## 架构

```text
浏览器 -> https://monitor.example.com -> Nginx -> FastAPI 127.0.0.1:8000 -> SQLite
中心 VPS Agent -> http://127.0.0.1:8000/api/
远程 VPS Agent -> http://中心VPS公网IP:8080/api/ -> Nginx :8080 -> FastAPI
```

地址用途：

- `127.0.0.1:8000`：中心 VPS 本机 API
- `https://monitor.example.com`：浏览器访问 Dashboard
- `http://中心VPS公网IP:8080`：远程 VPS Agent 上报

## 快速部署中心 VPS

在中心 VPS 安装基础依赖：

```bash
sudo apt-get update
```

```bash
sudo apt-get install -y git python3 python3-venv python3-pip nginx
```

clone 项目：

```bash
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
```

进入目录：

```bash
cd /opt/vps-monitor
```

生成 token：

```bash
openssl rand -hex 24
```

部署面板，把域名和 token 换成你自己的：

```bash
sudo bash deploy_panel.sh monitor.example.com change-this-token
```

检查 API：

```bash
curl http://127.0.0.1:8000/api/health
```

访问 Dashboard：

```text
http://monitor.example.com
```
确保已经将域名的A记录解析到本主机

## 快速开启 HTTPS

在中心 VPS 安装 certbot：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
```

申请证书：

```bash
sudo certbot --nginx -d monitor.example.com
```

Cloudflare 小黄云建议 SSL/TLS 使用 `Full` 或 `Full strict`，不要用 `Flexible`。

## 快速开启远程 Agent 8080 入口

在中心 VPS 执行：

```bash
cd /opt/vps-monitor
```

```bash
sudo bash deploy_agent_ingress.sh
```

只允许远程 VPS IP 访问 8080：

```bash
sudo bash allow_agent_ip.sh 远程VPS_IP
```

## 快速添加一台远程 VPS

在远程 VPS 安装依赖：

```bash
sudo apt-get install -y git python3 python3-venv python3-pip
```

clone 项目：

```bash
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
```

安装 Agent 依赖：

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

创建配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

示例。每台 VPS 的 `node_id` 必须不同，`name` 是 Dashboard 显示名：

```toml
server_url = "http://中心VPS公网IP:8080"
node_id = "vmrack"
token = "change-this-token"
interval = 1

name = "Vmrack 洛杉矶"
os_type = "Linux"

disk_paths = ["/"]
```

安装并启动 Agent：

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

中心 VPS：

```bash
systemctl status vps-monitor-api
```

远程 VPS：

```bash
systemctl status vps-monitor-agent
```

```bash
journalctl -u vps-monitor-agent -f
```

查看节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

## 详细文档

- [完整部署流程](docs/DEPLOYMENT.md)
- [常见问题排查](docs/TROUBLESHOOTING.md)
- [安全说明](docs/SECURITY.md)
- [运维命令](docs/OPERATIONS.md)
