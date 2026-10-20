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

find "$DEST_DIR" -maxdepth 1 -name '*.partial' -type f -delete   # interrupted earlier run
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="$DEST_DIR/sonder-state-$STAMP.tar.gz"
TMP="$OUT.partial"

# Only top-level regular files are archived: accounts, sessions, progress,
# settings, the catalog snapshot, pairing token, subscriptions. Every bulk
# directory (artwork, caches, HLS/VOD output, subtitles, logs) is regenerable
# and can be many GB, so a directory walk would be slow and unbounded.
# The server config lives outside the data dir; include it when present.
mapfile -d '' FILES < <(cd "$DATA_DIR" && find . -maxdepth 1 -type f ! -name '*.partial' ! -name '*.tmp' ! -name '.*.tmp' -print0)
[[ ${#FILES[@]} -gt 0 ]] || { echo "no state files found in $DATA_DIR" >&2; exit 1; }
EXTRA=()
[[ -f "${SONDER_CONFIG:-/etc/sonder/server.json}" ]] && EXTRA=(-C / "${SONDER_CONFIG:-/etc/sonder/server.json}")
# tar exits 1 when a file changed while being read (the server rewrites its
# JSON atomically, so this is a harmless race); anything higher is a failure.
rc=0
tar -czf "$TMP" -C "$DATA_DIR" "${FILES[@]}" "${EXTRA[@]}" || rc=$?
[[ $rc -le 1 ]] || { rm -f "$TMP"; echo "tar failed ($rc)" >&2; exit "$rc"; }
tar -tzf "$TMP" >/dev/null   # refuse to keep an unreadable archive
mv "$TMP" "$OUT"

# Retention: delete everything but the newest $KEEP archives.
ls -1t "$DEST_DIR"/sonder-state-*.tar.gz 2>/dev/null | tail -n +"$((KEEP + 1))" | xargs -r rm -f --
echo "$OUT"
