#!/usr/bin/env bash
# Run on an Incus host. The source must be an existing mounted NAS folder.
set -euo pipefail
instance="${SONDER_INCUS_INSTANCE:-sonder}"
source_dir="${1:?Usage: sudo ./install-youtube.sh /mounted/NAS/ytdl}"
[[ "$source_dir" = /* && -d "$source_dir" ]] || { echo 'Provide an existing absolute NAS folder'; exit 1; }
findmnt -T "$source_dir" -t nfs,nfs4,cifs >/dev/null || { echo 'Download folder must be on a mounted NAS filesystem'; exit 1; }
incus exec "$instance" -- test -x /usr/bin/ffmpeg
incus exec "$instance" -- test -x /usr/bin/ffprobe
stage=$(mktemp -d /var/tmp/sonder-youtube.XXXXXX)
trap 'rm -rf "$stage"' EXIT
curl -fsSL --retry 3 https://github.com/yt-dlp/yt-dlp/releases/download/2026.08.19/yt-dlp_linux -o "$stage/yt-dlp"
printf '%s  %s\n' 58162f9bfdc27458ea47bfcb311cf47028f17d8154a8bf7d689861d46399230a "$stage/yt-dlp" | sha256sum -c -
curl -fsSL --retry 3 https://github.com/denoland/deno/releases/download/v2.9.7/deno-x86_64-unknown-linux-gnu.zip -o "$stage/deno.zip"
printf '%s  %s\n' c6527f24f4b16031d3ae4fa9f658d5f11534c8d84ce7dc8502420280919c3490 "$stage/deno.zip" | sha256sum -c -
python3 - "$stage" <<'PY'
import sys,zipfile
from pathlib import Path
p=Path(sys.argv[1])
with zipfile.ZipFile(p/'deno.zip') as z:
    (p/'deno').write_bytes(z.read('deno'))
PY
for name in yt-dlp deno; do
 incus file push --quiet "$stage/$name" "$instance/usr/local/bin/$name" --mode=0755
 incus exec "$instance" -- chown root:root "/usr/local/bin/$name"
done
# Re-running must never silently switch an existing device's source.
if incus config device get "$instance" youtube-media source >/dev/null 2>&1; then
 [[ "$(incus config device get "$instance" youtube-media source)" = "$source_dir" ]] || { echo 'Existing YouTube mount uses a different source'; exit 1; }
else
 incus config device add "$instance" youtube-media disk "source=$source_dir" path=/media/ytdl readonly=false shift=false
fi
incus exec "$instance" -- runuser -u ubuntu -- test -w /media/ytdl
# Unprivileged containers may not read a legacy root-owned archive. Copy its
# deduplication IDs into private app state without altering the source file.
if [[ -r "$source_dir/downloaded.txt" ]]; then
 if ! cp "$source_dir/downloaded.txt" "$stage/existing-archive.txt"; then
  echo "Legacy archive cannot be read; provide a readable private youtube-existing-archive.txt in app state for deduplication."
 else
 incus file push --quiet "$stage/existing-archive.txt" "$instance/var/lib/sonder/youtube-existing-archive.txt" --mode=0600
 incus exec "$instance" -- chown ubuntu:ubuntu /var/lib/sonder/youtube-existing-archive.txt
 fi
fi
incus exec "$instance" -- mkdir -p /etc/systemd/system/sonder.service.d
printf '%s\n' '[Service]' 'Environment=SONDER_YOUTUBE_DIR=/media/ytdl' 'ReadWritePaths=/media/ytdl' > "$stage/youtube.conf"
incus file push --quiet "$stage/youtube.conf" "$instance/etc/systemd/system/sonder.service.d/youtube.conf" --mode=0644
incus exec "$instance" -- python3 - <<'PY'
import json,shutil,time
from pathlib import Path
p=Path('/etc/sonder/server.json');c=json.loads(p.read_text())
if not any(l.get('path')=='/media/ytdl' or l.get('id')=='youtube' for l in c['libraries']):
    backup=Path('/var/lib/sonder')/('server-before-youtube-'+str(int(time.time()))+'.json')
    shutil.copyfile(p,backup);backup.chmod(0o600)
    c['libraries'].append({'id':'youtube','name':'YouTube','path':'/media/ytdl','kind':'documentary'})
    p.write_text(json.dumps(c,indent=2)+'\n')
PY
incus exec "$instance" -- systemctl daemon-reload
incus exec "$instance" -- /usr/local/bin/yt-dlp --version
incus exec "$instance" -- /usr/local/bin/deno --version
printf 'NAS storage and dependencies ready. Deploy the yt-dlp branch, then open /youtube as the owner.\n'
