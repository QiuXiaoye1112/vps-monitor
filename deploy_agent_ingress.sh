#!/usr/bin/env bash
set -euo pipefail

AGENT_PORT="${AGENT_PORT:-8080}"
NGINX_SITE="/etc/nginx/sites-available/vps-monitor-agent.conf"
NGINX_LINK="/etc/nginx/sites-enabled/vps-monitor-agent.conf"

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Please run as root: sudo bash deploy_agent_ingress.sh"
  exit 1
fi

if ! [[ "$AGENT_PORT" =~ ^[0-9]+$ ]] || [[ "$AGENT_PORT" -lt 1 || "$AGENT_PORT" -gt 65535 ]]; then
  echo "AGENT_PORT must be a TCP port between 1 and 65535."
  exit 1
fi

if ! command -v nginx >/dev/null 2>&1; then
  echo "nginx was not found. Install nginx first, then rerun this script."
  exit 1
fi

cat > "$NGINX_SITE" <<EOF
server {
    listen $AGENT_PORT;
    server_name _;

    client_max_body_size 10m;

    location /api/ {
        proxy_pass http://127.0.0.1:8000;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }

    location / {
        return 404;
    }
}
EOF

ln -sf "$NGINX_SITE" "$NGINX_LINK"
nginx -t
systemctl reload nginx

echo "Agent ingress enabled on port $AGENT_PORT."
echo "Allowed path: /api/"
echo "Other paths return 404."
