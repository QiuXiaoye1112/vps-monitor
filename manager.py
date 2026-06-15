#!/usr/bin/env python3
from __future__ import annotations

import argparse
import getpass
import ipaddress
import json
import os
import re
import secrets
import shlex
import shutil
import socket
import subprocess
import sys
import textwrap
import time
import urllib.error
import urllib.request
from datetime import datetime
from pathlib import Path


PROJECT_DIR = Path(__file__).resolve().parent
VENV_DIR = PROJECT_DIR / ".venv"
AGENT_CONFIG = Path("/etc/vps-monitor-agent.toml")
SERVER_ENV = Path("/etc/vps-monitor.env")
SYSTEMD_DIR = Path("/etc/systemd/system")
LAUNCHER = Path("/usr/local/bin/vm")
ROLE_FILE = Path("/etc/vps-monitor-role")
API_SERVICE = "vps-monitor-api"
AGENT_SERVICE = "vps-monitor-agent"


def api_base_url() -> str:
    host = os.getenv("VPS_MONITOR_API_HOST", "127.0.0.1")
    port = os.getenv("VPS_MONITOR_API_PORT", "8000")
    return f"http://{host}:{port}"


def agent_port() -> str:
    return os.getenv("VPS_MONITOR_AGENT_PORT", "8080")

RESET = "\033[0m"
BOLD = "\033[1m"
CYAN = "\033[36m"
GREEN = "\033[32m"
YELLOW = "\033[33m"
RED = "\033[31m"
DIM = "\033[2m"


def color(value: str, code: str) -> str:
    return f"{code}{value}{RESET}" if sys.stdout.isatty() else value


def clear() -> None:
    if sys.stdout.isatty():
        print("\033[2J\033[H", end="")


def title(name: str) -> None:
    clear()
    print(color("VPS Monitor Control", BOLD + CYAN))
    print(color(name, BOLD))
    print(color("=" * 56, DIM))


def pause(message: str = "按回车键返回...") -> None:
    input(f"\n{message}")


def ask(prompt: str, default: str | None = None, secret: bool = False) -> str:
    suffix = f" [{default}]" if default else ""
    reader = getpass.getpass if secret else input
    value = reader(f"{prompt}{suffix}: ").strip()
    return value or (default or "")


def confirm(prompt: str, default: bool = False) -> bool:
    hint = "Y/n" if default else "y/N"
    value = input(f"{prompt} [{hint}]: ").strip().lower()
    if not value:
        return default
    return value in {"y", "yes", "是"}


def ask_port(prompt: str, default: int) -> int:
    while True:
        value = ask(prompt, str(default))
        try:
            port = int(value)
        except ValueError:
            print(color("端口必须是数字。", RED))
            continue
        if 1 <= port <= 65535:
            return port
        print(color("端口必须在 1 到 65535 之间。", RED))


def ask_ip(prompt: str) -> str:
    while True:
        value = ask(prompt)
        try:
            return str(ipaddress.ip_address(value))
        except ValueError:
            print(color("请输入有效的 IPv4 或 IPv6 地址，不能填写域名或 http://。", RED))


def server_url(host: str, port: int) -> str:
    address = ipaddress.ip_address(host.strip())
    rendered = f"[{address}]" if address.version == 6 else str(address)
    return f"http://{rendered}:{port}"


def agent_token() -> str:
    try:
        content = AGENT_CONFIG.read_text(encoding="utf-8")
    except OSError:
        return ""
    match = re.search(r'^\s*token\s*=\s*"((?:\\.|[^"\\])*)"\s*$', content, re.MULTILINE)
    if not match:
        return ""
    return match.group(1).replace('\\"', '"').replace("\\\\", "\\")


def show_token() -> None:
    title("查看 token")
    role = installation_role()
    token = read_env(SERVER_ENV).get("VPS_MONITOR_TOKEN", "") if role == "center" else agent_token()
    print(f"通信 token：{token or '未配置'}")
    pause()


def choose(prompt: str, options: list[tuple[str, str]], allow_back: bool = True) -> str | None:
    print(f"\n{prompt}")
    for key, label in options:
        print(f"  {color(key, CYAN)}. {label}")
    if allow_back:
        print(f"  {color('0', CYAN)}. 返回")
    while True:
        value = input("\n请选择: ").strip()
        if allow_back and value == "0":
            return None
        if any(value == key for key, _ in options):
            return value
        print(color("无效选项，请重新输入。", YELLOW))


def require_root() -> bool:
    if os.geteuid() == 0:
        return True
    print(color("此操作需要 root 权限。请退出后使用 sudo vm 启动。", RED))
    pause()
    return False


def command_exists(name: str) -> bool:
    return shutil.which(name) is not None


def run(
    args: list[str],
    *,
    check: bool = True,
    capture: bool = False,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        args,
        check=check,
        text=True,
        capture_output=capture,
        env=env,
    )


def service_state(name: str) -> tuple[str, str]:
    if not command_exists("systemctl"):
        return "unavailable", "unavailable"
    active = subprocess.run(
        ["systemctl", "is-active", name], capture_output=True, text=True, check=False
    ).stdout.strip()
    enabled = subprocess.run(
        ["systemctl", "is-enabled", name], capture_output=True, text=True, check=False
    ).stdout.strip()
    return active or "not-installed", enabled or "not-installed"


def state_badge(state: str) -> str:
    if state == "active":
        return color("运行中", GREEN)
    if state in {"enabled", "static"}:
        return color("已启用", GREEN)
    if state in {"not-installed", "unavailable", "disabled"}:
        return color(state, YELLOW)
    return color(state, RED)


def health_check(url: str, timeout: float = 2.0) -> tuple[bool, str]:
    try:
        with urllib.request.urlopen(url, timeout=timeout) as response:
            body = response.read(256).decode("utf-8", errors="replace")
            return response.status == 200, body
    except (urllib.error.URLError, TimeoutError, ValueError) as exc:
        return False, str(exc)


def read_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError:
        return values
    for line in lines:
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def toml_string(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def render_agent_config(values: dict[str, object]) -> str:
    paths = values.get("disk_paths") or ["/"]
    rendered_paths = ", ".join(toml_string(str(path)) for path in paths)
    return textwrap.dedent(
        f"""\
        server_url = {toml_string(str(values['server_url']))}
        node_id = {toml_string(str(values['node_id']))}
        token = {toml_string(str(values['token']))}
        interval = {int(values.get('interval', 1))}

        name = {toml_string(str(values.get('name') or values['node_id']))}
        ip = {toml_string(str(values.get('ip', '')))}
        region = {toml_string(str(values.get('region', '')))}
        os_type = {toml_string(str(values.get('os_type', 'Linux')))}
        note = {toml_string(str(values.get('note', '')))}

        disk_paths = [{rendered_paths}]
        """
    )


def write_text_secure(path: Path, content: str, mode: int = 0o600) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    path.chmod(mode)


def remove_path(path: Path) -> None:
    try:
        if path.is_symlink() or path.is_file():
            path.unlink()
        elif path.is_dir():
            shutil.rmtree(path)
    except FileNotFoundError:
        pass


def panel_nginx_config(domain: str) -> str:
    return textwrap.dedent(
        f"""\
        server {{
            listen 80;
            server_name {domain};
            client_max_body_size 1m;

            location / {{
                proxy_pass {api_base_url()};
                proxy_http_version 1.1;
                proxy_set_header Host $host;
                proxy_set_header X-Real-IP $remote_addr;
                proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
                proxy_set_header X-Forwarded-Proto $scheme;
            }}
        }}
        """
    )


def is_ip_address(value: str) -> bool:
    try:
        ipaddress.ip_address(value)
        return True
    except ValueError:
        return False


def enable_https(domain: str) -> bool:
    ensure_apt_packages(["certbot", "python3-certbot-nginx"])
    result = run(
        [
            "certbot",
            "--nginx",
            "-d",
            domain,
            "--non-interactive",
            "--agree-tos",
            "--register-unsafely-without-email",
            "--redirect",
        ],
        check=False,
    )
    if result.returncode == 0 and command_exists("systemctl"):
        subprocess.run(["systemctl", "enable", "--now", "certbot.timer"], check=False, capture_output=True)
    return result.returncode == 0


def install_launcher() -> None:
    venv_python = str(VENV_DIR / "bin" / "python")
    launcher = textwrap.dedent(
        f"""\
        #!/usr/bin/env bash
        cd {shlex_quote(str(PROJECT_DIR))} && exec {shlex_quote(venv_python)} {shlex_quote(str(PROJECT_DIR / 'manager.py'))} "$@" < /dev/tty
        """
    )
    write_text_secure(LAUNCHER, launcher, 0o755)


def shlex_quote(value: str) -> str:
    return shlex.quote(value)


def remove_firewall_port_rules(port: str) -> int:
    result = subprocess.run(
        ["iptables", "-S", "INPUT"], capture_output=True, text=True, check=False
    )
    removed = 0
    for line in result.stdout.splitlines():
        if f"--dport {port}" not in line:
            continue
        parts = shlex.split(line)
        if len(parts) < 2 or parts[0] != "-A" or parts[1] != "INPUT":
            continue
        run(["iptables", "-D", *parts[1:]], check=False)
        removed += 1
    return removed


def nginx_value(path: Path, directive: str) -> str:
    try:
        content = path.read_text(encoding="utf-8")
    except OSError:
        return ""
    match = re.search(rf"^\s*{re.escape(directive)}\s+([^;]+);", content, re.MULTILINE)
    return match.group(1).strip() if match else ""


def ensure_apt_packages(packages: list[str]) -> None:
    def installed(package: str) -> bool:
        if not command_exists("dpkg-query"):
            return False
        result = subprocess.run(
            ["dpkg-query", "-W", "-f=${Status}", package],
            capture_output=True,
            text=True,
            check=False,
        )
        return result.returncode == 0 and "install ok installed" in result.stdout

    missing = [package for package in packages if not installed(package)]
    if not missing:
        return
    if not command_exists("apt-get"):
        raise RuntimeError(f"缺少依赖：{', '.join(missing)}；当前系统不支持 apt-get 自动安装")
    print(f"需要安装系统依赖：{', '.join(packages)}")
    package_env = os.environ.copy()
    package_env["DEBIAN_FRONTEND"] = "noninteractive"
    run(["apt-get", "update"], env=package_env)
    run(["apt-get", "install", "-y", *packages], env=package_env)


def ensure_venv(requirements: str) -> None:
    if not VENV_DIR.exists():
        run([sys.executable, "-m", "venv", str(VENV_DIR)])
    run([str(VENV_DIR / "bin/python"), "-m", "pip", "install", "--upgrade", "pip", "-q"])
    run(
        [
            str(VENV_DIR / "bin/python"),
            "-m",
            "pip",
            "install",
            "-q",
            "-r",
            str(PROJECT_DIR / requirements),
        ]
    )


def api_unit() -> str:
    host = os.getenv("VPS_MONITOR_API_HOST", "127.0.0.1")
    port = os.getenv("VPS_MONITOR_API_PORT", "8000")
    return textwrap.dedent(
        f"""\
        [Unit]
        Description=VPS Monitor API
        After=network-online.target
        Wants=network-online.target

        [Service]
        Type=simple
        WorkingDirectory={PROJECT_DIR}
        EnvironmentFile={SERVER_ENV}
        ExecStart={VENV_DIR}/bin/python -m uvicorn server:app --host {host} --port {port}
        Restart=always
        RestartSec=5

        [Install]
        WantedBy=multi-user.target
        """
    )


def agent_unit() -> str:
    return textwrap.dedent(
        f"""\
        [Unit]
        Description=VPS Monitor Agent
        After=network-online.target
        Wants=network-online.target
        StartLimitIntervalSec=0

        [Service]
        Type=simple
        WorkingDirectory={PROJECT_DIR}
        ExecStart={VENV_DIR}/bin/python {PROJECT_DIR}/agent.py --config {AGENT_CONFIG}
        Restart=always
        RestartSec=5
        User=root

        [Install]
        WantedBy=multi-user.target
        """
    )


def show_overview() -> None:
    title("系统概览")
    role = installation_role()
    print(f"角色          {color('中心服务器', CYAN) if role == 'center' else color('监控节点', CYAN)}")
    print(f"项目目录      {PROJECT_DIR}")

    if role == "center":
        api_active, api_enabled = service_state(API_SERVICE)
        ok, _ = health_check(f"{api_base_url()}/api/health")
        print(f"中心 API      {state_badge(api_active)} / 自启 {state_badge(api_enabled)}")
        print(f"API 健康检查  {color('正常', GREEN) if ok else color('启动中', YELLOW)}")
        print(f"中心配置      {'已创建' if SERVER_ENV.exists() else '未创建'}")
        env = read_env(SERVER_ENV)
        db_path = Path(env.get("VPS_MONITOR_DB", PROJECT_DIR / "vps_monitor.db"))
        if db_path.exists():
            print(f"数据库        {db_path} ({db_path.stat().st_size / 1024 / 1024:.2f} MB)")

    agent_active, agent_enabled = service_state(AGENT_SERVICE)
    print(f"本机 Agent    {state_badge(agent_active)} / 自启 {state_badge(agent_enabled)}")
    print(f"Agent 配置    {'已创建' if AGENT_CONFIG.exists() else '未创建'}")
    pause()


def install_panel() -> None:
    title("部署中心面板")
    if not require_root():
        return
    domain = ask("面板域名或公网 IP")
    if not domain:
        print(color("域名或 IP 不能为空。", RED))
        pause()
        return
    if not re.fullmatch(r"[A-Za-z0-9.-]+", domain):
        print(color("域名或 IP 只能包含字母、数字、点和连字符。", RED))
        pause()
        return
    generated = secrets.token_hex(24)
    token = ask("通信 token（留空自动生成）", generated, secret=True)
    if any(character.isspace() for character in token):
        print(color("token 不能包含空格或换行。", RED))
        pause()
        return
    print(color("正在清空旧配置...", CYAN))
    # 停服务
    for svc in (API_SERVICE, AGENT_SERVICE):
        if command_exists("systemctl"):
            subprocess.run(["systemctl", "disable", "--now", svc], check=False, capture_output=True)
        remove_path(SYSTEMD_DIR / f"{svc}.service")
    if command_exists("systemctl"):
        subprocess.run(["systemctl", "daemon-reload"], check=False, capture_output=True)
    # 清防火墙
    if command_exists("iptables"):
        port = agent_port()
        rules = subprocess.run(["iptables", "-S", "INPUT"], capture_output=True, text=True, check=False).stdout
        for line in rules.splitlines():
            if f"--dport {port}" in line:
                parts = shlex.split(line)
                if parts[0] == "-A":
                    subprocess.run(["iptables", "-D", *parts[1:]], check=False, capture_output=True)
    # 清 Nginx
    for p in ["/etc/nginx/sites-available/vps-monitor.conf", "/etc/nginx/sites-enabled/vps-monitor.conf",
              "/etc/nginx/sites-available/vps-monitor-agent.conf", "/etc/nginx/sites-enabled/vps-monitor-agent.conf",
              "/etc/nginx/conf.d/vps-monitor.conf", "/etc/nginx/conf.d/vps-monitor-agent.conf"]:
        remove_path(Path(p))
    # 清配置和数据库
    env = read_env(SERVER_ENV)
    db = Path(env.get("VPS_MONITOR_DB", PROJECT_DIR / "vps_monitor.db"))
    remove_path(db)
    for backup in db.parent.glob(f"{db.name}.bak.*"):
        remove_path(backup)
    remove_path(SERVER_ENV)
    remove_path(AGENT_CONFIG)
    print(color("已清理干净，开始全新部署。", GREEN))
    print()
    try:
        ensure_apt_packages(["python3", "python3-venv", "python3-pip", "nginx", "curl", "sqlite3"])
        ensure_venv("requirements.txt")
        host = os.getenv("VPS_MONITOR_API_HOST", "127.0.0.1")
        port = os.getenv("VPS_MONITOR_API_PORT", "8000")
        write_text_secure(
            SERVER_ENV,
            f"VPS_MONITOR_TOKEN={token}\nVPS_MONITOR_DB={PROJECT_DIR / 'vps_monitor.db'}\nVPS_MONITOR_API_HOST={host}\nVPS_MONITOR_API_PORT={port}\n",
        )
        write_text_secure(SYSTEMD_DIR / f"{API_SERVICE}.service", api_unit(), 0o644)
        nginx_site = Path("/etc/nginx/sites-available/vps-monitor.conf")
        nginx_link = Path("/etc/nginx/sites-enabled/vps-monitor.conf")
        write_text_secure(nginx_site, panel_nginx_config(domain), 0o644)
        nginx_link.parent.mkdir(parents=True, exist_ok=True)
        if nginx_link.is_symlink() or nginx_link.exists():
            nginx_link.unlink()
        nginx_link.symlink_to(nginx_site)
        install_launcher()
        run(["nginx", "-t"])
        run(["systemctl", "daemon-reload"])
        run(["systemctl", "enable", API_SERVICE, "nginx"])
        run(["systemctl", "restart", API_SERVICE])
        run(["systemctl", "reload", "nginx"])
        ingress_env = os.environ.copy()
        ingress_env["AGENT_PORT"] = agent_port()
        run(["bash", str(PROJECT_DIR / "deploy_agent_ingress.sh")], env=ingress_env)
        print(color("\n中心面板部署完成。", GREEN))
        print(f"访问地址：http://{domain}")
        if not is_ip_address(domain):
            print(color("如需 HTTPS：sudo vm → SSL 证书设置", DIM))
        print(f"token：{token}")
    except (OSError, subprocess.CalledProcessError, RuntimeError) as exc:
        print(color(f"\n部署失败：{exc}", RED))
    pause()


def configure_agent(local: bool) -> None:
    title("配置本机监控" if local else "部署远程 Agent")
    if not require_root():
        return
    server_values = read_env(SERVER_ENV)
    default_token = server_values.get("VPS_MONITOR_TOKEN", "") if local else ""
    if local:
        port = ask_port("中心 API 端口", int(os.getenv("VPS_MONITOR_API_PORT", "8000")))
        center_url = f"http://127.0.0.1:{port}"
    else:
        host = ask_ip("中心 VPS IP")
        port = ask_port("Agent 接入端口", int(agent_port()))
        center_url = server_url(host, port)
    node_id = ask("节点 ID（每台机器必须不同）", "center" if local else socket.gethostname())
    name = ask("面板显示名", "中心 VPS" if local else socket.gethostname())
    token = default_token if local and default_token else ask("通信 token", secret=True)
    interval_text = ask("上报间隔（秒）", "1")
    try:
        interval = max(1, int(interval_text))
    except ValueError:
        print(color("上报间隔必须是整数。", RED))
        pause()
        return
    if not center_url or not node_id or not token:
        print(color("服务地址、节点 ID 和 token 不能为空。", RED))
        pause()
        return
    values: dict[str, object] = {
        "server_url": center_url,
        "node_id": node_id,
        "name": name,
        "token": token,
        "interval": interval,
        "disk_paths": ["/"],
        "os_type": "Linux",
    }
    print("\n即将安装 Agent 依赖、写入配置、测试上报并启用开机自启。")
    try:
        ensure_apt_packages(["python3", "python3-venv", "python3-pip"])
        ensure_venv("requirements-agent.txt")
        write_text_secure(AGENT_CONFIG, render_agent_config(values))
        write_text_secure(SYSTEMD_DIR / f"{AGENT_SERVICE}.service", agent_unit(), 0o644)
        install_launcher()
        run(["systemctl", "daemon-reload"])
        print("\n正在测试一次上报...")
        test = run(
            [str(VENV_DIR / "bin/python"), str(PROJECT_DIR / "agent.py"), "--config", str(AGENT_CONFIG), "--once"],
            check=False,
        )
        if test.returncode != 0:
            print(color("测试上报失败，服务仍会安装并启动，请稍后查看 Agent 日志。", YELLOW))
        run(["systemctl", "enable", "--now", AGENT_SERVICE])
        run(["systemctl", "restart", AGENT_SERVICE])
        print(color("\nAgent 已配置并启动，已启用开机自启和异常自动重启。", GREEN))
    except (OSError, subprocess.CalledProcessError, RuntimeError) as exc:
        print(color(f"\nAgent 部署失败：{exc}", RED))
    pause()


def service_menu() -> None:
    while True:
        title("服务管理")
        api, _ = service_state(API_SERVICE)
        agent, _ = service_state(AGENT_SERVICE)
        print(f"中心 API：{state_badge(api)}    Agent：{state_badge(agent)}")
        selected = choose(
            "服务操作",
            [
                ("1", "启动服务"),
                ("2", "停止服务"),
                ("3", "重启服务"),
                ("4", "查看最近日志"),
                ("5", "实时查看日志"),
            ],
        )
        if selected is None:
            return
        service = choose("选择服务", [("1", "中心 API"), ("2", "Agent")])
        if service is None:
            continue
        name = API_SERVICE if service == "1" else AGENT_SERVICE
        if selected in {"1", "2", "3"}:
            if not require_root():
                continue
            verb = {"1": "start", "2": "stop", "3": "restart"}[selected]
            run(["systemctl", verb, name], check=False)
            pause()
        elif selected == "4":
            run(["journalctl", "-u", name, "-n", "80", "--no-pager"], check=False)
            pause()
        else:
            print(color("按 Ctrl+C 返回菜单。", YELLOW))
            try:
                run(["journalctl", "-u", name, "-f", "-n", "30"], check=False)
            except KeyboardInterrupt:
                pass


def ingress_menu() -> None:
    while True:
        title("Agent 入口与防火墙")
        selected = choose(
            "安全操作",
            [
                ("1", f"开启或更新 {agent_port()} Agent 入口"),
                ("2", "允许一台 Agent IP"),
                ("3", f"查看 {agent_port()} 防火墙规则"),
                ("4", "保存当前防火墙规则"),
                ("5", "申请 Dashboard HTTPS 证书"),
                ("6", f"删除 {agent_port()} Agent 入口"),
                ("7", "删除一台 Agent IP 白名单"),
                ("8", "删除 Agent 端口全部防火墙规则"),
                ("9", "删除 HTTPS 证书并恢复 HTTP"),
            ],
        )
        if selected is None:
            return
        if not require_root():
            continue
        try:
            if selected == "1":
                port = ask("Agent 入口端口", agent_port())
                env = os.environ.copy()
                env["AGENT_PORT"] = port
                run(["bash", str(PROJECT_DIR / "deploy_agent_ingress.sh")], env=env)
            elif selected == "2":
                agent_ip = ask("Agent 公网 IP")
                ipaddress.ip_address(agent_ip)
                port = ask("Agent 入口端口", agent_port())
                env = os.environ.copy()
                env["AGENT_PORT"] = port
                run(["bash", str(PROJECT_DIR / "allow_agent_ip.sh"), agent_ip], env=env)
            elif selected == "3":
                run(["iptables", "-S", "INPUT"], check=False)
            elif selected == "4":
                if not command_exists("netfilter-persistent"):
                    run(["apt-get", "install", "-y", "iptables-persistent"])
                run(["netfilter-persistent", "save"])
            elif selected == "5":
                domain = ask("已解析到本机的域名")
                if enable_https(domain):
                    print(color("HTTPS 已启用。", GREEN))
                else:
                    print(color("SSL 申请失败，请检查域名解析和公网 80 端口。", RED))
            elif selected == "6":
                if confirm("确认删除 Agent 入口？"):
                    remove_path(Path("/etc/nginx/sites-enabled/vps-monitor-agent.conf"))
                    remove_path(Path("/etc/nginx/sites-available/vps-monitor-agent.conf"))
                    run(["nginx", "-t"])
                    run(["systemctl", "reload", "nginx"])
                    print(color("Agent 入口已删除。", GREEN))
            elif selected == "7":
                agent_ip = ask("要删除的 Agent 公网 IP")
                ipaddress.ip_address(agent_ip)
                port = ask("Agent 入口端口", agent_port())
                while subprocess.run(
                    ["iptables", "-C", "INPUT", "-p", "tcp", "-s", agent_ip, "--dport", port, "-j", "ACCEPT"],
                    check=False,
                    capture_output=True,
                ).returncode == 0:
                    run(["iptables", "-D", "INPUT", "-p", "tcp", "-s", agent_ip, "--dport", port, "-j", "ACCEPT"])
                print(color("IP 白名单已删除。", GREEN))
            elif selected == "8":
                port = ask("Agent 入口端口", agent_port())
                if confirm(f"确认删除 TCP {port} 的全部 iptables 规则？"):
                    removed = remove_firewall_port_rules(port)
                    print(color(f"已删除 {removed} 条防火墙规则。", GREEN))
            else:
                domain = ask("要删除证书的域名")
                if confirm(f"确认删除 {domain} 的 HTTPS 并恢复 HTTP？"):
                    if command_exists("certbot"):
                        run(["certbot", "delete", "--cert-name", domain, "--non-interactive"], check=False)
                    write_text_secure(
                        Path("/etc/nginx/sites-available/vps-monitor.conf"),
                        panel_nginx_config(domain),
                        0o644,
                    )
                    run(["nginx", "-t"])
                    run(["systemctl", "reload", "nginx"])
                    print(color("HTTPS 已删除，面板已恢复 HTTP。", GREEN))
        except (OSError, ValueError, subprocess.CalledProcessError, RuntimeError) as exc:
            print(color(f"操作失败：{exc}", RED))
        pause()


def https_is_on() -> bool:
    site = Path("/etc/nginx/sites-available/vps-monitor.conf")
    if site.exists():
        content = site.read_text(encoding="utf-8")
        return "listen 443" in content or "ssl_certificate" in content
    return False


def toggle_https() -> None:
    if https_is_on():
        disable_https()
    else:
        enable_https_for_domain()


def enable_https_for_domain() -> None:
    title("SSL 证书设置")
    current = nginx_value(Path("/etc/nginx/sites-available/vps-monitor.conf"), "server_name") or "-"
    is_ip = is_ip_address(current)
    print(f"当前地址：{color(current, CYAN)}")
    print()

    if is_ip:
        selected = choose("选择操作", [
            ("1", "换成域名并申请 CF SSL 证书"),
        ])
    else:
        selected = choose("选择操作", [
            ("1", "申请 CF SSL 证书"),
            ("2", "更换域名"),
            ("3", "更换为 IP"),
        ])

    if selected is None:
        return

    if is_ip and selected == "1":
        new_domain = ask("域名")
        if not new_domain: return
        write_text_secure(Path("/etc/nginx/sites-available/vps-monitor.conf"), panel_nginx_config(new_domain), 0o644)
        run(["nginx", "-t"], check=False); run(["systemctl", "reload", "nginx"], check=False)
        print(color(f"域名已更换：http://{new_domain}", GREEN))
        apply_cf_ssl(new_domain)
    elif not is_ip and selected == "1":
        apply_cf_ssl(current)
    elif not is_ip and selected == "2":
        new_domain = ask("新域名")
        if not new_domain: return
        if command_exists("certbot"):
            run(["certbot", "delete", "--cert-name", current, "--non-interactive"], check=False)
        write_text_secure(Path("/etc/nginx/sites-available/vps-monitor.conf"), panel_nginx_config(new_domain), 0o644)
        run(["nginx", "-t"], check=False); run(["systemctl", "reload", "nginx"], check=False)
        print(color(f"已更换为：http://{new_domain}", GREEN))
        print(color("如需 SSL：返回本菜单选择 1", DIM))
    elif not is_ip and selected == "3":
        if command_exists("certbot"):
            run(["certbot", "delete", "--cert-name", current, "--non-interactive"], check=False)
        ip = ask("公网 IP")
        if not ip: return
        write_text_secure(Path("/etc/nginx/sites-available/vps-monitor.conf"), panel_nginx_config(ip), 0o644)
        run(["nginx", "-t"], check=False); run(["systemctl", "reload", "nginx"], check=False)
        print(color(f"已更换为：http://{ip}", GREEN))
    pause()


def _show_cf_ssl_steps(domain: str) -> None:
    print()
    print(color("=== Cloudflare SSL 设置步骤 ===", BOLD + CYAN))
    print()
    print(f"  {color('1', CYAN)}. 登录 cloudflare.com，添加站点 {color(domain, GREEN)}")
    print(f"  {color('2', CYAN)}. DNS → 添加 A 记录，指向本机公网 IP")
    print(f"  {color('3', CYAN)}. 开启小黄云（橙色 = 代理模式）")
    print(f"  {color('4', CYAN)}. SSL/TLS → 设为 {color('Full', GREEN)} 或 {color('Full (strict)', GREEN)}")
    print(f"  {color('5', CYAN)}. 等待 DNS 生效，访问 {color('https://' + domain, GREEN)}")
    print()
    print(color("CF 会自动签发证书，无需在 VPS 上操作。", DIM))
    pause()

def apply_cf_ssl(domain: str) -> None:
    print()
    print(color("=== 通过 Cloudflare API 申请证书 ===", BOLD + CYAN))
    print()
    print("需要 CF API Token（Zone:DNS:Edit 权限）")
    print("获取：cloudflare.com → 个人资料 → API 令牌 → 创建令牌")
    print()
    token = ask("CF API Token", secret=True)
    if not token:
        return
    cred_file = Path("/root/.cloudflare.ini")
    write_text_secure(cred_file, f"dns_cloudflare_api_token = {token}\n")
    print(color("凭据已保存。", GREEN))
    try:
        ensure_apt_packages(["python3-certbot-dns-cloudflare"])
    except Exception:
        print(color("安装 dns-cloudflare 插件失败。", RED))
        return
    print(color("正在申请证书（DNS 验证）...", CYAN))
    result = run([
        "certbot", "certonly", "--dns-cloudflare",
        "--dns-cloudflare-credentials", str(cred_file),
        "-d", domain,
        "--non-interactive", "--agree-tos",
        "--register-unsafely-without-email",
    ], check=False)
    if result.returncode == 0:
        site_conf = Path("/etc/nginx/sites-available/vps-monitor.conf")
        conf = site_conf.read_text()
        if "listen 443" not in conf:
            conf = conf.replace(
                "server {\n    listen 80;",
                f"server {{\n    listen 80;\n    listen 443 ssl;\n    ssl_certificate /etc/letsencrypt/live/{domain}/fullchain.pem;\n    ssl_certificate_key /etc/letsencrypt/live/{domain}/privkey.pem;"
            )
        write_text_secure(site_conf, conf, 0o644)
        run(["nginx", "-t"], check=False)
        run(["systemctl", "reload", "nginx"], check=False)
        print(color(f"证书已安装：https://{domain}", GREEN))
        subprocess.run(["systemctl", "enable", "--now", "certbot.timer"], check=False, capture_output=True)
    else:
        print(color("证书申请失败，请确认 Token 权限和域名解析。", RED))
    pause()

def disable_https() -> None:
    title("关闭 HTTPS")
    domain = nginx_value(Path("/etc/nginx/sites-available/vps-monitor.conf"), "server_name")
    if not domain:
        domain = ask("当前面板域名")
    if not domain:
        return
    if not confirm(f"确认删除 {domain} 的 HTTPS 证书，恢复 HTTP？"):
        return
    try:
        if command_exists("certbot"):
            run(["certbot", "delete", "--cert-name", domain, "--non-interactive"], check=False)
        write_text_secure(
            Path("/etc/nginx/sites-available/vps-monitor.conf"),
            panel_nginx_config(domain),
            0o644,
        )
        run(["nginx", "-t"], check=False)
        run(["systemctl", "reload", "nginx"], check=False)
        print(color("HTTPS 已关闭，已恢复 HTTP。", GREEN))
    except Exception as e:
        print(color(f"操作失败：{e}", RED))
    pause()


def backup_database() -> None:
    env = read_env(SERVER_ENV)
    db = Path(env.get("VPS_MONITOR_DB", PROJECT_DIR / "vps_monitor.db"))
    if not db.exists():
        print(color("数据库不存在。", YELLOW))
        return
    target = db.with_name(f"{db.name}.bak.{datetime.now().strftime('%Y%m%d%H%M%S')}")
    shutil.copy2(db, target)
    print(color(f"数据库已备份到 {target}", GREEN))


def remove_service(name: str) -> None:
    if command_exists("systemctl"):
        run(["systemctl", "disable", "--now", name], check=False)
    remove_path(SYSTEMD_DIR / f"{name}.service")
    if command_exists("systemctl"):
        run(["systemctl", "daemon-reload"], check=False)


def remove_agent() -> None:
    title("删除 Agent")
    if not require_root() or not confirm("确认停止并删除 Agent 服务和配置？"):
        return
    remove_service(AGENT_SERVICE)
    remove_path(AGENT_CONFIG)
    if installation_role() == "agent":
        remove_path(ROLE_FILE)
    print(color("Agent 已删除。", GREEN))
    pause()


def remove_panel() -> None:
    title("删除中心面板")
    if not require_root() or not confirm("确认停止并删除中心 API 和 Nginx 面板配置？"):
        return
    env = read_env(SERVER_ENV)
    db = Path(env.get("VPS_MONITOR_DB", PROJECT_DIR / "vps_monitor.db"))
    remove_service(API_SERVICE)
    remove_path(Path("/etc/nginx/sites-enabled/vps-monitor.conf"))
    remove_path(Path("/etc/nginx/sites-available/vps-monitor.conf"))
    remove_path(SERVER_ENV)
    if command_exists("nginx"):
        run(["nginx", "-t"], check=False)
        run(["systemctl", "reload", "nginx"], check=False)
    if db.exists() and confirm("同时永久删除监控数据库？"):
        remove_path(db)
    if AGENT_CONFIG.exists():
        write_text_secure(ROLE_FILE, "agent\n")
    else:
        remove_path(ROLE_FILE)
    print(color("中心面板已删除。", GREEN))
    pause()


def delete_backups() -> None:
    env = read_env(SERVER_ENV)
    db = Path(env.get("VPS_MONITOR_DB", PROJECT_DIR / "vps_monitor.db"))
    backups = list(db.parent.glob(f"{db.name}.bak.*"))
    if not backups:
        print(color("没有找到数据库备份。", YELLOW))
        return
    print(f"找到 {len(backups)} 个备份。")
    if confirm("确认永久删除全部备份？"):
        for backup in backups:
            remove_path(backup)
        print(color("数据库备份已删除。", GREEN))


def full_uninstall() -> None:
    title("完整卸载")
    if not require_root():
        return
    print(color("将删除服务、配置、数据库、备份、Nginx 入口、项目目录和快捷命令。", RED))
    if not confirm("确认完整卸载 VPS Monitor？"):
        return
    env = read_env(SERVER_ENV)
    db = Path(env.get("VPS_MONITOR_DB", PROJECT_DIR / "vps_monitor.db"))
    panel_site = Path("/etc/nginx/sites-available/vps-monitor.conf")
    ingress_site = Path("/etc/nginx/sites-available/vps-monitor-agent.conf")
    domain = nginx_value(panel_site, "server_name")
    listen_value = nginx_value(ingress_site, "listen")
    ingress_port = listen_value.split()[0] if listen_value else agent_port()
    remove_service(AGENT_SERVICE)
    remove_service(API_SERVICE)
    for path in (
        AGENT_CONFIG,
        SERVER_ENV,
        ROLE_FILE,
        LAUNCHER,
        Path("/etc/nginx/sites-enabled/vps-monitor.conf"),
        Path("/etc/nginx/sites-available/vps-monitor.conf"),
        Path("/etc/nginx/sites-enabled/vps-monitor-agent.conf"),
        Path("/etc/nginx/sites-available/vps-monitor-agent.conf"),
    ):
        remove_path(path)
    if command_exists("nginx"):
        run(["nginx", "-t"], check=False)
        run(["systemctl", "reload", "nginx"], check=False)
    if command_exists("iptables") and ingress_port.isdigit():
        remove_firewall_port_rules(ingress_port)
    if command_exists("netfilter-persistent"):
        run(["netfilter-persistent", "save"], check=False)
    if command_exists("certbot") and domain and domain != "_":
        run(["certbot", "delete", "--cert-name", domain, "--non-interactive"], check=False)
    for backup in db.parent.glob(f"{db.name}.bak.*"):
        remove_path(backup)
    remove_path(db)
    remove_path(PROJECT_DIR)
    print(color("VPS Monitor 已卸载。", GREEN))
    raise SystemExit(0)


def update_project() -> None:
    if not (PROJECT_DIR / ".git").exists():
        print(color("无法更新：非 Git 安装。", YELLOW))
        return

    print(color("正在检查更新...", CYAN))
    fetch = subprocess.run(
        ["git", "-C", str(PROJECT_DIR), "fetch", "origin"],
        capture_output=True, text=True, check=False,
    )
    if fetch.returncode != 0:
        print(color("更新失败：无法连接 GitHub。", RED))
        return

    before = subprocess.run(
        ["git", "-C", str(PROJECT_DIR), "rev-parse", "--short", "HEAD"],
        capture_output=True, text=True, check=False,
    ).stdout.strip()
    after = subprocess.run(
        ["git", "-C", str(PROJECT_DIR), "rev-parse", "--short", "origin/master"],
        capture_output=True, text=True, check=False,
    ).stdout.strip()

    if before == after:
        print(color("已是最新版本。", GREEN))
        return

    subprocess.run(
        ["git", "-C", str(PROJECT_DIR), "reset", "--hard", "origin/master"],
        capture_output=True, check=False,
    )
    print(color("更新完成。", GREEN))

    try:
        if VENV_DIR.exists():
            requirement = "requirements.txt" if SERVER_ENV.exists() else "requirements-agent.txt"
            ensure_venv(requirement)
    except Exception:
        pass
    for service in (API_SERVICE, AGENT_SERVICE):
        active, _ = service_state(service)
        if active == "active":
            run(["systemctl", "restart", service], check=False)
    print(color("请退出后重新执行 sudo vm。", CYAN))
    raise SystemExit(0)


def maintenance_menu() -> None:
    while True:
        title("维护工具")
        selected = choose(
            "维护操作",
            [
                ("1", "备份数据库"),
                ("2", "在线更新项目"),
                ("3", "重新安装依赖"),
                ("4", "查看中心 token"),
                ("5", "运行综合诊断"),
            ],
        )
        if selected is None:
            return
        try:
            if selected == "1":
                backup_database()
            elif selected == "2":
                if require_root():
                    update_project()
            elif selected == "3":
                if require_root():
                    requirements = "requirements.txt" if SERVER_ENV.exists() else "requirements-agent.txt"
                    ensure_venv(requirements)
            elif selected == "4":
                token = read_env(SERVER_ENV).get("VPS_MONITOR_TOKEN")
                print(f"中心 token：{token or '未配置'}")
            else:
                run_diagnostics()
        except (OSError, subprocess.CalledProcessError, RuntimeError) as exc:
            print(color(f"操作失败：{exc}", RED))
        pause()


def run_diagnostics() -> None:
    checks: list[tuple[str, bool, str]] = []
    for service in (API_SERVICE, AGENT_SERVICE):
        active, enabled = service_state(service)
        checks.append((f"{service} 运行状态", active == "active", active))
        checks.append((f"{service} 开机自启", enabled == "enabled", enabled))
    ok, detail = health_check(f"{api_base_url()}/api/health")
    checks.append(("本机 API 健康检查", ok, detail))
    checks.append(("中心环境配置", SERVER_ENV.exists(), str(SERVER_ENV)))
    checks.append(("Agent 配置", AGENT_CONFIG.exists(), str(AGENT_CONFIG)))
    checks.append(("Python 虚拟环境", (VENV_DIR / "bin/python").exists(), str(VENV_DIR)))
    print()
    for label, passed, detail in checks:
        mark = color("PASS", GREEN) if passed else color("WARN", YELLOW)
        print(f"[{mark}] {label}: {detail}")


def installation_role() -> str:
    try:
        saved_role = ROLE_FILE.read_text(encoding="utf-8").strip()
    except OSError:
        saved_role = ""
    if saved_role in {"center", "agent"}:
        return saved_role
    if SERVER_ENV.exists():
        return "center"
    if AGENT_CONFIG.exists():
        return "agent"
    return "new"


def restart_services() -> None:
    title("重启服务")
    for svc in (API_SERVICE, AGENT_SERVICE):
        active, _ = service_state(svc)
        if active == "active":
            run(["systemctl", "restart", svc], check=False)
            print(color(f"{svc} 已重启。", GREEN))
        else:
            print(f"{svc} 未运行，跳过。")
    pause()


def edit_node_info() -> None:
    title("修改本机信息")
    if not AGENT_CONFIG.exists():
        print(color("Agent 尚未配置。", YELLOW))
        pause()
        return
    try:
        with AGENT_CONFIG.open("rb") as f:
            try:
                import tomllib
            except ModuleNotFoundError:
                import tomli as tomllib
            config = tomllib.load(f)
    except Exception:
        print(color("配置文件解析失败！请检查 /etc/vps-monitor-agent.toml", RED))
        print(color("修复后重试。引号必须是普通双引号，不能是 \\\"。", DIM))
        pause()
        return
    role = installation_role()
    if not config:
        print(color("配置文件为空或损坏。", RED))
        pause()
        return

    while True:
        title("修改本机信息")
        print(f"角色：{color('中心服务器' if role == 'center' else '远程节点', CYAN)}")
        print(f"  显示名称：{config.get('name', '-')}")
        print(f"  节点 ID：{config.get('node_id', '-')}")
        print(f"  流量重置：每月 {os.getenv('VPS_MONITOR_TRAFFIC_RESET_DAY', '1')} 号 {os.getenv('VPS_MONITOR_TRAFFIC_RESET_HOUR', '0')}:00")
        print(f"  月流量上限：{os.getenv('VPS_MONITOR_TRAFFIC_LIMIT_GB', '0')} GB")
        print(f"  上报间隔：{config.get('interval', '1')} 秒")
        if role == "center":
            print(f"  Agent 入口端口：{agent_port()}")
        print()
        options = [("1", "显示名称"), ("2", "节点 ID"), ("3", "流量重置时间"),
                   ("4", "月流量上限"), ("5", "上报间隔")]
        if role == "center":
            options.append(("6", "Agent 入口端口"))
        options.append(("7" if role == "center" else "6", "保存并退出"))
        selected = choose("选择要修改的项", options)
        save_key = "7" if role == "center" else "6"
        if selected is None or selected == save_key:
            break
        if selected == "1":
            val = ask("显示名称", config.get("name", ""))
            if val: config["name"] = val
        elif selected == "2":
            val = ask("节点 ID", config.get("node_id", ""))
            if val: config["node_id"] = val
        elif selected == "3":
            val = ask("重置日（几号）", os.getenv("VPS_MONITOR_TRAFFIC_RESET_DAY", "1"))
            if val: os.environ["VPS_MONITOR_TRAFFIC_RESET_DAY"] = val
            val = ask("重置时（几点，0-23）", os.getenv("VPS_MONITOR_TRAFFIC_RESET_HOUR", "0"))
            if val: os.environ["VPS_MONITOR_TRAFFIC_RESET_HOUR"] = val
        elif selected == "4":
            val = ask("月流量上限（GB，0=不限制）", os.getenv("VPS_MONITOR_TRAFFIC_LIMIT_GB", "0"))
            if val: os.environ["VPS_MONITOR_TRAFFIC_LIMIT_GB"] = val
        elif selected == "5":
            val = ask("上报间隔（秒）", str(config.get("interval", "1")))
            if val and val.isdigit(): config["interval"] = int(val)
        elif selected == "6" and role == "center":
            val = ask("Agent 入口端口", agent_port())
            if val: os.environ["VPS_MONITOR_AGENT_PORT"] = val

    content = textwrap.dedent(f"""\
        server_url = "{config.get('server_url', f'http://127.0.0.1:{os.getenv("VPS_MONITOR_API_PORT", "8000")}')}"
        node_id = "{config.get('node_id', '')}"
        token = "{config.get('token', 'change-me')}"
        interval = {config.get('interval', 1)}

        name = "{config.get('name', '')}"
        ip = "{config.get('ip', '')}"
        region = "{config.get('region', '')}"
        os_type = "{config.get('os_type', 'Linux')}"
        note = "{config.get('note', '')}"

        disk_paths = [{', '.join(f'"{p}"' for p in (config.get('disk_paths') or ['/']))}]
        """)
    write_text_secure(AGENT_CONFIG, content)
    env_path = SERVER_ENV
    env_lines = env_path.read_text().splitlines() if env_path.exists() else []
    for key in ("VPS_MONITOR_TRAFFIC_RESET_DAY", "VPS_MONITOR_TRAFFIC_RESET_HOUR", "VPS_MONITOR_TRAFFIC_LIMIT_GB", "VPS_MONITOR_TRAFFIC_OFFSET_GB", "VPS_MONITOR_AGENT_PORT"):
        val = os.getenv(key, "")
        found = False
        for i, line in enumerate(env_lines):
            if line.startswith(f"{key}="):
                env_lines[i] = f"{key}={val}"
                found = True
                break
        if not found:
            env_lines.append(f"{key}={val}")
    write_text_secure(env_path, "\\n".join(env_lines) + "\\n")
    print(color("已保存。", GREEN))
    active, _ = service_state(AGENT_SERVICE)
    if active == "active":
        run(["systemctl", "restart", AGENT_SERVICE], check=False)
    pause()


def quick_update() -> None:
    title("更新 VPS Monitor")
    if not require_root():
        return
    try:
        update_project()
    except (OSError, subprocess.CalledProcessError, RuntimeError) as exc:
        print(color(f"更新失败：{exc}", RED))
    pause()


def monitored_nodes() -> list[dict[str, Any]]:
    with urllib.request.urlopen(f"{api_base_url()}/api/nodes", timeout=3) as response:
        data = json.load(response)
    nodes = data.get("nodes", [])
    return nodes if isinstance(nodes, list) else []


def firewall_allows(ip: str, port: int | None = None) -> bool:
    if port is None:
        port = int(agent_port())
    if not command_exists("iptables") or not ip:
        return False
    return subprocess.run(
        ["iptables", "-C", "INPUT", "-p", "tcp", "-s", ip, "--dport", str(port), "-j", "ACCEPT"],
        check=False,
        capture_output=True,
    ).returncode == 0


def save_firewall() -> None:
    ensure_apt_packages(["iptables", "iptables-persistent"])
    run(["netfilter-persistent", "save"])


def allow_node_firewall(node: dict[str, Any]) -> None:
    ip = str(node.get("ip") or "")
    try:
        address = ipaddress.ip_address(ip)
    except ValueError:
        print(color("该主机还没有有效来源 IP，请等待它成功上报后重试。", RED))
        pause()
        return
    if address.is_loopback:
        print(color("本机节点不需要设置远程防火墙。", YELLOW))
        pause()
        return
    if not require_root():
        return
    ingress_env = os.environ.copy()
    ingress_env["AGENT_PORT"] = agent_port()
    run(["bash", str(PROJECT_DIR / "deploy_agent_ingress.sh")], env=ingress_env)
    ensure_apt_packages(["iptables"])
    run(["bash", str(PROJECT_DIR / "allow_agent_ip.sh"), str(address)], env=ingress_env)
    save_firewall()
    print(color(f"已允许 {address} 访问 {agent_port()}，并保存防火墙规则。", GREEN))
    pause()


def remove_node_firewall(node: dict[str, Any]) -> None:
    ip = str(node.get("ip") or "")
    try:
        address = str(ipaddress.ip_address(ip))
    except ValueError:
        print(color("该主机没有有效来源 IP。", RED))
        pause()
        return
    if not require_root():
        return
    while firewall_allows(address):
        run(["iptables", "-D", "INPUT", "-p", "tcp", "-s", address, "--dport", agent_port(), "-j", "ACCEPT"])
    if command_exists("netfilter-persistent"):
        run(["netfilter-persistent", "save"])
    print(color(f"已移除 {address} 的防火墙放行规则。", GREEN))
    pause()


def remove_remote_node(node: dict[str, Any]) -> None:
    if not require_root():
        return
    node_id = node.get("id", "")
    name = node.get("name") or node_id
    ip = str(node.get("ip") or "")
    title("删除主机")
    print(f"名称：{name}")
    print(f"节点 ID：{node_id}")
    print(f"来源 IP：{ip}")
    print()
    if not confirm(f"确认删除 {name}？这会移除防火墙规则、数据库记录和所有历史指标。"):
        return

    # 移除防火墙规则
    try:
        address = str(ipaddress.ip_address(ip))
        if not address.startswith("127."):
            while firewall_allows(address):
                run(["iptables", "-D", "INPUT", "-p", "tcp", "-s", address, "--dport", agent_port(), "-j", "ACCEPT"], check=False)
            if command_exists("netfilter-persistent"):
                run(["netfilter-persistent", "save"], check=False)
            print(color("防火墙规则已移除。", GREEN))
    except ValueError:
        pass

    # 从数据库删除
    try:
        token = read_env(SERVER_ENV).get("VPS_MONITOR_TOKEN", "")
        req = urllib.request.Request(
            f"{api_base_url()}/api/nodes/{node_id}?token={token}",
            method="DELETE",
        )
        with urllib.request.urlopen(req, timeout=5) as resp:
            if resp.status == 200:
                print(color(f"主机 {name} 已从数据库删除。", GREEN))
            else:
                print(color(f"删除失败：HTTP {resp.status}", RED))
    except urllib.error.HTTPError as e:
        if e.code == 404:
            print(color("该主机在数据库中不存在，可能已被删除。", YELLOW))
        else:
            print(color(f"删除失败：HTTP {e.code}", RED))
    except Exception as e:
        print(color(f"删除失败：{e}", RED))

    print(color(f"主机 {name} 已删除。远程 VPS 上的 Agent 如需停用，请到该 VPS 执行：sudo vm → 高级设置 → 删除 Agent", DIM))
    pause()


def temp_open_for_new_agent() -> None:
    if not require_root():
        return
    title(f"临时开放 {agent_port()}")
    port = ask("Agent 入口端口", agent_port())
    seconds = 300
    print()
    print(color(f"即将临时移除 TCP {port} 的 DROP 规则，持续 5 分钟。", YELLOW))
    print(color("在此期间新 Agent 可以上报，刷新列表即可看到新 IP。", CYAN))
    print()
    if not confirm("确认临时开放？"):
        return

    # 移除 DROP（兼容旧版无 ! -i lo 的规则和新版有 ! -i lo 的规则）
    dropped = False
    for drop_rule in (
        ["iptables", "-C", "INPUT", "-p", "tcp", "--dport", port, "!", "-i", "lo", "-j", "DROP"],
        ["iptables", "-C", "INPUT", "-p", "tcp", "--dport", port, "-j", "DROP"],
    ):
        while subprocess.run(drop_rule, check=False, capture_output=True).returncode == 0:
            del_rule = drop_rule[:]
            del_rule[1] = "-D"
            run(del_rule, check=False)
            dropped = True

    if not dropped:
        print(color(f"端口 {port} 当前没有 DROP 规则，无需临时开放。", YELLOW))
        pause()
        return

    print(color("已开放，请在 5 分钟内启动新 Agent...", GREEN))
    print(color("按回车键提前关闭", DIM))
    import select
    for i in range(seconds, 0, -5):
        m, s = divmod(i, 60)
        print(f"\r  剩余 {m}:{s:02d}（按回车关闭）...", end="", flush=True)
        ready, _, _ = select.select([sys.stdin], [], [], 5)
        if ready:
            sys.stdin.readline()
            print()
            break
    print()

    # 重新加上 DROP（排除本地回路）
    run(["iptables", "-A", "INPUT", "-p", "tcp", "--dport", port, "!", "-i", "lo", "-j", "DROP"], check=False)
    save_firewall()
    print(color("DROP 规则已恢复。新 Agent IP 如已出现在列表中，请选择该主机 → 允许访问。", GREEN))
    pause()


def firewall_rules_menu() -> None:
    while True:
        title(f"防火墙规则（端口 {agent_port()}）")
        port = agent_port()
        result = subprocess.run(
            ["iptables", "-S", "INPUT"], capture_output=True, text=True, check=False,
        )
        lines = [l for l in result.stdout.splitlines() if f"--dport {port}" in l]
        if not lines:
            print(f"端口 {port} 暂无防火墙规则。")
            pause()
            return

        print(f"端口 {port} 当前规则：\n")
        import re
        delete_options: list[tuple[str, str]] = []
        for i, line in enumerate(lines, 1):
            tag = color("允许", GREEN) if "ACCEPT" in line else color("拦截", RED)
            ip_match = re.search(r'-s\s+(\S+)', line)
            ip = ip_match.group(1) if ip_match else "所有 IP"
            print(f"  {tag} | {ip}")
            print(f"     {color(line, DIM)}")
            delete_options.append((str(i), f"删除这条规则（{tag} | {ip}）"))
        print()

        selected = choose("选择要删除的规则（或 0 返回）", delete_options)
        if selected is None:
            continue
        idx = int(selected) - 1
        line = lines[idx]
        parts = shlex.split(line)
        if parts[0] == "-A":
            subprocess.run(["iptables", "-D", *parts[1:]], check=False)
            print(color("规则已删除。", GREEN))
        if command_exists("netfilter-persistent"):
            run(["netfilter-persistent", "save"], check=False)


def monitored_hosts_menu() -> None:
    while True:
        title("监控主机")
        try:
            nodes = monitored_nodes()
        except (OSError, ValueError, urllib.error.URLError) as exc:
            print(color(f"无法读取主机列表：{exc}", RED))
            pause()
            return
        if not nodes:
            print("暂时没有主机上报。")
            pause()
            return
        options: list[tuple[str, str]] = []
        for index, node in enumerate(nodes, start=1):
            ip = str(node.get("ip") or "等待上报")
            try:
                is_local = ipaddress.ip_address(ip).is_loopback
            except ValueError:
                is_local = False
            tag = color("本机", DIM) if is_local else ("已放行" if firewall_allows(ip) else "未放行")
            label = f"{node.get('name') or node.get('id')} | {node.get('status', 'unknown')} | {ip} | {tag}"
            options.append((str(index), label))
        selected = choose("选择一台主机", options)
        if selected is None:
            return
        node = nodes[int(selected) - 1]
        ip = str(node.get("ip") or "-")
        try:
            is_local = ipaddress.ip_address(ip).is_loopback
        except ValueError:
            is_local = False
        title("主机详情")
        print(f"名称：{node.get('name') or '-'}")
        print(f"节点 ID：{node.get('id') or '-'}")
        print(f"状态：{node.get('status') or '-'}")
        print(f"来源 IP：{ip}")
        if is_local:
            print()
            print(color("本机节点通过 127.0.0.1 直接访问 API，无需配置远程防火墙。", GREEN))
            if choose("本机操作", [("1", "删除本机监控")]) == "1":
                remove_agent()
            continue
        action = choose(
            "主机操作",
            [
                ("1", f"允许该主机访问 {agent_port()} 并保存"),
                ("2", "移除该主机放行规则并保存"),
                ("3", "删除该主机（防火墙规则+数据）"),
            ],
        )
        if action == "1":
            allow_node_firewall(node)
        elif action == "2":
            remove_node_firewall(node)
        elif action == "3":
            remove_remote_node(node)


def advanced_menu() -> None:
    while True:
        title("高级设置")
        options = [
            ("1", "服务启停与实时日志"),
            ("2", "重新安装依赖与综合诊断"),
        ]
        selected = choose("只有需要自定义时才使用这里", options)
        if selected is None:
            return
        if selected == "1":
            service_menu()
        elif selected == "2":
            maintenance_menu()


def first_setup(role: str) -> None:
    if role == "center":
        install_panel()
    else:
        configure_agent(local=False)


def main() -> int:
    while True:
        role = installation_role()
        title("主菜单")
        api, _ = service_state(API_SERVICE)
        agent, _ = service_state(AGENT_SERVICE)
        if role == "new":
            print("检测结果：尚未配置")
            selected = choose(
                "这台 VPS 用来做什么？",
                [
                    ("1", "作为中心服务器（打开监控网页）"),
                    ("2", "作为监控节点（接入已有中心）"),
                    ("3", "高级设置"),
                    ("0", "退出"),
                ],
                allow_back=False,
            )
            if selected == "1":
                install_panel()
            elif selected == "2":
                configure_agent(local=False)
            elif selected == "3":
                advanced_menu()
            else:
                return 0
            continue

        role_name = "中心服务器" if role == "center" else "监控节点"
        relevant_state = api if role == "center" else agent
        print(f"检测结果：{role_name}  |  服务 {state_badge(relevant_state)}")
        selected = choose(
            "常用操作",
            (
                [
                    ("1", "安装中心 VPS 本机监控"),
                    ("2", "查看运行状态"),
                    ("3", "查看 token"),
                    ("4", "监控主机"),
                    ("5", "添加新主机"),
                    ("6", f"防火墙配置（端口 {agent_port()}）"),
                    ("7", "重新部署中心面板"),
                    ("8", "SSL 证书设置" if not https_is_on() else "SSL 证书设置"),
                    ("9", "更新程序"),
                    ("10", "重启服务"),
                    ("11", "完整卸载"),
                    ("0", "退出"),
                ]
                if role == "center" and not AGENT_CONFIG.exists()
                else [
                    ("1", "查看运行状态"),
                    ("2", "查看 token"),
                    ("3", "修改本机信息"),
                    ("4", "重新设置本机 Agent"),
                    ("5", "监控主机"),
                    ("6", "添加新主机"),
                    ("7", f"防火墙配置（端口 {agent_port()}）"),
                    ("8", "重新部署中心面板"),
                    ("9", "SSL 证书设置" if not https_is_on() else "SSL 证书设置"),
                    ("10", "更新程序"),
                    ("11", "重启服务"),
                    ("12", "完整卸载"),
                    ("0", "退出"),
                ]
                if role == "center"
                else [
                    ("1", "查看运行状态"),
                    ("2", "查看 token"),
                    ("3", "修改本机信息"),
                    ("4", "重新设置 Agent"),
                    ("5", "更新程序"),
                    ("6", "重启服务"),
                    ("7", "完整卸载"),
                    ("0", "退出"),
                ]
            ),
            allow_back=False,
        )
        if role == "center" and not AGENT_CONFIG.exists():
            if selected == "1":
                configure_agent(local=True)
            elif selected == "2":
                show_overview()
            elif selected == "3":
                show_token()
            elif selected == "4":
                monitored_hosts_menu()
            elif selected == "5":
                temp_open_for_new_agent()
            elif selected == "6":
                firewall_rules_menu()
            elif selected == "7":
                install_panel()
            elif selected == "8":
                toggle_https()
            elif selected == "9":
                quick_update()
            elif selected == "10":
                restart_services()
            elif selected == "11":
                full_uninstall()
            else:
                return 0
            continue
        if role == "center":
            if selected == "1":
                show_overview()
            elif selected == "2":
                show_token()
            elif selected == "3":
                edit_node_info()
            elif selected == "4":
                configure_agent(local=True)
            elif selected == "5":
                monitored_hosts_menu()
            elif selected == "6":
                temp_open_for_new_agent()
            elif selected == "7":
                firewall_rules_menu()
            elif selected == "8":
                install_panel()
            elif selected == "9":
                toggle_https()
            elif selected == "10":
                quick_update()
            elif selected == "11":
                restart_services()
            elif selected == "12":
                full_uninstall()
            else:
                return 0
            continue
        if selected == "1":
            show_overview()
        elif selected == "2":
            show_token()
        elif selected == "3":
            edit_node_info()
        elif selected == "4":
            configure_agent(local=False)
        elif selected == "5":
            quick_update()
        elif selected == "6":
            restart_services()
        elif selected == "7":
            full_uninstall()
        else:
            return 0


if __name__ == "__main__":
    try:
        parser = argparse.ArgumentParser(description="VPS Monitor terminal manager")
        parser.add_argument("--setup", choices=("center", "agent"))
        arguments = parser.parse_args()
        if arguments.setup:
            first_setup(arguments.setup)
        raise SystemExit(main())
    except KeyboardInterrupt:
        print("\n已退出。")
