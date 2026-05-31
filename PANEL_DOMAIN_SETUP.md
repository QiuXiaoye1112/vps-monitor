# 域名面板部署

目标效果：

```text
https://你的域名        打开轻量 VPS 状态面板
https://你的域名/api    给各台 VPS Agent 上报数据
```

中心 VPS 只运行一个 `vps-monitor-api.service`。FastAPI 同时提供 API 和 HTML Dashboard。

## 1. 域名解析

把域名 A 记录解析到中心 VPS 公网 IP。

```text
monitor.example.com -> 1.2.3.4
```

## 2. 上传项目

把本项目上传到中心 VPS 的 `/opt/vps-monitor`：

```bash
mkdir -p /opt/vps-monitor
cd /opt/vps-monitor
```

确保目录里有：

```text
server.py
agent.py
monitor_common.py
storage.py
settings.py
deploy_panel.sh
requirements.txt
requirements-agent.txt
vps-monitor-agent.service
```

不要上传 `.venv`、日志、数据库或缓存文件。

## 3. 一键启动面板

```bash
cd /opt/vps-monitor
sudo bash deploy_panel.sh monitor.example.com 换成你的复杂token
```

完成后访问：

```text
http://monitor.example.com
```

## 4. 开启 HTTPS

Debian/Ubuntu 示例：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d monitor.example.com
```

之后访问：

```text
https://monitor.example.com
```

其他 VPS 的 Agent 配置使用：

```toml
server_url = "https://monitor.example.com"
```

## 5. 监控中心 VPS 自己

创建配置：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

内容：

```toml
server_url = "http://127.0.0.1:8000"
node_id = "main"
token = "换成你的复杂token"
interval = 10

name = "主面板 VPS"
os_type = "Linux"

disk_paths = ["/"]
```

启动：

```bash
cd /opt/vps-monitor
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
```

## 6. 监控其他 VPS

在每台被监控 VPS 上上传 Agent 需要的文件，安装 `requirements-agent.txt`，并创建：

```bash
sudo nano /etc/vps-monitor-agent.toml
```

示例：

```toml
server_url = "https://monitor.example.com"
node_id = "hk-01"
token = "换成你的复杂token"
interval = 10

name = "香港 VPS"
os_type = "Linux"

disk_paths = ["/"]
```

测试：

```bash
cd /opt/vps-monitor
. .venv/bin/activate
python agent.py --config /etc/vps-monitor-agent.toml --once
```

后台运行：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
```

## 7. 常用检查

```bash
systemctl status vps-monitor-api
journalctl -u vps-monitor-api -f
```

```bash
systemctl status vps-monitor-agent
journalctl -u vps-monitor-agent -f
```
