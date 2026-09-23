#!/bin/bash
# Remove the TM Sonder macOS menu-bar status indicator LaunchAgent.
set -euo pipefail

PLIST_DST="$HOME/Library/LaunchAgents/com.tm-sonder.status.plist"
LABEL="com.tm-sonder.status"
DOMAIN="gui/$(id -u)"

launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
rm -f "$PLIST_DST"
echo "OK: removed $LABEL"
