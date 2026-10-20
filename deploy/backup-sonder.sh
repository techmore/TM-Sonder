#!/usr/bin/env bash
# Back up Sonder's irreplaceable state: accounts, sessions, progress, settings
# and the catalog snapshot. Media files and regenerable caches are excluded.
#
# Usage: backup-sonder.sh [DATA_DIR] [DEST_DIR]   (defaults: /var/lib/sonder, /var/backups/sonder)
# Keeps the newest $KEEP archives (default 14). Archives are mode 0600 because
# they contain password hashes and session tokens. Run from cron/systemd inside
# the container, or `incus exec sonder -- /path/backup-sonder.sh`.
set -euo pipefail

DATA_DIR="${1:-/var/lib/sonder}"
DEST_DIR="${2:-/var/backups/sonder}"
KEEP="${KEEP:-14}"

[[ -d "$DATA_DIR" ]] || { echo "data dir not found: $DATA_DIR" >&2; exit 1; }
umask 077
mkdir -p "$DEST_DIR"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="$DEST_DIR/sonder-state-$STAMP.tar.gz"
TMP="$OUT.partial"

# Exclusions are regenerable bulk: transcodes, HLS segments, thumbnails and
# metadata caches. Everything else under the data dir is state worth keeping.
tar -C "$DATA_DIR" -czf "$TMP" \
  --warning=no-file-changed \
  --exclude='./hls-runtime' --exclude='./transcode*' --exclude='./metadata-cache' \
  --exclude='./thumbs' --exclude='./thumbnails' --exclude='./media-cache' \
  --exclude='*.partial' --exclude='*.tmp' .
tar -tzf "$TMP" >/dev/null   # refuse to keep an unreadable archive
mv "$TMP" "$OUT"

# Retention: delete everything but the newest $KEEP archives.
ls -1t "$DEST_DIR"/sonder-state-*.tar.gz 2>/dev/null | tail -n +"$((KEEP + 1))" | xargs -r rm -f --
echo "$OUT"
