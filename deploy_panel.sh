#!/usr/bin/env bash
set -euo pipefail

DOMAIN="${1:-}"; TOKEN="${2:-}"
APP_DIR="${APP_DIR:-/opt/vps-monitor}"; PYTHON_BIN="${PYTHON_BIN:-python3}"
API_HOST="${API_HOST:-127.0.0.1}"; API_PORT="${API_PORT:-8000}"
ENV_FILE="/etc/vps-monitor.env"; SERVICE_NAME="vps-monitor-api"
LEGACY_DASHBOARD="vps-monitor-dashboard"; REPO_URL="${REPO_URL:-https://github.com/QiuXiaoye1112/vps-monitor.git}"

BOLD='\033[1m'; DIM='\033[2m'; RED='\033[31m'; GREEN='\033[32m'
YELLOW='\033[33m'; BLUE='\033[34m'; CYAN='\033[36m'; RESET='\033[0m'

step=0; total_steps=7; is_domain=false; https_ok=false

_step() { step=$((step+1)); printf "\n${BOLD}${BLUE}[%d/%d]${RESET} %s\n" "$step" "$total_steps" "$*"; }
ok()     { printf "  ${GREEN}✓${RESET} %s\n" "$*"; }
warn()   { printf "  ${YELLOW}⚠${RESET} %s\n" "$*" >&2; }
fail()   { printf "\n${RED}✗${RESET} %s\n" "$*" >&2; exit 1; }
info()   { printf "  ${CYAN}›${RESET} %s\n" "$*"; }
detail() { printf "    ${DIM}%s${RESET}\n" "$*"; }

check_root() { [[ "${EUID:-$(id -u)}" -eq 0 ]] || fail "请以 root 运行：sudo bash deploy_panel.sh <域名或IP> <token>"; }

validate_input() {
  if [[ -z "$DOMAIN" || -z "$TOKEN" ]]; then
    clear 2>/dev/null || true
    printf "${BOLD}${CYAN}╔══════════════════════════════════════════╗\n║       VPS Monitor · 中心面板部署          ║\n╚══════════════════════════════════════════╝${RESET}\n\n"
    printf "${BOLD}用法：${RESET}\n  sudo bash deploy_panel.sh ${CYAN}<域名或IP>${RESET} ${CYAN}<token>${RESET}\n\n"
    printf "${BOLD}示例：${RESET}\n  # 域名 → 自动 HTTPS\n  sudo bash deploy_panel.sh monitor.example.com \$(openssl rand -hex 24)\n\n  # IP → HTTP\n  sudo bash deploy_panel.sh 1.2.3.4 \$(openssl rand -hex 24)\n\n"
    printf "${BOLD}可选 ENV：${RESET} API_HOST API_PORT APP_DIR\n"
    exit 1
  fi
  [[ "$TOKEN" =~ [[:space:]] ]] && fail "token 不能包含空格或换行"
}

check_python() {
  command -v "$PYTHON_BIN" >/dev/null 2>&1 || fail "未找到 $PYTHON_BIN。安装 Python 3.8+"
  local ver; ver=$("$PYTHON_BIN" -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')" 2>/dev/null) || ver="0.0"
  [[ "${ver%%.*}" -ge 3 && "${ver#*.}" -ge 8 ]] || fail "需要 Python 3.8+，当前：$ver"
  detail "Python $ver"
}

check_project() {
  [[ -f "$APP_DIR/server.py" ]] && { detail "项目已存在：$APP_DIR"; return; }
  command -v git >/dev/null 2>&1 || fail "git 未安装且项目目录 $APP_DIR 不存在。"
  info "正在自动 clone 项目..."
  mkdir -p "$(dirname "$APP_DIR")"
  git clone --depth 1 "$REPO_URL" "$APP_DIR" 2>&1 | tail -3 || fail "clone 失败。git clone $REPO_URL $APP_DIR"
  ok "项目已 clone 到 $APP_DIR"
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

backup_if_exists() { local p="$1"; [[ -f "$p" ]] && cp "$p" "${p}.bak.$(date +%Y%m%d%H%M%S)" && detail "已备份 $p"; true; }

install_system_deps() {
  local missing=()
  for pkg in python3 python3-venv python3-pip nginx curl sqlite3; do command -v "${pkg}" >/dev/null 2>&1 || missing+=("$pkg"); done
  "$PYTHON_BIN" -c "import venv" 2>/dev/null || missing+=("python3-venv")
  [[ ${#missing[@]} -eq 0 ]] && { ok "系统依赖已就绪"; return; }
  info "正在安装：${missing[*]}"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq 2>/dev/null || warn "apt update 失败，继续安装..."
    apt-get install -y -qq "${missing[@]}" 2>/dev/null || fail "apt 安装失败，请检查网络后手动执行：apt-get install -y ${missing[*]}"
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q "${missing[@]}" || fail "dnf 安装失败。"
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q "${missing[@]}" || fail "yum 安装失败。"
  else fail "不支持的包管理器。请手动安装：${missing[*]}"; fi
  ok "系统依赖安装完成"
}

setup_venv() {
  if [[ ! -d "$APP_DIR/.venv" ]]; then
    info "创建 Python 虚拟环境..."
    "$PYTHON_BIN" -m venv "$APP_DIR/.venv" || fail "创建虚拟环境失败，请确认 python3-venv 已安装。"
  fi
  info "安装 Python 依赖..."
  "$APP_DIR/.venv/bin/pip" install --upgrade pip -q 2>/dev/null || true
  "$APP_DIR/.venv/bin/pip" install -r "$APP_DIR/requirements.txt" -q 2>/dev/null || fail "pip 安装依赖失败，请检查网络连接。"
  ok "Python 环境就绪"
}

write_env_file() {
  backup_if_exists "$ENV_FILE"
  cat > "$ENV_FILE" <<EOF
VPS_MONITOR_TOKEN=$TOKEN
VPS_MONITOR_DB=$APP_DIR/vps_monitor.db
VPS_MONITOR_API_HOST=$API_HOST
VPS_MONITOR_API_PORT=$API_PORT
EOF
  chmod 600 "$ENV_FILE"; ok "中心配置已写入 $ENV_FILE"
}

write_nginx_config() {
  local site_path="$NGINX_AVAILABLE/vps-monitor.conf"; backup_if_exists "$site_path"
  cat > "$site_path" <<EOF
server {
    listen 80;
    server_name $DOMAIN;
    client_max_body_size 1m;
    location / {
        proxy_pass http://${API_HOST}:${API_PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
  chmod 644 "$site_path"
  $NGINX_USE_SYMLINK && { rm -f "$NGINX_ENABLED/vps-monitor.conf"; ln -sf "$site_path" "$NGINX_ENABLED/vps-monitor.conf"; }
  ok "Nginx 配置已写入"
}

write_systemd_service() {
  local unit_path="/etc/systemd/system/$SERVICE_NAME.service"; backup_if_exists "$unit_path"
  cat > "$unit_path" <<EOF
[Unit]
Description=VPS Monitor API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_FILE
ExecStart=$APP_DIR/.venv/bin/python -m uvicorn server:app --host ${API_HOST} --port ${API_PORT}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
  chmod 644 "$unit_path"; ok "systemd 服务已写入"
}

apply_and_start() {
  if systemctl is-enabled "$LEGACY_DASHBOARD" >/dev/null 2>&1; then
    systemctl disable --now "$LEGACY_DASHBOARD" 2>/dev/null || true
    rm -f "/etc/systemd/system/$LEGACY_DASHBOARD.service"; detail "已清理旧版 dashboard 服务"
  fi
  info "测试 Nginx 配置..."
  if ! nginx -t >/dev/null 2>&1; then warn "Nginx 配置测试失败，旧配置已备份。"; return 1; fi
  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME" nginx 2>/dev/null || true
  systemctl restart "$SERVICE_NAME"
  systemctl reload nginx
  ok "服务已启动"
}

enable_https() {
  if ! $is_domain; then detail "IP 部署，跳过 HTTPS"; https_ok=false; return 0; fi
  info "检测到域名，正在申请 HTTPS 证书..."
  if ! command -v certbot >/dev/null 2>&1; then
    detail "安装 certbot..."
    { command -v apt-get >/dev/null 2>&1 && apt-get install -y -qq certbot python3-certbot-nginx 2>&1 | tail -1; } || \
    { command -v dnf >/dev/null 2>&1 && dnf install -y -q certbot python3-certbot-nginx 2>&1 | tail -1; } || \
    { command -v yum >/dev/null 2>&1 && yum install -y -q certbot python3-certbot-nginx 2>&1 | tail -1; } || \
    { warn "无法自动安装 certbot，跳过 HTTPS"; https_ok=false; return; }
  fi
  if certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email --redirect 2>&1 | tail -5; then
    ok "HTTPS 证书已启用：https://${DOMAIN}"; https_ok=true
    systemctl enable --now certbot.timer 2>/dev/null || true
    detail "证书自动续期已启用"
  else
    warn "证书申请失败（80 端口公网不可达或 DNS 未生效？）"
    warn "HTTP 仍然可用：http://${DOMAIN}"; detail "稍后手动：sudo certbot --nginx -d ${DOMAIN}"; https_ok=false
  fi
}

wait_for_api() {
  info "等待 API 就绪..."
  local dots=""
  for i in $(seq 1 15); do
    if curl -sf "http://${API_HOST}:${API_PORT}/api/health" >/dev/null 2>&1; then
      printf "\r  ${GREEN}✓${RESET} API 响应正常        \n"; return 0
    fi
    dots="${dots}."; printf "\r  ${DIM}检查中%s${RESET}" "$dots"; sleep 1
  done
  printf "\n"; warn "API 未在 15 秒内响应。稍后检查：curl http://${API_HOST}:${API_PORT}/api/health"; return 1
}

print_preflight() {
  echo; printf "${BOLD}部署摘要：${RESET}\n"
  printf "  %-16s ${CYAN}%s${RESET}\n" "面板地址" "http${is_domain:+s}://${DOMAIN}"
  printf "  %-16s ${CYAN}%s${RESET}\n" "API 监听" "${API_HOST}:${API_PORT}"
  printf "  %-16s ${CYAN}%s${RESET}\n" "项目目录" "$APP_DIR"
  printf "  %-16s ${GREEN}%s${RESET}\n" "Token" "${TOKEN:0:12}..."
  echo; read -r -p "确认开始部署？[Y/n] " confirm < /dev/tty
  [[ -z "$confirm" || "$confirm" =~ ^[Yy]$ ]] || { info "已取消。"; exit 0; }
}

print_result() {
  local proto; ${https_ok:-false} && proto="https" || proto="http"
  echo; printf "${BOLD}${GREEN}╔══════════════════════════════════════════╗\n║          ✓  部署完成！                   ║\n╚══════════════════════════════════════════╝${RESET}\n\n"
  printf "${BOLD}访问地址：${RESET}\n  Dashboard  ${CYAN}%s://%s${RESET}\n  API        ${CYAN}http://%s:%s${RESET}\n  Token      ${DIM}%s${RESET}\n" "$proto" "$DOMAIN" "$API_HOST" "$API_PORT" "$TOKEN"
  echo; printf "${BOLD}管理命令：${RESET} ${CYAN}sudo vm${RESET}\n"
  if ! ${https_ok:-false} && $is_domain; then echo; warn "HTTPS 未启用，稍后：sudo certbot --nginx -d ${DOMAIN}"; fi
  echo; printf "${BOLD}下一步：${RESET}\n  1 浏览器打开 ${CYAN}%s://%s${RESET}\n  2 开放 Agent 入口：${DIM}sudo bash %s/deploy_agent_ingress.sh${RESET}\n\n" "$proto" "$DOMAIN" "$APP_DIR"
}

main() {
  check_root; validate_input; check_python; check_project
  [[ ! "$DOMAIN" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] && is_domain=true
  clear 2>/dev/null || true
  printf "${BOLD}${CYAN}╔══════════════════════════════════════════╗\n║       VPS Monitor · 中心面板部署          ║\n╚══════════════════════════════════════════╝${RESET}\n"
  print_preflight; detect_nginx_dirs; cd "$APP_DIR"
  _step "检查并安装系统依赖"; install_system_deps
  _step "配置 Python 虚拟环境"; setup_venv
  _step "写入环境配置";       write_env_file
  _step "配置 Nginx 反向代理"; write_nginx_config
  _step "配置 systemd 服务";  write_systemd_service
  _step "启动服务"
  if ! apply_and_start; then
    warn "服务启动遇到问题，配置已备份。"; warn "journalctl -u $SERVICE_NAME -n 50"; exit 1
  fi
  _step "申请 HTTPS 证书";    enable_https
  _step "验证 API 可用性";    wait_for_api || true
  print_result
}

main "$@"
