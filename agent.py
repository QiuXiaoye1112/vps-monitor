from __future__ import annotations

import argparse
import os
import platform
import sys
import time
from pathlib import Path
from typing import Any

import requests

from monitor_common import collect_metrics, default_disk_paths, parse_items

try:
    import tomllib
except ModuleNotFoundError:  # pragma: no cover - only used on Python < 3.11
    import tomli as tomllib


def load_config(path: Path | None) -> dict[str, Any]:
    config: dict[str, Any] = {}
    if path and path.exists():
        with path.open("rb") as handle:
            config.update(tomllib.load(handle))

    config.setdefault("server_url", os.getenv("VPS_MONITOR_SERVER_URL", "http://127.0.0.1:8000"))
    config.setdefault("node_id", os.getenv("VPS_MONITOR_NODE_ID", platform.node() or "local-node"))
    config.setdefault("token", os.getenv("VPS_MONITOR_TOKEN", "change-me"))
    config.setdefault("interval", int(os.getenv("VPS_MONITOR_AGENT_INTERVAL", "1")))
    config.setdefault("name", os.getenv("VPS_MONITOR_NODE_NAME", config["node_id"]))
    config.setdefault("ip", os.getenv("VPS_MONITOR_NODE_IP", ""))
    config.setdefault("region", os.getenv("VPS_MONITOR_NODE_REGION", ""))
    config.setdefault("os_type", os.getenv("VPS_MONITOR_NODE_OS", platform.system()))
    config.setdefault("note", os.getenv("VPS_MONITOR_NODE_NOTE", ""))
    config.setdefault("disk_paths", os.getenv("VPS_MONITOR_DISK_PATHS", ",".join(default_disk_paths())))

    config["disk_paths"] = parse_items(config.get("disk_paths"))
    config["interval"] = max(1, int(config["interval"]))
    return config


def api_url(config: dict[str, Any], path: str) -> str:
    return f"{str(config['server_url']).rstrip('/')}{path}"


def headers(config: dict[str, Any]) -> dict[str, str]:
    return {"Authorization": f"Bearer {config['token']}", "Content-Type": "application/json"}


def register_node(config: dict[str, Any]) -> None:
    payload = {
        "node_id": config["node_id"],
        "name": config["name"],
        "ip": config["ip"],
        "region": config["region"],
        "os_type": config["os_type"],
        "note": config["note"],
        "services": [],
    }
    response = requests.post(api_url(config, "/api/nodes/register"), json=payload, headers=headers(config), timeout=8)
    response.raise_for_status()


def report_once(config: dict[str, Any], previous_net: dict[str, float] | None) -> dict[str, float]:
    if previous_net is None:
        _, previous_net = collect_metrics(disk_paths=config["disk_paths"], previous_net=None)
        time.sleep(min(1.0, float(config["interval"])))

    metrics, current_net = collect_metrics(disk_paths=config["disk_paths"], previous_net=previous_net)
    payload = {"node_id": config["node_id"], **metrics}
    response = requests.post(api_url(config, "/api/metrics"), json=payload, headers=headers(config), timeout=8)
    response.raise_for_status()
    return current_net


def run_agent(config: dict[str, Any], once: bool = False) -> int:
    previous_net: dict[str, float] | None = None
    backoff = 2

    while True:
        try:
            register_node(config)
            previous_net = report_once(config, previous_net)
            backoff = 2
            print(f"reported metrics for {config['node_id']}", flush=True)
            if once:
                return 0
            time.sleep(config["interval"])
        except KeyboardInterrupt:
            print("agent stopped", flush=True)
            return 0
        except requests.RequestException as exc:
            print(f"report failed: {exc}", file=sys.stderr, flush=True)
            if once:
                return 1
            time.sleep(backoff)
            backoff = min(backoff * 2, 60)
        except Exception as exc:
            print(f"agent error: {exc}", file=sys.stderr, flush=True)
            if once:
                return 1
            time.sleep(backoff)
            backoff = min(backoff * 2, 60)


def main() -> int:
    parser = argparse.ArgumentParser(description="VPS Monitor lightweight agent")
    parser.add_argument("--config", type=Path, default=Path("agent.toml"), help="agent TOML config path")
    parser.add_argument("--once", action="store_true", help="collect and report once")
    args = parser.parse_args()

    config = load_config(args.config)
    return run_agent(config, once=args.once)


if __name__ == "__main__":
    raise SystemExit(main())
