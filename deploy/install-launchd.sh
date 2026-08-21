#!/bin/bash
# Install the TM Sonder Go server as a launchd service (macOS, arm64).
set -euo pipefail

PREFIX="${PREFIX:-/usr/local}"
PLIST_SRC="$(cd "$(dirname "$0")/launchd" && pwd)/com.tm-sonder.server.plist"
PLIST_DST="$HOME/Library/LaunchAgents/com.tm-sonder.server.plist"
CONFIG_DIR="${SONDER_CONFIG_DIR:-$HOME/.config/sonder}"
LOG_FILE="$HOME/Library/Logs/sonder.log"

echo "==> Building sonder (darwin/arm64)"
cd "$(cd "$(dirname "$0")/.." && pwd)/server"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o sonder ./cmd/sonder

echo "==> Installing binary to $PREFIX/bin"
sudo mkdir -p "$PREFIX/bin"
sudo install -m 0755 sonder "$PREFIX/bin/sonder"
rm -f sonder

echo "==> Preparing config at $CONFIG_DIR/server.json"
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_DIR/server.json" ]; then
    SONDER_CONFIG="$CONFIG_DIR/server.json" "$PREFIX/bin/sonder" -config "$CONFIG_DIR/server.json" &
    sleep 1
    pkill -f "$PREFIX/bin/sonder -config $CONFIG_DIR/server.json" 2>/dev/null || true
fi
[ -f "$CONFIG_DIR/server.json" ] || { echo "config missing after generation"; exit 1; }

echo "==> Writing LaunchAgent to $PLIST_DST"
mkdir -p "$(dirname "$PLIST_DST")" "$(dirname "$LOG_FILE")"
sed -e "s|/etc/sonder/server.json|$CONFIG_DIR/server.json|" \
    -e "s|/var/log/sonder.log|$LOG_FILE|g" \
    "$PLIST_SRC" > "$PLIST_DST"

echo "==> Bootstrapping service"
launchctl bootout "gui/$(id -u)/com.tm-sonder.server" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_DST"
launchctl enable "gui/$(id -u)/com.tm-sonder.server"
launchctl kickstart -k "gui/$(id -u)/com.tm-sonder.server"

echo "==> Health check"
PORT=$(python3 -c "import json;print(json.load(open('$CONFIG_DIR/server.json')).get('port',8797))")
for i in $(seq 1 20); do
    if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then
        echo "OK: sonder healthy on port $PORT"
        echo "Label: com.tm-sonder.server  |  uninstall: deploy/uninstall-launchd.sh"
        exit 0
    fi
    sleep 0.5
done
echo "FAILED: service did not become healthy; see $LOG_FILE" >&2
exit 1
