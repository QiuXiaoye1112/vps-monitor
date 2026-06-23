from __future__ import annotations

import ipaddress
import secrets
from contextlib import asynccontextmanager
from typing import Any

from fastapi import Depends, FastAPI, Header, HTTPException, Request, status
from fastapi.responses import HTMLResponse
from pydantic import BaseModel, Field

import storage
from monitor_common import iso_now
from settings import SERVER_TOKEN


@asynccontextmanager
async def lifespan(_: FastAPI):
    storage.init_db()
    yield


app = FastAPI(title="VPS Monitor API", version="1.0.0", lifespan=lifespan)


DASHBOARD_HTML = """<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>VPS Monitor</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f5f5f7;
      --surface: rgba(255, 255, 255, 0.82);
      --surface-solid: #ffffff;
      --surface-soft: rgba(255, 255, 255, 0.58);
      --text: #1d1d1f;
      --muted: #6e6e73;
      --subtle: #86868b;
      --line: rgba(0, 0, 0, 0.08);
      --shadow: 0 24px 70px rgba(0, 0, 0, 0.10);
      --shadow-soft: 0 12px 30px rgba(0, 0, 0, 0.07);
      --blue: #007aff;
      --green: #34c759;
      --red: #ff3b30;
      --orange: #ff9500;
      --purple: #af52de;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Segoe UI", sans-serif;
      background:
        radial-gradient(circle at top left, rgba(0, 122, 255, 0.16), transparent 34rem),
        radial-gradient(circle at top right, rgba(175, 82, 222, 0.13), transparent 30rem),
        linear-gradient(180deg, #fbfbfd 0%, var(--bg) 48%, #ededf1 100%);
    }
    body::before {
      content: "";
      position: fixed;
      inset: 0;
      pointer-events: none;
      background-image: linear-gradient(rgba(255,255,255,0.35) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.35) 1px, transparent 1px);
      background-size: 44px 44px;
      mask-image: linear-gradient(to bottom, rgba(0,0,0,0.42), transparent 62%);
    }
    main {
      position: relative;
      width: min(1180px, calc(100% - 40px));
      margin: 0 auto;
      padding: 36px 0 48px;
    }
    header {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 20px;
      align-items: end;
      margin-bottom: 22px;
    }
    .eyebrow {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      color: var(--muted);
      font-size: 13px;
      font-weight: 650;
      letter-spacing: 0.02em;
      padding: 7px 11px;
      border: 1px solid var(--line);
      border-radius: 999px;
      background: rgba(255, 255, 255, 0.58);
      backdrop-filter: blur(18px);
      -webkit-backdrop-filter: blur(18px);
    }
    .eyebrow::before {
      content: "";
      width: 7px;
      height: 7px;
      border-radius: 999px;
      background: var(--green);
      box-shadow: 0 0 0 4px rgba(52, 199, 89, 0.16);
    }
    h1 {
      margin: 14px 0 0;
      font-size: clamp(34px, 7vw, 68px);
      line-height: 0.96;
      letter-spacing: -0.06em;
      font-weight: 780;
    }
    .sub {
      max-width: 560px;
      margin-top: 14px;
      color: var(--muted);
      font-size: 16px;
      line-height: 1.5;
    }
    .summary {
      display: grid;
      grid-template-columns: repeat(2, minmax(108px, 1fr));
      gap: 10px;
    }
    .badge {
      min-width: 108px;
      border: 1px solid var(--line);
      border-radius: 22px;
      background: var(--surface);
      padding: 13px 15px;
      box-shadow: var(--shadow-soft);
      backdrop-filter: blur(22px);
      -webkit-backdrop-filter: blur(22px);
    }
    .badge strong {
      display: block;
      font-size: 25px;
      line-height: 1;
      letter-spacing: -0.04em;
    }
    .badge span {
      display: block;
      margin-top: 5px;
      color: var(--muted);
      font-size: 12px;
      font-weight: 650;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(292px, 1fr));
      gap: 16px;
    }
    .card {
      position: relative;
      overflow: hidden;
      min-width: 0;
      border: 1px solid var(--line);
      border-radius: 30px;
      background: var(--surface);
      box-shadow: var(--shadow-soft);
      backdrop-filter: blur(24px) saturate(1.18);
      -webkit-backdrop-filter: blur(24px) saturate(1.18);
      padding: 20px;
    }
    .card::after {
      content: "";
      position: absolute;
      inset: 0;
      pointer-events: none;
      background: linear-gradient(135deg, rgba(255,255,255,0.72), transparent 42%);
    }
    .card > * { position: relative; z-index: 1; }
    .card-head {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      gap: 14px;
      margin-bottom: 18px;
    }
    .name {
      font-size: 23px;
      line-height: 1.12;
      letter-spacing: -0.035em;
      font-weight: 760;
      overflow-wrap: anywhere;
    }
    .status {
      flex: 0 0 auto;
      display: inline-flex;
      align-items: center;
      gap: 7px;
      border-radius: 999px;
      padding: 7px 10px;
      font-size: 12px;
      font-weight: 720;
      border: 1px solid var(--line);
      background: rgba(255, 255, 255, 0.62);
      color: var(--muted);
    }
    .status::before {
      content: "";
      width: 8px;
      height: 8px;
      border-radius: 999px;
      background: currentColor;
    }
    .status.online { color: var(--green); }
    .status.offline { color: var(--red); }
    .metrics {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }
    .metric {
      min-width: 0;
      border: 1px solid var(--line);
      border-radius: 22px;
      background: rgba(255, 255, 255, 0.62);
      padding: 13px;
    }
    .metric.wide { grid-column: 1 / -1; }
    .label {
      display: flex;
      justify-content: space-between;
      gap: 10px;
      color: var(--muted);
      font-size: 12px;
      font-weight: 680;
      margin-bottom: 7px;
    }
    .value {
      font-size: 21px;
      line-height: 1.08;
      letter-spacing: -0.035em;
      font-weight: 760;
      overflow-wrap: anywhere;
    }
    .value.small { font-size: 15px; letter-spacing: -0.015em; }
    .detail {
      margin-top: 6px;
      color: var(--subtle);
      font-size: 12px;
      font-weight: 640;
      line-height: 1.25;
      overflow-wrap: anywhere;
    }
    .bar {
      height: 8px;
      overflow: hidden;
      border-radius: 999px;
      background: rgba(120, 120, 128, 0.16);
      margin-top: 10px;
    }
    .fill {
      height: 100%;
      width: 0%;
      border-radius: inherit;
      background: linear-gradient(90deg, var(--blue), #5ac8fa);
      transition: width .45s ease;
    }
    .fill.warn { background: linear-gradient(90deg, var(--orange), #ffcc00); }
    .fill.danger { background: linear-gradient(90deg, var(--red), #ff6b61); }
    .traffic-line {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 7px;
    }
    .traffic-title {
      color: var(--muted);
      font-size: 12px;
      font-weight: 700;
    }
    .traffic-value {
      font-size: 17px;
      font-weight: 760;
      letter-spacing: -0.025em;
      text-align: right;
    }
    .traffic-meta {
      margin: -1px 0 8px;
      color: var(--subtle);
      font-size: 12px;
      font-weight: 620;
    }
    .empty, .error {
      grid-column: 1 / -1;
      border: 1px solid var(--line);
      border-radius: 30px;
      background: var(--surface);
      box-shadow: var(--shadow-soft);
      padding: 24px;
      color: var(--muted);
      backdrop-filter: blur(22px);
      -webkit-backdrop-filter: blur(22px);
    }
    .error { color: var(--red); }
    @media (prefers-color-scheme: dark) {
      :root {
        color-scheme: dark;
        --bg: #050506;
        --surface: rgba(28, 28, 30, 0.74);
        --surface-solid: #1c1c1e;
        --surface-soft: rgba(44, 44, 46, 0.66);
        --text: #f5f5f7;
        --muted: #a1a1a6;
        --subtle: #8e8e93;
        --line: rgba(255, 255, 255, 0.11);
        --shadow: 0 24px 70px rgba(0, 0, 0, 0.34);
        --shadow-soft: 0 16px 36px rgba(0, 0, 0, 0.26);
      }
      body {
        background:
          radial-gradient(circle at top left, rgba(0, 122, 255, 0.22), transparent 32rem),
          radial-gradient(circle at top right, rgba(175, 82, 222, 0.18), transparent 30rem),
          linear-gradient(180deg, #08080a 0%, #101014 56%, #050506 100%);
      }
      .eyebrow, .badge, .metric, .status { background: rgba(44, 44, 46, 0.62); }
      .card::after { background: linear-gradient(135deg, rgba(255,255,255,0.10), transparent 44%); }
      body::before { opacity: 0.18; }
    }
    @media (max-width: 720px) {
      main { width: min(100% - 22px, 1180px); padding: 18px 0 30px; }
      header { display: block; margin-bottom: 14px; }
      h1 { font-size: 38px; }
      .sub { font-size: 14px; margin-top: 10px; }
      .summary { grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: 14px; }
      .badge { border-radius: 18px; padding: 11px 12px; }
      .badge strong { font-size: 22px; }
      .grid { grid-template-columns: 1fr; gap: 12px; }
      .card { border-radius: 24px; padding: 15px; }
      .card-head { margin-bottom: 13px; }
      .name { font-size: 21px; }
      .metrics { gap: 8px; }
      .metric { border-radius: 18px; padding: 11px; }
      .value { font-size: 19px; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <div>
        <div class="eyebrow">Live Monitor</div>
        <h1>VPS Monitor</h1>
        <div class="sub">轻量服务器状态面板，自动刷新 CPU、内存、磁盘、网络和流量状态。</div>
      </div>
      <div class="summary">
        <div class="badge"><strong id="online-count">0</strong><span>在线</span></div>
        <div class="badge"><strong id="offline-count">0</strong><span>离线</span></div>
      </div>
    </header>
    <section id="content" class="grid"></section>
  </main>
  <script>
    const content = document.querySelector("#content");
    const onlineCount = document.querySelector("#online-count");
    const offlineCount = document.querySelector("#offline-count");

    function fmtPercent(value) {
      return value === null || value === undefined || Number.isNaN(Number(value)) ? "-" : `${Number(value).toFixed(1)}%`;
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

    function fmtGB(value) {
      if (value === null || value === undefined || Number.isNaN(Number(value))) return "-";
      return `${(Number(value) / 1073741824).toFixed(1)} GB`;
    }

    function fmtUsage(used, total) {
      if (
        used === null || used === undefined || Number.isNaN(Number(used)) ||
        total === null || total === undefined || Number.isNaN(Number(total))
      ) {
        return "-";
      }
      return `已用 ${fmtBytes(used)} / ${fmtBytes(total)}`;
    }

    function fmtSpeed(value) {
      return `${fmtBytes(value)}/s`;
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

    function element(tag, className, text) {
      const node = document.createElement(tag);
      if (className) node.className = className;
      if (text !== undefined) node.textContent = text;
      return node;
    }

    function colorClass(percent) {
      if (percent >= 90) return "danger";
      if (percent >= 75) return "warn";
      return "";
    }

    function fmtTrafficCycle(value) {
      if (!value || value === "never") return "";
      const match = String(value).match(/^\d{4}-(\d{2})-(\d{2})T(\d{2}):(\d{2})/);
      if (!match) return `重置周期：${value}`;
      return `重置规则：每月 ${Number(match[2])} 日 ${match[3]}:${match[4]}`;
    }

    function metricBlock(label, value, percent, detail) {
      const wrap = element("div", "metric");
      const labelEl = element("div", "label", label);
      const valueEl = element("div", "value", value);
      wrap.append(labelEl, valueEl);
      if (detail !== undefined && detail !== null && detail !== "") {
        wrap.append(element("div", "detail", detail));
      }
      if (percent !== undefined && percent !== null && !Number.isNaN(Number(percent))) {
        const bar = element("div", "bar");
        const fill = element("div", `fill ${colorClass(Number(percent))}`.trim());
        fill.style.width = `${Math.max(0, Math.min(100, Number(percent)))}%`;
        bar.append(fill);
        wrap.append(bar);
      }
      return wrap;
    }

    function trafficBlock(metric, node) {
      const reportedTraffic = Number(metric.net_tx_month || 0) + Number(metric.net_rx_month || 0);
      const totalTraffic = reportedTraffic + (Number(node.traffic_offset_gb || 0) * 1073741824);
      const limitGB = Number(metric.traffic_limit_gb || 0);
      const label = metric.traffic_reset_enabled ? "月流量" : "累计流量";
      const wrap = element("div", "metric wide");
      const line = element("div", "traffic-line");
      const title = element("div", "traffic-title", label);
      const value = element("div", "traffic-value");
      const meta = element("div", "traffic-meta");
      const bar = element("div", "bar");
      const fill = element("div", "fill");
      const cycleText = metric.traffic_reset_enabled ? fmtTrafficCycle(metric.traffic_cycle) : "";

      if (limitGB > 0) {
        const actualGB = totalTraffic / 1073741824;
        const pctNumber = Math.min(100, Math.max(0, (actualGB / limitGB) * 100));
        value.textContent = `${actualGB.toFixed(1)} / ${limitGB.toFixed(1)} GB`;
        fill.style.width = `${pctNumber}%`;
        fill.className = `fill ${colorClass(pctNumber)}`.trim();
      } else {
        value.textContent = fmtGB(totalTraffic);
        fill.style.width = "100%";
      }

      line.append(title, value);
      bar.append(fill);
      wrap.append(line);
      if (cycleText) {
        meta.textContent = cycleText;
        wrap.append(meta);
      }
      wrap.append(bar);
      return wrap;
    }

    function render(nodes) {
      content.innerHTML = "";
      if (!nodes.length) {
        const empty = element("div", "empty", "还没有节点上报。启动 Agent 后这里会显示 VPS 状态。");
        content.append(empty);
        onlineCount.textContent = "0";
        offlineCount.textContent = "0";
        return;
      }

      let online = 0;
      let offline = 0;
      for (const node of nodes) {
        if (node.status === "online") online += 1;
        else offline += 1;

        const metric = node.latest_metric || {};
        const card = element("article", "card");
        const head = element("div", "card-head");
        const title = element("div");
        const name = element("div", "name", node.name || node.id);
        const status = element("div", `status ${node.status === "online" ? "online" : "offline"}`, node.status === "online" ? "在线" : "离线");

        title.append(name);
        head.append(title, status);

        const metrics = element("div", "metrics");
        metrics.append(
          metricBlock("CPU", fmtPercent(metric.cpu_percent)),
          metricBlock("运行时间", fmtDuration(metric.uptime_seconds)),
          metricBlock("磁盘", fmtPercent(metric.disk_percent), metric.disk_percent, fmtUsage(metric.disk_used, metric.disk_total)),
          metricBlock("内存", fmtPercent(metric.memory_percent), metric.memory_percent, fmtUsage(metric.memory_used, metric.memory_total)),
          metricBlock("上行", fmtSpeed(metric.net_upload_bps)),
          metricBlock("下行", fmtSpeed(metric.net_download_bps)),
          trafficBlock(metric, node)
        );

        card.append(head, metrics);
        content.append(card);
      }

      onlineCount.textContent = online;
      offlineCount.textContent = offline;
    }

    async function refresh() {
      try {
        const res = await fetch("/api/nodes", { cache: "no-store" });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        render(data.nodes || []);
      } catch (err) {
        content.innerHTML = "";
        content.append(element("div", "error", `无法加载节点数据：${err.message}`));
      }
    }

    refresh();
    setInterval(refresh, 1000);
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
    net_tx_month: int = 0
    net_rx_month: int = 0
    traffic_limit_gb: float = 0.0
    traffic_reset_enabled: bool = False
    traffic_cycle: str = ""
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


class TrafficOffsetPayload(BaseModel):
    used_gb: float = Field(default=0.0, ge=0)


def token_from_request(
    authorization: str | None = Header(default=None),
    x_monitor_token: str | None = Header(default=None),
) -> str | None:
    if x_monitor_token:
        return x_monitor_token
    if authorization and authorization.lower().startswith("bearer "):
        return authorization.split(" ", 1)[1].strip()
    return None


def require_token(token: str | None = Depends(token_from_request)) -> None:
    if SERVER_TOKEN and (token is None or not secrets.compare_digest(token, SERVER_TOKEN)):
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


@app.put("/api/nodes/{node_id}/traffic-offset", dependencies=[Depends(require_token)])
def update_traffic_offset(node_id: str, payload: TrafficOffsetPayload) -> dict[str, Any]:
    try:
        node = storage.set_node_traffic_offset(node_id, payload.used_gb)
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    if node is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="node not found")
    return {"node": node}


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
