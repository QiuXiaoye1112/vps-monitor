from __future__ import annotations

from typing import Any

from fastapi import Depends, FastAPI, Header, HTTPException, Request, status
from pydantic import BaseModel, Field

import storage
from settings import SERVER_TOKEN


app = FastAPI(title="VPS Monitor API", version="1.0.0")


class NodePayload(BaseModel):
    node_id: str | None = None
    name: str
    ip: str = ""
    region: str = ""
    os_type: str = ""
    note: str = ""
    services: list[str] = Field(default_factory=list)


class MetricPayload(BaseModel):
    node_id: str
    collected_at: str | None = None
    cpu_percent: float
    cpu_count: int
    memory_total: int
    memory_used: int
    memory_percent: float
    swap_total: int
    swap_used: int
    swap_percent: float
    disk_total: int
    disk_used: int
    disk_percent: float
    disk_path: str = ""
    disks: list[dict[str, Any]] = Field(default_factory=list)
    net_upload_bps: float
    net_download_bps: float
    net_bytes_sent: int
    net_bytes_recv: int
    uptime_seconds: float | None = None
    load_1: float | None = None
    load_5: float | None = None
    load_15: float | None = None
    process_count: int | None = None
    os_name: str = ""
    kernel_version: str = ""
    architecture: str = ""
    hostname: str = ""
    services: list[dict[str, Any]] = Field(default_factory=list)


def token_from_request(
    request: Request,
    authorization: str | None = Header(default=None),
    x_monitor_token: str | None = Header(default=None),
) -> str | None:
    if x_monitor_token:
        return x_monitor_token
    if authorization and authorization.lower().startswith("bearer "):
        return authorization.split(" ", 1)[1].strip()
    return request.query_params.get("token")


def require_token(token: str | None = Depends(token_from_request)) -> None:
    if SERVER_TOKEN and token != SERVER_TOKEN:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid monitor token")


@app.on_event("startup")
def on_startup() -> None:
    storage.init_db()


@app.get("/api/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/api/nodes/register", dependencies=[Depends(require_token)])
def register_node(payload: NodePayload) -> dict[str, Any]:
    return {"node": storage.create_or_update_node(payload.model_dump())}


@app.put("/api/nodes/{node_id}", dependencies=[Depends(require_token)])
def update_node(node_id: str, payload: NodePayload) -> dict[str, Any]:
    data = payload.model_dump()
    data["node_id"] = node_id
    return {"node": storage.create_or_update_node(data)}


@app.get("/api/nodes", dependencies=[Depends(require_token)])
def get_nodes() -> dict[str, Any]:
    return {"nodes": storage.list_nodes()}


@app.get("/api/nodes/{node_id}", dependencies=[Depends(require_token)])
def get_node(node_id: str) -> dict[str, Any]:
    node = storage.get_node(node_id)
    if node is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="node not found")
    return {"node": node}


@app.post("/api/metrics", dependencies=[Depends(require_token)])
def report_metrics(payload: MetricPayload) -> dict[str, Any]:
    return {"metric": storage.insert_metric(payload.model_dump())}


@app.get("/api/nodes/{node_id}/metrics", dependencies=[Depends(require_token)])
def get_node_metrics(node_id: str, window: str = "5m") -> dict[str, Any]:
    if storage.get_node(node_id) is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="node not found")
    return {"metrics": storage.history_for_node(node_id, window)}


if __name__ == "__main__":
    import uvicorn

    uvicorn.run("server:app", host="0.0.0.0", port=8000, reload=False)
