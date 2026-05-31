# 完整部署流程

本文是从零搭建 `vps-monitor` 的完整步骤。

## 1. 准备条件

你需要：

- 一台中心 VPS
- 一台或多台被监控 VPS
- 一个域名，例如 `monitor.example.com`
- Python 3
- Nginx
- Git
- 一个随机 token

角色区别：

- 中心 VPS：运行 FastAPI、SQLite、Dashboard、Nginx
- 被监控 VPS：只运行 `agent.py`
- 中心 VPS 也可以监控自己

地址用途：

- `127.0.0.1:8000`：中心 VPS 本机 API
- `https://monitor.example.com`：浏览器访问 Dashboard
- `http://中心VPS公网IP:8080`：远程 VPS Agent 上报

生成 token：

```bash
openssl rand -hex 24
```

## 2. 中心 VPS 部署

以下命令都在中心 VPS 执行。

安装依赖：

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

运行部署脚本：

```bash
sudo bash deploy_panel.sh monitor.example.com change-this-token
```

检查服务：

```bash
systemctl status vps-monitor-api
```

检查 API：

```bash
curl http://127.0.0.1:8000/api/health
```

访问 Dashboard：

```text
http://monitor.example.com
```

## 3. HTTPS 配置

以下命令都在中心 VPS 执行。

安装 certbot：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
```

申请证书：

```bash
sudo certbot --nginx -d monitor.example.com
```

选择重定向后，HTTP 会自动跳转 HTTPS。

如果使用 Cloudflare 小黄云：

- SSL/TLS 推荐 `Full` 或 `Full strict`
- 不推荐 `Flexible`

Dashboard 可以走 Cloudflare；远程 Agent 默认建议走 `http://中心VPS公网IP:8080`。

## 4. 中心 VPS 自监控

以下命令都在中心 VPS 执行。

中心 VPS 自己不需要绕域名、Cloudflare、HTTPS，直接使用本机 FastAPI：

```toml
server_url = "http://127.0.0.1:8000"
```

创建配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

示例：

```toml
server_url = "http://127.0.0.1:8000"
node_id = "vmrack"
token = "change-this-token"
interval = 1

name = "Vmrack 洛杉矶"
os_type = "Linux"

disk_paths = ["/"]
```

测试上报：

```bash
cd /opt/vps-monitor
```

```bash
. .venv/bin/activate
```

```bash
python agent.py --config /etc/vps-monitor-agent.toml --once
```

安装为 systemd：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
```

```bash
sudo systemctl daemon-reload
```

```bash
sudo systemctl enable --now vps-monitor-agent
```

## 5. 远程 VPS Agent 部署

以下命令都在被监控 VPS 执行。

安装依赖：

```bash
sudo apt-get update
```

```bash
sudo apt-get install -y git python3 python3-venv python3-pip
```

clone 项目：

```bash
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
```

进入目录：

```bash
cd /opt/vps-monitor
```

创建虚拟环境：

```bash
python3 -m venv .venv
```

进入虚拟环境：

```bash
. .venv/bin/activate
```

安装 Agent 依赖：

```bash
pip install -r requirements-agent.txt
```

创建配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

推荐配置：

```toml
server_url = "http://中心VPS公网IP:8080"
node_id = "vmrack"
token = "change-this-token"
interval = 1

name = "Vmrack 洛杉矶"
os_type = "Linux"

disk_paths = ["/"]
```

测试：

```bash
python agent.py --config /etc/vps-monitor-agent.toml --once
```

安装 systemd：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
```

```bash
sudo systemctl daemon-reload
```

```bash
sudo systemctl enable --now vps-monitor-agent
```

查看日志：

```bash
journalctl -u vps-monitor-agent -f
```

## 6. Agent 专用 8080 入口

以下命令都在中心 VPS 执行。

为什么需要 8080：

- Dashboard 走 HTTPS + Cloudflare
- Agent 上报走中心 VPS IP:8080
- 绕开 Cloudflare 规则和 HTTPS 反代问题
- `8080` 只开放 `/api/`
- 其他路径返回 `404`

开启入口：

```bash
cd /opt/vps-monitor
```

```bash
sudo bash deploy_agent_ingress.sh
```

检查 `/api/`：

```bash
curl http://127.0.0.1:8080/api/health
```

检查根路径返回 404：

```bash
curl -i http://127.0.0.1:8080/
```

## 7. 8080 IP 限制

以下命令都在中心 VPS 执行。

先在远程 VPS 获取公网 IP：

```bash
curl -4 ifconfig.me
```

在中心 VPS 放行这个 IP：

```bash
sudo iptables -I INPUT -p tcp -s 远程VPS_IP --dport 8080 -j ACCEPT
```

拒绝其他 IP：

```bash
sudo iptables -A INPUT -p tcp --dport 8080 -j DROP
```

也可以使用项目脚本：

```bash
sudo bash allow_agent_ip.sh 远程VPS_IP
```

注意：iptables 规则重启后会丢。确认稳定后再考虑持久化，不要默认自动安装 `iptables-persistent`。

## 8. 多 VPS 添加流程

每台 VPS 使用不同 `node_id`。

第一台：

```toml
node_id = "vmrack"
name = "Vmrack 洛杉矶"
```

第二台：

```toml
node_id = "vimss"
name = "Vimss 洛杉矶"
```

每台远程 VPS 重复“远程 VPS Agent 部署”流程，改掉 `node_id` 和 `name` 即可。
