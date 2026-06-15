import sqlite3
from datetime import datetime, timedelta, timezone

import storage


def traffic_metric(node_id: str, cycle: str, *, reset_enabled: bool = True) -> dict[str, object]:
    return {
        "node_id": node_id,
        "net_tx_month": 10 * 1073741824,
        "net_rx_month": 5 * 1073741824,
        "traffic_limit_gb": 500,
        "traffic_reset_enabled": reset_enabled,
        "traffic_cycle": cycle,
    }


def test_cleanup_removes_old_history_but_keeps_each_nodes_latest_metric(tmp_path, monkeypatch) -> None:
    database = tmp_path / "monitor.db"
    monkeypatch.setattr(storage, "DB_PATH", database)
    monkeypatch.setattr(storage, "METRIC_RETENTION_DAYS", 0)
    storage.init_db()

    now = datetime(2026, 6, 15, tzinfo=timezone.utc)
    old = (now - timedelta(days=10)).isoformat()
    with sqlite3.connect(database) as conn:
        conn.execute(
            "INSERT INTO nodes (id, name, created_at, updated_at, services_json) VALUES (?, ?, ?, ?, '[]')",
            ("node-1", "Node 1", old, old),
        )
        first = conn.execute(
            "INSERT INTO metrics (node_id, collected_at, received_at) VALUES (?, ?, ?)",
            ("node-1", old, old),
        ).lastrowid
        latest = conn.execute(
            "INSERT INTO metrics (node_id, collected_at, received_at) VALUES (?, ?, ?)",
            ("node-1", old, old),
        ).lastrowid
        conn.execute("UPDATE nodes SET last_metric_id = ? WHERE id = 'node-1'", (latest,))
        conn.commit()

    assert storage.cleanup_metrics(2, now=now) == 1
    with sqlite3.connect(database) as conn:
        remaining = [row[0] for row in conn.execute("SELECT id FROM metrics")]
    assert first not in remaining
    assert remaining == [latest]


def test_list_nodes_loads_latest_metrics_without_per_node_queries(tmp_path, monkeypatch) -> None:
    database = tmp_path / "monitor.db"
    monkeypatch.setattr(storage, "DB_PATH", database)
    monkeypatch.setattr(storage, "METRIC_RETENTION_DAYS", 0)
    storage.init_db()

    timestamp = datetime(2026, 6, 15, tzinfo=timezone.utc).isoformat()
    with sqlite3.connect(database) as conn:
        conn.execute(
            "INSERT INTO nodes (id, name, created_at, updated_at, services_json) VALUES (?, ?, ?, ?, '[]')",
            ("node-1", "Node 1", timestamp, timestamp),
        )
        conn.execute(
            "INSERT INTO nodes (id, name, created_at, updated_at, services_json) VALUES (?, ?, ?, ?, '[]')",
            ("node-2", "Node 2", timestamp, timestamp),
        )
        metric_id = conn.execute(
            "INSERT INTO metrics (node_id, collected_at, received_at, cpu_percent, traffic_reset_enabled) VALUES (?, ?, ?, ?, ?)",
            ("node-1", timestamp, timestamp, 42.5, 1),
        ).lastrowid
        conn.execute("UPDATE nodes SET last_metric_id = ? WHERE id = 'node-1'", (metric_id,))
        conn.commit()

    def fail_if_called(_: int):
        raise AssertionError("list_nodes must not query metrics once per node")

    monkeypatch.setattr(storage, "get_metric", fail_if_called)
    nodes = {node["id"]: node for node in storage.list_nodes()}
    assert nodes["node-1"]["latest_metric"]["cpu_percent"] == 42.5
    assert nodes["node-1"]["latest_metric"]["traffic_reset_enabled"] == 1
    assert nodes["node-2"]["latest_metric"] is None


def test_center_traffic_offset_expires_when_agent_cycle_changes(tmp_path, monkeypatch) -> None:
    database = tmp_path / "monitor.db"
    monkeypatch.setattr(storage, "DB_PATH", database)
    monkeypatch.setattr(storage, "METRIC_RETENTION_DAYS", 0)
    storage.init_db()
    storage.create_or_update_node({"node_id": "node-1", "name": "Node 1"})

    storage.insert_metric(traffic_metric("node-1", "2026-06-15T04:00:00+00:00"))
    node = storage.set_node_traffic_offset("node-1", 120)
    assert node is not None
    assert node["traffic_offset_gb"] == 120
    assert node["traffic_offset_cycle"] == "2026-06-15T04:00:00+00:00"

    storage.insert_metric(traffic_metric("node-1", "2026-06-15T04:00:00+00:00"))
    assert storage.get_node("node-1")["traffic_offset_gb"] == 120

    storage.insert_metric(traffic_metric("node-1", "2026-07-15T04:00:00+00:00"))
    reset_node = storage.get_node("node-1")
    assert reset_node["traffic_offset_gb"] == 0
    assert reset_node["traffic_offset_cycle"] == ""


def test_center_traffic_offset_requires_monthly_reset(tmp_path, monkeypatch) -> None:
    database = tmp_path / "monitor.db"
    monkeypatch.setattr(storage, "DB_PATH", database)
    monkeypatch.setattr(storage, "METRIC_RETENTION_DAYS", 0)
    storage.init_db()
    storage.create_or_update_node({"node_id": "node-1", "name": "Node 1"})
    storage.insert_metric(traffic_metric("node-1", "never", reset_enabled=False))

    try:
        storage.set_node_traffic_offset("node-1", 10)
    except ValueError as exc:
        assert "未配置每月流量重置时间" in str(exc)
    else:
        raise AssertionError("positive center traffic offset must require a reset time")

    node = storage.set_node_traffic_offset("node-1", 0)
    assert node is not None
    assert node["traffic_offset_gb"] == 0
