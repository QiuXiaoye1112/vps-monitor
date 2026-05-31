# 域名面板部署

目标效果：

```text
https://你的域名        打开 VPS 状态面板
https://你的域名/api    给各台 VPS Agent 上报数据
```

## 1. 域名解析

先把域名 A 记录解析到“面板 VPS”的公网 IP。

例如：

```text
monitor.example.com -> 1.2.3.4
```

## 2. 上传项目

把本项目上传到面板 VPS 的 `/opt/vps-monitor`：

```bash
mkdir -p /opt/vps-monitor
cd /opt/vps-monitor
```

确保目录里有 `app.py`、`server.py`、`agent.py`、`requirements.txt` 等文件。

## 3. 一键启动面板

在面板 VPS 上运行：

```bash
cd /opt/vps-monitor
sudo bash deploy_panel.sh monitor.example.com 换成你的复杂token
```

完成后浏览器打开：

```text
http://monitor.example.com
```

## 4. 给域名加 HTTPS

如果是 Debian/Ubuntu，并且域名已经解析到这台 VPS：

```bash
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d monitor.example.com
```

之后浏览器访问：

```text
https://monitor.example.com
```

其他 VPS 的 `server_url` 也改成：

```toml
server_url = "https://monitor.example.com"
```

## 5. 监控面板 VPS 自己

在面板 VPS 上创建：

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

启动本机 Agent：

```bash
cd /opt/vps-monitor
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
```

## 6. 监控其他 VPS

在面板里打开左侧：

```text
SSH 部署 Agent
```

填写：

```text
主机 IP / 域名：另一台 VPS 的 IP
端口：22
用户名：root
密码：这台 VPS 的 SSH 密码
Node ID：hk-01
名称：香港 VPS
中心服务端 URL：https://monitor.example.com
监控 Token：换成你的复杂token
磁盘路径：/
```

点“登录并部署 Agent”即可。

如果没有 HTTPS，就先用：

```text
http://monitor.example.com
```

## 7. 常用检查

查看服务状态：

```bash
systemctl status vps-monitor-api
systemctl status vps-monitor-dashboard
systemctl status vps-monitor-agent
```

看日志：

```bash
journalctl -u vps-monitor-api -f
journalctl -u vps-monitor-dashboard -f
journalctl -u vps-monitor-agent -f
```
