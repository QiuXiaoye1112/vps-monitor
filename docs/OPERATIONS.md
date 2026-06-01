# 运维命令

## 查看 API 状态

在中心 VPS 执行：

```bash
systemctl status vps-monitor-api
```

查看 API 日志：

```bash
journalctl -u vps-monitor-api -f
```

健康检查：

```bash
curl http://127.0.0.1:8000/api/health
```

## 查看 Agent 状态

在被监控 VPS 执行：

```bash
systemctl status vps-monitor-agent
```

## 查看 Agent 日志

在被监控 VPS 执行：

```bash
journalctl -u vps-monitor-agent -f
```

## 保活和开机自启

每台需要上报的 VPS 都要启用 Agent 自启。机器宕机或重启后，systemd 会自动拉起 `vps-monitor-agent` 继续上报。

在被监控 VPS 执行：

```bash
sudo systemctl enable --now vps-monitor-agent
```

确认已经设置为开机自启：

```bash
systemctl is-enabled vps-monitor-agent
```

确认当前正在运行：

```bash
systemctl status vps-monitor-agent
```

如果 Agent 配置改过，重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

中心 VPS 的 API 服务也建议保持开机自启：

```bash
sudo systemctl enable --now vps-monitor-api
```

确认中心 API 自启：

```bash
systemctl is-enabled vps-monitor-api
```

## 查看节点数据库

在中心 VPS 执行：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "SELECT id,name,last_seen_at FROM nodes;"
```

## 删除脏节点

先备份数据库：

```bash
cp /opt/vps-monitor/vps_monitor.db "/opt/vps-monitor/vps_monitor.db.bak.$(date +%Y%m%d%H%M%S)"
```

只保留指定节点：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM nodes WHERE id NOT IN ('vmrack','vimss');"
```

清理旧指标：

```bash
sqlite3 /opt/vps-monitor/vps_monitor.db "DELETE FROM metrics WHERE node_id NOT IN ('vmrack','vimss');"
```

## 修改显示名

在对应 VPS 编辑 Agent 配置：

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

## 8080 防火墙规则

这些命令都在中心 VPS 执行。

8080 是远程 Agent 上报入口。建议只允许你的远程 VPS 公网 IP 访问，不要全网开放。

### 放行指定 VPS IP 访问 8080

放行一台远程 VPS：

```bash
iptables -I INPUT -p tcp -s 38.207.187.37 --dport 8080 -j ACCEPT
```

再放行另一台远程 VPS：

```bash
iptables -I INPUT -p tcp -s 43.110.32.2 --dport 8080 -j ACCEPT
```

### 拒绝其他所有 IP 访问 8080

```bash
iptables -A INPUT -p tcp --dport 8080 -j DROP
```

规则顺序很重要：指定 IP 的 `ACCEPT` 必须在通用 `DROP` 前面。

### 查看当前 8080 规则

查看原始规则：

```bash
iptables -S INPUT | grep 8080
```

查看详细规则和编号：

```bash
iptables -L INPUT -n -v --line-numbers | grep 8080
```

### 删除 8080 防火墙规则

先查看规则编号：

```bash
iptables -L INPUT -n --line-numbers | grep 8080
```

按编号删除某一条规则，例如删除第 2 条：

```bash
iptables -D INPUT 2
```

如果要删除多条规则，先删编号大的，再删编号小的。因为删除一条后，后面的编号会变化。

按具体 IP 删除放行规则：

```bash
iptables -D INPUT -p tcp -s 38.207.187.37 --dport 8080 -j ACCEPT
```

```bash
iptables -D INPUT -p tcp -s 43.110.32.2 --dport 8080 -j ACCEPT
```

删除 8080 的通用 DROP 规则：

```bash
iptables -D INPUT -p tcp --dport 8080 -j DROP
```

### 保存防火墙规则，重启后继续生效

安装持久化工具：

```bash
apt install -y iptables-persistent
```

保存当前规则：

```bash
netfilter-persistent save
```

查看持久化服务状态：

```bash
systemctl status netfilter-persistent
```

查看保存到文件里的 8080 规则：

```bash
grep 8080 /etc/iptables/rules.v4
```

手动重新加载已保存规则：

```bash
netfilter-persistent reload
```

每次修改 iptables 规则后，如果希望重启后仍然生效，都要再次执行：

```bash
netfilter-persistent save
```

### 验证 8080 是否只对指定 IP 开放

在被允许的远程 VPS 上执行：

```bash
curl http://中心VPS公网IP:8080/api/health
```

正常应该返回：

```json
{"status":"ok"}
```

在未被允许的第三台机器上执行：

```bash
curl --connect-timeout 5 http://中心VPS公网IP:8080/api/health
```

如果返回连接超时，说明 8080 已经被防火墙拦住。

## 同步 GitHub 最新代码

在中心 VPS 或远程 VPS 执行：

```bash
cd /opt/vps-monitor
```

拉取远端状态：

```bash
git fetch --all
```

强制同步到远端 master：

```bash
git reset --hard origin/master
```

注意：这会丢弃服务器当前目录里的本地代码改动。

中心 VPS 同步后安装中心依赖：

```bash
. .venv/bin/activate
```

```bash
pip install -r requirements.txt
```

远程 VPS 同步后安装 Agent 依赖：

```bash
. .venv/bin/activate
```

```bash
pip install -r requirements-agent.txt
```

## 重启服务

中心 VPS 重启 API：

```bash
sudo systemctl restart vps-monitor-api
```

被监控 VPS 重启 Agent：

```bash
sudo systemctl restart vps-monitor-agent
```

重载 Nginx：

```bash
sudo nginx -t
```

```bash
sudo systemctl reload nginx
```
