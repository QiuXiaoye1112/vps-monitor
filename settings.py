from __future__ import annotations

import os
from pathlib import Path


BASE_DIR = Path(__file__).resolve().parent

DB_PATH = Path(os.getenv("VPS_MONITOR_DB", BASE_DIR / "vps_monitor.db"))
SERVER_TOKEN = os.getenv("VPS_MONITOR_TOKEN", "change-me")
OFFLINE_AFTER_SECONDS = int(os.getenv("VPS_MONITOR_OFFLINE_AFTER", "30"))

DASHBOARD_API_URL = os.getenv("VPS_MONITOR_API_URL", "http://127.0.0.1:8000")
