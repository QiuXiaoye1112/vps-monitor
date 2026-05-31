# VPS Monitor

VPS Monitor 是一个适合 512MB 小内存 VPS 长期运行的轻量监控面板。

中心 VPS 只运行一个 FastAPI 服务，负责：

- 接收多台 VPS Agent 上报
- 写入 SQLite
- 提供极简 HTML Dashboard
- 提供查询 API

被监控 VPS 只运行 `agent.py`，定时采集本机 CPU、内存、Swap、磁盘和网络状态，然后通过 HTTP 上报到中心服务端。

本版本不再依赖 Streamlit、pandas 或 paramiko，也不提供 Dashboard 里的 SSH 一键部署 Agent。

## 架构

```text
VPS Agent 1 ┐
VPS Agent 2 ├─ HTTP + Bearer Token ─> FastAPI + SQLite + HTML Dashboard
VPS Agent 3 ┘
```

## 采集内容

- CPU 使用率和核心数
- 内存、Swap 使用率
- 磁盘容量、使用率和多磁盘路径
- 网络当前上传 / 下载速度
- 网络累计上传 / 下载流量
- 最后上报时间

Dashboard 显示：

- 节点名称
- online / offline
- CPU 使用率
- 内存使用率
- 磁盘使用率
- 当前上传速度
- 当前下载速度
- 最后上报时间

页面每 5 秒自动刷新一次，不使用图表、历史曲线或前端框架。

## 依赖

中心服务端：

```text
fastapi
uvicorn
psutil
requests
```

Agent：

```text
psutil
requests
tomli  # Python < 3.11 时需要
```

## 本地运行

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt

export VPS_MONITOR_TOKEN="change-this-token"
export VPS_MONITOR_DB="./vps_monitor.db"

python -m uvicorn server:app --host 0.0.0.0 --port 8000
```

浏览器打开：

```text
http://127.0.0.1:8000
```

健康检查：

```bash
curl http://127.0.0.1:8000/api/health
```

## 域名部署

先把域名 A 记录解析到中心 VPS。

例如：

```text
monitor.example.com -> 中心 VPS 公网 IP
```

把项目上传到中心 VPS 的 `/opt/vps-monitor`，然后执行：

```bash
cd /opt/vps-monitor
sudo bash deploy_panel.sh monitor.example.com change-this-token
```

脚本会创建一个 systemd 服务：

```text
vps-monitor-api.service
```

并用 Nginx 把整个站点反代到：

```text
http://127.0.0.1:8000
```

部署后访问：

```text
http://monitor.example.com
```

如需 HTTPS：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d monitor.example.com
```

## 监控中心 VPS 自己

在中心 VPS 上创建 Agent 配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

示例：

```toml
server_url = "http://127.0.0.1:8000"
node_id = "main"
token = "change-this-token"
interval = 10

name = "Main VPS"
os_type = "Linux"

disk_paths = ["/"]
```

启动 Agent：

```bash
cd /opt/vps-monitor
. .venv/bin/activate
python agent.py --config /etc/vps-monitor-agent.toml --once
```

确认能上报后，可以安装 systemd 服务：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
```

## 监控其他 VPS

在每台被监控 VPS 上上传这些文件：

```text
agent.py
monitor_common.py
settings.py
requirements-agent.txt
vps-monitor-agent.service
```

安装依赖：

```bash
mkdir -p /opt/vps-monitor
cd /opt/vps-monitor
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements-agent.txt
```

创建配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

示例：

```toml
server_url = "https://monitor.example.com"
node_id = "hk-01"
token = "change-this-token"
interval = 10

name = "Hong Kong VPS"
os_type = "Linux"

disk_paths = ["/"]
```

如果有多个磁盘路径：

```toml
disk_paths = ["/", "/data", "/www"]
```

测试上报：

```bash
cd /opt/vps-monitor
. .venv/bin/activate
python agent.py --config /etc/vps-monitor-agent.toml --once
```

安装为后台服务：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
```

## Token

Agent 写入接口需要 token。中心服务端和所有 Agent 必须使用同一个 token：

```bash
export VPS_MONITOR_TOKEN="change-this-token"
```

Agent 会使用：

```text
Authorization: Bearer <token>
```

写入接口 token 不正确会返回 `401`。

Dashboard 读取 `/api/nodes`，不需要在浏览器里配置 token。

## API

保留接口：

- `GET /api/health`
- `POST /api/nodes/register`
- `PUT /api/nodes/{node_id}`
- `GET /api/nodes`
- `GET /api/nodes/{node_id}`
- `POST /api/metrics`
- `GET /api/nodes/{node_id}/metrics?window=5m|1h|24h`

写入接口仍需要 token：

- `POST /api/nodes/register`
- `PUT /api/nodes/{node_id}`
- `POST /api/metrics`

## 常用命令

中心服务状态：

```bash
systemctl status vps-monitor-api
journalctl -u vps-monitor-api -f
```

Agent 状态：

```bash
systemctl status vps-monitor-agent
journalctl -u vps-monitor-agent -f
```

节点离线判断默认是 30 秒。可以通过环境变量调整：

```bash
export VPS_MONITOR_OFFLINE_AFTER=60
```
