import sqlite3
from datetime import datetime, timedelta, timezone

import storage


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
