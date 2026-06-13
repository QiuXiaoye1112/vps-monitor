#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${VPS_MONITOR_DIR:-/opt/vps-monitor}"

RED='\033[31m'; GREEN='\033[32m'; CYAN='\033[36m'; RESET='\033[0m'

info() { printf "${CYAN}[VPS Monitor]${RESET} %s\n" "$*"; }
ok()   { printf "${GREEN}[OK]${RESET} %s\n" "$*"; }
fail() { printf "${RED}[错误]${RESET} %s\n" "$*" >&2; exit 1; }

rerun_as_root() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then return; fi
  command -v sudo >/dev/null 2>&1 || fail "请使用 root 账号运行。"
  local temp_script; temp_script="$(mktemp)"
  if [[ -f "${BASH_SOURCE[0]}" ]]; then
    cat "${BASH_SOURCE[0]}" > "$temp_script"
  else
    local raw_url="https://raw.githubusercontent.com/QiuXiaoye1112/vps-monitor/master/uninstall.sh"
    curl -fsSL --max-time 30 "$raw_url" -o "$temp_script" || { rm -f "$temp_script"; fail "无法重新获取脚本。下载后运行：curl -fsSL $raw_url -o uninstall.sh && sudo bash uninstall.sh"; }
  fi
  chmod 700 "$temp_script"
  sudo env VPS_MONITOR_DIR="$INSTALL_DIR" bash "$temp_script"
  rm -f "$temp_script"; exit 0
}

remove_service() {
  local service="$1"
  command -v systemctl >/dev/null 2>&1 && systemctl disable --now "$service" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/$service.service"
}

remove_firewall_rules() {
  command -v iptables >/dev/null 2>&1 || return
  local rules line removed=0; rules="$(iptables -S INPUT 2>/dev/null || true)"
  while IFS= read -r line; do
    [[ "$line" == *"--dport 8080"* ]] || continue
    read -r -a parts <<< "$line"
    [[ "${parts[0]:-}" == "-A" && "${parts[1]:-}" == "INPUT" ]] || continue
    parts[0]="-D"; iptables "${parts[@]}" >/dev/null 2>&1 || true; ((removed++))
  done <<< "$rules"
  [[ $removed -gt 0 ]] && info "已移除 $removed 条 8080 防火墙规则"
  command -v netfilter-persistent >/dev/null 2>&1 && netfilter-persistent save >/dev/null 2>&1 || true
}

remove_certificates() {
  command -v certbot >/dev/null 2>&1 || return
  local domain=""
  for site in /etc/nginx/sites-available/vps-monitor.conf /etc/nginx/conf.d/vps-monitor.conf; do
    if [[ -f "$site" ]]; then
      domain="$(sed -nE 's/^[[:space:]]*server_name[[:space:]]+([^;]+);.*/\1/p' "$site" | head -n 1)"
      [[ -n "$domain" && "$domain" != "_" ]] && break
    fi
  done
  [[ -n "$domain" && "$domain" != "_" ]] && { certbot delete --cert-name "$domain" --non-interactive >/dev/null 2>&1 || true; info "已删除域名 $domain 的 SSL 证书"; }
}

remove_databases() {
  local db_path="$INSTALL_DIR/vps_monitor.db"
  if [[ -f /etc/vps-monitor.env ]]; then
    local configured; configured="$(sed -n 's/^VPS_MONITOR_DB=//p' /etc/vps-monitor.env | tail -n 1)"
    [[ -n "$configured" ]] && db_path="$configured"
  fi
  [[ -n "$db_path" && "$db_path" == *vps_monitor* ]] && { rm -f "$db_path" "$db_path".bak.*; info "已删除数据库：$db_path"; }
  rm -f "$INSTALL_DIR/nodes.db" "$INSTALL_DIR/vps_monitor.sqlite" "$INSTALL_DIR/vps_monitor.sqlite3"
}

main() {
  rerun_as_root
  info "正在卸载 VPS Monitor..."

  remove_service vps-monitor-api
  remove_service vps-monitor-agent
  remove_service vps-monitor-dashboard
  remove_certificates
  remove_firewall_rules
  remove_databases

  rm -f /etc/vps-monitor.env /etc/vps-monitor-agent.toml /etc/vps-monitor-role
  rm -f /usr/local/bin/vps-monitor /usr/local/bin/vm

  for pattern in "sites-available/vps-monitor*.conf" "sites-enabled/vps-monitor*.conf" "conf.d/vps-monitor*.conf"; do
    rm -f /etc/nginx/$pattern /etc/nginx/$pattern.bak.* 2>/dev/null || true
  done

  [[ -d "$INSTALL_DIR" ]] && { rm -rf "$INSTALL_DIR"; info "已删除项目目录：$INSTALL_DIR"; }

  command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload >/dev/null 2>&1 || true
  command -v nginx >/dev/null 2>&1 && nginx -t >/dev/null 2>&1 && systemctl reload nginx >/dev/null 2>&1 || true

  ok "VPS Monitor 已完整卸载。"
}

main "$@"
