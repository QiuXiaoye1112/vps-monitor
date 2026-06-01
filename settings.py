from __future__ import annotations

import os
from pathlib import Path


BASE_DIR = Path(__file__).resolve().parent

DB_PATH = Path(os.getenv("VPS_MONITOR_DB", BASE_DIR / "vps_monitor.db"))
SERVER_TOKEN = os.getenv("VPS_MONITOR_TOKEN", "change-me")
OFFLINE_AFTER_SECONDS = int(os.getenv("VPS_MONITOR_OFFLINE_AFTER", "30"))

DASHBOARD_API_URL = os.getenv("VPS_MONITOR_API_URL", "http://127.0.0.1:8000")
LOCAL_MONITOR_ENABLED = os.getenv("VPS_MONITOR_LOCAL_ENABLED", "1").lower() not in {"0", "false", "no"}
LOCAL_MONITOR_NODE_ID = os.getenv("VPS_MONITOR_LOCAL_NODE_ID", "center")
LOCAL_MONITOR_NODE_NAME = os.getenv("VPS_MONITOR_LOCAL_NODE_NAME", "中心 VPS")
LOCAL_MONITOR_INTERVAL = max(1, int(os.getenv("VPS_MONITOR_LOCAL_INTERVAL", "1")))
LOCAL_MONITOR_DISK_PATHS = os.getenv("VPS_MONITOR_LOCAL_DISK_PATHS", "/")
