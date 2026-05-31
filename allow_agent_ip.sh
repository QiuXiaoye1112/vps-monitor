#!/usr/bin/env bash
set -euo pipefail

AGENT_IP="${1:-}"
AGENT_PORT="${AGENT_PORT:-8080}"

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Please run as root: sudo bash allow_agent_ip.sh <agent-ip>"
  exit 1
fi

if [[ -z "$AGENT_IP" ]]; then
  echo "Usage: sudo bash allow_agent_ip.sh 1.2.3.4"
  exit 1
fi

if ! [[ "$AGENT_PORT" =~ ^[0-9]+$ ]] || [[ "$AGENT_PORT" -lt 1 || "$AGENT_PORT" -gt 65535 ]]; then
  echo "AGENT_PORT must be a TCP port between 1 and 65535."
  exit 1
fi

if ! command -v iptables >/dev/null 2>&1; then
  echo "iptables was not found. Install it first, then rerun this script."
  exit 1
fi

if ! iptables -C INPUT -p tcp -s "$AGENT_IP" --dport "$AGENT_PORT" -j ACCEPT 2>/dev/null; then
  iptables -I INPUT 1 -p tcp -s "$AGENT_IP" --dport "$AGENT_PORT" -j ACCEPT
fi

if ! iptables -C INPUT -p tcp --dport "$AGENT_PORT" -j DROP 2>/dev/null; then
  iptables -A INPUT -p tcp --dport "$AGENT_PORT" -j DROP
fi

echo "Allowed $AGENT_IP to access TCP $AGENT_PORT."
echo
echo "Current iptables rules for TCP $AGENT_PORT:"
iptables -S INPUT | grep -- "--dport $AGENT_PORT" || true
echo
echo "Note: these iptables rules may be lost after reboot."
echo "This script does not install iptables-persistent and does not enable ufw."
