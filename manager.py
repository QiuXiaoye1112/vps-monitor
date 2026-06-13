#!/usr/bin/env python3
from __future__ import annotations

import argparse
import getpass
import ipaddress
import os
import re
import secrets
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
LAUNCHER = Path("/usr/local/bin/vps-monitor")
ROLE_FILE = Path("/etc/vps-monitor-role")
API_SERVICE = "vps-monitor-api"
AGENT_SERVICE = "vps-monitor-agent"

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
    print(color("此操作需要 root 权限。请退出后使用 sudo vps-monitor 启动。", RED))
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
    print(color(f"\n> {' '.join(args)}", DIM))
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


def install_launcher() -> None:
    launcher = textwrap.dedent(
        f"""\
        #!/usr/bin/env bash
        exec {shlex_quote(sys.executable)} {shlex_quote(str(PROJECT_DIR / 'manager.py'))} "$@"
        """
    )
    write_text_secure(LAUNCHER, launcher, 0o755)


def shlex_quote(value: str) -> str:
    import shlex

    return shlex.quote(value)


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
    run(["apt-get", "update"])
    run(["apt-get", "install", "-y", *packages])


def ensure_venv(requirements: str) -> None:
    if not VENV_DIR.exists():
        run([sys.executable, "-m", "venv", str(VENV_DIR)])
    run([str(VENV_DIR / "bin/python"), "-m", "pip", "install", "--upgrade", "pip"])
    run(
        [
            str(VENV_DIR / "bin/python"),
            "-m",
            "pip",
            "install",
            "-r",
            str(PROJECT_DIR / requirements),
        ]
    )


def api_unit() -> str:
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
        ExecStart={VENV_DIR}/bin/python -m uvicorn server:app --host 127.0.0.1 --port 8000
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
    api_active, api_enabled = service_state(API_SERVICE)
    agent_active, agent_enabled = service_state(AGENT_SERVICE)
    ok, _ = health_check("http://127.0.0.1:8000/api/health")

    print(f"项目目录      {PROJECT_DIR}")
    print(f"中心 API      {state_badge(api_active)} / 自启 {state_badge(api_enabled)}")
    print(f"本机 Agent    {state_badge(agent_active)} / 自启 {state_badge(agent_enabled)}")
    print(f"API 健康检查  {color('正常', GREEN) if ok else color('不可用', RED)}")
    print(f"中心配置      {'已创建' if SERVER_ENV.exists() else '未创建'}")
    print(f"Agent 配置    {'已创建' if AGENT_CONFIG.exists() else '未创建'}")

    env = read_env(SERVER_ENV)
    db_path = Path(env.get("VPS_MONITOR_DB", PROJECT_DIR / "vps_monitor.db"))
    if db_path.exists():
        print(f"数据库        {db_path} ({db_path.stat().st_size / 1024 / 1024:.2f} MB)")
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
    print("\n即将安装 Python、Nginx、API 服务并写入中心配置。")
    if not confirm("确认继续？", True):
        return
    try:
        ensure_apt_packages(["python3", "python3-venv", "python3-pip", "nginx", "curl", "sqlite3"])
        ensure_venv("requirements.txt")
        write_text_secure(
            SERVER_ENV,
            f"VPS_MONITOR_TOKEN={token}\nVPS_MONITOR_DB={PROJECT_DIR / 'vps_monitor.db'}\n",
        )
        write_text_secure(SYSTEMD_DIR / f"{API_SERVICE}.service", api_unit(), 0o644)
        nginx_site = Path("/etc/nginx/sites-available/vps-monitor.conf")
        nginx_link = Path("/etc/nginx/sites-enabled/vps-monitor.conf")
        nginx_config = textwrap.dedent(
            f"""\
            server {{
                listen 80;
                server_name {domain};
                client_max_body_size 1m;

                location / {{
                    proxy_pass http://127.0.0.1:8000;
                    proxy_http_version 1.1;
                    proxy_set_header Host $host;
                    proxy_set_header X-Real-IP $remote_addr;
                    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
                    proxy_set_header X-Forwarded-Proto $scheme;
                }}
            }}
            """
        )
        write_text_secure(nginx_site, nginx_config, 0o644)
        nginx_link.parent.mkdir(parents=True, exist_ok=True)
        if nginx_link.is_symlink() or nginx_link.exists():
            nginx_link.unlink()
        nginx_link.symlink_to(nginx_site)
        install_launcher()
        run(["nginx", "-t"])
        run(["systemctl", "daemon-reload"])
        run(["systemctl", "enable", "--now", API_SERVICE, "nginx"])
        run(["systemctl", "reload", "nginx"])
        ok, detail = health_check("http://127.0.0.1:8000/api/health", timeout=5)
        print(color("\n中心面板部署完成。", GREEN))
        print(f"访问地址：http://{domain}")
        print(f"健康检查：{'正常' if ok else '失败 - ' + detail}")
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
    default_url = "http://127.0.0.1:8000" if local else "http://中心VPS公网IP:8080"
    server_url = ask("中心服务地址", default_url)
    node_id = ask("节点 ID（每台机器必须不同）", "center" if local else socket.gethostname())
    name = ask("面板显示名", "中心 VPS" if local else socket.gethostname())
    token = ask("通信 token", default_token, secret=True)
    interval_text = ask("上报间隔（秒）", "1")
    disk_paths = ask("监控磁盘路径，多个用逗号分隔", "/")
    try:
        interval = max(1, int(interval_text))
    except ValueError:
        print(color("上报间隔必须是整数。", RED))
        pause()
        return
    if not server_url or not node_id or not token:
        print(color("服务地址、节点 ID 和 token 不能为空。", RED))
        pause()
        return
    values: dict[str, object] = {
        "server_url": server_url,
        "node_id": node_id,
        "name": name,
        "token": token,
        "interval": interval,
        "disk_paths": [item.strip() for item in disk_paths.split(",") if item.strip()],
        "os_type": "Linux",
    }
    print("\n即将安装 Agent 依赖、写入配置、测试上报并启用开机自启。")
    if not confirm("确认继续？", True):
        return
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
        if test.returncode != 0 and not confirm("测试上报失败，仍然安装并启动服务？"):
            return
        run(["systemctl", "enable", "--now", AGENT_SERVICE])
        run(["systemctl", "restart", AGENT_SERVICE])
        print(color("\nAgent 已配置并启动。", GREEN))
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
                ("1", "开启或更新 8080 Agent 入口"),
                ("2", "允许一台 Agent IP"),
                ("3", "查看 8080 防火墙规则"),
                ("4", "保存当前防火墙规则"),
                ("5", "申请 Dashboard HTTPS 证书"),
            ],
        )
        if selected is None:
            return
        if not require_root():
            continue
        try:
            if selected == "1":
                port = ask("Agent 入口端口", "8080")
                env = os.environ.copy()
                env["AGENT_PORT"] = port
                run(["bash", str(PROJECT_DIR / "deploy_agent_ingress.sh")], env=env)
            elif selected == "2":
                agent_ip = ask("Agent 公网 IP")
                ipaddress.ip_address(agent_ip)
                port = ask("Agent 入口端口", "8080")
                env = os.environ.copy()
                env["AGENT_PORT"] = port
                run(["bash", str(PROJECT_DIR / "allow_agent_ip.sh"), agent_ip], env=env)
            elif selected == "3":
                run(["iptables", "-S", "INPUT"], check=False)
            elif selected == "4":
                if not command_exists("netfilter-persistent"):
                    run(["apt-get", "install", "-y", "iptables-persistent"])
                run(["netfilter-persistent", "save"])
            else:
                domain = ask("已解析到本机的域名")
                ensure_apt_packages(["certbot", "python3-certbot-nginx"])
                run(["certbot", "--nginx", "-d", domain])
        except (OSError, ValueError, subprocess.CalledProcessError, RuntimeError) as exc:
            print(color(f"操作失败：{exc}", RED))
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


def update_project() -> None:
    if not (PROJECT_DIR / ".git").exists():
        print(color("当前目录不是 Git 仓库，无法在线更新。", YELLOW))
        return
    status = run(["git", "status", "--porcelain"], capture=True, check=False)
    if status.stdout.strip():
        print(color("检测到本地改动，为避免覆盖，本次更新已取消。", RED))
        return
    run(["git", "pull", "--ff-only"])
    if VENV_DIR.exists():
        requirement = "requirements.txt" if SERVER_ENV.exists() else "requirements-agent.txt"
        ensure_venv(requirement)
    for service in (API_SERVICE, AGENT_SERVICE):
        active, _ = service_state(service)
        if active == "active":
            run(["systemctl", "restart", service])
    print(color("项目更新完成。", GREEN))


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
    ok, detail = health_check("http://127.0.0.1:8000/api/health")
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


def quick_logs() -> None:
    role = installation_role()
    if role == "center":
        selected = choose("查看日志", [("1", "中心 API"), ("2", "本机 Agent")])
        if selected is None:
            return
        service = API_SERVICE if selected == "1" else AGENT_SERVICE
    elif role == "agent":
        service = AGENT_SERVICE
    else:
        print(color("当前还没有安装服务。", YELLOW))
        pause()
        return
    run(["journalctl", "-u", service, "-n", "80", "--no-pager"], check=False)
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


def advanced_menu() -> None:
    while True:
        title("高级设置")
        selected = choose(
            "只有需要自定义时才使用这里",
            [
                ("1", "重新部署中心面板"),
                ("2", "重新配置本机或远程 Agent"),
                ("3", "服务启停与实时日志"),
                ("4", "Agent 入口、防火墙与 HTTPS"),
                ("5", "备份、依赖与综合诊断"),
            ],
        )
        if selected is None:
            return
        if selected == "1":
            install_panel()
        elif selected == "2":
            configure_agent(local=SERVER_ENV.exists())
        elif selected == "3":
            service_menu()
        elif selected == "4":
            ingress_menu()
        else:
            maintenance_menu()


def first_setup(role: str) -> None:
    if role == "center":
        install_panel()
        if SERVER_ENV.exists():
            configure_agent(local=True)
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
            [
                ("1", "查看运行状态"),
                ("2", "查看最近日志"),
                ("3", "更新程序"),
                ("4", "高级设置"),
                ("0", "退出"),
            ],
            allow_back=False,
        )
        if selected == "1":
            show_overview()
        elif selected == "2":
            quick_logs()
        elif selected == "3":
            quick_update()
        elif selected == "4":
            advanced_menu()
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
