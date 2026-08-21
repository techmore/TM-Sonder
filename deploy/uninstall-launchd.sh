#!/bin/bash
# Uninstall the TM Sonder launchd service.
set -euo pipefail

PLIST_DST="$HOME/Library/LaunchAgents/com.tm-sonder.server.plist"
PREFIX="${PREFIX:-/usr/local}"

echo "==> Stopping and removing LaunchAgent"
launchctl bootout "gui/$(id -u)/com.tm-sonder.server" 2>/dev/null || true
rm -f "$PLIST_DST"

echo "==> Removing binary"
sudo rm -f "$PREFIX/bin/sonder"

echo "==> Verifying shutdown"
sleep 1
if curl -fsS --max-time 2 "http://127.0.0.1:${SONDER_PORT:-8797}/api/health" >/dev/null 2>&1; then
    echo "WARNING: something still answers on the port (another instance?)" >&2
else
    echo "OK: service removed and port closed"
fi
