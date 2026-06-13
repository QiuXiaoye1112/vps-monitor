#!/usr/bin/env bash
set -euo pipefail

AGENT_IP="${1:-}"; AGENT_PORT="${AGENT_PORT:-8080}"; APP_DIR="${APP_DIR:-/opt/vps-monitor}"
BOLD='\033[1m'; DIM='\033[2m'; RED='\033[31m'; GREEN='\033[32m'
YELLOW='\033[33m'; CYAN='\033[36m'; RESET='\033[0m'

ok()     { printf "  ${GREEN}✓${RESET} %s\n" "$*"; }
warn()   { printf "  ${YELLOW}⚠${RESET} %s\n" "$*" >&2; }
fail()   { printf "\n${RED}✗${RESET} %s\n" "$*" >&2; exit 1; }
info()   { printf "  ${CYAN}›${RESET} %s\n" "$*"; }
detail() { printf "    ${DIM}%s${RESET}\n" "$*"; }

[[ "${EUID:-$(id -u)}" -eq 0 ]] || fail "请以 root 运行：sudo bash allow_agent_ip.sh <IP>"
[[ -n "$AGENT_IP" ]] || { printf "${BOLD}用法：${RESET} sudo bash allow_agent_ip.sh ${CYAN}<远程VPS公网IP>${RESET}\n"; exit 1; }
[[ "$AGENT_IP" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ || "$AGENT_IP" =~ : ]] || fail "IP 格式不正确：$AGENT_IP"
[[ "$AGENT_PORT" =~ ^[0-9]+$ && "$AGENT_PORT" -ge 1 && "$AGENT_PORT" -le 65535 ]] || fail "端口无效：$AGENT_PORT"

clear 2>/dev/null || true
printf "${BOLD}${CYAN}╔══════════════════════════════════════════╗\n║    VPS Monitor · Agent IP 白名单          ║\n╚══════════════════════════════════════════╝${RESET}\n\n"

if command -v iptables >/dev/null 2>&1; then
  echo; info "当前 ${AGENT_PORT} 端口白名单："
  iptables -S INPUT 2>/dev/null | grep -- "--dport ${AGENT_PORT}" || echo "  ${DIM}（暂无规则）${RESET}"
  echo

  if iptables -C INPUT -p tcp -s "$AGENT_IP" --dport "$AGENT_PORT" -j ACCEPT 2>/dev/null; then
    ok "$AGENT_IP 已在白名单中"
  else
    iptables -I INPUT 1 -p tcp -s "$AGENT_IP" --dport "$AGENT_PORT" -j ACCEPT
    ok "已放行 $AGENT_IP → TCP ${AGENT_PORT}"
  fi

  if iptables -C INPUT -p tcp --dport "$AGENT_PORT" ! -i lo -j DROP 2>/dev/null; then
    ok "DROP 规则已存在（阻止其他 IP）"
  else
    printf "\n${BOLD}${YELLOW}╔══════════════════════════════════════════╗\n║  ⚠  即将阻止所有其他 IP 访问端口 %-5s ║\n╚══════════════════════════════════════════╝${RESET}\n\n" "$AGENT_PORT"
    printf "  当前已放行：${GREEN}%s${RESET}\n" "$AGENT_IP"
    printf "  即将阻止：  ${RED}所有其他 IP${RESET}\n"
    printf "  ${YELLOW}如有其他远程 VPS 未放行，它们将立即断连。${RESET}\n\n"
    read -r -p "  确认添加 DROP 规则？[y/N] " confirm
    if [[ "$confirm" =~ ^[Yy]$ ]]; then
      iptables -A INPUT -p tcp --dport "$AGENT_PORT" ! -i lo -j DROP
      ok "已阻止外部 IP 访问（本地回路不受限） TCP ${AGENT_PORT}"
    else
      info "已跳过 DROP。稍后手动添加：sudo iptables -A INPUT -p tcp --dport ${AGENT_PORT} ! -i lo -j DROP"
    fi
  fi

  echo; printf "${BOLD}端口 %s 当前规则：${RESET}\n" "$AGENT_PORT"
  iptables -S INPUT 2>/dev/null | grep -- "--dport ${AGENT_PORT}" | while read -r line; do
    [[ "$line" == *ACCEPT* ]] && printf "  ${GREEN}● ACCEPT${RESET} %s\n" "$line" || { [[ "$line" == *DROP* ]] && printf "  ${RED}● DROP${RESET}   %s\n" "$line" || printf "  ${DIM}●${RESET} %s\n" "$line"; }
  done
  echo; info "持久化防火墙规则..."
  if ! command -v netfilter-persistent >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then
      apt-get install -y -qq iptables-persistent 2>/dev/null || true
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y -q iptables-persistent 2>/dev/null || true
    fi
  fi
  if command -v netfilter-persistent >/dev/null 2>&1; then
    netfilter-persistent save >/dev/null 2>&1 && ok "规则已持久化，重启不丢失"
  else
    warn "无法持久化，重启后规则丢失。手动：sudo netfilter-persistent save"
  fi

elif command -v firewall-cmd >/dev/null 2>&1; then
  local rule="rule family='ipv4' source address='$AGENT_IP' port port='$AGENT_PORT' protocol='tcp' accept"
  if firewall-cmd --query-rich-rule="$rule" 2>/dev/null; then
    ok "$AGENT_IP 已有 firewalld 放行规则"
  else
    firewall-cmd --permanent --add-rich-rule="$rule"
    ok "已添加 firewalld 放行规则：$AGENT_IP → TCP ${AGENT_PORT}"
  fi
  firewall-cmd --reload; ok "firewalld 已重载"
else
  fail "未找到 iptables 或 firewalld。"
fi

echo; printf "${BOLD}验证连接：${RESET}\n  在远程 VPS 执行：${CYAN}curl http://<中心VPS公网IP>:${AGENT_PORT}/api/health${RESET}\n  应返回：${GREEN}{\"status\":\"ok\"}${RESET}\n"
echo; printf "  管理更多 IP：${CYAN}sudo vm${RESET} → 监控主机 → 防火墙操作\n\n"
