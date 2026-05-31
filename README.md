# VPS Monitor

VPS Monitor 是一个适合 512MB 小内存 VPS 长期运行的轻量监控面板。项目已经从 Streamlit 管理面板改成 FastAPI 一体化方案：

- 中心 VPS 运行一个 FastAPI 服务，监听 `127.0.0.1:8000`
- FastAPI 同时提供极简 HTML Dashboard、API 和 SQLite 写入
- Dashboard 通过 Nginx + HTTPS 暴露给浏览器访问
- 中心 VPS 自己的 Agent 上报到 `http://127.0.0.1:8000`
- 其他远程 VPS Agent 推荐上报到 `http://中心VPS公网IP:8080`
- `8080` 只开放 `/api/`，其他路径返回 `404`
- `8080` 应通过 iptables 只允许指定远程 VPS IP 访问

本项目不再依赖 Streamlit、pandas、paramiko，也不提供 Dashboard SSH 一键部署 Agent。

## 架构说明

```text
浏览器
  │
  ├─ HTTPS https://monitor.example.com
  │       │
  │       └─ Nginx :443/:80 ──> FastAPI 127.0.0.1:8000 ──> SQLite
  │                                  ▲
  │                                  │
中心 VPS Agent ── http://127.0.0.1:8000/api/
远程 VPS Agent ── http://中心VPS公网IP:8080/api/ ── Nginx :8080 ┘
```

Dashboard 每 1 秒自动刷新一次。Agent 默认每 1 秒上报一次。

Dashboard 显示：

- 节点名称
- online / offline
- CPU 使用率
- 内存使用率
- 磁盘使用率
- 运行时间
- 当前上传速度
- 当前下载速度
- 最后上报时间

## 中心 VPS 部署

把项目上传到中心 VPS：

```bash
sudo mkdir -p /opt/vps-monitor
sudo chown -R "$USER":"$USER" /opt/vps-monitor
cd /opt/vps-monitor
```

确保目录里至少有：

```text
server.py
agent.py
monitor_common.py
storage.py
settings.py
deploy_panel.sh
deploy_agent_ingress.sh
allow_agent_ip.sh
requirements.txt
requirements-agent.txt
vps-monitor-agent.service
```

执行中心面板部署：

```bash
cd /opt/vps-monitor
sudo bash deploy_panel.sh monitor.example.com change-this-token
```

脚本会：

- 安装中心服务依赖
- 创建 `/etc/vps-monitor.env`
- 创建并启动 `vps-monitor-api.service`
- 让 FastAPI 监听 `127.0.0.1:8000`
- 创建 Nginx 站点，把 Dashboard 域名反代到 `127.0.0.1:8000`
- 如果旧版 `vps-monitor-dashboard.service` 存在，会尝试停用并删除

中心服务检查：

```bash
systemctl status vps-monitor-api
journalctl -u vps-monitor-api -f
curl http://127.0.0.1:8000/api/health
```

## HTTPS 部署

先把域名 A 记录解析到中心 VPS 公网 IP：

```text
monitor.example.com -> 中心VPS公网IP
```

Debian/Ubuntu 可以使用 certbot：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d monitor.example.com
```

之后浏览器访问：

```text
https://monitor.example.com
```

说明：Dashboard 适合走 HTTPS 域名。远程 Agent 上报不推荐默认走 `https://monitor.example.com`，因为 Cloudflare、反代缓存、防火墙或 HTTPS 配置都可能让 1 秒上报变得不稳定。除非你明确确认该链路稳定，否则远程 Agent 推荐走 `http://中心VPS公网IP:8080`。

## 开启远程 Agent 8080 上报入口

在中心 VPS 上运行：

```bash
cd /opt/vps-monitor
sudo bash deploy_agent_ingress.sh
```

默认监听端口是 `8080`。如果要换端口：

```bash
sudo AGENT_PORT=18080 bash deploy_agent_ingress.sh
```

该脚本会创建：

```text
/etc/nginx/sites-available/vps-monitor-agent.conf
/etc/nginx/sites-enabled/vps-monitor-agent.conf
```

Nginx 行为：

- `http://中心VPS公网IP:8080/api/` 反代到 `http://127.0.0.1:8000/api/`
- `http://中心VPS公网IP:8080/` 返回 `404`
- 其他非 `/api/` 路径返回 `404`

检查：

```bash
curl http://127.0.0.1:8080/api/health
curl -i http://127.0.0.1:8080/
```

## 8080 安全限制

`8080` 是给远程 VPS Agent 上报用的入口，应只允许你的远程 VPS IP 访问。

放行一个远程 VPS：

```bash
sudo bash allow_agent_ip.sh 1.2.3.4
```

多台 VPS 就重复执行：

```bash
sudo bash allow_agent_ip.sh 2.2.2.2
sudo bash allow_agent_ip.sh 3.3.3.3
```

脚本会：

- 允许指定 IP 访问 TCP `8080`
- 拒绝其他 IP 访问 TCP `8080`
- 输出当前与 `8080` 相关的 iptables 规则

注意：脚本只写当前运行中的 iptables 规则，重启后可能丢失。它不会自动安装 `iptables-persistent`，也不会自动启用 ufw。确认规则没问题后，你可以按自己的系统方式持久化。

查看当前规则：

```bash
sudo iptables -S INPUT | grep -- '--dport 8080'
```

## 中心 VPS 自监控 Agent 配置

中心 VPS 自己的 Agent 直接走本机 FastAPI，不经过 Nginx：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

```toml
server_url = "http://127.0.0.1:8000"
node_id = "main"
token = "change-this-token"
interval = 1

name = "中心 VPS"
os_type = "Linux"

disk_paths = ["/"]
```

测试上报：

```bash
cd /opt/vps-monitor
. .venv/bin/activate
python agent.py --config /etc/vps-monitor-agent.toml --once
```

安装为 systemd：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
```

## 远程 VPS Agent 配置

在每台远程 VPS 上上传 Agent 需要的文件：

```text
agent.py
monitor_common.py
settings.py
requirements-agent.txt
vps-monitor-agent.service
```

安装依赖：

```bash
sudo mkdir -p /opt/vps-monitor
sudo chown -R "$USER":"$USER" /opt/vps-monitor
cd /opt/vps-monitor
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements-agent.txt
```

创建配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

推荐示例：

```toml
server_url = "http://中心VPS公网IP:8080"
node_id = "vmrack"
token = "change-this-token"
interval = 1

name = "Vmrack 洛杉矶"
os_type = "Linux"

disk_paths = ["/"]
```

多个磁盘路径：

```toml
disk_paths = ["/", "/data", "/www"]
```

测试上报：

```bash
cd /opt/vps-monitor
. .venv/bin/activate
python agent.py --config /etc/vps-monitor-agent.toml --once
```

安装为 systemd：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
```

Agent 日志：

```bash
journalctl -u vps-monitor-agent -f
```

## 如何修改节点显示名

节点显示名来自 Agent 配置里的 `name`。

编辑对应 VPS 上的配置：

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

Agent 会重新注册节点，Dashboard 下一次刷新后显示新名称。

## 如何删除脏节点

脏节点通常是测试时留下的旧 `node_id`。在中心 VPS 上用 SQLite 删除。

先备份数据库：

```bash
cd /opt/vps-monitor
cp vps_monitor.db "vps_monitor.db.bak.$(date +%Y%m%d%H%M%S)"
```

查看节点：

```bash
sqlite3 vps_monitor.db "SELECT id,name,last_seen_at FROM nodes ORDER BY created_at DESC;"
```

删除指定节点和历史指标：

```bash
sqlite3 vps_monitor.db "DELETE FROM metrics WHERE node_id='old-node-id'; DELETE FROM nodes WHERE id='old-node-id';"
```

刷新 Dashboard 即可。

如果系统没有 `sqlite3` 命令：

```bash
sudo apt-get install -y sqlite3
```

## 如何同步 GitHub 最新代码到服务器

如果服务器上是 Git clone 的仓库：

```bash
cd /opt/vps-monitor
git pull
. .venv/bin/activate
pip install -r requirements.txt
sudo systemctl restart vps-monitor-api
```

如果 Agent 文件也更新了，远程 VPS 上同步后重启 Agent：

```bash
cd /opt/vps-monitor
git pull
. .venv/bin/activate
pip install -r requirements-agent.txt
sudo systemctl restart vps-monitor-agent
```

如果不是 Git clone，而是手动上传文件，请覆盖代码后重启对应服务：

```bash
sudo systemctl restart vps-monitor-api
sudo systemctl restart vps-monitor-agent
```

## 常用排错命令

中心服务：

```bash
systemctl status vps-monitor-api
journalctl -u vps-monitor-api -f
curl http://127.0.0.1:8000/api/health
```

Dashboard 反代：

```bash
nginx -t
systemctl status nginx
curl -I http://127.0.0.1:8000/
curl -I https://monitor.example.com
```

8080 Agent 入口：

```bash
curl http://127.0.0.1:8080/api/health
curl -i http://127.0.0.1:8080/
sudo iptables -S INPUT | grep -- '--dport 8080'
```

Agent：

```bash
systemctl status vps-monitor-agent
journalctl -u vps-monitor-agent -f
python /opt/vps-monitor/agent.py --config /etc/vps-monitor-agent.toml --once
```

查看 API 数据：

```bash
curl http://127.0.0.1:8000/api/nodes
```

返回 `401` 时，检查中心服务和 Agent 的 `token` 是否一致。

节点离线时，检查：

- Agent 是否运行
- `server_url` 是否能从该 VPS 访问
- 中心 VPS 的 8080 是否放行该远程 VPS IP
- `token` 是否一致
- `VPS_MONITOR_OFFLINE_AFTER` 是否设置得太短

默认离线阈值是 30 秒，可以在中心 VPS 的 `/etc/vps-monitor.env` 中增加：

```bash
VPS_MONITOR_OFFLINE_AFTER=60
```

然后重启：

```bash
sudo systemctl restart vps-monitor-api
```

## API

核心接口：

- `GET /api/health`
- `POST /api/nodes/register`
- `POST /api/metrics`
- `GET /api/nodes`

仍然保留：

- `PUT /api/nodes/{node_id}`
- `GET /api/nodes/{node_id}`
- `GET /api/nodes/{node_id}/metrics?window=5m|1h|24h`

写入接口需要：

```text
Authorization: Bearer <token>
```

Dashboard 读取 `/api/nodes`，浏览器端不保存 token。
