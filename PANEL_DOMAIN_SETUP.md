# 域名面板部署

当前部署方案已经固化在 [README.md](README.md)。

关键点：

- Dashboard 通过 Nginx + HTTPS 暴露
- FastAPI 只监听 `127.0.0.1:8000`
- 中心 VPS 自己的 Agent 使用 `http://127.0.0.1:8000`
- 远程 VPS Agent 推荐使用 `http://中心VPS公网IP:8080`
- `8080` 只开放 `/api/`，其他路径返回 `404`
- `8080` 应通过 `allow_agent_ip.sh` 只允许指定远程 VPS IP 访问
- Agent 默认 `interval = 1`
- Dashboard 每 1 秒刷新

部署顺序：

```bash
sudo bash deploy_panel.sh monitor.example.com change-this-token
sudo bash deploy_agent_ingress.sh
sudo bash allow_agent_ip.sh 1.2.3.4
```

完整说明请看 [README.md](README.md)。
