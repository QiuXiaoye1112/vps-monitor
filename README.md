# VPS Monitor

轻量自托管 VPS 服务器监控面板。

## 部署中心主机

```bash
mkdir -p /opt/monitor
curl -fsSL https://github.com/QiuXiaoye1112/vps-monitor/releases/download/v1.0.0/vps-monitor-linux-amd64 -o /opt/monitor/vps-monitor
chmod +x /opt/monitor/vps-monitor
ADMIN_USERNAME=admin ADMIN_PASSWORD=你的密码 nohup /opt/monitor/vps-monitor server -l 0.0.0.0:25774 &
```

访问 `http://<IP>:25774/admin` 登录后台，添加节点。

## 部署 Agent

在后台节点列表点击"安装"按钮生成安装命令，在被控服务器上执行。
