#!/usr/bin/env bash
# Install (idempotently) the daily state-backup timer inside the Sonder Incus
# container and copy the newest archive out to the host, so a lost container
# does not take its only backup with it. Run by the Deploy workflow.
set -euo pipefail

INSTANCE="${SONDER_INCUS_INSTANCE:-sonder}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOST_DEST="${SONDER_HOST_BACKUP_DIR:-$HOME/sonder-backups}"
HOST_KEEP="${SONDER_HOST_BACKUP_KEEP:-14}"

incus file push --mode 0755 --uid 0 --gid 0 "$SCRIPT_DIR/backup-sonder.sh" "$INSTANCE/usr/local/bin/backup-sonder"
incus exec "$INSTANCE" -- sh -c 'cat > /etc/systemd/system/sonder-backup.service <<UNIT
[Unit]
Description=Back up Sonder state
[Service]
Type=oneshot
ExecStart=/usr/local/bin/backup-sonder /var/lib/sonder /var/backups/sonder
UNIT
cat > /etc/systemd/system/sonder-backup.timer <<UNIT
[Unit]
Description=Daily Sonder state backup
[Timer]
OnCalendar=*-*-* 03:30:00
RandomizedDelaySec=900
Persistent=true
[Install]
WantedBy=timers.target
UNIT
systemctl daemon-reload
systemctl enable --now sonder-backup.timer'

# Run the real systemd unit now (not the script directly) so the exact thing
# the timer fires is proven to work on every deploy. start blocks for oneshot
# units and fails if the unit fails.
incus exec "$INSTANCE" -- systemctl start sonder-backup.service
latest="$(incus exec "$INSTANCE" -- sh -c 'ls -1t /var/backups/sonder/sonder-state-*.tar.gz | head -n 1')"
[[ -n "$latest" ]] || { echo "backup unit produced no archive" >&2; exit 1; }
incus exec "$INSTANCE" -- tar -tzf "$latest" >/dev/null
install -d -m 0700 "$HOST_DEST"
( umask 077; incus file pull "$INSTANCE$latest" "$HOST_DEST/" )
ls -1t "$HOST_DEST"/sonder-state-*.tar.gz | tail -n +"$((HOST_KEEP + 1))" | xargs -r rm -f --
echo "backup installed; latest copy: $HOST_DEST/$(basename "$latest")"
