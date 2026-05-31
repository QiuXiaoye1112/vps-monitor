from __future__ import annotations

import time
from typing import Any

import pandas as pd
import requests
import streamlit as st

from monitor_common import format_bytes
from settings import DASHBOARD_API_URL, SERVER_TOKEN
from ssh_deploy import DeploymentError, deploy_agent


APP_TITLE = "VPS Monitor"
WINDOW_OPTIONS = {"最近 5 分钟": "5m", "最近 1 小时": "1h", "最近 24 小时": "24h"}


def api_headers() -> dict[str, str]:
    return {"Authorization": f"Bearer {SERVER_TOKEN}", "Content-Type": "application/json"}


def api_request(method: str, path: str, **kwargs: Any) -> dict[str, Any]:
    url = f"{DASHBOARD_API_URL.rstrip('/')}{path}"
    response = requests.request(method, url, headers=api_headers(), timeout=8, **kwargs)
    response.raise_for_status()
    return response.json()


def safe_api_request(method: str, path: str, **kwargs: Any) -> tuple[dict[str, Any] | None, str | None]:
    try:
        return api_request(method, path, **kwargs), None
    except requests.HTTPError as exc:
        message = f"API 返回错误：{exc.response.status_code} {exc.response.text}"
    except requests.RequestException as exc:
        message = f"无法连接中心服务端：{exc}"
    return None, message


def inject_style() -> None:
    st.markdown(
        """
        <style>
        .stApp {
            background: #090d16;
            color: #e5e7eb;
        }
        .block-container {
            max-width: 1240px;
            padding-top: 1.6rem;
            padding-bottom: 2rem;
        }
        [data-testid="stSidebar"] {
            background: #0d1320;
            border-right: 1px solid rgba(148, 163, 184, 0.16);
        }
        [data-testid="stMetric"],
        [data-testid="stVerticalBlockBorderWrapper"] {
            background: rgba(15, 23, 42, 0.78);
            border-color: rgba(148, 163, 184, 0.2);
        }
        [data-testid="stMetric"] {
            border: 1px solid rgba(148, 163, 184, 0.2);
            border-radius: 8px;
            padding: 14px 16px;
        }
        .status-pill {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            border-radius: 999px;
            padding: 3px 9px;
            font-size: 12px;
            font-weight: 700;
            letter-spacing: 0;
        }
        .status-online {
            color: #bbf7d0;
            background: rgba(34, 197, 94, 0.18);
            border: 1px solid rgba(34, 197, 94, 0.38);
        }
        .status-offline {
            color: #fecaca;
            background: rgba(239, 68, 68, 0.18);
            border: 1px solid rgba(239, 68, 68, 0.38);
        }
        .muted {
            color: #94a3b8;
            font-size: 13px;
        }
        .node-title {
            font-size: 18px;
            font-weight: 800;
            margin-bottom: 2px;
        }
        .stProgress > div > div > div > div {
            background: #38bdf8;
        }
        </style>
        """,
        unsafe_allow_html=True,
    )


def status_pill(status: str) -> str:
    label = "ONLINE" if status == "online" else "OFFLINE"
    klass = "status-online" if status == "online" else "status-offline"
    return f"<span class='status-pill {klass}'>{label}</span>"


def pct(value: Any) -> float:
    try:
        return max(0.0, min(100.0, float(value)))
    except (TypeError, ValueError):
        return 0.0


def metric_or_dash(metric: dict[str, Any] | None, key: str, suffix: str = "") -> str:
    if not metric or metric.get(key) is None:
        return "-"
    value = metric[key]
    if isinstance(value, float):
        return f"{value:.1f}{suffix}"
    return f"{value}{suffix}"


def latest_metrics(nodes: list[dict[str, Any]], online_only: bool = True) -> list[dict[str, Any]]:
    selected = nodes
    if online_only:
        selected = [node for node in nodes if node["status"] == "online"]
    return [node["latest_metric"] for node in selected if node.get("latest_metric")]


def average_metric(metrics: list[dict[str, Any]], key: str) -> str:
    values = [float(metric[key]) for metric in metrics if metric.get(key) is not None]
    if not values:
        return "-"
    return f"{sum(values) / len(values):.1f}%"


def render_sidebar() -> None:
    with st.sidebar:
        st.header("控制台")
        auto_refresh = st.toggle("自动刷新", value=False)
        refresh_interval = st.slider("刷新间隔（秒）", 5, 120, 15, disabled=not auto_refresh)

        st.divider()
        with st.expander("添加 / 更新 VPS", expanded=False):
            with st.form("node_form"):
                node_id = st.text_input("Node ID", placeholder="hk-01")
                name = st.text_input("名称", placeholder="香港 01")
                submitted = st.form_submit_button("保存 VPS", type="primary", use_container_width=True)

            if submitted:
                if not name.strip():
                    st.warning("名称必填。")
                else:
                    payload = {
                        "node_id": node_id.strip() or None,
                        "name": name.strip(),
                        "ip": "",
                        "region": "",
                        "os_type": "",
                        "note": "",
                        "services": [],
                    }
                    result, error = safe_api_request("POST", "/api/nodes/register", json=payload)
                    if error:
                        st.error(error)
                    else:
                        saved_id = result["node"]["id"] if result else ""
                        st.success(f"已保存：{saved_id}")
                        st.rerun()

        st.divider()
        with st.expander("SSH 部署 Agent", expanded=False):
            st.caption("密码只用于本次 SSH 登录，不会保存到数据库。")
            with st.form("ssh_deploy_form"):
                host = st.text_input("主机 IP / 域名", placeholder="1.2.3.4")
                ssh_cols = st.columns([0.8, 1.2])
                port = ssh_cols[0].number_input("端口", min_value=1, max_value=65535, value=22, step=1)
                username = ssh_cols[1].text_input("用户名", value="root")
                password = st.text_input("密码", type="password")
                use_sudo = st.checkbox("非 root 用户使用 sudo", value=False)

                st.markdown("**监控标识**")
                node_cols = st.columns(2)
                node_id = node_cols[0].text_input("Node ID", placeholder="hk-01", key="deploy_node_id")
                name = node_cols[1].text_input("名称", placeholder="香港 01", key="deploy_name")
                remote_dir = st.text_input("远程安装目录", value="/opt/vps-monitor")
                server_url = st.text_input("中心服务端 URL", value=DASHBOARD_API_URL)
                token = st.text_input("监控 Token", value=SERVER_TOKEN, type="password")
                interval = st.number_input("上报间隔（秒）", min_value=2, max_value=300, value=10, step=1)
                disk_paths = st.text_input("磁盘路径", value="/")
                deploy_submitted = st.form_submit_button("登录并部署 Agent", type="primary", use_container_width=True)

            if deploy_submitted:
                payload = {
                    "host": host,
                    "port": port,
                    "username": username,
                    "password": password,
                    "use_sudo": use_sudo,
                    "remote_dir": remote_dir,
                    "server_url": server_url,
                    "node_id": node_id,
                    "token": token,
                    "interval": interval,
                    "name": name,
                    "node_ip": "",
                    "region": "",
                    "os_type": "Linux",
                    "note": "",
                    "services": [],
                    "disk_paths": disk_paths,
                }
                with st.spinner("正在通过 SSH 登录并部署 Agent..."):
                    try:
                        logs = deploy_agent(payload)
                    except (DeploymentError, OSError, Exception) as exc:
                        st.error(f"部署失败：{exc}")
                    else:
                        st.success("Agent 已部署并启动。")
                        st.code("\n".join(logs[-12:]), language="text")

        st.divider()
        st.caption(f"API：{DASHBOARD_API_URL}")
        st.caption("Token 来自 VPS_MONITOR_TOKEN")

        if auto_refresh:
            time.sleep(refresh_interval)
            st.rerun()


def render_overview_metrics(nodes: list[dict[str, Any]]) -> None:
    online = [node for node in nodes if node["status"] == "online"]
    offline = [node for node in nodes if node["status"] == "offline"]
    metrics = latest_metrics(nodes)
    online_metrics = latest_metrics(nodes, online_only=True)
    total_up = sum(float(metric.get("net_upload_bps") or 0) for metric in online_metrics)
    total_down = sum(float(metric.get("net_download_bps") or 0) for metric in online_metrics)

    cols = st.columns(5)
    cols[0].metric("在线 VPS", len(online))
    cols[1].metric("离线 VPS", len(offline))
    cols[2].metric("平均 CPU", average_metric(online_metrics or metrics, "cpu_percent"))
    cols[3].metric("平均内存", average_metric(online_metrics or metrics, "memory_percent"))
    cols[4].metric("总上行 / 下行", f"{format_bytes(total_up)}/s / {format_bytes(total_down)}/s")


def render_node_card(node: dict[str, Any]) -> None:
    metric = node.get("latest_metric") or {}
    with st.container(border=True):
        st.markdown(
            f"""
            <div class="node-title">{node['name']}</div>
            <div class="muted">{node['id']}</div>
            {status_pill(node['status'])}
            """,
            unsafe_allow_html=True,
        )
        st.progress(pct(metric.get("cpu_percent")) / 100, text=f"CPU {metric_or_dash(metric, 'cpu_percent', '%')}")
        st.progress(pct(metric.get("memory_percent")) / 100, text=f"内存 {metric_or_dash(metric, 'memory_percent', '%')}")
        st.progress(pct(metric.get("swap_percent")) / 100, text=f"Swap {metric_or_dash(metric, 'swap_percent', '%')}")
        st.progress(pct(metric.get("disk_percent")) / 100, text=f"磁盘 {metric_or_dash(metric, 'disk_percent', '%')}")

        c1, c2 = st.columns(2)
        c1.caption(f"上行 {format_bytes(metric.get('net_upload_bps'))}/s")
        c2.caption(f"下行 {format_bytes(metric.get('net_download_bps'))}/s")
        st.caption(f"CPU 核心：{metric.get('cpu_count') or '-'}")
        st.caption(
            f"累计流量：上传 {format_bytes(metric.get('net_bytes_sent'))} / 下载 {format_bytes(metric.get('net_bytes_recv'))}"
        )
        st.caption(f"最近上报：{node.get('last_seen_at') or '-'}")

        if st.button("查看详情", key=f"detail_{node['id']}", use_container_width=True):
            st.query_params["node"] = node["id"]
            st.rerun()


def render_overview(nodes: list[dict[str, Any]]) -> None:
    st.title(APP_TITLE)
    st.caption("多 VPS 轻量级监控面板")
    render_overview_metrics(nodes)

    st.subheader("VPS 列表")
    if not nodes:
        st.info("还没有 VPS。先在左侧添加节点，再把 agent 部署到对应机器。")
        return

    for start in range(0, len(nodes), 3):
        cols = st.columns(3)
        for col, node in zip(cols, nodes[start : start + 3]):
            with col:
                render_node_card(node)


def history_dataframe(metrics: list[dict[str, Any]]) -> pd.DataFrame:
    if not metrics:
        return pd.DataFrame()
    df = pd.DataFrame(metrics)
    df["time"] = pd.to_datetime(df["collected_at"])
    return df.sort_values("time")


def render_resource_charts(df: pd.DataFrame) -> None:
    if df.empty:
        st.info("这个时间范围还没有历史数据。")
        return

    resource_cols = ["cpu_percent", "memory_percent", "swap_percent", "disk_percent"]
    resource_df = df[["time", *resource_cols]].rename(
        columns={"cpu_percent": "CPU", "memory_percent": "内存", "swap_percent": "Swap", "disk_percent": "磁盘"}
    )
    st.line_chart(resource_df.set_index("time"), height=240)

    up_df = df[["time", "net_upload_bps"]].copy()
    down_df = df[["time", "net_download_bps"]].copy()
    up_df["上行 KB/s"] = up_df["net_upload_bps"] / 1024
    down_df["下行 KB/s"] = down_df["net_download_bps"] / 1024

    net_cols = st.columns(2)
    with net_cols[0]:
        st.caption("网络上行")
        st.line_chart(up_df[["time", "上行 KB/s"]].set_index("time"), height=220)
    with net_cols[1]:
        st.caption("网络下行")
        st.line_chart(down_df[["time", "下行 KB/s"]].set_index("time"), height=220)


def render_disk_paths(metric: dict[str, Any] | None) -> None:
    metric = metric or {}
    disks = metric.get("disks") or []
    if not disks:
        st.info("最近一次上报没有磁盘路径数据。")
        return

    rows = []
    for disk in disks:
        rows.append(
            {
                "路径": disk.get("path") or "-",
                "状态": disk.get("state") or "-",
                "已用": format_bytes(disk.get("used")),
                "总容量": format_bytes(disk.get("total")),
                "剩余": format_bytes(disk.get("free")),
                "使用率": f"{float(disk.get('percent') or 0):.1f}%" if disk.get("state") == "ok" else "-",
                "说明": disk.get("detail") or "",
            }
        )
    st.dataframe(pd.DataFrame(rows), use_container_width=True, hide_index=True)


def render_network_totals(metric: dict[str, Any] | None) -> None:
    metric = metric or {}
    rows = [
        {"项目": "当前上行", "值": f"{format_bytes(metric.get('net_upload_bps'))}/s"},
        {"项目": "当前下行", "值": f"{format_bytes(metric.get('net_download_bps'))}/s"},
        {"项目": "累计上传", "值": format_bytes(metric.get("net_bytes_sent"))},
        {"项目": "累计下载", "值": format_bytes(metric.get("net_bytes_recv"))},
    ]
    st.dataframe(pd.DataFrame(rows), use_container_width=True, hide_index=True)


def render_detail(node_id: str) -> None:
    node_result, node_error = safe_api_request("GET", f"/api/nodes/{node_id}")
    if node_error:
        st.error(node_error)
        if st.button("返回总览"):
            st.query_params.clear()
            st.rerun()
        return

    node = node_result["node"] if node_result else None
    if not node:
        st.error("节点不存在。")
        return

    top_left, top_right = st.columns([1, 0.2])
    with top_left:
        st.markdown(f"### {node['name']} {status_pill(node['status'])}", unsafe_allow_html=True)
        st.caption(f"{node['id']} · 最近上报 {node.get('last_seen_at') or '-'}")
    with top_right:
        if st.button("返回总览", use_container_width=True):
            st.query_params.clear()
            st.rerun()

    metric = node.get("latest_metric") or {}
    metric_cols = st.columns(5)
    metric_cols[0].metric("CPU / 核心", f"{metric_or_dash(metric, 'cpu_percent', '%')} / {metric.get('cpu_count') or '-'}")
    metric_cols[1].metric("内存", f"{format_bytes(metric.get('memory_used'))} / {format_bytes(metric.get('memory_total'))}")
    metric_cols[2].metric("Swap", f"{format_bytes(metric.get('swap_used'))} / {format_bytes(metric.get('swap_total'))}")
    metric_cols[3].metric("磁盘", f"{format_bytes(metric.get('disk_used'))} / {format_bytes(metric.get('disk_total'))}")
    metric_cols[4].metric("上行 / 下行", f"{format_bytes(metric.get('net_upload_bps'))}/s / {format_bytes(metric.get('net_download_bps'))}/s")

    window_label = st.radio("历史范围", list(WINDOW_OPTIONS.keys()), horizontal=True, label_visibility="collapsed")
    history_result, history_error = safe_api_request(
        "GET", f"/api/nodes/{node_id}/metrics", params={"window": WINDOW_OPTIONS[window_label]}
    )
    if history_error:
        st.error(history_error)
        return

    st.subheader("资源曲线")
    history_df = history_dataframe((history_result or {}).get("metrics", []))
    render_resource_charts(history_df)

    lower_cols = st.columns([1.2, 0.8])
    with lower_cols[0]:
        st.subheader("磁盘路径")
        render_disk_paths(metric)
    with lower_cols[1]:
        st.subheader("网络流量")
        render_network_totals(metric)


def main() -> None:
    st.set_page_config(page_title=APP_TITLE, layout="wide")
    inject_style()
    render_sidebar()

    result, error = safe_api_request("GET", "/api/nodes")
    if error:
        st.title(APP_TITLE)
        st.error(error)
        st.code("python -m uvicorn server:app --host 0.0.0.0 --port 8000", language="powershell")
        return

    nodes = (result or {}).get("nodes", [])
    selected_node = st.query_params.get("node")
    if selected_node:
        render_detail(str(selected_node))
    else:
        render_overview(nodes)


if __name__ == "__main__":
    main()
