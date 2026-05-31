from __future__ import annotations

import shlex
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import paramiko

from monitor_common import parse_items


BASE_DIR = Path(__file__).resolve().parent
AGENT_FILES = ["agent.py", "monitor_common.py", "settings.py", "requirements-agent.txt"]


class DeploymentError(RuntimeError):
    pass


@dataclass
class DeployConfig:
    host: str
    port: int
    username: str
    password: str
    use_sudo: bool
    remote_dir: str
    server_url: str
    node_id: str
    token: str
    interval: int
    name: str
    node_ip: str
    region: str
    os_type: str
    note: str
    services: list[str]
    disk_paths: list[str]


def q(value: str) -> str:
    return shlex.quote(value)


def agent_config(config: DeployConfig) -> str:
    disk_paths = config.disk_paths or ["/"]
    return "\n".join(
        [
            f"server_url = {config.server_url!r}",
            f"node_id = {config.node_id!r}",
            f"token = {config.token!r}",
            f"interval = {config.interval}",
            "",
            f"name = {config.name!r}",
            f"ip = {config.node_ip!r}",
            f"region = {config.region!r}",
            f"os_type = {config.os_type!r}",
            f"note = {config.note!r}",
            "",
            "disk_paths = [" + ", ".join(repr(path) for path in disk_paths) + "]",
            "",
        ]
    )


def service_file(config: DeployConfig) -> str:
    remote_dir = config.remote_dir.rstrip("/")
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


def validate_config(data: dict[str, Any]) -> DeployConfig:
    required = ["host", "username", "password", "server_url", "node_id", "name"]
    missing = [field for field in required if not str(data.get(field) or "").strip()]
    if missing:
        raise DeploymentError("缺少必填项：" + ", ".join(missing))

    remote_dir = str(data.get("remote_dir") or "/opt/vps-monitor").strip()
    if not remote_dir.startswith("/"):
        raise DeploymentError("远程安装目录必须是 Linux 绝对路径。")

    return DeployConfig(
        host=str(data["host"]).strip(),
        port=int(data.get("port") or 22),
        username=str(data["username"]).strip(),
        password=str(data["password"]),
        use_sudo=bool(data.get("use_sudo")),
        remote_dir=remote_dir,
        server_url=str(data["server_url"]).strip(),
        node_id=str(data["node_id"]).strip(),
        token=str(data.get("token") or "").strip(),
        interval=max(2, int(data.get("interval") or 10)),
        name=str(data["name"]).strip(),
        node_ip=str(data.get("node_ip") or "").strip(),
        region=str(data.get("region") or "").strip(),
        os_type=str(data.get("os_type") or "Linux").strip(),
        note=str(data.get("note") or "").strip(),
        services=[],
        disk_paths=parse_items(data.get("disk_paths")) or ["/"],
    )


def validate_agent_files() -> list[Path]:
    files = [BASE_DIR / filename for filename in AGENT_FILES]
    missing = [str(path) for path in files if not path.exists()]
    if missing:
        raise DeploymentError("缺少 agent 文件：" + ", ".join(missing))
    return files


def connect(config: DeployConfig) -> paramiko.SSHClient:
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(
        hostname=config.host,
        port=config.port,
        username=config.username,
        password=config.password,
        look_for_keys=False,
        allow_agent=False,
        timeout=15,
        auth_timeout=15,
        banner_timeout=15,
    )
    return client


def run_command(
    client: paramiko.SSHClient,
    command: str,
    config: DeployConfig,
    logs: list[str],
    *,
    privileged: bool = False,
    timeout: int = 180,
) -> str:
    if privileged and config.use_sudo:
        command = f"sudo -S -p '' {command}"

    logs.append(f"$ {command.replace(config.password, '******')}")
    stdin, stdout, stderr = client.exec_command(command, get_pty=privileged and config.use_sudo, timeout=timeout)
    if privileged and config.use_sudo:
        stdin.write(config.password + "\n")
        stdin.flush()

    exit_code = stdout.channel.recv_exit_status()
    output = stdout.read().decode("utf-8", errors="replace").strip()
    error = stderr.read().decode("utf-8", errors="replace").strip()
    if output:
        logs.append(output[-1200:])
    if error:
        logs.append(error[-1200:])
    if exit_code != 0:
        raise DeploymentError(f"远程命令失败（exit {exit_code}）：{command}")
    return output


def upload_text(sftp: paramiko.SFTPClient, path: str, content: str) -> None:
    with sftp.file(path, "w") as remote_file:
        remote_file.write(content)


def deploy_agent(data: dict[str, Any]) -> list[str]:
    config = validate_config(data)
    files = validate_agent_files()
    logs = [f"连接 {config.username}@{config.host}:{config.port}"]
    remote_dir = config.remote_dir.rstrip("/")
    tmp_config = f"/tmp/vps-monitor-agent-{config.node_id}.toml"
    tmp_service = f"/tmp/vps-monitor-agent-{config.node_id}.service"

    client = connect(config)
    try:
        run_command(client, f"mkdir -p {q(remote_dir)}", config, logs, privileged=True)
        if config.use_sudo:
            run_command(client, f"chown {q(config.username)} {q(remote_dir)}", config, logs, privileged=True)

        with client.open_sftp() as sftp:
            for file_path in files:
                sftp.put(str(file_path), f"{remote_dir}/{file_path.name}")
            upload_text(sftp, tmp_config, agent_config(config))
            upload_text(sftp, tmp_service, service_file(config))

        run_command(client, f"mv {q(tmp_config)} /etc/vps-monitor-agent.toml", config, logs, privileged=True)
        run_command(client, f"mv {q(tmp_service)} /etc/systemd/system/vps-monitor-agent.service", config, logs, privileged=True)
        run_command(client, "chmod 600 /etc/vps-monitor-agent.toml", config, logs, privileged=True)
        run_command(client, f"python3 -m venv {q(remote_dir)}/.venv", config, logs)
        run_command(
            client,
            f"{q(remote_dir)}/.venv/bin/python -m pip install -r {q(remote_dir)}/requirements-agent.txt",
            config,
            logs,
            timeout=300,
        )
        run_command(client, "systemctl daemon-reload", config, logs, privileged=True)
        run_command(client, "systemctl enable --now vps-monitor-agent", config, logs, privileged=True)
        state = run_command(client, "systemctl is-active vps-monitor-agent", config, logs, privileged=True, timeout=30)
        logs.append(f"Agent 状态：{state or 'unknown'}")
        return logs
    finally:
        client.close()
