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


DASHBOARD_HTML = """<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover">
<meta name="color-scheme" content="dark">
<title>VPS Monitor</title>
<style>
  :root{
    --bg:#0c0f14;
    --bg-elevated:#11151c;
    --card-bg:#141922;
    --border:#252b35;
    --border-soft:#1c222b;
    --text:#e6edf3;
    --text-dim:#8b949e;
    --text-faint:#5f6975;
    --accent:#5b9dff;   /* CPU / 磁盘 */
    --purple:#b18cff;   /* 内存 */
    --teal:#3ad1c6;     /* 流量 */
    --green:#3fb950;    /* 在线 */
    --red:#f85149;      /* 离线 / 危险 */
    --yellow:#d9a52b;   /* 警告 */
    --track:#1c2128;
    --radius:8px;
  }

  *{ box-sizing:border-box; margin:0; padding:0; }
  [hidden]{ display:none !important; }

  html,body{
    background:var(--bg);
    color:var(--text);
    min-height:100%;
  }

  body{
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",Helvetica,Arial,sans-serif;
    font-size:14px;
    line-height:1.45;
    -webkit-font-smoothing:antialiased;
  }

  /* 数值与百分比使用等宽字体，便于多卡片纵向扫读对齐，呼应"监控终端"的气质 */
  .mono{
    font-family:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,monospace;
    font-variant-numeric:tabular-nums;
  }

  a{ color:var(--accent); }

  /* ---------- 顶部栏 ---------- */
  .topbar{
    position:sticky;
    top:0;
    z-index:20;
    display:flex;
    flex-wrap:wrap;
    align-items:center;
    justify-content:space-between;
    gap:10px 16px;
    padding:16px 20px;
    background:var(--bg-elevated);
    border-bottom:1px solid var(--border);
  }
  .topbar-left{ display:flex; align-items:baseline; gap:14px; flex-wrap:wrap; }
  .topbar h1{
    font-size:1.15rem;
    font-weight:700;
    letter-spacing:.01em;
  }
  .summary{ display:flex; gap:8px; }
  .badge{
    display:inline-flex;
    align-items:center;
    gap:6px;
    font-size:.82rem;
    font-weight:600;
    padding:4px 11px;
    border-radius:20px;
    white-space:nowrap;
  }
  .badge .dot{ width:7px; height:7px; border-radius:50%; background:currentColor; flex-shrink:0; }
  .badge.online{ color:var(--green); background:rgba(63,185,80,.13); }
  .badge.offline{ color:var(--red); background:rgba(248,81,73,.13); }

  .topbar-right{ display:flex; flex-direction:column; align-items:flex-end; gap:4px; }
  .server-time{ font-size:.76rem; color:var(--text-dim); }
  .server-time .mono{ color:var(--text-dim); }

  /* ---------- 状态条 / 空状态 ---------- */
  .status-bar{
    margin:14px 20px 0;
    padding:9px 14px;
    background:rgba(248,81,73,.1);
    border:1px solid rgba(248,81,73,.3);
    color:var(--red);
    border-radius:8px;
    font-size:.84rem;
  }
  .empty-state{
    padding:80px 20px;
    text-align:center;
    color:var(--text-dim);
    font-size:.95rem;
  }

  /* ---------- 卡片网格 ---------- */
  main.grid{
    display:grid;
    grid-template-columns:repeat(auto-fit,minmax(300px,1fr));
    gap:16px;
    padding:20px;
  }

  .card{
    display:flex;
    flex-direction:column;
    gap:14px;
    background:var(--card-bg);
    border:1px solid var(--border);
    border-top:3px solid var(--green);
    border-radius:var(--radius);
    padding:16px 16px 18px;
    transition:border-color .3s ease;
  }
  .card.offline{ border-top-color:var(--red); }
  .card.offline .card-body{ opacity:.72; }

  .card-header{
    display:flex;
    align-items:flex-start;
    justify-content:space-between;
    gap:10px;
  }
  .node-name{
    font-size:1.02rem;
    font-weight:700;
    overflow-wrap:anywhere;
    word-break:break-word;
    min-width:0;
  }
  .status-badge{
    display:inline-flex;
    align-items:center;
    gap:6px;
    font-size:.74rem;
    font-weight:600;
    padding:3px 10px;
    border-radius:20px;
    white-space:nowrap;
    flex-shrink:0;
  }
  .status-badge .dot{ width:6px; height:6px; border-radius:50%; background:currentColor; }
  .status-badge.online{ color:var(--green); background:rgba(63,185,80,.13); }
  .status-badge.offline{ color:var(--red); background:rgba(248,81,73,.13); }

  .card-body{ display:flex; flex-direction:column; gap:14px; transition:opacity .3s ease; }

  /* ---------- 双列小指标（CPU / 运行时间，上行 / 下行） ---------- */
  .stat-row{
    display:grid;
    grid-template-columns:1fr 1fr;
    gap:12px 14px;
  }
  .stat-item{ display:flex; flex-direction:column; gap:6px; min-width:0; }
  .stat-label{
    font-size:.7rem;
    font-weight:600;
    color:var(--text-dim);
    text-transform:uppercase;
    letter-spacing:.06em;
    white-space:nowrap;
  }
  .stat-value{
    font-size:1.05rem;
    font-weight:700;
    word-break:break-word;
  }

  .mini-track{
    height:4px;
    border-radius:3px;
    background:var(--track);
    overflow:hidden;
  }
  .mini-fill{
    height:100%;
    border-radius:3px;
    background:var(--accent);
    transition:width .5s ease, background-color .3s ease;
  }

  /* ---------- 磁盘 / 内存 / 流量 大指标块 ---------- */
  .metric-block{ display:flex; flex-direction:column; gap:7px; }
  .metric-label{
    font-size:.76rem;
    font-weight:600;
    color:var(--text-dim);
    white-space:nowrap;
  }
  .metric-value{
    font-size:1.5rem;
    font-weight:700;
    line-height:1.25;
    word-break:break-word;
  }
  .metric-value .metric-limit{
    font-size:.82rem;
    font-weight:500;
    color:var(--text-faint);
    margin-left:5px;
  }
  .metric-detail{
    font-size:.84rem;
    color:var(--text-dim);
    line-height:1.5;
    white-space:normal;
    overflow-wrap:anywhere;
    word-break:break-word;
  }

  .track{
    height:8px;
    border-radius:4px;
    background:var(--track);
    overflow:hidden;
  }
  .fill{
    height:100%;
    border-radius:4px;
    transition:width .5s ease, background-color .3s ease;
  }
  .fill.disk{ background:var(--accent); }
  .fill.mem{ background:var(--purple); }
  .fill.traffic{ background:var(--teal); }
  .fill.level-warn{ background:var(--yellow) !important; }
  .fill.level-danger{ background:var(--red) !important; }
  .mini-fill.level-warn{ background:var(--yellow) !important; }
  .mini-fill.level-danger{ background:var(--red) !important; }

  .divider{ height:1px; background:var(--border-soft); border:none; }

  /* ---------- 响应式 ---------- */
  @media (max-width:480px){
    main.grid{ grid-template-columns:1fr; padding:14px; gap:14px; }
    .topbar{ padding:13px 14px; }
    .topbar-right{ align-items:flex-start; }
  }
  @media (max-width:360px){
    .stat-row{ grid-template-columns:1fr; }
  }
</style>
</head>
<body>

<header class="topbar">
  <div class="topbar-left">
    <h1>VPS Monitor</h1>
    <div class="summary">
      <span class="badge online"><span class="dot"></span>在线 <span id="onlineCount" class="mono">0</span></span>
      <span class="badge offline"><span class="dot"></span>离线 <span id="offlineCount" class="mono">0</span></span>
    </div>
  </div>
  <div class="topbar-right">
    <div class="server-time" id="serverTime">&nbsp;</div>
  </div>
</header>

<div class="status-bar" id="statusBar" hidden></div>
<div class="empty-state" id="loadingState">正在加载节点数据…</div>
<div class="empty-state" id="emptyState" hidden>暂无节点数据</div>

<main class="grid" id="nodesGrid"></main>

<noscript>
  <div class="empty-state">请启用 JavaScript 以查看实时监控数据。</div>
</noscript>

<script>
(function(){
  "use strict";

  /* =========================================================
   * 数据单位假设（如后端实际单位不同，请调整此处逻辑）：
   * - disk_used / disk_total / memory_used / memory_total      → 字节 (Byte)
   * - net_upload_bps / net_download_bps                        → 字节/秒 (Byte/s)
   * - net_tx_month / net_rx_month                               → 字节 (Byte)，本月累计上/下行
   * - traffic_offset_gb / traffic_limit_gb                     → GiB（1024^3 字节），与月流量字节数合并计算
   * ========================================================= */

  var GiB = Math.pow(1024, 3);

  /* ---------------- 格式化函数 ---------------- */

  function fmtPercent(value, decimals){
    decimals = (decimals === undefined) ? 1 : decimals;
    var n = Number(value);
    if (value === null || value === undefined || isNaN(n)) return "--";
    return n.toFixed(decimals) + "%";
  }

  function fmtBytes(bytes, decimals){
    decimals = (decimals === undefined) ? 1 : decimals;
    var n = Number(bytes);
    if (bytes === null || bytes === undefined || isNaN(n)) return "--";
    if (n === 0) return "0 B";
    var units = ["B","KB","MB","GB","TB","PB"];
    var negative = n < 0;
    var abs = Math.abs(n);
    var i = Math.floor(Math.log(abs) / Math.log(1024));
    i = Math.max(0, Math.min(i, units.length - 1));
    var val = abs / Math.pow(1024, i);
    var str = val.toFixed(decimals) + " " + units[i];
    return negative ? "-" + str : str;
  }

  function fmtSpeed(bytesPerSec, decimals){
    decimals = (decimals === undefined) ? 1 : decimals;
    var n = Number(bytesPerSec);
    if (bytesPerSec === null || bytesPerSec === undefined || isNaN(n)) return "--";
    return fmtBytes(n, decimals) + "/s";
  }

  function fmtDuration(seconds){
    var n = Number(seconds);
    if (seconds === null || seconds === undefined || isNaN(n) || n < 0) return "--";
    n = Math.floor(n);
    var day = 86400, hour = 3600, minute = 60;
    var d = Math.floor(n / day);
    var h = Math.floor((n % day) / hour);
    var m = Math.floor((n % hour) / minute);
    var s = n % minute;
    if (d > 0) return d + " 天 " + h + " 时";
    if (h > 0) return h + " 时 " + m + " 分";
    if (m > 0) return m + " 分 " + s + " 秒";
    return s + " 秒";
  }

  function fmtUsage(used, total, decimals){
    decimals = (decimals === undefined) ? 1 : decimals;
    return fmtBytes(used, decimals) + " / " + fmtBytes(total, decimals);
  }

  function fmtTrafficCycle(value){
    if (!value || value === "never") return "";
    var match = String(value).match(/^\d{4}-(\d{2})-(\d{2})T(\d{2}):(\d{2})/);
    if (!match) return value;
    return "每月 " + Number(match[2]) + " 日 " + match[3] + ":" + match[4];
  }

  function clampPercent(n){
    if (n === null || n === undefined || isNaN(n)) return 0;
    return Math.max(0, Math.min(100, n));
  }

  function levelClass(percent){
    var n = Number(percent);
    if (percent === null || percent === undefined || isNaN(n)) return "";
    if (n >= 90) return "level-danger";
    if (n >= 75) return "level-warn";
    return "";
  }

  /* ---------------- 卡片构建与更新 ---------------- */

  var cards = new Map(); // id -> refs

  function buildCard(){
    var el = document.createElement("div");
    el.className = "card";
    el.innerHTML =
      '<div class="card-header">' +
        '<div class="node-name"></div>' +
        '<div class="status-badge"><span class="dot"></span><span class="status-text"></span></div>' +
      '</div>' +
      '<div class="card-body">' +
        '<div class="stat-row">' +
          '<div class="stat-item">' +
            '<div class="stat-label">CPU</div>' +
            '<div class="stat-value mono cpu-value">--</div>' +
            '<div class="mini-track"><div class="mini-fill cpu-fill" style="width:0%"></div></div>' +
          '</div>' +
          '<div class="stat-item">' +
            '<div class="stat-label">运行时间</div>' +
            '<div class="stat-value mono uptime-value">--</div>' +
          '</div>' +
        '</div>' +

        '<hr class="divider">' +

        '<div class="metric-block">' +
          '<div class="metric-label">磁盘</div>' +
          '<div class="metric-value mono disk-value">--</div>' +
          '<div class="metric-detail mono disk-detail">--</div>' +
          '<div class="track"><div class="fill disk disk-fill" style="width:0%"></div></div>' +
        '</div>' +

        '<div class="metric-block">' +
          '<div class="metric-label">内存</div>' +
          '<div class="metric-value mono mem-value">--</div>' +
          '<div class="metric-detail mono mem-detail">--</div>' +
          '<div class="track"><div class="fill mem mem-fill" style="width:0%"></div></div>' +
        '</div>' +

        '<hr class="divider">' +

        '<div class="stat-row">' +
          '<div class="stat-item">' +
            '<div class="stat-label">↑ 上行</div>' +
            '<div class="stat-value mono up-value">--</div>' +
          '</div>' +
          '<div class="stat-item">' +
            '<div class="stat-label">↓ 下行</div>' +
            '<div class="stat-value mono down-value">--</div>' +
          '</div>' +
        '</div>' +

        '<div class="metric-block">' +
          '<div class="metric-label traffic-label">流量</div>' +
          '<div class="metric-value mono traffic-value">--</div>' +
          '<div class="metric-detail mono traffic-detail">--</div>' +
          '<div class="track traffic-track"><div class="fill traffic traffic-fill" style="width:0%"></div></div>' +
        '</div>' +
      '</div>';

    return {
      el: el,
      name: el.querySelector(".node-name"),
      statusBadge: el.querySelector(".status-badge"),
      statusText: el.querySelector(".status-text"),
      body: el.querySelector(".card-body"),

      cpuValue: el.querySelector(".cpu-value"),
      cpuFill: el.querySelector(".cpu-fill"),
      uptimeValue: el.querySelector(".uptime-value"),

      diskValue: el.querySelector(".disk-value"),
      diskDetail: el.querySelector(".disk-detail"),
      diskFill: el.querySelector(".disk-fill"),

      memValue: el.querySelector(".mem-value"),
      memDetail: el.querySelector(".mem-detail"),
      memFill: el.querySelector(".mem-fill"),

      upValue: el.querySelector(".up-value"),
      downValue: el.querySelector(".down-value"),

      trafficLabel: el.querySelector(".traffic-label"),
      trafficValue: el.querySelector(".traffic-value"),
      trafficDetail: el.querySelector(".traffic-detail"),
      trafficFill: el.querySelector(".traffic-fill"),
      trafficTrack: el.querySelector(".traffic-track")
    };
  }

  function setFill(fillEl, percent, baseClass, online){
    var pct = online ? clampPercent(percent) : 0;
    fillEl.style.width = pct + "%";
    var lvl = online ? levelClass(percent) : "";
    fillEl.className = ("fill " + baseClass + " " + lvl).trim();
  }

  function setMiniFill(fillEl, percent, online){
    var pct = online ? clampPercent(percent) : 0;
    fillEl.style.width = pct + "%";
    var lvl = online ? levelClass(percent) : "";
    fillEl.className = ("mini-fill cpu-fill " + lvl).trim();
  }

  function updateCard(refs, node){
    var online = node.status === "online";
    var hasMetric = !!node.latest_metric;

    refs.el.classList.toggle("offline", !online);
    refs.statusBadge.classList.toggle("online", online);
    refs.statusBadge.classList.toggle("offline", !online);
    refs.statusText.textContent = online ? "在线" : "离线";
    refs.name.textContent = node.name || node.id || "未命名节点";

    var m = node.latest_metric || {};

    // CPU
    refs.cpuValue.textContent = hasMetric ? fmtPercent(m.cpu_percent) : "--";
    setMiniFill(refs.cpuFill, m.cpu_percent, hasMetric);

    // 运行时间
    refs.uptimeValue.textContent = hasMetric ? fmtDuration(m.uptime_seconds) : "--";

    // 磁盘
    refs.diskValue.textContent = hasMetric ? fmtPercent(m.disk_percent) : "--";
    refs.diskDetail.textContent = hasMetric ? fmtUsage(m.disk_used, m.disk_total) : "--";
    setFill(refs.diskFill, m.disk_percent, "disk", hasMetric);

    // 内存
    refs.memValue.textContent = hasMetric ? fmtPercent(m.memory_percent) : "--";
    refs.memDetail.textContent = hasMetric ? fmtUsage(m.memory_used, m.memory_total) : "--";
    setFill(refs.memFill, m.memory_percent, "mem", hasMetric);

    // 上 / 下行速度
    refs.upValue.textContent = hasMetric ? fmtSpeed(m.net_upload_bps) : "--";
    refs.downValue.textContent = hasMetric ? fmtSpeed(m.net_download_bps) : "--";

    // 流量（月流量 或 累计流量）
    var offsetBytes = (Number(node.traffic_offset_gb) || 0) * GiB;
    var txBytes = Number(m.net_tx_month) || 0;
    var rxBytes = Number(m.net_rx_month) || 0;
    var usedBytes = txBytes + rxBytes + offsetBytes;
    var hasLimit = !!m.traffic_reset_enabled && Number(m.traffic_limit_gb) > 0;

    if (!hasMetric) {
      refs.trafficLabel.textContent = "流量";
      refs.trafficValue.textContent = "--";
      refs.trafficDetail.textContent = "--";
      refs.trafficTrack.style.display = "none";
    } else if (hasLimit) {
      var limitBytes = Number(m.traffic_limit_gb) * GiB;
      var pct = limitBytes > 0 ? (usedBytes / limitBytes) * 100 : 0;
      var cycleText = fmtTrafficCycle(m.traffic_cycle);
      refs.trafficLabel.textContent = "月流量" + (cycleText ? "（" + cycleText + "）" : "");
      refs.trafficValue.innerHTML = fmtBytes(usedBytes) +
        ' <span class="metric-limit">/ ' + fmtBytes(limitBytes) + '</span>';
      refs.trafficDetail.textContent = "↑ " + fmtBytes(txBytes) + "   ↓ " + fmtBytes(rxBytes);
      refs.trafficTrack.style.display = "";
      setFill(refs.trafficFill, pct, "traffic", true);
    } else {
      refs.trafficLabel.textContent = "累计流量";
      refs.trafficValue.textContent = fmtBytes(usedBytes);
      refs.trafficDetail.textContent = "↑ " + fmtBytes(txBytes) + "   ↓ " + fmtBytes(rxBytes);
      refs.trafficTrack.style.display = "none";
    }
  }

  /* ---------------- 渲染 ---------------- */

  function setCounts(nodes){
    var online = 0;
    for (var i = 0; i < nodes.length; i++) {
      if (nodes[i].status === "online") online++;
    }
    document.getElementById("onlineCount").textContent = online;
    document.getElementById("offlineCount").textContent = nodes.length - online;
  }

  function updateServerTime(iso){
    var el = document.getElementById("serverTime");
    if (!iso) { el.innerHTML = "&nbsp;"; return; }
    try {
      var d = new Date(iso);
      var text = d.toLocaleString("zh-CN", { hour12: false });
      el.innerHTML = '服务器时间：<span class="mono">' + text + '</span>';
    } catch (e) {
      el.innerHTML = "&nbsp;";
    }
  }

  function render(data){
    var nodes = (data && Array.isArray(data.nodes)) ? data.nodes : [];

    document.getElementById("loadingState").hidden = true;
    setCounts(nodes);
    updateServerTime(data && data.server_now);

    var grid = document.getElementById("nodesGrid");
    var seen = new Set();

    for (var i = 0; i < nodes.length; i++) {
      var node = nodes[i];
      seen.add(node.id);
      var refs = cards.get(node.id);
      if (!refs) {
        refs = buildCard();
        cards.set(node.id, refs);
        grid.appendChild(refs.el);
      }
      updateCard(refs, node);
    }

    // 移除已不存在的节点卡片
    cards.forEach(function(refs, id){
      if (!seen.has(id)) {
        refs.el.remove();
        cards.delete(id);
      }
    });

    document.getElementById("emptyState").hidden = nodes.length > 0;
  }

  function setConnError(message){
    var bar = document.getElementById("statusBar");
    if (!message) {
      bar.hidden = true;
      return;
    }
    bar.hidden = false;
    bar.textContent = message;
  }

  /* ---------------- 拉取数据 ---------------- */

  var inFlight = false;

  function refresh(){
    if (inFlight) return; // 避免请求堆积导致的并发问题
    inFlight = true;

    fetch("/api/nodes", { cache: "no-store" })
      .then(function(res){
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function(data){
        render(data);
        setConnError(null);
      })
      .catch(function(err){
        console.error("获取节点数据失败：", err);
        setConnError("数据获取失败，正在重试…");
      })
      .finally(function(){
        inFlight = false;
      });
  }

  refresh();
  setInterval(refresh, 1000);
})();
</script>
</body>
</html>"""


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
