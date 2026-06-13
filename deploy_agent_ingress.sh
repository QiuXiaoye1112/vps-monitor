#!/usr/bin/env bash
set -euo pipefail

AGENT_PORT="${AGENT_PORT:-8080}"
APP_DIR="${APP_DIR:-/opt/vps-monitor}"
API_HOST="${API_HOST:-127.0.0.1}"
API_PORT="${API_PORT:-8000}"

BOLD='\033[1m'; DIM='\033[2m'; RED='\033[31m'; GREEN='\033[32m'
YELLOW='\033[33m'; BLUE='\033[34m'; CYAN='\033[36m'; RESET='\033[0m'

_step()  { printf "\n${BOLD}${BLUE}[%d/%d]${RESET} %s\n" "$1" "$2" "$3"; }
ok()     { printf "  ${GREEN}✓${RESET} %s\n" "$*"; }
warn()   { printf "  ${YELLOW}⚠${RESET} %s\n" "$*" >&2; }
fail()   { printf "\n${RED}✗${RESET} %s\n" "$*" >&2; exit 1; }
info()   { printf "  ${CYAN}›${RESET} %s\n" "$*"; }
detail() { printf "    ${DIM}%s${RESET}\n" "$*"; }

check_root()     { [[ "${EUID:-$(id -u)}" -eq 0 ]] || fail "请以 root 运行：sudo bash deploy_agent_ingress.sh"; }
validate_port()  { [[ "$AGENT_PORT" =~ ^[0-9]+$ && "$AGENT_PORT" -ge 1 && "$AGENT_PORT" -le 65535 ]] || fail "端口无效：$AGENT_PORT"; }
check_nginx()    { command -v nginx >/dev/null 2>&1 || fail "nginx 未安装。apt-get install -y nginx"; }

check_upstream() {
  if curl -sf "http://${API_HOST}:${API_PORT}/api/health" >/dev/null 2>&1; then
    ok "上游 API 运行中（${API_HOST}:${API_PORT}）"
  else
    warn "上游 API 未响应，入口配置将继续写入但暂不可用。"
    detail "请先部署中心面板：sudo bash ${APP_DIR}/deploy_panel.sh <域名或IP> <token>"
  fi
}

detect_nginx_dirs() {
  if [[ -d "/etc/nginx/sites-available" && -d "/etc/nginx/sites-enabled" ]]; then
    NGINX_AVAILABLE="/etc/nginx/sites-available"; NGINX_ENABLED="/etc/nginx/sites-enabled"; NGINX_USE_SYMLINK=true
  elif [[ -d "/etc/nginx/conf.d" ]]; then
    NGINX_AVAILABLE="/etc/nginx/conf.d"; NGINX_ENABLED="/etc/nginx/conf.d"; NGINX_USE_SYMLINK=false
  else
    mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
    NGINX_AVAILABLE="/etc/nginx/sites-available"; NGINX_ENABLED="/etc/nginx/sites-enabled"; NGINX_USE_SYMLINK=true
  fi
}

backup_if_exists() { local p="$1"; [[ -f "$p" ]] && cp "$p" "${p}.bak.$(date +%Y%m%d%H%M%S)" && detail "已备份 $p"; }

write_config() {
  local site_path="$NGINX_AVAILABLE/vps-monitor-agent.conf"
  backup_if_exists "$site_path"
  cat > "$site_path" <<EOF
server {
    listen ${AGENT_PORT};
    server_name _;
    client_max_body_size 1m;
    location /api/ {
        proxy_pass http://${API_HOST}:${API_PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
    location / { return 404; }
}
EOF
  chmod 644 "$site_path"
  $NGINX_USE_SYMLINK && { rm -f "$NGINX_ENABLED/vps-monitor-agent.conf"; ln -sf "$site_path" "$NGINX_ENABLED/vps-monitor-agent.conf"; }
  ok "Agent 入口配置已写入（端口 ${AGENT_PORT}，仅开放 /api/）"
}

apply_config() {
  info "测试 Nginx 配置..."
  nginx -t 2>&1 | tail -3 || { warn "Nginx 配置测试失败，旧配置已备份。"; return 1; }
  systemctl reload nginx
  ok "Nginx 已重载"
}

verify_ingress() {
  info "验证入口..."
  local hs; hs=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${AGENT_PORT}/api/health" 2>/dev/null || echo "000")
  [[ "$hs" == "200" ]] && ok "/api/health → 200 ✓" || warn "/api/health → ${hs}（预期 200）"
  local rs; rs=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${AGENT_PORT}/" 2>/dev/null || echo "000")
  [[ "$rs" == "404" ]] && ok "/ → 404 ✓" || warn "/ → ${rs}（预期 404）"
}

main() {
  check_root; validate_port; check_nginx
  clear 2>/dev/null || true
  printf "${BOLD}${CYAN}╔══════════════════════════════════════════╗\n║     VPS Monitor · Agent 入口配置          ║\n╚══════════════════════════════════════════╝${RESET}\n\n"
  detect_nginx_dirs
  _step 1 4 "检查上游 API";   check_upstream
  _step 2 4 "写入配置";       write_config
  _step 3 4 "应用配置";       apply_config || { warn "配置应用失败。"; exit 1; }
  _step 4 4 "验证入口";       verify_ingress
  echo; printf "${BOLD}${GREEN}✓ Agent 入口已就绪${RESET}\n"
  printf "  远程 Agent 的 server_url: ${CYAN}http://<中心VPS公网IP>:${AGENT_PORT}${RESET}\n"
  printf "  下一步：${CYAN}sudo bash ${APP_DIR}/allow_agent_ip.sh <远程VPS_IP>${RESET}\n\n"
}

main "$@"
