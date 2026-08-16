#!/data/data/com.termux/files/usr/bin/bash
# boot.sh — Termux:Boot entry point: wake-lock + supervised start.
# Install: cp scripts/boot.sh ~/.termux/boot/start-mcp.sh && chmod +x
set -euo pipefail

# Keep the device awake while the server runs.
termux-wake-lock

# Prefer termux-services supervision when installed.
if command -v sv >/dev/null 2>&1 && [ -f "$PREFIX/var/service/termux-mcp/run" ]; then
  sv up termux-mcp
  exit 0
fi

# Fallback: plain background process.
exec termux-mcp serve http --config "$PREFIX/var/lib/termux-mcp/config.yaml"
