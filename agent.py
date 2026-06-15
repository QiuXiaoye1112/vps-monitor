from __future__ import annotations

import argparse
import calendar
import json
import os
import platform
import sys
import threading
import time
from datetime import datetime
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
    config.setdefault("traffic_reset_day", int(os.getenv("VPS_MONITOR_TRAFFIC_RESET_DAY", "0")))
    config.setdefault("traffic_reset_hour", int(os.getenv("VPS_MONITOR_TRAFFIC_RESET_HOUR", "0")))
    config.setdefault("traffic_limit_gb", float(os.getenv("VPS_MONITOR_TRAFFIC_LIMIT_GB", "0")))
    config.setdefault("traffic_offset_gb", float(os.getenv("VPS_MONITOR_TRAFFIC_OFFSET_GB", "0")))
    default_state_path = (path or Path("agent.toml")).with_suffix(".traffic-state.json")
    config.setdefault("traffic_state_path", os.getenv("VPS_MONITOR_TRAFFIC_STATE", str(default_state_path)))

    config["disk_paths"] = parse_items(config.get("disk_paths"))
    config["interval"] = max(1, int(config["interval"]))
    config["traffic_reset_day"] = min(31, max(0, int(config["traffic_reset_day"])))
    config["traffic_reset_hour"] = min(23, max(0, int(config["traffic_reset_hour"])))
    config["traffic_limit_gb"] = max(0.0, float(config["traffic_limit_gb"]))
    config["traffic_offset_gb"] = max(0.0, float(config["traffic_offset_gb"]))
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
        print(f"register failed: {exc.__class__.__name__}", file=sys.stderr, flush=True)
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


def traffic_cycle_key(now: datetime, reset_day: int, reset_hour: int) -> str:
    def cycle_start(year: int, month: int) -> datetime:
        day = min(reset_day, calendar.monthrange(year, month)[1])
        return now.replace(year=year, month=month, day=day, hour=reset_hour, minute=0, second=0, microsecond=0)

    start = cycle_start(now.year, now.month)
    if now < start:
        if now.month == 1:
            start = cycle_start(now.year - 1, 12)
        else:
            start = cycle_start(now.year, now.month - 1)
    return start.isoformat()


def load_traffic_state(path: Path) -> dict[str, Any]:
    try:
        loaded = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError, TypeError):
        return {}
    return loaded if isinstance(loaded, dict) else {}


def save_traffic_state(path: Path, state: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f".{path.name}.tmp")
    persisted = {key: value for key, value in state.items() if not key.startswith("_")}
    temporary.write_text(json.dumps(persisted, separators=(",", ":")), encoding="utf-8")
    temporary.chmod(0o600)
    temporary.replace(path)


def update_monthly_traffic(
    current_net: dict[str, float],
    config: dict[str, Any],
    state: dict[str, Any],
    *,
    now: datetime | None = None,
) -> bool:
    current_time = now or datetime.now().astimezone()
    reset_day = int(config["traffic_reset_day"])
    cycle = (
        traffic_cycle_key(current_time, reset_day, int(config["traffic_reset_hour"]))
        if reset_day > 0
        else "never"
    )
    current_sent = int(current_net["bytes_sent"])
    current_recv = int(current_net["bytes_recv"])
    cycle_changed = state.get("cycle") != cycle
    if cycle_changed:
        offset_bytes = int(float(config["traffic_offset_gb"]) * 1073741824)
        state.clear()
        state.update(
            {
                "cycle": cycle,
                "last_sent": current_sent,
                "last_recv": current_recv,
                "tx": offset_bytes // 2,
                "rx": offset_bytes - offset_bytes // 2,
            }
        )
        return True

    previous_sent = int(state.get("last_sent", current_sent))
    previous_recv = int(state.get("last_recv", current_recv))
    sent_delta = current_sent - previous_sent if current_sent >= previous_sent else current_sent
    recv_delta = current_recv - previous_recv if current_recv >= previous_recv else current_recv
    state["tx"] = max(0, int(state.get("tx", 0)) + sent_delta)
    state["rx"] = max(0, int(state.get("rx", 0)) + recv_delta)
    state["last_sent"] = current_sent
    state["last_recv"] = current_recv
    return False


def report_once(
    config: dict[str, Any],
    previous_net: dict[str, float] | None,
    cpu_percent: float | None = None,
    traffic_state: dict[str, Any] | None = None,
) -> tuple[dict[str, float], dict[str, Any]]:
    metrics, current_net = collect_metrics(
        disk_paths=config["disk_paths"],
        previous_net=previous_net,
        cpu_percent=cpu_percent,
    )
    traffic_state = traffic_state if traffic_state is not None else {}
    cycle_changed = update_monthly_traffic(current_net, config, traffic_state)
    metrics["net_tx_month"] = int(traffic_state["tx"])
    metrics["net_rx_month"] = int(traffic_state["rx"])
    metrics["traffic_limit_gb"] = float(config["traffic_limit_gb"])
    metrics["traffic_reset_enabled"] = int(config["traffic_reset_day"]) > 0

    save_after = float(traffic_state.get("_save_after", 0))
    if cycle_changed or time.monotonic() >= save_after:
        try:
            save_traffic_state(Path(str(config["traffic_state_path"])), traffic_state)
        except OSError as exc:
            print(f"traffic state save failed: {exc.__class__.__name__}", file=sys.stderr, flush=True)
        traffic_state["_save_after"] = time.monotonic() + 60

    payload = {"node_id": config["node_id"], **metrics}
    response = requests.post(
        api_url(config, "/api/metrics"),
        json=payload,
        headers=headers(config),
        timeout=request_timeout(config),
    )
    response.raise_for_status()
    return current_net, traffic_state


def run_agent(config: dict[str, Any], once: bool = False) -> int:
    previous_net: dict[str, float] | None = None
    traffic_state = load_traffic_state(Path(str(config["traffic_state_path"])))
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
            previous_net, traffic_state = report_once(
                config,
                previous_net,
                cpu_percent=cpu_percent,
                traffic_state=traffic_state,
            )
            elapsed_ms = int((time.monotonic() - report_started_at) * 1000)
            print(f"reported metrics in {elapsed_ms}ms", flush=True)
            if once:
                return 0
            next_report_at = max(next_report_at + float(config["interval"]), time.monotonic())
        except KeyboardInterrupt:
            print("agent stopped", flush=True)
            try:
                save_traffic_state(Path(str(config["traffic_state_path"])), traffic_state)
            except OSError:
                pass
            if cpu_sampler:
                cpu_sampler.stop()
            return 0
        except requests.RequestException as exc:
            print(f"report failed: {exc.__class__.__name__}", file=sys.stderr, flush=True)
            if once:
                return 1
            next_report_at = max(next_report_at + float(config["interval"]), time.monotonic())
        except Exception as exc:
            print(f"agent error: {exc.__class__.__name__}", file=sys.stderr, flush=True)
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
