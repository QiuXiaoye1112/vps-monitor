from __future__ import annotations

import ipaddress
from typing import Any

from fastapi import Depends, FastAPI, Header, HTTPException, Request, status
from fastapi.responses import HTMLResponse
from pydantic import BaseModel, Field

import storage
from monitor_common import iso_now
from settings import SERVER_TOKEN


app = FastAPI(title="VPS Monitor API", version="1.0.0")


DASHBOARD_HTML = """<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>VPS Monitor</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0b1020;
      --panel: #111827;
      --panel-2: #162033;
      --text: #e5e7eb;
      --muted: #94a3b8;
      --border: #253246;
      --online: #22c55e;
      --offline: #ef4444;
      --accent: #38bdf8;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      background: var(--bg);
      color: var(--text);
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    main {
      width: min(1120px, calc(100% - 28px));
      margin: 0 auto;
      padding: 28px 0 40px;
    }
    header {
      display: flex;
      align-items: flex-end;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 22px;
    }
    h1 {
      margin: 0;
      font-size: 28px;
      line-height: 1.1;
      font-weight: 760;
      letter-spacing: 0;
    }
    .sub {
      margin-top: 8px;
      color: var(--muted);
      font-size: 14px;
    }
    .summary {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-end;
      color: var(--muted);
      font-size: 13px;
    }
    .badge {
      border: 1px solid var(--border);
      border-radius: 8px;
      background: rgba(17, 24, 39, 0.72);
      padding: 7px 10px;
      color: var(--muted);
      font: inherit;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
      gap: 12px;
    }
    .card {
      border: 1px solid var(--border);
      border-radius: 8px;
      background: var(--panel);
      padding: 14px;
      min-width: 0;
    }
    .card-head {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      gap: 10px;
      margin-bottom: 12px;
    }
    .name {
      font-size: 18px;
      font-weight: 730;
      overflow-wrap: anywhere;
    }
    .node-id {
      color: var(--muted);
      font-size: 12px;
      margin-top: 3px;
      overflow-wrap: anywhere;
    }
    .status {
      flex: 0 0 auto;
      border-radius: 999px;
      padding: 4px 9px;
      font-size: 12px;
      font-weight: 730;
      text-transform: lowercase;
      border: 1px solid;
    }
    .status.online {
      color: #bbf7d0;
      background: rgba(34, 197, 94, 0.14);
      border-color: rgba(34, 197, 94, 0.42);
    }
    .status.offline {
      color: #fecaca;
      background: rgba(239, 68, 68, 0.14);
      border-color: rgba(239, 68, 68, 0.42);
    }
    .metrics {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 8px;
    }
    .metric {
      border-radius: 8px;
      background: var(--panel-2);
      padding: 9px 10px;
      min-width: 0;
    }
    .label {
      color: var(--muted);
      font-size: 12px;
      margin-bottom: 5px;
    }
    .value {
      font-size: 17px;
      font-weight: 700;
      overflow-wrap: anywhere;
    }
    .last-seen {
      margin-top: 11px;
      color: var(--muted);
      font-size: 12px;
      overflow-wrap: anywhere;
    }
    .empty, .error {
      border: 1px solid var(--border);
      border-radius: 8px;
      background: var(--panel);
      padding: 18px;
      color: var(--muted);
    }
    .error { color: #fecaca; border-color: rgba(239, 68, 68, 0.45); }
    @media (max-width: 640px) {
      main { width: min(100% - 18px, 1120px); padding-top: 12px; padding-bottom: 22px; }
      header { display: block; }
      h1, .sub { display: none; }
      .summary {
        justify-content: flex-start;
        gap: 7px;
        margin-top: 0;
        margin-bottom: 12px;
        font-size: 12px;
      }
      .badge { padding: 5px 8px; border-radius: 7px; }
      .grid { grid-template-columns: 1fr; gap: 10px; }
      .card { padding: 12px; }
      .card-head { align-items: center; margin-bottom: 10px; }
      .name { font-size: 19px; line-height: 1.15; }
      .node-id { display: none; }
      .status { padding: 3px 8px; font-size: 12px; }
      .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 7px; }
      .metric { padding: 8px; border-radius: 7px; }
      .label { font-size: 12px; margin-bottom: 3px; }
      .value { font-size: 18px; line-height: 1.18; }
      .last-seen { margin-top: 9px; font-size: 12px; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <div>
        <h1>VPS Monitor</h1>
        <div class="sub">每 1 秒自动刷新</div>
      </div>
      <div class="summary">
        <div class="badge" id="online-count">online 0</div>
        <div class="badge" id="offline-count">offline 0</div>
      </div>
    </header>
    <section id="content" class="grid"></section>
  </main>
  <script>
    const content = document.querySelector("#content");
    const onlineCount = document.querySelector("#online-count");
    const offlineCount = document.querySelector("#offline-count");

    function fmtPercent(value) {
      return value === null || value === undefined ? "-" : `${Number(value).toFixed(1)}%`;
    }

    function fmtBytes(value) {
      if (value === null || value === undefined || Number.isNaN(Number(value))) return "-";
      let size = Number(value);
      const units = ["B", "KB", "MB", "GB", "TB", "PB"];
      let unit = 0;
      while (Math.abs(size) >= 1024 && unit < units.length - 1) {
        size /= 1024;
        unit += 1;
      }
      return `${unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`;
    }

    function fmtSpeed(value) {
      return `${fmtBytes(value)}/s`;
    }

    function fmtTime(value) {
      if (!value) return "-";
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return value;
      return date.toLocaleString();
    }

    function fmtAge(value, serverNow) {
      if (!value) return "-";
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return "-";
      const now = serverNow ? new Date(serverNow) : new Date();
      const seconds = Math.max(0, Math.round((now.getTime() - date.getTime()) / 1000));
      return `${seconds}s ago`;
    }

    function fmtDuration(seconds) {
      if (seconds === null || seconds === undefined || Number.isNaN(Number(seconds))) return "-";
      const total = Math.max(0, Math.floor(Number(seconds)));
      const days = Math.floor(total / 86400);
      const hours = Math.floor((total % 86400) / 3600);
      const minutes = Math.floor((total % 3600) / 60);
      if (days > 0) return `${days}d ${hours}h`;
      if (hours > 0) return `${hours}h ${minutes}m`;
      return `${minutes}m`;
    }

    function metricBlock(label, value) {
      const wrap = document.createElement("div");
      wrap.className = "metric";
      const labelEl = document.createElement("div");
      labelEl.className = "label";
      labelEl.textContent = label;
      const valueEl = document.createElement("div");
      valueEl.className = "value";
      valueEl.textContent = value;
      wrap.append(labelEl, valueEl);
      return wrap;
    }

    function render(nodes, serverNow) {
      content.innerHTML = "";
      if (!nodes.length) {
        content.className = "";
        const empty = document.createElement("div");
        empty.className = "empty";
        empty.textContent = "还没有节点上报。启动 agent.py 后这里会显示 VPS 状态。";
        content.append(empty);
      } else {
        content.className = "grid";
      }

      let online = 0;
      let offline = 0;
      for (const node of nodes) {
        if (node.status === "online") online += 1;
        else offline += 1;

        const metric = node.latest_metric || {};
        const card = document.createElement("article");
        card.className = "card";

        const head = document.createElement("div");
        head.className = "card-head";
        const title = document.createElement("div");
        const name = document.createElement("div");
        name.className = "name";
        name.textContent = node.name || node.id;
        const nodeId = document.createElement("div");
        nodeId.className = "node-id";
        nodeId.textContent = node.id;
        title.append(name, nodeId);
        const status = document.createElement("div");
        status.className = `status ${node.status === "online" ? "online" : "offline"}`;
        status.textContent = node.status === "online" ? "online" : "offline";
        head.append(title, status);

        const metrics = document.createElement("div");
        metrics.className = "metrics";
        metrics.append(
          metricBlock("CPU", fmtPercent(metric.cpu_percent)),
          metricBlock("内存", fmtPercent(metric.memory_percent)),
          metricBlock("磁盘", fmtPercent(metric.disk_percent)),
          metricBlock("运行时间", fmtDuration(metric.uptime_seconds)),
          metricBlock("上传", fmtSpeed(metric.net_upload_bps)),
          metricBlock("下载", fmtSpeed(metric.net_download_bps))
        );

        const lastSeen = document.createElement("div");
        lastSeen.className = "last-seen";
        lastSeen.textContent = `最后上报：${fmtAge(node.last_seen_at, serverNow)} · ${fmtTime(node.last_seen_at)}`;
        card.append(head, metrics, lastSeen);
        content.append(card);
      }

      onlineCount.textContent = `online ${online}`;
      offlineCount.textContent = `offline ${offline}`;
    }

    async function refresh() {
      try {
        const res = await fetch("/api/nodes", { cache: "no-store" });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        render(data.nodes || [], data.server_now);
      } catch (err) {
        content.className = "";
        content.innerHTML = "";
        const error = document.createElement("div");
        error.className = "error";
        error.textContent = `无法加载节点数据：${err.message}`;
        content.append(error);
      }
    }

    refresh();
    setInterval(refresh, 500);
  </script>
</body>
</html>
"""


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


def request_client_ip(request: Request) -> str:
    candidates: list[str] = []
    forwarded = request.headers.get("x-forwarded-for", "")
    if forwarded:
        candidates.append(forwarded.split(",", 1)[0].strip())
    real_ip = request.headers.get("x-real-ip", "").strip()
    if real_ip:
        candidates.append(real_ip)
    if request.client:
        candidates.append(request.client.host)
    for candidate in candidates:
        try:
            return str(ipaddress.ip_address(candidate))
        except ValueError:
            continue
    return ""


@app.on_event("startup")
def on_startup() -> None:
    storage.init_db()


@app.get("/api/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/", response_class=HTMLResponse)
def dashboard() -> HTMLResponse:
    return HTMLResponse(DASHBOARD_HTML)


@app.post("/api/nodes/register", dependencies=[Depends(require_token)])
def register_node(payload: NodePayload, request: Request) -> dict[str, Any]:
    data = payload.model_dump()
    observed_ip = request_client_ip(request)
    if observed_ip:
        data["ip"] = observed_ip
    return {"node": storage.create_or_update_node(data)}


@app.put("/api/nodes/{node_id}", dependencies=[Depends(require_token)])
def update_node(node_id: str, payload: NodePayload) -> dict[str, Any]:
    data = payload.model_dump()
    data["node_id"] = node_id
    return {"node": storage.create_or_update_node(data)}


@app.get("/api/nodes")
def get_nodes() -> dict[str, Any]:
    return {"server_now": iso_now(), "nodes": storage.list_nodes()}


@app.get("/api/nodes/{node_id}")
def get_node(node_id: str) -> dict[str, Any]:
    node = storage.get_node(node_id)
    if node is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="node not found")
    return {"node": node}


@app.delete("/api/nodes/{node_id}", dependencies=[Depends(require_token)])
def delete_node(node_id: str) -> dict[str, Any]:
    if not storage.delete_node(node_id):
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="node not found")
    return {"status": "deleted", "node_id": node_id}


@app.post("/api/metrics", dependencies=[Depends(require_token)])
def report_metrics(payload: MetricPayload) -> dict[str, Any]:
    return {"metric": storage.insert_metric(payload.model_dump())}


@app.get("/api/nodes/{node_id}/metrics")
def get_node_metrics(node_id: str, window: str = "5m") -> dict[str, Any]:
    if storage.get_node(node_id) is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="node not found")
    return {"metrics": storage.history_for_node(node_id, window)}


if __name__ == "__main__":
    import uvicorn

    uvicorn.run("server:app", host="0.0.0.0", port=8000, reload=False)
