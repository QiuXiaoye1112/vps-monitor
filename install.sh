#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${VPS_MONITOR_REPO:-https://github.com/QiuXiaoye1112/vps-monitor.git}"
INSTALL_DIR="${VPS_MONITOR_DIR:-/opt/vps-monitor}"
BRANCH="${VPS_MONITOR_BRANCH:-}"
SETUP_ROLE="${VPS_MONITOR_SETUP_ROLE:-}"
ROLE_FILE="/etc/vps-monitor-role"

info() {
  printf '\033[36m[VPS Monitor]\033[0m %s\n' "$*"
}

fail() {
  printf '\033[31m[错误]\033[0m %s\n' "$*" >&2
  exit 1
}

rerun_as_root() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    return
  fi
  command -v sudo >/dev/null 2>&1 || fail "需要 root 权限，但系统没有 sudo。请先切换到 root 后重试。"

  local temp_script
  temp_script="$(mktemp)"
  cat "${BASH_SOURCE[0]}" > "$temp_script"
  chmod 700 "$temp_script"
  info "需要 root 权限，正在请求 sudo..."
  sudo env \
    VPS_MONITOR_REPO="$REPO_URL" \
    VPS_MONITOR_DIR="$INSTALL_DIR" \
    VPS_MONITOR_BRANCH="$BRANCH" \
    VPS_MONITOR_SETUP_ROLE="$SETUP_ROLE" \
    bash "$temp_script"
  rm -f "$temp_script"
  exit 0
}

choose_role() {
  if [[ -f "$ROLE_FILE" ]]; then
    local saved_role
    saved_role="$(tr -d '[:space:]' < "$ROLE_FILE")"
    if [[ "$saved_role" == "center" || "$saved_role" == "agent" ]]; then
      SETUP_ROLE=""
      return
    fi
  fi
  if [[ -f /etc/vps-monitor.env ]]; then
    printf 'center\n' > "$ROLE_FILE"
    chmod 600 "$ROLE_FILE"
    SETUP_ROLE=""
    return
  fi
  if [[ -f /etc/vps-monitor-agent.toml ]]; then
    printf 'agent\n' > "$ROLE_FILE"
    chmod 600 "$ROLE_FILE"
    SETUP_ROLE=""
    return
  fi
  if [[ "$SETUP_ROLE" == "center" || "$SETUP_ROLE" == "agent" ]]; then
    return
  fi

  printf '\n请选择这台 VPS 的用途：\n\n'
  printf '  1. 中心 VPS（安装监控面板）\n'
  printf '  2. 远程 VPS（接入已有中心 VPS）\n'
  printf '  0. 退出\n\n'
  while true; do
    read -r -p "请选择 [1/2/0]: " choice
    case "$choice" in
      1) SETUP_ROLE="center"; printf 'center\n' > "$ROLE_FILE"; chmod 600 "$ROLE_FILE"; return ;;
      2) SETUP_ROLE="agent"; printf 'agent\n' > "$ROLE_FILE"; chmod 600 "$ROLE_FILE"; return ;;
      0) exit 0 ;;
      *) printf '请输入 1、2 或 0。\n' ;;
    esac
  done
}

install_dependencies() {
  if command -v git >/dev/null 2>&1 && command -v python3 >/dev/null 2>&1; then
    return
  fi
  if command -v apt-get >/dev/null 2>&1; then
    info "正在安装基础依赖..."
    apt-get update
    apt-get install -y git python3 python3-venv python3-pip ca-certificates curl
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y git python3 python3-pip ca-certificates curl
  elif command -v yum >/dev/null 2>&1; then
    yum install -y git python3 python3-pip ca-certificates curl
  else
    fail "暂不支持当前包管理器，请先安装 git 和 python3。"
  fi
}

install_or_update() {
  if [[ -d "$INSTALL_DIR/.git" ]]; then
    if [[ -n "$(git -C "$INSTALL_DIR" status --porcelain)" ]]; then
      info "检测到 $INSTALL_DIR 存在本地改动，为避免覆盖，跳过自动更新。"
      return
    fi
    info "正在更新已有安装..."
    git -C "$INSTALL_DIR" fetch --all --prune
    git -C "$INSTALL_DIR" pull --ff-only
    return
  fi

  if [[ -e "$INSTALL_DIR" ]]; then
    fail "$INSTALL_DIR 已存在但不是 Git 仓库。请备份或移走该目录后重试。"
  fi

  info "正在下载 VPS Monitor..."
  mkdir -p "$(dirname "$INSTALL_DIR")"
  if [[ -n "$BRANCH" ]]; then
    git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$INSTALL_DIR"
  else
    git clone --depth 1 "$REPO_URL" "$INSTALL_DIR"
  fi
}

install_entrypoint() {
  cat > /usr/local/bin/vps-monitor <<EOF
#!/usr/bin/env bash
exec python3 "$INSTALL_DIR/manager.py" "\$@"
EOF
  chmod 755 /usr/local/bin/vps-monitor
}

main() {
  rerun_as_root
  choose_role
  install_dependencies
  install_or_update
  [[ -f "$INSTALL_DIR/manager.py" ]] || fail "安装包缺少 manager.py，请检查仓库地址或分支。"
  install_entrypoint
  info "安装完成，正在打开终端管理面板..."
  if [[ -n "$SETUP_ROLE" ]]; then
    exec python3 "$INSTALL_DIR/manager.py" --setup "$SETUP_ROLE"
  fi
  exec python3 "$INSTALL_DIR/manager.py"
}

main "$@"
