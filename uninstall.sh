#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${VPS_MONITOR_DIR:-/opt/vps-monitor}"

info() {
  printf '\033[36m[VPS Monitor]\033[0m %s\n' "$*"
}

rerun_as_root() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    return
  fi
  if ! command -v sudo >/dev/null 2>&1; then
    printf '\033[31m[错误]\033[0m 请使用 root 账号运行。\n' >&2
    exit 1
  fi
  local temp_script
  temp_script="$(mktemp)"
  cat "${BASH_SOURCE[0]}" > "$temp_script"
  chmod 700 "$temp_script"
  sudo env VPS_MONITOR_DIR="$INSTALL_DIR" bash "$temp_script"
  rm -f "$temp_script"
  exit 0
}

remove_service() {
  local service="$1"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now "$service" >/dev/null 2>&1 || true
  fi
  rm -f "/etc/systemd/system/$service.service"
}

remove_firewall_rules() {
  command -v iptables >/dev/null 2>&1 || return
  local rules line
  rules="$(iptables -S INPUT 2>/dev/null || true)"
  while IFS= read -r line; do
    [[ "$line" == *"--dport 8080"* ]] || continue
    # Rules come from iptables itself, preserving the exact arguments is required for deletion.
    read -r -a parts <<< "$line"
    [[ "${parts[0]:-}" == "-A" && "${parts[1]:-}" == "INPUT" ]] || continue
    parts[0]="-D"
    iptables "${parts[@]}" >/dev/null 2>&1 || true
  done <<< "$rules"
  if command -v netfilter-persistent >/dev/null 2>&1; then
    netfilter-persistent save >/dev/null 2>&1 || true
  fi
}

remove_certificates() {
  command -v certbot >/dev/null 2>&1 || return
  local site="/etc/nginx/sites-available/vps-monitor.conf"
  [[ -f "$site" ]] || return
  local domain
  domain="$(sed -nE 's/^[[:space:]]*server_name[[:space:]]+([^;]+);.*/\1/p' "$site" | head -n 1)"
  [[ -n "$domain" && "$domain" != "_" ]] || return
  certbot delete --cert-name "$domain" --non-interactive >/dev/null 2>&1 || true
}

remove_databases() {
  local db_path="$INSTALL_DIR/vps_monitor.db"
  if [[ -f /etc/vps-monitor.env ]]; then
    local configured
    configured="$(sed -n 's/^VPS_MONITOR_DB=//p' /etc/vps-monitor.env | tail -n 1)"
    [[ -n "$configured" ]] && db_path="$configured"
  fi
  rm -f "$db_path" "$db_path".bak.*
  rm -f "$INSTALL_DIR/nodes.db" "$INSTALL_DIR"/*.sqlite "$INSTALL_DIR"/*.sqlite3
}

main() {
  rerun_as_root
  info "正在清理新旧版本 VPS Monitor..."

  remove_service vps-monitor-api
  remove_service vps-monitor-agent
  remove_service vps-monitor-dashboard
  remove_certificates
  remove_firewall_rules
  remove_databases

  rm -f /etc/vps-monitor.env
  rm -f /etc/vps-monitor-agent.toml
  rm -f /etc/vps-monitor-role
  rm -f /usr/local/bin/vps-monitor
  rm -f /etc/nginx/sites-enabled/vps-monitor.conf
  rm -f /etc/nginx/sites-available/vps-monitor.conf
  rm -f /etc/nginx/sites-enabled/vps-monitor-agent.conf
  rm -f /etc/nginx/sites-available/vps-monitor-agent.conf
  rm -rf "$INSTALL_DIR"

  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
  fi
  if command -v nginx >/dev/null 2>&1 && nginx -t >/dev/null 2>&1; then
    systemctl reload nginx >/dev/null 2>&1 || true
  fi

  info "VPS Monitor 新旧版本已完整删除。"
}

main "$@"
