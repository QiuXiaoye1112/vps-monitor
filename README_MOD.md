# VPS-Monitor 魔改版说明

## 项目结构

```
VPS-Monitor-Complete/          # 中心主机面板（Go 后端）
├── main.go                    # 入口
├── cmd/                       # CLI 命令 + 服务器启动
├── pkg/                       # 核心包（配置/RPC/定时任务）
├── database/                  # 数据层（SQLite/GORM）
├── web/
│   ├── api/                   # API handlers
│   ├── router/                # 路由注册
│   ├── rpc/jsonrpc/           # JSON-RPC 方法实现
│   ├── public/
│   │   ├── public.go          # 静态文件服务 + 嵌入
│   │   ├── defaultTheme/dist/ # 后台管理前端（React SPA，编译后）
│   │   └── vpsTheme/dist/     # 前台首页主题（Vue SPA，编译后）
│   └── agent/                 # Agent 连接管理
├── protocol/                  # 通信协议定义
├── utils/                     # 工具函数
├── data/                      # 运行时数据（SQLite DB 等）
└── vps-monitor-complete       # 编译产物（macOS arm64）

monitor-agent/                 # 被控节点 Agent（Go，独立项目）
├── main.go
├── cmd/                       # CLI + 配置
├── monitoring/                # 数据采集（CPU/内存/磁盘/网络/GPU）
│   └── unit/                  # 各平台采集实现
├── server/                    # 上报 + WebSocket 连接
├── protocol/                  # 通信协议
└── monitor-agent-linux-amd64  # 编译产物（Linux amd64）
```

## 已修改内容

### 中心主机

| 文件 | 改动 | 说明 |
|------|------|------|
| `web/public/defaultTheme/dist/assets/chunk-index-riPcAHfw-vps20260706.js` | 删除 `,e.jsx(gi,{node:t})` | 移除节点列表操作栏的金币（账单）按钮 |
| `web/public/defaultTheme/dist/assets/chunk-account-Bl-m9mBW-vps20260706.js` | 删除右侧 2FA/SSO 列 | 账户页只保留用户名和密码修改 |
| `web/public/defaultTheme/dist/vps-admin-clean.js` | 清空 `fieldHints` 数组 | 不再隐藏任何后台设置项 |
| `web/public/vpsTheme/dist/index.html` | 已还原 | 无额外脚本引用 |

### Agent（monitor-agent/）

| 文件 | 改动 | 说明 |
|------|------|------|
| `monitoring/monitoring.go` | 删除 Load 采集 | 前端不显示系统负载 |
| `server/basicInfo.go` | 删除 9 个字段 | cpu_name/cores/arch/kernel/ip/gpu/virtualization/version |
| 全局 | Komari → Monitor | 所有文案/日志/二进制名改名 |
| `cmd/root.go` | 移除自动更新 | `update` 包不再导入调用 |
| `go.mod` | `komari-agent` → `monitor-agent` | 模块名更改 |

### 数据库

| 字段 | 说明 |
|------|------|
| `traffic_reset_day` / `traffic_reset_hour` | 流量重置时间（中心主机 Asia/Shanghai 时区计算） |
| `traffic_compensation` | 流量补偿值 |

## 编译

### 中心主机
```bash
cd VPS-Monitor-Complete
go build -o vps-monitor-complete .

# Linux 交叉编译
GOOS=linux GOARCH=amd64 go build -o vps-monitor-linux-amd64 .
```

### Agent
```bash
cd monitor-agent
go build -o monitor-agent .

# Linux 交叉编译
GOOS=linux GOARCH=amd64 go build -o monitor-agent-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o monitor-agent-linux-arm64 .
```

## 部署

### 中心主机
```bash
# 上传到云服务器
scp vps-monitor-linux-amd64 root@<IP>:/opt/monitor/

# 启动
ssh root@<IP>
chmod +x /opt/monitor/vps-monitor-linux-amd64
ADMIN_USERNAME=admin ADMIN_PASSWORD=xxx \
  nohup /opt/monitor/vps-monitor-linux-amd64 server -l 0.0.0.0:25774 &
```

### Agent
```bash
# 上传
scp monitor-agent-linux-amd64 root@<节点IP>:/usr/local/bin/monitor-agent

# 创建 systemd 服务
cat > /etc/systemd/system/monitor-agent.service << 'EOF'
[Unit]
Description=Monitor Agent
After=network.target
[Service]
Type=simple
ExecStart=/usr/local/bin/monitor-agent -e http://<中心IP>:25774 -t <Token> -i 3
Restart=always
[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload && systemctl enable --now monitor-agent
```

## 注意事项

1. **前端文件是编译后的压缩 JS**，直接改很容易出语法错误。改之前用 `python3 -c` 验证上下文。
2. **所有静态文件编译时嵌入二进制**（`//go:embed`），改完必须重新 `go build`。
3. **密码重置**：`./vps-monitor-complete chpasswd -p <新密码>`
4. **Agent Token**：后台添加节点后自动生成，可在节点列表查看/复制。
5. **流量计算**全部在服务端（`database/records/monthly_traffic.go`），Agent 只上报网卡累计计数器，无需感知重置逻辑。
