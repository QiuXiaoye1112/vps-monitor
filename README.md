# VPS Monitor

VPS Monitor 是一个轻量级多 VPS 资源监控系统。它由中心服务端、SQLite 数据库、Streamlit Dashboard 和每台 VPS 上的 Python Agent 组成，适合先用最小成本监控 CPU、核心数、内存、Swap、磁盘路径和网络流量。

## 架构说明

```text
VPS Agent 1 ┐
VPS Agent 2 ├─ HTTP + Token ─> FastAPI Server ─> SQLite
VPS Agent 3 ┘                         │
                                       └─ Streamlit Dashboard
```

- Agent：运行在每台 VPS 上，使用 `psutil` 采集系统状态，定时上报。
- Server：FastAPI 服务，负责节点注册、指标写入、节点列表、详情和历史查询。
- Dashboard：Streamlit 页面，只从 Server API 读取数据，不生成假数据。
- 数据库：默认 SQLite 文件 `vps_monitor.db`，后续可以把 `storage.py` 替换成 PostgreSQL 实现。

## 服务端部署

```powershell
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
$env:VPS_MONITOR_TOKEN="change-this-token"
$env:VPS_MONITOR_DB="C:\opt\vps-monitor\vps_monitor.db"
.\.venv\Scripts\python.exe -m uvicorn server:app --host 0.0.0.0 --port 8000
```

Linux 示例：

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
export VPS_MONITOR_TOKEN="change-this-token"
export VPS_MONITOR_DB="/opt/vps-monitor/vps_monitor.db"
python -m uvicorn server:app --host 0.0.0.0 --port 8000
```

## Dashboard 部署

Dashboard 需要知道中心服务端地址和同一个 token。

```powershell
$env:VPS_MONITOR_API_URL="http://127.0.0.1:8000"
$env:VPS_MONITOR_TOKEN="change-this-token"
.\.venv\Scripts\python.exe -m streamlit run app.py
```

打开 Streamlit 显示的本地地址即可查看多 VPS 面板。

## Agent 部署

在每台 VPS 上复制项目文件，安装依赖，然后创建配置：

```bash
cp agent.example.toml /etc/vps-monitor-agent.toml
nano /etc/vps-monitor-agent.toml
```

配置项：

```toml
server_url = "http://YOUR_SERVER_IP:8000"
node_id = "hk-01"
token = "change-this-token"
interval = 10

name = "香港 01"
os_type = "Linux"

disk_paths = ["/"]
```

手动运行一次：

```bash
. /opt/vps-monitor/.venv/bin/activate
python /opt/vps-monitor/agent.py --config /etc/vps-monitor-agent.toml --once
```

持续运行：

```bash
python /opt/vps-monitor/agent.py --config /etc/vps-monitor-agent.toml
```

上报失败时 Agent 不会崩溃，会按退避策略重试。

## 通过 SSH 登录安装 Agent

Dashboard 里也提供了可视化入口：左侧打开「SSH 部署 Agent」，输入主机 IP、端口、用户名、密码和监控标识，然后点击「登录并部署 Agent」。密码只用于本次 SSH 连接，不会写入 SQLite。

如果你希望中心机器直接登录 VPS 完成安装，可以使用 `ssh_bootstrap.py`。它只使用你的 SSH key 临时登录，不会把 VPS 密码保存进数据库。

```bash
python ssh_bootstrap.py \
  --host 1.2.3.4 \
  --user root \
  --key ~/.ssh/id_rsa \
  --server-url http://YOUR_SERVER_IP:8000 \
  --node-id hk-01 \
  --token change-this-token \
  --name "香港 01"
```

如果 SSH 用户不是 root，但有 sudo 权限，加上：

```bash
--sudo
```

这个命令会在远端创建 `/opt/vps-monitor`，上传 agent 文件，写入 `/etc/vps-monitor-agent.toml`，安装 Python 依赖，并启用 `vps-monitor-agent` systemd 服务。

## systemd 配置

项目内提供了 `vps-monitor-agent.service` 示例。按你的安装路径调整 `WorkingDirectory` 和 `ExecStart` 后安装：

```bash
sudo cp vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now vps-monitor-agent
sudo systemctl status vps-monitor-agent
```

查看日志：

```bash
journalctl -u vps-monitor-agent -f
```

## 如何添加一台 VPS

1. 在 Dashboard 左侧「添加 / 更新 VPS」填写 `Node ID` 和名称。
2. 保存后记下返回的 `node_id`。也可以自己填写固定 `node_id`，例如 `hk-01`。
3. 在该 VPS 的 `/etc/vps-monitor-agent.toml` 中填入同一个 `node_id`、`server_url` 和 `token`。
4. 启动 Agent。
5. Dashboard 会在下一次刷新后显示在线状态和真实指标。

如果 Agent 停止上报超过 30 秒，该 VPS 会显示为 `offline`。可用环境变量 `VPS_MONITOR_OFFLINE_AFTER` 修改阈值。

## Token 配置

服务端和 Dashboard/Agent 必须使用同一个 token：

```bash
export VPS_MONITOR_TOKEN="change-this-token"
```

API 支持两种传递方式：

- `Authorization: Bearer <token>`
- `X-Monitor-Token: <token>`

默认 token 是 `change-me`，生产环境请务必修改。

## API 简要说明

所有 `/api` 写入和查询接口都需要 token，`/api/health` 除外。

- `GET /api/health`：健康检查
- `POST /api/nodes/register`：注册或更新节点
- `PUT /api/nodes/{node_id}`：更新节点
- `GET /api/nodes`：节点列表，包含在线状态和最新指标
- `GET /api/nodes/{node_id}`：单节点详情
- `POST /api/metrics`：Agent 上报指标
- `GET /api/nodes/{node_id}/metrics?window=5m|1h|24h`：历史指标

## 采集内容

Agent 只采集系统状态，不采集文件内容、用户数据、环境变量或命令历史。

- CPU 使用率和核心数
- 内存、Swap、磁盘容量与使用率
- 网络当前上行 / 下行速度与累计流量
- 多磁盘路径容量、剩余空间和使用率
- 少量节点识别信息：OS、内核版本、架构、hostname

## 常见问题

### Dashboard 提示无法连接中心服务端

先确认 Server 正在运行：

```bash
curl http://127.0.0.1:8000/api/health
```

如果 Dashboard 和 Server 不在同一台机器，设置：

```bash
export VPS_MONITOR_API_URL="http://SERVER_IP:8000"
```

### 返回 401

Dashboard、Agent 和 Server 的 `VPS_MONITOR_TOKEN` 不一致。统一 token 后重启相关进程。

### 节点一直 offline

检查 Agent 是否运行、`server_url` 是否能访问、token 是否正确。超过 30 秒没有上报时节点会自动变成 offline。

### 没有历史曲线

历史曲线只显示真实上报数据。新节点需要等待 Agent 上报几次后才会有曲线。
