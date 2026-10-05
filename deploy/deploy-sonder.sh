#!/usr/bin/env bash
# Deploy the built binary into the running Sonder Incus container.
# The self-hosted runner lives on Ser8 and talks to Incus locally. User data and
# configuration stay in their existing container mounts; only the app binary is
# replaced. The previous binary is retained on the host and restored if restart
# or the in-container health/version check fails.
set -euo pipefail

REPO="${SONDER_REPO:-$HOME/TM-Sonder}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$REPO/bin"
STAGED="${SONDER_STAGED_BINARY:-$BIN_DIR/.sonder-linux-amd64.incoming}"
INSTANCE="${SONDER_INCUS_INSTANCE:-sonder}"
TARGET="${SONDER_CONTAINER_BINARY:-/usr/local/bin/sonder}"
CONFIG="${SONDER_CONTAINER_CONFIG:-/etc/sonder/server.json}"
SERVICE="${SONDER_CONTAINER_SERVICE:-sonder}"
SERVICE_USER="${SONDER_CONTAINER_USER:-ubuntu}"
DATA_DIR="${SONDER_CONTAINER_DATA_DIR:-/var/lib/sonder}"
EXPECTED_VERSION="${1:-}"
HEALTH_ATTEMPTS="${SONDER_HEALTH_ATTEMPTS:-20}"
HEALTH_INTERVAL="${SONDER_HEALTH_INTERVAL:-3}"

log() { printf '%s deploy: %s\n' "$(date -u +%H:%M:%S)" "$*"; }
die() { printf '%s deploy: FAILED: %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; exit 1; }

[[ -f "$STAGED" ]] || die "no staged binary at $STAGED"
[[ -x "$STAGED" ]] || chmod +x "$STAGED"

# Refuse the wrong architecture before touching the live container.
elf_machine() {
  local machine
  machine=$(od -An -tx1 -j18 -N2 "$STAGED" 2>/dev/null | tr -d ' \n')
  case "$machine" in
    3e00) echo x86-64 ;;
    b700) echo aarch64 ;;
    *) echo "unknown($machine)" ;;
  esac
}
detected="$(elf_machine)"
[[ "$detected" == x86-64 ]] || die "artifact is $detected; this instance requires x86_64"

incus info "$INSTANCE" >/dev/null 2>&1 || die "Incus instance '$INSTANCE' is unavailable"
incus exec "$INSTANCE" -- test -x "$TARGET" || die "no existing binary at $INSTANCE:$TARGET to replace"
incus exec "$INSTANCE" -- getent passwd "$SERVICE_USER" >/dev/null || die "service user '$SERVICE_USER' is missing in $INSTANCE"
incus exec "$INSTANCE" -- runuser -u "$SERVICE_USER" -- test -w "$DATA_DIR" || die "service user '$SERVICE_USER' cannot write $DATA_DIR; refusing restart"
if incus exec "$INSTANCE" -- test -e "$DATA_DIR/runtime-state.json"; then
  incus exec "$INSTANCE" -- runuser -u "$SERVICE_USER" -- test -w "$DATA_DIR/runtime-state.json" || die "service user '$SERVICE_USER' cannot write $DATA_DIR/runtime-state.json; refusing restart"
fi

# Refresh the OAuth client from the existing Tasks/Homeboard profile. The
# helper transfers secrets directly between Incus containers without logging.
"$SCRIPT_DIR/configure-google-oauth.sh" || die "could not configure Sonder Google sign-in"

install -d -m 0755 "$BIN_DIR"
BACKUP="$BIN_DIR/sonder-linux-amd64.incus-bak-$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -e "$BACKUP" ]]; then
  n=2
  while [[ -e "$BACKUP.$n" ]]; do n=$((n + 1)); done
  BACKUP="$BACKUP.$n"
fi
incus file pull "$INSTANCE$TARGET" "$BACKUP" || die "could not back up the current container binary"
chmod 0755 "$BACKUP"
log "backed up current container binary to ${BACKUP##*/}"

remote_stage="/tmp/sonder-linux-amd64.incoming.$$"
remote_backup="/tmp/sonder-linux-amd64.rollback.$$"
SERVED_VERSION=""
wait_for_healthy() {
  local expected="${1:-}" status="" served="" healthy="" active=""
  for ((attempt = 1; attempt <= HEALTH_ATTEMPTS; attempt++)); do
    status="$(incus exec "$INSTANCE" -- "$TARGET" app status --json --config "$CONFIG" 2>/dev/null || true)"
    if [[ -n "$status" ]]; then
      served="$(printf '%s' "$status" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("version", ""))' 2>/dev/null || true)"
      healthy="$(printf '%s' "$status" | python3 -c 'import json,sys; d=json.load(sys.stdin); s=d.get("status",{}); print(str(bool(s.get("webHealthy")) and bool(s.get("apiHealthy"))).lower())' 2>/dev/null || true)"
      active="$(incus exec "$INSTANCE" -- systemctl is-active "$SERVICE" 2>/dev/null || true)"
      if [[ -n "$served" && "$healthy" == true && "$active" == active && ( -z "$expected" || "$served" == "$expected" ) ]]; then
        SERVED_VERSION="$served"
        return 0
      fi
    fi
    sleep "$HEALTH_INTERVAL"
  done
  return 1
}
rollback() {
  log "restoring prior binary in Incus instance '$INSTANCE'"
  incus file push --mode 0644 --uid 0 --gid 0 "$BACKUP" "$INSTANCE$remote_backup" || return 1
  incus exec "$INSTANCE" -- install -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0755 "$remote_backup" "$TARGET.rollback" || return 1
  incus exec "$INSTANCE" -- mv -f "$TARGET.rollback" "$TARGET" || return 1
  incus restart "$INSTANCE" --timeout 30 || return 1
  wait_for_healthy "" || return 1
  incus exec "$INSTANCE" -- rm -f "$remote_backup" || true
}
fail_with_rollback() {
  local reason="$1"
  if rollback; then die "$reason; rolled back to ${BACKUP##*/}"; fi
  die "$reason; rollback failed, recovery binary is $BACKUP"
}

incus file push --create-dirs --mode 0644 --uid 0 --gid 0 "$STAGED" "$INSTANCE$remote_stage" || die "could not stage binary in $INSTANCE"
incus exec "$INSTANCE" -- install -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0755 "$remote_stage" "$TARGET.incoming" || die "could not install staged binary"
incus exec "$INSTANCE" -- mv -f "$TARGET.incoming" "$TARGET" || fail_with_rollback "could not atomically replace binary"
rm -f "$STAGED"
log "installed new binary in $INSTANCE:$TARGET ($(wc -c < "$BACKUP" | tr -d ' ') bytes in previous binary)"

if ! incus restart "$INSTANCE" --timeout 30; then
  fail_with_rollback "service restart failed"
fi

if ! wait_for_healthy "$EXPECTED_VERSION"; then
  fail_with_rollback "container app health check failed or served an unexpected version"
fi
log "healthy: $INSTANCE is serving $SERVED_VERSION (web and API healthy)"
log "done (previous binary kept at $BACKUP)"
