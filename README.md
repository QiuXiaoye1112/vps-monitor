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
