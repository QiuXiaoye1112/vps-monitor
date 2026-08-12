# VPS Monitor

轻量自托管 VPS 服务器监控面板。

## 项目位置

| 项目 | 本地路径 | GitHub |
|------|----------|--------|
| 中心主机 | `~/Desktop/VPS-Monitor-Complete/` | `QiuXiaoye1112/vps-monitor` |
| Agent | `~/Desktop/monitor-agent/` | `QiuXiaoye1112/monitor-agent` |

## 部署中心主机

适用于 Linux amd64（Debian/Ubuntu/CentOS 等）。

### 1. 下载

```bash
mkdir -p /opt/monitor
curl -fsSL https://github.com/QiuXiaoye1112/vps-monitor/releases/latest/download/vps-monitor-linux-amd64 -o /opt/monitor/vps-monitor
chmod +x /opt/monitor/vps-monitor
```

### 2. 启动

```bash
# 前台运行（测试用）
ADMIN_USERNAME=admin ADMIN_PASSWORD=你的密码 /opt/monitor/vps-monitor server -l 0.0.0.0:25774

# 后台运行
ADMIN_USERNAME=admin ADMIN_PASSWORD=你的密码 nohup /opt/monitor/vps-monitor server -l 0.0.0.0:25774 > /opt/monitor/nohup.out 2>&1 &
```

环境变量:
- `ADMIN_USERNAME` — 管理员用户名（默认 admin）
- `ADMIN_PASSWORD` — 管理员密码

### 3. 注册系统服务（推荐）

```bash
cat > /etc/systemd/system/vps-monitor.service << 'EOF'
[Unit]
Description=VPS Monitor
After=network.target
[Service]
Type=simple
Environment="ADMIN_USERNAME=admin"
Environment="ADMIN_PASSWORD=你的密码"
ExecStart=/opt/monitor/vps-monitor server -l 0.0.0.0:25774
Restart=always
RestartSec=10
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now vps-monitor
```

### 4. 开放端口

```bash
# 云服务器控制台安全组放行 TCP 25774
# 或者系统防火墙:
iptables -I INPUT -p tcp --dport 25774 -j ACCEPT
```

### 5. 访问

浏览器打开 `http://<服务器IP>:25774/admin`，用设置的用户名密码登录。

首次登录后在后台添加节点，生成 Token，然后在被控服务器上安装 Agent。

## Nginx HTTPS 反向代理

如果通过域名访问，建议让 Nginx 监听 80/443，并将请求反代到本机的 `127.0.0.1:25774`。Agent 使用 WebSocket 心跳，因此必须转发 `Upgrade` 和 `Connection` 请求头，否则 Agent 会出现 `400 Require WebSocket upgrade` 并显示离线。

### 1. 配置 WebSocket 连接升级

在 Nginx 的 `http {}` 上下文中加入映射。Debian/Ubuntu 可以保存为 `/etc/nginx/conf.d/connection-upgrade.conf`：

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}
```

### 2. 配置站点反代

将下面内容保存为 `/etc/nginx/sites-available/monitor.example.com.conf`，把 `monitor.example.com` 替换成你的域名：

```nginx
server {
    listen 80;
    listen [::]:80;

    server_name monitor.example.com;

    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;

    server_name monitor.example.com;

    ssl_certificate /etc/letsencrypt/live/monitor.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/monitor.example.com/privkey.pem;

    client_max_body_size 10m;

    location / {
        proxy_pass http://127.0.0.1:25774;
        proxy_http_version 1.1;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;

        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

启用站点并检查配置：

```bash
ln -sfn /etc/nginx/sites-available/monitor.example.com.conf \\
  /etc/nginx/sites-enabled/monitor.example.com.conf
nginx -t
systemctl reload nginx
```

证书可以使用 Certbot 申请。使用 Cloudflare DNS 验证时，证书续期也会自动沿用 DNS-01：

```bash
certbot certonly --dns-cloudflare \\
  --dns-cloudflare-credentials /root/.secrets/certbot/cloudflare.ini \\
  --dns-cloudflare-propagation-seconds 60 \\
  -d monitor.example.com
```

配置完成后，浏览器访问 `https://monitor.example.com`，并确认 Agent 日志出现：

```text
WebSocket heartbeat probe connected using v2 protocol
Server heartbeat confirmed; enabling all reports
```

## Agent 部署

Agent 项目: [QiuXiaoye1112/monitor-agent](https://github.com/QiuXiaoye1112/monitor-agent)

在后台节点管理页面点击"安装"按钮生成安装命令，复制到被控服务器执行即可。

## 本地开发

```bash
# 编译运行
go build -o vps-monitor-complete .
./vps-monitor-complete server -l 0.0.0.0:25774

# 交叉编译 Linux 版本
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="zig cc -target x86_64-linux-musl" go build -o vps-monitor-linux-amd64 .
```
