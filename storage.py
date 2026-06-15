from __future__ import annotations

import json
import sqlite3
import threading
import time
import uuid
from contextlib import closing
from datetime import timedelta
from typing import Any

from monitor_common import iso_now, parse_datetime, utc_now
from settings import (
    DB_PATH,
    METRIC_CLEANUP_INTERVAL_SECONDS,
    METRIC_RETENTION_DAYS,
    OFFLINE_AFTER_SECONDS,
)


_cleanup_lock = threading.Lock()
_next_cleanup_at = 0.0


METRIC_FIELDS = [
    "cpu_percent",
    "cpu_count",
    "memory_total",
    "memory_used",
    "memory_percent",
    "swap_total",
    "swap_used",
    "swap_percent",
    "disk_total",
    "disk_used",
    "disk_percent",
    "disk_path",
    "net_upload_bps",
    "net_download_bps",
    "net_bytes_sent",
    "net_bytes_recv",
    "net_tx_month",
    "net_rx_month",
    "traffic_limit_gb",
    "traffic_reset_enabled",
    "uptime_seconds",
    "load_1",
    "load_5",
    "load_15",
    "process_count",
    "os_name",
    "kernel_version",
    "architecture",
    "hostname",
]


def connect() -> sqlite3.Connection:
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys = ON")
    return conn


def init_db() -> None:
    with closing(connect()) as conn:
        conn.executescript(
            """
            CREATE TABLE IF NOT EXISTS nodes (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                ip TEXT DEFAULT '',
                region TEXT DEFAULT '',
                os_type TEXT DEFAULT '',
                note TEXT DEFAULT '',
                services_json TEXT NOT NULL DEFAULT '[]',
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                last_seen_at TEXT,
                last_metric_id INTEGER,
                system_os TEXT DEFAULT '',
                kernel_version TEXT DEFAULT '',
                architecture TEXT DEFAULT '',
                hostname TEXT DEFAULT ''
            );

            CREATE TABLE IF NOT EXISTS metrics (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                node_id TEXT NOT NULL,
                collected_at TEXT NOT NULL,
                received_at TEXT NOT NULL,
                cpu_percent REAL,
                cpu_count INTEGER,
                memory_total INTEGER,
                memory_used INTEGER,
                memory_percent REAL,
                swap_total INTEGER,
                swap_used INTEGER,
                swap_percent REAL,
                disk_total INTEGER,
                disk_used INTEGER,
                disk_percent REAL,
                disk_path TEXT DEFAULT '',
                net_upload_bps REAL,
                net_download_bps REAL,
                net_bytes_sent INTEGER,
                net_bytes_recv INTEGER,
                uptime_seconds REAL,
                load_1 REAL,
                load_5 REAL,
                load_15 REAL,
                process_count INTEGER,
                os_name TEXT DEFAULT '',
                kernel_version TEXT DEFAULT '',
                architecture TEXT DEFAULT '',
                hostname TEXT DEFAULT '',
                services_json TEXT NOT NULL DEFAULT '[]',
                disks_json TEXT NOT NULL DEFAULT '[]',
                FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
            );

            CREATE INDEX IF NOT EXISTS idx_metrics_node_collected ON metrics(node_id, collected_at DESC);
            CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes(last_seen_at);

            """
        )
        conn.commit()
        # 兼容旧数据库（逐条执行，失败不中断）
        for col in ("net_tx_month", "net_rx_month", "traffic_limit_gb", "traffic_reset_enabled"):
            try:
                conn.execute(f"ALTER TABLE metrics ADD COLUMN {col} INTEGER DEFAULT 0")
            except sqlite3.OperationalError:
                pass
        conn.commit()
    cleanup_metrics()


def cleanup_metrics(
    retention_days: float | None = None,
    *,
    now: Any = None,
) -> int:
    days = METRIC_RETENTION_DAYS if retention_days is None else max(0.0, retention_days)
    if days <= 0:
        return 0
    current = now or utc_now()
    cutoff = (current - timedelta(days=days)).isoformat()
    with closing(connect()) as conn:
        cursor = conn.execute(
            """
            DELETE FROM metrics
            WHERE received_at < ?
              AND id NOT IN (
                  SELECT last_metric_id
                  FROM nodes
                  WHERE last_metric_id IS NOT NULL
              )
            """,
            (cutoff,),
        )
        conn.commit()
        return cursor.rowcount


def maybe_cleanup_metrics() -> None:
    global _next_cleanup_at
    if METRIC_RETENTION_DAYS <= 0 or time.monotonic() < _next_cleanup_at:
        return
    with _cleanup_lock:
        if time.monotonic() < _next_cleanup_at:
            return
        cleanup_metrics()
        _next_cleanup_at = time.monotonic() + METRIC_CLEANUP_INTERVAL_SECONDS


def normalize_services(services: Any) -> list[str]:
    if isinstance(services, str):
        try:
            loaded = json.loads(services)
            if isinstance(loaded, list):
                return [str(item) for item in loaded if str(item).strip()]
        except json.JSONDecodeError:
            return [item.strip() for item in services.split(",") if item.strip()]
    if isinstance(services, list):
        return [str(item).strip() for item in services if str(item).strip()]
    return []


def service_json(services: Any) -> str:
    return json.dumps(normalize_services(services), ensure_ascii=False)


def row_to_dict(row: sqlite3.Row | None) -> dict[str, Any] | None:
    return dict(row) if row is not None else None


def status_from_last_seen(last_seen_at: str | None) -> str:
    seen = parse_datetime(last_seen_at)
    if seen is None:
        return "offline"
    return "online" if utc_now() - seen <= timedelta(seconds=OFFLINE_AFTER_SECONDS) else "offline"


def decode_json(value: str | None, default: Any) -> Any:
    if not value:
        return default
    try:
        return json.loads(value)
    except json.JSONDecodeError:
        return default


def create_or_update_node(payload: dict[str, Any]) -> dict[str, Any]:
    node_id = str(payload.get("node_id") or payload.get("id") or uuid.uuid4().hex[:12])
    timestamp = iso_now()
    values = {
        "id": node_id,
        "name": payload.get("name") or node_id,
        "ip": payload.get("ip") or "",
        "region": payload.get("region") or "",
        "os_type": payload.get("os_type") or "",
        "note": payload.get("note") or "",
        "services_json": service_json(payload.get("services")),
        "updated_at": timestamp,
    }

    with closing(connect()) as conn:
        exists = conn.execute("SELECT id FROM nodes WHERE id = ?", (node_id,)).fetchone()
        if exists:
            conn.execute(
                """
                UPDATE nodes
                SET name = ?, ip = ?, region = ?, os_type = ?, note = ?, services_json = ?, updated_at = ?
                WHERE id = ?
                """,
                (
                    values["name"],
                    values["ip"],
                    values["region"],
                    values["os_type"],
                    values["note"],
                    values["services_json"],
                    values["updated_at"],
                    node_id,
                ),
            )
        else:
            conn.execute(
                """
                INSERT INTO nodes (id, name, ip, region, os_type, note, services_json, created_at, updated_at)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    values["id"],
                    values["name"],
                    values["ip"],
                    values["region"],
                    values["os_type"],
                    values["note"],
                    values["services_json"],
                    timestamp,
                    timestamp,
                ),
            )
        conn.commit()

    node = get_node(node_id)
    if node is None:
        raise RuntimeError("node was not persisted")
    return node


def insert_metric(payload: dict[str, Any]) -> dict[str, Any]:
    node_id = str(payload["node_id"])
    collected_at = payload.get("collected_at") or iso_now()
    received_at = iso_now()
    services_json = json.dumps(payload.get("services") or [], ensure_ascii=False)
    disks_json = json.dumps(payload.get("disks") or [], ensure_ascii=False)

    values = [payload.get(field) for field in METRIC_FIELDS]
    placeholders = ", ".join("?" for _ in METRIC_FIELDS)

    with closing(connect()) as conn:
        if not conn.execute("SELECT id FROM nodes WHERE id = ?", (node_id,)).fetchone():
            conn.execute(
                """
                INSERT INTO nodes (id, name, created_at, updated_at, services_json)
                VALUES (?, ?, ?, ?, ?)
                """,
                (node_id, node_id, received_at, received_at, service_json([])),
            )

        cursor = conn.execute(
            f"""
            INSERT INTO metrics (
                node_id,
                collected_at,
                received_at,
                {", ".join(METRIC_FIELDS)},
                services_json,
                disks_json
            )
            VALUES (?, ?, ?, {placeholders}, ?, ?)
            """,
            (node_id, collected_at, received_at, *values, services_json, disks_json),
        )
        metric_id = int(cursor.lastrowid)
        conn.execute(
            """
            UPDATE nodes
            SET last_seen_at = ?,
                last_metric_id = ?,
                system_os = COALESCE(?, system_os),
                kernel_version = COALESCE(?, kernel_version),
                architecture = COALESCE(?, architecture),
                hostname = COALESCE(?, hostname),
                updated_at = ?
            WHERE id = ?
            """,
            (
                received_at,
                metric_id,
                payload.get("os_name"),
                payload.get("kernel_version"),
                payload.get("architecture"),
                payload.get("hostname"),
                received_at,
                node_id,
            ),
        )
        conn.commit()

    maybe_cleanup_metrics()
    metric = get_metric(metric_id)
    if metric is None:
        raise RuntimeError("metric was not persisted")
    return metric


def metric_from_row(
    row: sqlite3.Row | dict[str, Any] | None,
    prefix: str = "",
) -> dict[str, Any] | None:
    if row is None:
        return None
    data = dict(row)
    if prefix and data.get(f"{prefix}id") is None:
        return None
    metric = {field: data.get(f"{prefix}{field}") for field in METRIC_FIELDS}
    metric.update(
        {
            "id": data.get(f"{prefix}id"),
            "node_id": data.get(f"{prefix}node_id"),
            "collected_at": data.get(f"{prefix}collected_at"),
            "received_at": data.get(f"{prefix}received_at"),
            "services": decode_json(data.get(f"{prefix}services_json"), []),
            "disks": decode_json(data.get(f"{prefix}disks_json"), []),
        }
    )
    return metric


def node_from_row(row: sqlite3.Row, latest_metric: dict[str, Any] | None = None) -> dict[str, Any]:
    data = dict(row)
    return {
        "id": data["id"],
        "name": data["name"],
        "ip": data["ip"],
        "region": data["region"],
        "os_type": data["os_type"],
        "note": data["note"],
        "services": decode_json(data["services_json"], []),
        "created_at": data["created_at"],
        "updated_at": data["updated_at"],
        "last_seen_at": data["last_seen_at"],
        "status": status_from_last_seen(data["last_seen_at"]),
        "system_os": data["system_os"],
        "kernel_version": data["kernel_version"],
        "architecture": data["architecture"],
        "hostname": data["hostname"],
        "latest_metric": latest_metric,
    }


def list_nodes() -> list[dict[str, Any]]:
    metric_columns = [
        "id",
        "node_id",
        "collected_at",
        "received_at",
        *METRIC_FIELDS,
        "services_json",
        "disks_json",
    ]
    latest_metric_select = ", ".join(
        f"m.{column} AS metric_{column}" for column in metric_columns
    )
    with closing(connect()) as conn:
        rows = conn.execute(
            f"""
            SELECT n.*, {latest_metric_select}
            FROM nodes AS n
            LEFT JOIN metrics AS m ON m.id = n.last_metric_id
            ORDER BY n.created_at DESC
            """
        ).fetchall()

    return [node_from_row(row, metric_from_row(row, "metric_")) for row in rows]


def get_node(node_id: str) -> dict[str, Any] | None:
    with closing(connect()) as conn:
        row = conn.execute("SELECT * FROM nodes WHERE id = ?", (node_id,)).fetchone()
    if row is None:
        return None
    return node_from_row(row, get_metric(row["last_metric_id"]) if row["last_metric_id"] else None)


def get_metric(metric_id: int) -> dict[str, Any] | None:
    with closing(connect()) as conn:
        row = conn.execute("SELECT * FROM metrics WHERE id = ?", (metric_id,)).fetchone()
    return metric_from_row(row)


def delete_node(node_id: str) -> bool:
    with closing(connect()) as conn:
        cursor = conn.execute("DELETE FROM nodes WHERE id = ?", (node_id,))
        conn.commit()
        return cursor.rowcount > 0


def history_for_node(node_id: str, window: str) -> list[dict[str, Any]]:
    seconds_by_window = {"5m": 5 * 60, "1h": 60 * 60, "24h": 24 * 60 * 60}
    seconds = seconds_by_window.get(window, seconds_by_window["5m"])
    cutoff = (utc_now() - timedelta(seconds=seconds)).isoformat()

    with closing(connect()) as conn:
        rows = conn.execute(
            """
            SELECT *
            FROM metrics
            WHERE node_id = ? AND collected_at >= ?
            ORDER BY collected_at ASC
            """,
            (node_id, cutoff),
        ).fetchall()
    return [metric_from_row(row) for row in rows if row is not None]
