#!/usr/bin/env bash
set -euo pipefail

# Use the GitHub Contents API so branch updates are not hidden by Raw CDN caching.
API_URL="https://api.github.com/repos/QiuXiaoye1112/vps-monitor/contents/install.sh?ref=master"
TEMP_SCRIPT="$(mktemp)"
trap 'rm -f "$TEMP_SCRIPT"' EXIT

curl -fsSL \
  -H "Accept: application/vnd.github.raw+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "$API_URL" > "$TEMP_SCRIPT"

exec bash "$TEMP_SCRIPT"
