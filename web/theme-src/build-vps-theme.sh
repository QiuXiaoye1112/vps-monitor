#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
SOURCE_DIR="$SCRIPT_DIR/vps-theme"
PUBLIC_DIR="$ROOT_DIR/web/public/vpsTheme"

cd "$SOURCE_DIR"
bun install --frozen-lockfile
bun run type-check
bun run lint
VPS_EMBED_BUILD=1 bun run build-only

rsync -a --delete "$SOURCE_DIR/dist/" "$PUBLIC_DIR/dist/"
install -m 0644 "$SCRIPT_DIR/admin/admin.html" "$PUBLIC_DIR/dist/admin.html"
install -m 0644 "$SOURCE_DIR/komari-theme.json" "$PUBLIC_DIR/monitor-theme.json"

printf 'VPS theme rebuilt from source: %s\n' "$SOURCE_DIR"
