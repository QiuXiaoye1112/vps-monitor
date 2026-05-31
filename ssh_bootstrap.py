from __future__ import annotations

import argparse
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path

from monitor_common import parse_items


BASE_DIR = Path(__file__).resolve().parent
AGENT_FILES = ["agent.py", "monitor_common.py", "settings.py", "requirements-agent.txt"]


def run(command: list[str], *, timeout: int = 120) -> None:
    print("+ " + " ".join(command), flush=True)
    result = subprocess.run(command, timeout=timeout, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"command failed with exit code {result.returncode}")


def ssh_base(args: argparse.Namespace) -> list[str]:
    command = ["ssh", "-p", str(args.port), "-o", "StrictHostKeyChecking=accept-new"]
    if args.key:
        command.extend(["-i", str(args.key)])
    return command


def scp_base(args: argparse.Namespace) -> list[str]:
    command = ["scp", "-P", str(args.port), "-o", "StrictHostKeyChecking=accept-new"]
    if args.key:
        command.extend(["-i", str(args.key)])
    return command


def remote(args: argparse.Namespace) -> str:
    return f"{args.user}@{args.host}"


def sudo_command(args: argparse.Namespace, command: str) -> str:
    return f"sudo {command}" if args.sudo else command


def ssh(args: argparse.Namespace, command: str, *, timeout: int = 120) -> None:
    run([*ssh_base(args), remote(args), command], timeout=timeout)


def scp(args: argparse.Namespace, sources: list[Path], target: str, *, timeout: int = 120) -> None:
    run([*scp_base(args), *[str(source) for source in sources], f"{remote(args)}:{target}"], timeout=timeout)


def agent_config(args: argparse.Namespace) -> str:
    disk_paths = parse_items(args.disk_paths) or ["/"]
    return "\n".join(
        [
            f"server_url = {args.server_url!r}",
            f"node_id = {args.node_id!r}",
            f"token = {args.token!r}",
            f"interval = {args.interval}",
            "",
            f"name = {args.name!r}",
            f"os_type = {args.os_type!r}",
            "",
            "disk_paths = [" + ", ".join(repr(path) for path in disk_paths) + "]",
            "",
        ]
    )


def service_file(args: argparse.Namespace) -> str:
    remote_dir = args.remote_dir.rstrip("/")
    return f"""[Unit]
Description=VPS Monitor Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory={remote_dir}
ExecStart={remote_dir}/.venv/bin/python {remote_dir}/agent.py --config /etc/vps-monitor-agent.toml
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
"""


def write_temp_file(content: str, suffix: str) -> Path:
    handle = tempfile.NamedTemporaryFile("w", suffix=suffix, encoding="utf-8", delete=False)
    with handle:
        handle.write(content)
    return Path(handle.name)


def validate_files() -> list[Path]:
    files = [BASE_DIR / name for name in AGENT_FILES]
    missing = [str(path) for path in files if not path.exists()]
    if missing:
        raise FileNotFoundError("missing files: " + ", ".join(missing))
    return files


def bootstrap(args: argparse.Namespace) -> None:
    if not args.remote_dir.startswith("/"):
        raise ValueError("--remote-dir must be an absolute Linux path")

    files = validate_files()
    config_path = write_temp_file(agent_config(args), ".toml")
    service_path = write_temp_file(service_file(args), ".service")

    try:
        quoted_remote_dir = shlex.quote(args.remote_dir)
        ssh(args, sudo_command(args, f"mkdir -p {quoted_remote_dir}"))
        ssh(args, sudo_command(args, f"chown {shlex.quote(args.user)} {quoted_remote_dir}"))
        scp(args, files, f"{args.remote_dir.rstrip('/')}/")
        scp(args, [config_path], "/tmp/vps-monitor-agent.toml")
        scp(args, [service_path], "/tmp/vps-monitor-agent.service")

        ssh(args, sudo_command(args, "mv /tmp/vps-monitor-agent.toml /etc/vps-monitor-agent.toml"))
        ssh(args, sudo_command(args, "mv /tmp/vps-monitor-agent.service /etc/systemd/system/vps-monitor-agent.service"))
        ssh(args, sudo_command(args, "chmod 600 /etc/vps-monitor-agent.toml"))
        ssh(args, f"python3 -m venv {quoted_remote_dir}/.venv")
        ssh(args, f"{quoted_remote_dir}/.venv/bin/python -m pip install -r {quoted_remote_dir}/requirements-agent.txt", timeout=300)
        ssh(args, sudo_command(args, "systemctl daemon-reload"))
        ssh(args, sudo_command(args, "systemctl enable --now vps-monitor-agent"))
        ssh(args, sudo_command(args, "systemctl --no-pager --full status vps-monitor-agent"), timeout=30)
    finally:
        config_path.unlink(missing_ok=True)
        service_path.unlink(missing_ok=True)


def main() -> int:
    parser = argparse.ArgumentParser(description="Install VPS Monitor agent over SSH")
    parser.add_argument("--host", required=True, help="VPS SSH host or IP")
    parser.add_argument("--user", default="root", help="SSH user")
    parser.add_argument("--port", type=int, default=22, help="SSH port")
    parser.add_argument("--key", type=Path, help="SSH private key path")
    parser.add_argument("--sudo", action="store_true", help="Use sudo for system paths and systemd")
    parser.add_argument("--remote-dir", default="/opt/vps-monitor", help="Remote install directory")

    parser.add_argument("--server-url", required=True, help="Monitor server URL, for example http://1.2.3.4:8000")
    parser.add_argument("--node-id", required=True, help="Stable node id, for example hk-01")
    parser.add_argument("--token", required=True, help="Monitor token")
    parser.add_argument("--interval", type=int, default=10, help="Report interval in seconds")

    parser.add_argument("--name", required=True, help="Node display name")
    parser.add_argument("--os-type", default="Linux", help="Node OS type label")
    parser.add_argument("--disk-paths", default="/", help="Comma-separated disk paths")
    args = parser.parse_args()

    try:
        bootstrap(args)
    except Exception as exc:
        print(f"bootstrap failed: {exc}", file=sys.stderr)
        return 1

    print("VPS Monitor agent installed and started.", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
