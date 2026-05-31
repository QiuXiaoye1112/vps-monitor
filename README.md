# VPS Monitor

这是一个给小内存 VPS 用的轻量监控面板。中心 VPS 只跑一个 FastAPI 服务，FastAPI 同时提供 API、SQLite 存储和极简 HTML Dashboard。被监控 VPS 只跑 `agent.py`，每 1 秒上报一次状态。

项目当前方案：

- Dashboard 每 1 秒刷新
- Agent 默认每 1 秒上报
- Dashboard 通过域名 + Nginx + HTTPS 访问
- FastAPI 只监听中心 VPS 本机的 `127.0.0.1:8000`
- 中心 VPS 自己的 Agent 直接上报 `http://127.0.0.1:8000`
- 远程 VPS Agent 推荐上报 `http://中心VPS公网IP:8080`

## 1. 准备条件

你需要准备：

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

地址怎么用：

- `127.0.0.1:8000`：只给中心 VPS 本机使用，FastAPI 实际监听这里
- `https://monitor.example.com`：给浏览器访问 Dashboard
- `http://中心VPS公网IP:8080`：给远程 VPS Agent 上报数据

## 架构说明

```text
浏览器
  │
  └─ https://monitor.example.com
       │
       └─ Nginx :443/:80
            │
            └─ FastAPI 127.0.0.1:8000
                 │
                 └─ SQLite /opt/vps-monitor/vps_monitor.db

中心 VPS Agent
  │
  └─ http://127.0.0.1:8000/api/

远程 VPS Agent
  │
  └─ http://中心VPS公网IP:8080/api/
       │
       └─ Nginx :8080 只允许 /api/
            │
            └─ FastAPI 127.0.0.1:8000
```

生成一个 token。这个 token 要在中心服务和所有 Agent 配置里保持一致。

在中心 VPS 执行：

```bash
openssl rand -hex 24
```

如果没有 `openssl`，也可以自己生成一串足够复杂的随机字符串，例如：

```text
vps_9KxP3mQ7zR2tL8sA6nD4
```

## 2. 中心 VPS 部署流程

以下命令都在中心 VPS 执行。

先安装基础依赖。Debian/Ubuntu 示例：

```bash
sudo apt-get update
```

```bash
sudo apt-get install -y git python3 python3-venv python3-pip nginx
```

创建项目目录：

```bash
sudo mkdir -p /opt/vps-monitor
```

把目录权限交给当前用户，方便后面 `git clone`：

```bash
sudo chown -R "$USER":"$USER" /opt/vps-monitor
```

clone 仓库：

```bash
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
```

进入项目目录：

```bash
cd /opt/vps-monitor
```

运行中心面板部署脚本。把域名和 token 换成你自己的：

```bash
sudo bash deploy_panel.sh monitor.example.com change-this-token
```

这个脚本会创建并启动：

```text
vps-monitor-api.service
```

FastAPI 会监听：

```text
127.0.0.1:8000
```

检查 API 服务状态：

```bash
systemctl status vps-monitor-api
```

检查本机 API 健康状态：

```bash
curl http://127.0.0.1:8000/api/health
```

正常会返回：

```json
{"status":"ok"}
```

部署脚本会先创建 HTTP 站点。DNS 已经解析到中心 VPS 后，可以先访问：

```text
http://monitor.example.com
```

如果打不开，先检查 Nginx：

```bash
nginx -t
```

```bash
systemctl status nginx
```

## 3. HTTPS 配置流程

以下命令都在中心 VPS 执行。

安装 certbot：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
```

申请证书。把域名换成你自己的：

```bash
sudo certbot --nginx -d monitor.example.com
```

按 certbot 提示选择即可。选择重定向后，HTTP 会自动跳转 HTTPS。

之后浏览器访问：

```text
https://monitor.example.com
```

如果你使用 Cloudflare 小黄云：

- SSL/TLS 推荐使用 `Full` 或 `Full strict`
- 不推荐使用 `Flexible`

原因是 `Flexible` 会让 Cloudflare 到源站之间走 HTTP，容易造成跳转、协议判断和反代异常。Dashboard 可以走 Cloudflare；Agent 上报建议走中心 VPS 公网 IP 的 `8080`，不要默认走 Cloudflare 域名。

## 4. 中心 VPS 自监控流程

以下命令都在中心 VPS 执行。

中心 VPS 自己不需要绕域名、Cloudflare、HTTPS。它直接打本机 FastAPI 最稳定：

```toml
server_url = "http://127.0.0.1:8000"
```

创建 Agent 配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

示例配置：

```toml
server_url = "http://127.0.0.1:8000"
node_id = "每台ID不能相同"
token = "change-this-token"
interval = 1

name = "主机名称"
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

安装 Agent systemd 服务：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
```

```bash
sudo systemctl daemon-reload
```

```bash
sudo systemctl enable --now vps-monitor-agent
```

查看 Agent 状态：

```bash
systemctl status vps-monitor-agent
```

查看 Agent 日志：

```bash
journalctl -u vps-monitor-agent -f
```

## 5. 远程 VPS Agent 部署流程

以下命令都在被监控 VPS 执行，不是在中心 VPS 执行。

安装基础依赖。Debian/Ubuntu 示例：

```bash
sudo apt-get update
```

```bash
sudo apt-get install -y git python3 python3-venv python3-pip
```

创建项目目录：

```bash
sudo mkdir -p /opt/vps-monitor
```

把目录权限交给当前用户：

```bash
sudo chown -R "$USER":"$USER" /opt/vps-monitor
```

clone 仓库：

```bash
git clone https://github.com/QiuXiaoye1112/vps-monitor.git /opt/vps-monitor
```

进入项目目录：

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

创建 Agent 配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

远程 VPS 推荐配置。把 `中心VPS公网IP` 和 token 换成你自己的：

```toml
server_url = "http://中心VPS公网IP:8080"
node_id = "vmrack"
token = "change-this-token"
interval = 1

name = "Vmrack 洛杉矶"
os_type = "Linux"

disk_paths = ["/"]
```

不要默认推荐远程 Agent 使用：

```toml
server_url = "https://monitor.example.com"
```

原因是 Cloudflare、HTTPS、Nginx 跳转或反代规则不一致时，Agent 可能遇到 `404`、重定向或连接不稳定。远程 Agent 默认走 `http://中心VPS公网IP:8080` 更直、更稳、更容易排错。

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

安装 systemd 服务：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
```

```bash
sudo systemctl daemon-reload
```

```bash
sudo systemctl enable --now vps-monitor-agent
```

查看 Agent 状态：

```bash
systemctl status vps-monitor-agent
```

查看 Agent 日志：

```bash
journalctl -u vps-monitor-agent -f
```

## 6. Agent 专用 8080 入口

以下命令都在中心 VPS 执行。

为什么需要 8080：

- Dashboard 走 HTTPS + Cloudflare
- Agent 上报走中心 VPS IP:8080
- 这样绕开 Cloudflare 规则和 HTTPS 反代问题
- `8080` 只开放 `/api/`
- 其他路径返回 `404`

开启 8080 Agent 入口：

```bash
cd /opt/vps-monitor
```

```bash
sudo bash deploy_agent_ingress.sh
```

这个脚本会创建 Nginx 配置：

```text
/etc/nginx/sites-available/vps-monitor-agent.conf
```

并启用：

```text
/etc/nginx/sites-enabled/vps-monitor-agent.conf
```

检查 `/api/` 是否可用：

```bash
curl http://127.0.0.1:8080/api/health
```

检查根路径是否返回 `404`：

```bash
curl -i http://127.0.0.1:8080/
```

如果要改端口，例如 `18080`：

```bash
sudo AGENT_PORT=18080 bash deploy_agent_ingress.sh
```

## 7. 8080 安全限制

以下命令都在中心 VPS 执行。

`8080` 不应该全网裸开。它只应该允许你的远程 VPS 公网 IP 访问。

先在远程 VPS 上查看它的公网 IP：

```bash
curl -4 ifconfig.me
```

或者：

```bash
curl -4 ip.sb
```

然后在中心 VPS 放行这个 IP 访问 `8080`。示例：

```bash
sudo iptables -I INPUT -p tcp -s 远程VPS_IP --dport 8080 -j ACCEPT
```

拒绝其他 IP 访问 `8080`：

```bash
sudo iptables -A INPUT -p tcp --dport 8080 -j DROP
```

项目也提供了脚本，可以完成同样的操作：

```bash
cd /opt/vps-monitor
```

```bash
sudo bash allow_agent_ip.sh 远程VPS_IP
```

查看当前 `8080` 规则：

```bash
sudo iptables -S INPUT | grep -- '--dport 8080'
```

注意：

- iptables 规则重启后会丢
- 确认稳定后再考虑持久化
- 本项目不会默认自动安装 `iptables-persistent`
- 本项目不会默认启用 ufw

## 8. 节点命名规则

`node_id` 是内部唯一身份 ID。

`name` 是 Dashboard 显示名。

每台 VPS 的 `node_id` 必须不同。`name` 可以自由改。

不要让两台机器使用同一个 `node_id`，否则会出现名称来回跳、指标互相覆盖、最后上报时间混乱。

第一台示例：

```toml
node_id = "vmrack"
name = "Vmrack 洛杉矶"
```

第二台示例：

```toml
node_id = "vimss"
name = "Vimss 洛杉矶"
```

修改显示名时，只改对应 VPS 的配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

改完重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

## 9. 常见问题排查

### Dashboard 能打开，但节点 offline

在中心 VPS 检查 API：

```bash
curl http://127.0.0.1:8000/api/health
```

在中心 VPS 查看节点数据：

```bash
curl http://127.0.0.1:8000/api/nodes
```

在被监控 VPS 查看 Agent 状态：

```bash
systemctl status vps-monitor-agent
```

在被监控 VPS 查看 Agent 日志：

```bash
journalctl -u vps-monitor-agent -f
```

在被监控 VPS 手动上报一次：

```bash
python /opt/vps-monitor/agent.py --config /etc/vps-monitor-agent.toml --once
```

在中心 VPS 检查是否放行了该远程 VPS IP：

```bash
sudo iptables -S INPUT | grep -- '--dport 8080'
```

### Agent 日志出现 404

通常是 `server_url` 写错了，或者远程 Agent 打到了 Dashboard 域名的非 API 入口。

在被监控 VPS 查看配置：

```bash
cat /etc/vps-monitor-agent.toml
```

远程 VPS 推荐：

```toml
server_url = "http://中心VPS公网IP:8080"
```

在被监控 VPS 测试 8080 API：

```bash
curl -i http://中心VPS公网IP:8080/api/health
```

在中心 VPS 测试 8080 根路径是否 404：

```bash
curl -i http://127.0.0.1:8080/
```

### Agent 日志出现 401

`401` 表示 token 不一致。

在中心 VPS 查看 token：

```bash
sudo cat /etc/vps-monitor.env
```

在被监控 VPS 查看 token：

```bash
sudo cat /etc/vps-monitor-agent.toml
```

改完被监控 VPS 的 token 后重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

### 同一个节点名字来回跳

这是两台机器用了同一个 `node_id`。

在中心 VPS 查看节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

在每台被监控 VPS 查看自己的 `node_id`：

```bash
grep node_id /etc/vps-monitor-agent.toml
```

每台 VPS 必须使用不同 `node_id`。改完后重启对应 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

### 页面出现脏节点

脏节点通常是测试时留下的旧 `node_id`。

在中心 VPS 查看节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

删除不需要的节点。示例只保留 `vmrack` 和 `vimss`：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM nodes WHERE id NOT IN ('vmrack','vimss');"
```

如果也要清理旧指标：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM metrics WHERE node_id NOT IN ('vmrack','vimss');"
```

### git pull 后本地修改丢失

如果服务器上你直接改过文件，`git pull` 可能冲突或覆盖你的改动。

先看本地改动：

```bash
git status
```

如果你确认服务器上的本地改动不要了，要强制同步 GitHub 最新代码，在中心 VPS 或远程 VPS 的项目目录执行：

```bash
git fetch --all
```

```bash
git reset --hard origin/master
```

注意：这会丢弃服务器当前目录里的本地代码改动。

### HTTPS 后 Agent 无法上报

远程 Agent 不建议默认走 HTTPS 域名。

在被监控 VPS 检查配置：

```bash
cat /etc/vps-monitor-agent.toml
```

推荐改成：

```toml
server_url = "http://中心VPS公网IP:8080"
```

在被监控 VPS 测试：

```bash
curl -i http://中心VPS公网IP:8080/api/health
```

改完重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

### Cloudflare 小黄云导致访问异常

Dashboard 可以通过 Cloudflare 访问，但 SSL/TLS 不建议用 Flexible。

检查 Cloudflare SSL/TLS 模式：

```text
SSL/TLS -> Overview -> Full 或 Full strict
```

在中心 VPS 检查源站 Nginx：

```bash
nginx -t
```

在中心 VPS 检查 HTTPS：

```bash
curl -I https://monitor.example.com
```

如果 Agent 通过域名上报异常，把 Agent 改回中心 VPS 公网 IP 的 8080：

```toml
server_url = "http://中心VPS公网IP:8080"
```

## 10. 运维命令

以下命令请在对应机器执行。

在中心 VPS 查看 API 状态：

```bash
systemctl status vps-monitor-api
```

在被监控 VPS 查看 Agent 状态：

```bash
systemctl status vps-monitor-agent
```

在被监控 VPS 查看 Agent 日志：

```bash
journalctl -u vps-monitor-agent -f
```

在中心 VPS 查看节点数据库：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

在中心 VPS 删除脏节点，只保留 `主机名`：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM nodes WHERE id NOT IN ('主机名','主机名');"
```

在中心 VPS 或远程 VPS 同步 GitHub 最新代码：

```bash
cd /opt/vps-monitor
```

```bash
git fetch --all
```

```bash
git reset --hard origin/master
```

在中心 VPS 重启 API：

```bash
systemctl restart vps-monitor-api
```

在被监控 VPS 重启 Agent：

```bash
systemctl restart vps-monitor-agent
```

## 11. API 说明

Dashboard 使用：

```text
GET /api/nodes
```

Agent 使用：

```text
POST /api/nodes/register
POST /api/metrics
```

健康检查：

```text
GET /api/health
```

写入接口需要：

```text
Authorization: Bearer <token>
```

浏览器 Dashboard 不保存 token。远程 Agent 走 8080 时，Nginx 只允许 `/api/` 到 FastAPI，根路径和其他路径返回 `404`。
