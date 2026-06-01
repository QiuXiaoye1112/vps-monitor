from __future__ import annotations

import argparse
import os
import platform
import sys
import threading
import time
from pathlib import Path
from typing import Any

import psutil
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


def request_timeout(config: dict[str, Any]) -> float:
    return min(2.0, max(0.5, float(config["interval"]) * 0.8))


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
    response = requests.post(
        api_url(config, "/api/nodes/register"),
        json=payload,
        headers=headers(config),
        timeout=request_timeout(config),
    )
    response.raise_for_status()


def try_register_node(config: dict[str, Any]) -> bool:
    try:
        register_node(config)
    except requests.RequestException as exc:
        print(f"register failed: {exc}", file=sys.stderr, flush=True)
        return False
    return True


class CpuSampler:
    def __init__(self, interval: float) -> None:
        self.interval = max(1.0, interval)
        self.value = 0.0
        self.lock = threading.Lock()
        self.stop_event = threading.Event()
        self.thread = threading.Thread(target=self.run, daemon=True)

    def start(self) -> None:
        psutil.cpu_percent(interval=None)
        self.thread.start()

    def stop(self) -> None:
        self.stop_event.set()

    def current(self) -> float:
        with self.lock:
            return self.value

    def run(self) -> None:
        while not self.stop_event.is_set():
            value = psutil.cpu_percent(interval=self.interval)
            with self.lock:
                self.value = float(value)


def report_once(
    config: dict[str, Any],
    previous_net: dict[str, float] | None,
    cpu_percent: float | None = None,
) -> dict[str, float]:
    metrics, current_net = collect_metrics(
        disk_paths=config["disk_paths"],
        previous_net=previous_net,
        cpu_percent=cpu_percent,
    )
    payload = {"node_id": config["node_id"], **metrics}
    response = requests.post(
        api_url(config, "/api/metrics"),
        json=payload,
        headers=headers(config),
        timeout=request_timeout(config),
    )
    response.raise_for_status()
    return current_net


def run_agent(config: dict[str, Any], once: bool = False) -> int:
    previous_net: dict[str, float] | None = None
    register_after = 0.0
    next_report_at = time.monotonic()
    cpu_sampler = None if once else CpuSampler(float(config["interval"]))
    if cpu_sampler:
        cpu_sampler.start()

    while True:
        try:
            wait_seconds = next_report_at - time.monotonic()
            if wait_seconds > 0:
                time.sleep(wait_seconds)

            started_at = time.monotonic()
            if started_at >= register_after:
                try_register_node(config)
                register_after = started_at + 300
            cpu_percent = cpu_sampler.current() if cpu_sampler else None
            report_started_at = time.monotonic()
            previous_net = report_once(config, previous_net, cpu_percent=cpu_percent)
            elapsed_ms = int((time.monotonic() - report_started_at) * 1000)
            print(f"reported metrics for {config['node_id']} in {elapsed_ms}ms", flush=True)
            if once:
                return 0
            next_report_at = max(next_report_at + float(config["interval"]), time.monotonic())
        except KeyboardInterrupt:
            print("agent stopped", flush=True)
            if cpu_sampler:
                cpu_sampler.stop()
            return 0
        except requests.RequestException as exc:
            print(f"report failed: {exc}", file=sys.stderr, flush=True)
            if once:
                return 1
            next_report_at = max(next_report_at + float(config["interval"]), time.monotonic())
        except Exception as exc:
            print(f"agent error: {exc}", file=sys.stderr, flush=True)
            if once:
                return 1
            next_report_at = max(next_report_at + float(config["interval"]), time.monotonic())


def main() -> int:
    parser = argparse.ArgumentParser(description="VPS Monitor lightweight agent")
    parser.add_argument("--config", type=Path, default=Path("agent.toml"), help="agent TOML config path")
    parser.add_argument("--once", action="store_true", help="collect and report once")
    args = parser.parse_args()

    config = load_config(args.config)
    return run_agent(config, once=args.once)


if __name__ == "__main__":
    raise SystemExit(main())
