#!/usr/bin/env bash
set -euo pipefail

DOMAIN="${1:-}"
TOKEN="${2:-}"
APP_DIR="${APP_DIR:-/opt/vps-monitor}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
NGINX_SITE="/etc/nginx/sites-available/vps-monitor.conf"
NGINX_LINK="/etc/nginx/sites-enabled/vps-monitor.conf"
ENV_FILE="/etc/vps-monitor.env"

if [[ -z "$DOMAIN" || -z "$TOKEN" ]]; then
  echo "Usage: sudo bash deploy_panel.sh your-domain.com your-secret-token"
  exit 1
fi

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Please run as root: sudo bash deploy_panel.sh $DOMAIN <token>"
  exit 1
fi

if [[ ! -f "$APP_DIR/server.py" ]]; then
  echo "Project files were not found in $APP_DIR"
  echo "Upload/copy this project to $APP_DIR first, or run with APP_DIR=/path/to/project."
  exit 1
fi

cd "$APP_DIR"

if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  echo "$PYTHON_BIN was not found. Install Python 3 first."
  exit 1
fi

if ! command -v nginx >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y nginx
  else
    echo "nginx was not found. Install nginx first, then rerun this script."
    exit 1
  fi
fi

"$PYTHON_BIN" -m venv "$APP_DIR/.venv"
"$APP_DIR/.venv/bin/python" -m pip install --upgrade pip
"$APP_DIR/.venv/bin/python" -m pip install -r "$APP_DIR/requirements.txt"

cat > "$ENV_FILE" <<EOF
VPS_MONITOR_TOKEN=$TOKEN
VPS_MONITOR_DB=$APP_DIR/vps_monitor.db
EOF
chmod 600 "$ENV_FILE"

cat > /etc/systemd/system/vps-monitor-api.service <<EOF
[Unit]
Description=VPS Monitor API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_FILE
ExecStart=$APP_DIR/.venv/bin/python -m uvicorn server:app --host 127.0.0.1 --port 8000
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

cat > "$NGINX_SITE" <<EOF
server {
    listen 80;
    server_name $DOMAIN;

    client_max_body_size 10m;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

ln -sf "$NGINX_SITE" "$NGINX_LINK"
nginx -t

systemctl daemon-reload
systemctl disable --now vps-monitor-dashboard 2>/dev/null || true
rm -f /etc/systemd/system/vps-monitor-dashboard.service
systemctl enable --now vps-monitor-api nginx
systemctl reload nginx

echo
echo "Done."
echo "Dashboard: http://$DOMAIN"
echo "Agent server_url for other VPS: http://$DOMAIN"
echo "Token: $TOKEN"
