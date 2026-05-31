from __future__ import annotations

import platform
import socket
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import psutil


def utc_now() -> datetime:
    return datetime.now(timezone.utc)


def iso_now() -> str:
    return utc_now().isoformat()


def parse_datetime(value: str | None) -> datetime | None:
    if not value:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def format_bytes(value: float | int | None) -> str:
    if value is None:
        return "-"
    size = float(value)
    for unit in ("B", "KB", "MB", "GB", "TB", "PB"):
        if abs(size) < 1024 or unit == "PB":
            return f"{size:.1f} {unit}" if unit != "B" else f"{size:.0f} {unit}"
        size /= 1024
    return f"{size:.1f} PB"


def format_duration(seconds: float | int | None) -> str:
    if seconds is None:
        return "-"
    total_seconds = max(0, int(seconds))
    days, remainder = divmod(total_seconds, 86400)
    hours, remainder = divmod(remainder, 3600)
    minutes, _ = divmod(remainder, 60)
    if days:
        return f"{days}d {hours}h"
    if hours:
        return f"{hours}h {minutes}m"
    return f"{minutes}m"


def parse_items(value: str | list[str] | tuple[str, ...] | None) -> list[str]:
    if value is None:
        return []
    if isinstance(value, (list, tuple)):
        raw_items = value
    else:
        raw_items = value.replace("\n", ",").split(",")

    items: list[str] = []
    for raw_item in raw_items:
        item = str(raw_item).strip()
        if item and item not in items:
            items.append(item)
    return items


def default_disk_paths() -> list[str]:
    if platform.system().lower() == "windows":
        return [Path.home().anchor or "C:\\"]
    return ["/"]


def disk_snapshot(paths: list[str]) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    disk_rows: list[dict[str, Any]] = []
    primary: dict[str, Any] = {"total": 0, "used": 0, "percent": 0.0, "path": ""}

    for path in paths or default_disk_paths():
        try:
            usage = psutil.disk_usage(path)
        except (FileNotFoundError, PermissionError, OSError) as exc:
            disk_rows.append({"path": path, "state": "unknown", "detail": str(exc)})
            continue

        row = {
            "path": path,
            "state": "ok",
            "total": usage.total,
            "used": usage.used,
            "free": usage.free,
            "percent": usage.percent,
        }
        disk_rows.append(row)
        if not primary["path"]:
            primary = {"total": usage.total, "used": usage.used, "percent": usage.percent, "path": path}

    return primary, disk_rows


def system_info() -> dict[str, str]:
    return {
        "hostname": socket.gethostname(),
        "os_name": f"{platform.system()} {platform.release()}".strip(),
        "kernel_version": platform.version(),
        "architecture": platform.machine(),
    }


def collect_metrics(
    service_names: list[str] | None = None,
    disk_paths: list[str] | None = None,
    previous_net: dict[str, float] | None = None,
) -> tuple[dict[str, Any], dict[str, float]]:
    disk_paths = disk_paths or default_disk_paths()

    cpu_percent = psutil.cpu_percent(interval=0.35)
    memory = psutil.virtual_memory()
    swap = psutil.swap_memory()
    disk, disks = disk_snapshot(disk_paths)
    net = psutil.net_io_counters()
    current_net = {"at": time.time(), "bytes_sent": float(net.bytes_sent), "bytes_recv": float(net.bytes_recv)}

    upload_bps = 0.0
    download_bps = 0.0
    if previous_net:
        elapsed = max(0.001, current_net["at"] - previous_net.get("at", current_net["at"]))
        upload_bps = max(0.0, (current_net["bytes_sent"] - previous_net.get("bytes_sent", current_net["bytes_sent"])) / elapsed)
        download_bps = max(0.0, (current_net["bytes_recv"] - previous_net.get("bytes_recv", current_net["bytes_recv"])) / elapsed)

    info = system_info()

    metrics = {
        "collected_at": iso_now(),
        "cpu_percent": float(cpu_percent),
        "cpu_count": psutil.cpu_count(logical=True) or 0,
        "memory_total": int(memory.total),
        "memory_used": int(memory.used),
        "memory_percent": float(memory.percent),
        "swap_total": int(swap.total),
        "swap_used": int(swap.used),
        "swap_percent": float(swap.percent),
        "disk_total": int(disk["total"]),
        "disk_used": int(disk["used"]),
        "disk_percent": float(disk["percent"]),
        "disk_path": str(disk["path"]),
        "disks": disks,
        "net_upload_bps": float(upload_bps),
        "net_download_bps": float(download_bps),
        "net_bytes_sent": int(net.bytes_sent),
        "net_bytes_recv": int(net.bytes_recv),
        "uptime_seconds": None,
        "load_1": None,
        "load_5": None,
        "load_15": None,
        "process_count": None,
        "os_name": info["os_name"],
        "kernel_version": info["kernel_version"],
        "architecture": info["architecture"],
        "hostname": info["hostname"],
        "services": [],
    }
    return metrics, current_net
