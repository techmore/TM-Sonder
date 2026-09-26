#!/usr/bin/env bash
# Deploy a built sonder binary to this host, safely.
#
# Runs ON the server, executed by the self-hosted GitHub Actions runner, so it
# needs no inbound network access: Actions cannot reach this box over SSH (it
# sits behind a NAT with only 80/443/8096 forwarded), which is exactly why the
# runner is self-hosted.
#
# The sequence is deliberately paranoid, because a bad binary on this host takes
# the library UI down for everyone:
#
#   1. refuse to deploy unless the artifact looks like a Linux x86-64 binary
#   2. keep the running binary until the new one has started successfully
#   3. health-check the version actually being served, not just that the process
#      is up -- a process can listen and still be serving the old build
#   4. roll back automatically on any failure, and say so in the exit code
set -euo pipefail

REPO="${SONDER_REPO:-$HOME/TM-Sonder}"
BIN_DIR="$REPO/bin"
TARGET="$BIN_DIR/sonder-linux-amd64"
STAGED="$BIN_DIR/.sonder-linux-amd64.incoming"
CONFIG="${SONDER_CONFIG:-$HOME/.config/sonder/server.json}"
API_PORT="${SONDER_API_PORT:-8097}"
EXPECTED_VERSION="${1:-}"
HEALTH_ATTEMPTS="${SONDER_HEALTH_ATTEMPTS:-20}"
HEALTH_INTERVAL="${SONDER_HEALTH_INTERVAL:-3}"

log() { printf '%s deploy: %s\n' "$(date -u +%H:%M:%S)" "$*"; }
die() { printf '%s deploy: FAILED: %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; exit 1; }

export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

# --- 1. the artifact must be what we think it is --------------------------
[[ -f "$STAGED" ]] || die "no staged binary at $STAGED"
[[ -x "$STAGED" ]] || chmod +x "$STAGED"

# `file` may be absent; fall back to reading the ELF header directly, because
# deploying an arm64 binary onto an x86_64 host fails at exec time with a
# message that looks like a permissions problem.
elf_machine() {
  local machine
  machine=$(od -An -tx1 -j18 -N2 "$STAGED" 2>/dev/null | tr -d ' \n')
  case "$machine" in
    3e00) echo x86-64 ;;   # EM_X86_64
    b700) echo aarch64 ;;  # EM_AARCH64
    *)    echo "unknown($machine)" ;;
  esac
}
detected="$(elf_machine)"
[[ "$detected" == "x86-64" ]] || die "artifact is $detected; this host is x86_64"

# --- backup the binary currently in service --------------------------------
[[ -f "$TARGET" ]] || die "no existing binary at $TARGET to replace"
# Never clobber an existing backup. A second-resolution timestamp collides when
# two deploys land in the same second, and `cp` would then quietly overwrite the
# only way back to the previous revision. A counter suffix costs nothing.
BACKUP="$TARGET.bak-$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -e "$BACKUP" ]]; then
  n=2
  while [[ -e "$BACKUP.$n" ]]; do n=$((n + 1)); done
  BACKUP="$BACKUP.$n"
fi
cp -p "$TARGET" "$BACKUP"
log "backed up current binary to ${BACKUP##*/}"

# --- swap and restart ------------------------------------------------------
mv -f "$STAGED" "$TARGET"
# `wc -c` rather than `stat -c %s`: the -c form is GNU-only, and a script that
# only runs correctly on the deploy target is a script nobody can test anywhere
# else. This runs the same way on a Mac as on the server.
log "installed new binary ($(wc -c < "$TARGET" | tr -d ' ') bytes)"

if ! systemctl --user restart tm-sonder; then
  log "restart failed; rolling back"
  cp -p "$BACKUP" "$TARGET"
  systemctl --user restart tm-sonder || true
  die "rolled back to ${BACKUP##*/}"
fi

# --- 3. health-check what is actually being served -------------------------
token="$(python3 -c "import json,os,sys;print(json.load(open(os.path.expanduser(sys.argv[1]))).get('pairingToken',''))" "$CONFIG" 2>/dev/null || true)"
served=""
for ((i = 1; i <= HEALTH_ATTEMPTS; i++)); do
  body="$(curl -sS --max-time 10 "http://127.0.0.1:${API_PORT}/api/status?token=${token}" 2>/dev/null || true)"
  if [[ -n "$body" ]]; then
    served="$(printf '%s' "$body" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("version",""))' 2>/dev/null || true)"
    [[ -n "$served" ]] && break
  fi
  sleep "$HEALTH_INTERVAL"
done

if [[ -z "$served" ]]; then
  log "no healthy response from :${API_PORT}; rolling back"
  systemctl --user stop tm-sonder || true
  cp -p "$BACKUP" "$TARGET"
  systemctl --user restart tm-sonder || true
  die "rolled back to ${BACKUP##*/}"
fi

# A process can be up and still serving a stale build, so compare what it
# reports against what we were asked to install.
if [[ -n "$EXPECTED_VERSION" && "$served" != "$EXPECTED_VERSION" ]]; then
  log "serving version '$served' but expected '$EXPECTED_VERSION'; rolling back"
  cp -p "$BACKUP" "$TARGET"
  systemctl --user restart tm-sonder || true
  die "version mismatch after restart"
fi

log "healthy: serving $served"
systemctl --user is-active tm-sonder >/dev/null && log "tm-sonder is active"
log "done (previous binary kept at ${BACKUP##*/})"
