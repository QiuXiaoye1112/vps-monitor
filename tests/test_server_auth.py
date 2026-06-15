from fastapi.testclient import TestClient

import server


def metric_payload(cycle: str) -> dict[str, object]:
    return {
        "node_id": "node-1",
        "cpu_percent": 10,
        "cpu_count": 2,
        "memory_total": 1,
        "memory_used": 1,
        "memory_percent": 10,
        "swap_total": 0,
        "swap_used": 0,
        "swap_percent": 0,
        "disk_total": 1,
        "disk_used": 1,
        "disk_percent": 10,
        "net_upload_bps": 0,
        "net_download_bps": 0,
        "net_bytes_sent": 0,
        "net_bytes_recv": 0,
        "net_tx_month": 10 * 1073741824,
        "net_rx_month": 5 * 1073741824,
        "traffic_limit_gb": 100,
        "traffic_reset_enabled": True,
        "traffic_cycle": cycle,
    }


def test_query_parameter_token_is_rejected(tmp_path, monkeypatch) -> None:
    monkeypatch.setattr(server, "SERVER_TOKEN", "secret")
    monkeypatch.setattr(server.storage, "DB_PATH", tmp_path / "monitor.db")
    with TestClient(server.app) as client:
        response = client.delete("/api/nodes/missing?token=secret")
    assert response.status_code == 401


def test_authorization_header_is_accepted(tmp_path, monkeypatch) -> None:
    monkeypatch.setattr(server, "SERVER_TOKEN", "secret")
    monkeypatch.setattr(server.storage, "DB_PATH", tmp_path / "monitor.db")
    with TestClient(server.app) as client:
        response = client.delete(
            "/api/nodes/missing",
            headers={"Authorization": "Bearer secret"},
        )
    assert response.status_code == 404


def test_center_can_set_used_traffic_for_current_agent_cycle(tmp_path, monkeypatch) -> None:
    monkeypatch.setattr(server, "SERVER_TOKEN", "secret")
    monkeypatch.setattr(server.storage, "DB_PATH", tmp_path / "monitor.db")
    monkeypatch.setattr(server.storage, "METRIC_RETENTION_DAYS", 0)
    headers = {"Authorization": "Bearer secret"}
    with TestClient(server.app) as client:
        client.post(
            "/api/nodes/register",
            json={"node_id": "node-1", "name": "Node 1"},
            headers=headers,
        )
        client.post(
            "/api/metrics",
            json=metric_payload("2026-06-15T04:00:00+00:00"),
            headers=headers,
        )
        response = client.put(
            "/api/nodes/node-1/traffic-offset",
            json={"used_gb": 20},
            headers=headers,
        )
        assert response.status_code == 200
        assert response.json()["node"]["traffic_offset_gb"] == 20

        client.post(
            "/api/metrics",
            json=metric_payload("2026-07-15T04:00:00+00:00"),
            headers=headers,
        )
        node = client.get("/api/nodes/node-1").json()["node"]
        assert node["traffic_offset_gb"] == 0
