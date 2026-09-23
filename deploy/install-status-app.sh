#!/bin/bash
# Install and start the TM Sonder macOS menu-bar status indicator.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PLIST_SRC="$ROOT_DIR/deploy/launchd/com.tm-sonder.status.plist"
PLIST_DST="$HOME/Library/LaunchAgents/com.tm-sonder.status.plist"
APP_EXECUTABLE="$ROOT_DIR/bin/Sonder Status.app/Contents/MacOS/SonderStatus"
LOG_FILE="$HOME/Library/Logs/tm-sonder-status.log"
LABEL="com.tm-sonder.status"
DOMAIN="gui/$(id -u)"

echo "==> Building menu-bar indicator"
make -C "$ROOT_DIR" status-app

echo "==> Installing LaunchAgent to $PLIST_DST"
mkdir -p "$(dirname "$PLIST_DST")" "$(dirname "$LOG_FILE")"
sed -e "s|__STATUS_APP_EXECUTABLE__|$APP_EXECUTABLE|g" \
    -e "s|__STATUS_LOG_FILE__|$LOG_FILE|g" \
    "$PLIST_SRC" > "$PLIST_DST"

echo "==> Starting menu-bar indicator"
launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
for attempt in $(seq 1 20); do
    if ! launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; then
        break
    fi
    sleep 0.25
done
launchctl bootstrap "$DOMAIN" "$PLIST_DST"
launchctl enable "$DOMAIN/$LABEL"
launchctl kickstart -k "$DOMAIN/$LABEL"

echo "OK: TM Sonder menu icon is running"
echo "Label: $LABEL  |  uninstall: deploy/uninstall-status-app.sh"
