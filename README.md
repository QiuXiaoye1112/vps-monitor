# VPS Monitor

轻量 VPS 监控面板，支持通过终端菜单完成安装、配置和维护。

```bash
curl -fsSL https://raw.githubusercontent.com/QiuXiaoye1112/vps-monitor/master/install.sh | bash
```

安装完成后：

```bash
sudo vm
```

## 开发验证

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements-dev.txt
.venv/bin/python -m pytest -q
```
