# VPS Monitor

轻量 VPS 监控面板，支持通过终端菜单完成安装、配置和维护。

```bash
bash <(curl -fsSL -H "Accept: application/vnd.github.raw+json" -H "X-GitHub-Api-Version: 2022-11-28" "https://api.github.com/repos/QiuXiaoye1112/vps-monitor/contents/install.sh?ref=master&cache=$(date +%s)")
```

安装完成后再次打开管理面板：

```bash
sudo vm
```

删除新旧版本：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/QiuXiaoye1112/vps-monitor/master/uninstall.sh)
```
