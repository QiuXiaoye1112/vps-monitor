from fastapi.testclient import TestClient

import server


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
