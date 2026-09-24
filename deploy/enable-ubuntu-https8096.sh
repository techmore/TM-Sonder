#!/usr/bin/env bash
# Move an Ubuntu host from direct HTTP on :8096 to HTTPS on :8096 through the
# system Caddy instance, while keeping Sonder's API on loopback :8097.
#
# This is intentionally an explicit host migration script rather than part of
# the normal installer: it changes a root-owned Caddyfile and assumes that the
# host's DNS and router already point at the configured address.
set -Eeuo pipefail

APP_USER="${SONDER_APP_USER:-${SUDO_USER:-sdolbec}}"
APP_HOME="${SONDER_APP_HOME:-$(getent passwd "$APP_USER" | cut -d: -f6)}"
DOMAIN="${SONDER_PUBLIC_DOMAIN:-stoverparc.org}"
BIND_ADDRESS="${SONDER_PUBLIC_BIND:-192.168.3.251}"
PUBLIC_PORT="${SONDER_PUBLIC_PORT:-8096}"
INTERNAL_PORT="${SONDER_INTERNAL_PORT:-8098}"
API_PORT="${SONDER_API_PORT:-8097}"
CONFIG_DIR="$APP_HOME/.config/sonder"
CONFIG="$CONFIG_DIR/server.json"
DATA_DIR="$CONFIG_DIR/data"
RUNTIME="$DATA_DIR/runtime-state.json"
CADDYFILE="${SONDER_CADDYFILE:-/etc/caddy/Caddyfile}"
HEALTH_URL="${SONDER_PUBLIC_HEALTH_URL:-https://${DOMAIN}:${PUBLIC_PORT}/account/login}"

if [[ "$(id -u)" -ne 0 ]]; then
  exec sudo --preserve-env=SONDER_APP_USER,SONDER_APP_HOME,SONDER_PUBLIC_DOMAIN,SONDER_PUBLIC_BIND,SONDER_PUBLIC_PORT,SONDER_INTERNAL_PORT,SONDER_API_PORT,SONDER_CADDYFILE,SONDER_PUBLIC_HEALTH_URL "$0" "$@"
fi

APP_UID="$(id -u "$APP_USER")"
APP_GID="$(id -g "$APP_USER")"
APP_GROUP="$(id -gn "$APP_USER")"

for command in awk caddy curl getent install jq mktemp runuser ss systemctl; do
  command -v "$command" >/dev/null || {
    echo "required command is missing: $command" >&2
    exit 1
  }
done

[[ -f "$CONFIG" ]] || { echo "Sonder config not found: $CONFIG" >&2; exit 1; }
[[ -f "$RUNTIME" ]] || { echo "Sonder runtime state not found: $RUNTIME" >&2; exit 1; }
[[ -f "$CADDYFILE" ]] || { echo "Caddyfile not found: $CADDYFILE" >&2; exit 1; }

userctl() {
  runuser -u "$APP_USER" -- env \
    XDG_RUNTIME_DIR="/run/user/$APP_UID" \
    DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$APP_UID/bus" \
    systemctl --user "$@"
}
if ! userctl is-active tm-sonder.service >/dev/null 2>&1; then
  echo "could not reach the $APP_USER user systemd session" >&2
  exit 1
fi

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP_DIR="$DATA_DIR/migration-backups/https8096-$STAMP"
CADDY_BACKUP="${CADDYFILE}.tm-sonder-backup-${STAMP}"
mkdir -p "$BACKUP_DIR"
chown "$APP_UID:$APP_GID" "$BACKUP_DIR"
install -o "$APP_USER" -g "$APP_GROUP" -m 0600 "$CONFIG" "$BACKUP_DIR/server.json"
install -o "$APP_USER" -g "$APP_GROUP" -m 0600 "$RUNTIME" "$BACKUP_DIR/runtime-state.json"
install -o root -g root -m 0644 "$CADDYFILE" "$CADDY_BACKUP"

server_tmp="$(mktemp "$CONFIG_DIR/.server.json.XXXXXX")"
runtime_tmp="$(mktemp "$DATA_DIR/.runtime-state.json.XXXXXX")"
caddy_tmp="$(mktemp /tmp/tm-sonder-caddy.XXXXXX)"
cleanup() {
  rm -f "$server_tmp" "$runtime_tmp" "$caddy_tmp"
}
trap cleanup EXIT

jq --argjson webPort "$INTERNAL_PORT" --argjson apiPort "$API_PORT" \
  '.port = $webPort | .webPort = $webPort | .apiPort = $apiPort' \
  "$CONFIG" > "$server_tmp"
chown "$APP_UID:$APP_GID" "$server_tmp"
chmod 0600 "$server_tmp"

jq --arg mode "loopback" \
  --arg interfaceID "lo" \
  --arg ipv4 "127.0.0.1" \
  --arg domain "$DOMAIN" \
  --arg health "$HEALTH_URL" \
  --arg bind "$BIND_ADDRESS" \
  --arg upstream "127.0.0.1:${INTERNAL_PORT}" \
  --argjson webPort "$INTERNAL_PORT" \
  --argjson apiPort "$API_PORT" \
  '.selectedMode = $mode |
   .selectedInterface = $interfaceID |
   .selectedIPv4 = $ipv4 |
   .webBindAddress = $ipv4 |
   .apiBindAddress = $ipv4 |
   .webPort = $webPort |
   .apiPort = $apiPort |
   .publicDomain = $domain |
   .publicHealthCheckURL = $health |
   .caddyEnabled = false |
   .caddyBindAddress = $bind |
   .caddyUpstream = $upstream |
   del(.lastError)' \
  "$RUNTIME" > "$runtime_tmp"
chown "$APP_UID:$APP_GID" "$runtime_tmp"
chmod 0600 "$runtime_tmp"

# Remove a previous migration block, then append the exact managed HTTPS site.
# Everything else in the existing Caddyfile, including the main Swartzit site,
# is preserved byte-for-byte apart from the removed prior marker block.
awk '
  /# BEGIN TM-Sonder HTTPS 8096$/ { skip=1; next }
  /# END TM-Sonder HTTPS 8096$/ { skip=0; next }
  !skip { print }
' "$CADDYFILE" > "$caddy_tmp"
cat >> "$caddy_tmp" <<EOF

# BEGIN TM-Sonder HTTPS 8096
https://${DOMAIN}:${PUBLIC_PORT} {
    bind ${BIND_ADDRESS}
    encode zstd gzip
    reverse_proxy 127.0.0.1:${INTERNAL_PORT}
}
# END TM-Sonder HTTPS 8096
EOF

echo "Validating Caddy candidate..."
caddy validate --config "$caddy_tmp" --adapter caddyfile

app_changed=0
caddy_changed=0
rollback() {
  set +e
  echo "Migration failed; restoring the previous Sonder and Caddy state..." >&2
  userctl stop tm-sonder.service >/dev/null 2>&1
  if [[ "$app_changed" -eq 1 ]]; then
    install -o "$APP_USER" -g "$APP_GROUP" -m 0600 "$BACKUP_DIR/server.json" "$CONFIG"
    install -o "$APP_USER" -g "$APP_GROUP" -m 0600 "$BACKUP_DIR/runtime-state.json" "$RUNTIME"
  fi
  if [[ "$caddy_changed" -eq 1 ]]; then
    install -o root -g root -m 0644 "$CADDY_BACKUP" "$CADDYFILE"
    systemctl restart caddy >/dev/null 2>&1
  fi
  userctl start tm-sonder.service >/dev/null 2>&1
  cleanup
}
trap 'rc=$?; if [[ "$rc" -ne 0 ]]; then rollback; fi; exit "$rc"' EXIT

app_changed=1
install -o "$APP_USER" -g "$APP_GROUP" -m 0600 "$server_tmp" "$CONFIG"
install -o "$APP_USER" -g "$APP_GROUP" -m 0600 "$runtime_tmp" "$RUNTIME"

echo "Stopping Sonder before claiming external port ${PUBLIC_PORT}..."
userctl stop tm-sonder.service

caddy_changed=1
install -o root -g root -m 0644 "$caddy_tmp" "$CADDYFILE"
echo "Restarting Caddy with HTTPS on ${BIND_ADDRESS}:${PUBLIC_PORT}..."
systemctl restart caddy
systemctl is-active --quiet caddy.service

echo "Starting Sonder on 127.0.0.1:${INTERNAL_PORT}..."
userctl start tm-sonder.service

ready=0
for _ in $(seq 1 30); do
  if curl --noproxy '*' -fsS --max-time 2 "http://127.0.0.1:${INTERNAL_PORT}/api/health" >/dev/null && \
     curl --noproxy '*' -fsS --max-time 2 "http://127.0.0.1:${API_PORT}/api/health" >/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" -ne 1 ]]; then
  echo "Sonder did not make both local listeners healthy" >&2
  exit 1
fi

if ! curl --noproxy '*' -fsS --max-time 10 \
  --resolve "${DOMAIN}:${PUBLIC_PORT}:${BIND_ADDRESS}" \
  "$HEALTH_URL" >/dev/null; then
  echo "HTTPS health check failed: $HEALTH_URL" >&2
  exit 1
fi

if ! ss -ltn | awk -v wanted=":${PUBLIC_PORT}" '$0 ~ wanted { found=1 } END { exit(found ? 0 : 1) }'; then
  echo "Caddy is not listening on ${BIND_ADDRESS}:${PUBLIC_PORT}" >&2
  exit 1
fi

trap - EXIT
cleanup
echo "HTTPS migration complete."
echo "  public:  https://${DOMAIN}:${PUBLIC_PORT}"
echo "  web:     127.0.0.1:${INTERNAL_PORT}"
echo "  api:     127.0.0.1:${API_PORT}"
echo "  backup:  $BACKUP_DIR"
echo "  caddy backup: $CADDY_BACKUP"
