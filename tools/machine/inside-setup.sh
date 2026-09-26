#!/usr/bin/env bash
# Runs INSIDE the container machine: build the server, install the dev config,
# start it. Invoked by tools/machine/machine-setup.sh on the Mac.
#
# Two things about this environment are not obvious and both cost a debugging
# cycle, so they are written down here:
#
#   * `container machine run -- <exe> <args...>` forwards plain arguments fine,
#     but `sh -c '<script>'` and `bash -lc '<script>'` silently run nothing: the
#     script never reaches the shell. So this file exists instead of an inline
#     command, and the Mac side calls it by path.
#   * There is no systemd *user* session. machined and machinectl are absent and
#     uid 501 has neither a runtime directory nor a session bus, so a
#     `systemctl --user` unit cannot be installed or started here. The process is
#     therefore supervised by a pidfile. The deployed unit on the server host is
#     unaffected; what is reproduced here is the binary, the config paths and the
#     command line, not the supervisor.
set -euo pipefail

repo="$HOME/TM-Sonder"
[[ -d "$repo" ]] || { echo "no repo at $repo" >&2; exit 1; }

echo "==> building"
cd "$repo/server"
mkdir -p "$repo/bin"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$repo/bin/sonder-machine" ./cmd/sonder
echo "    built $repo/bin/sonder-machine"

echo "==> config"
mkdir -p "$HOME/.config/sonder/data" "$HOME/.config/sonder/logs"
# %MEDIA% resolves through the same symlink as the repo, so the config never
# hardcodes whether the Mac home landed in /home or /Users.
sed -e "s#%MEDIA%#$repo/media/fixtures#g" \
    -e "s#%DATA%#$HOME/.config/sonder/data#g" \
    "$repo/deploy/machine/machine-dev-server.json" \
    > "$HOME/.config/sonder/server.json"
echo "    wrote $HOME/.config/sonder/server.json"

echo "==> stopping any previous run"
if [[ -f "$HOME/.config/sonder/sonder.pid" ]]; then
  old="$(cat "$HOME/.config/sonder/sonder.pid")"
  if kill -0 "$old" 2>/dev/null; then kill "$old" 2>/dev/null || true; sleep 1; fi
  rm -f "$HOME/.config/sonder/sonder.pid"
fi

echo "==> starting"
# nohup + setsid so the server outlives the `container machine run` session that
# started it; otherwise it dies with the exec and the machine looks broken.
setsid nohup "$repo/bin/sonder-machine" -config "$HOME/.config/sonder/server.json" \
  >> "$HOME/.config/sonder/logs/sonder.log" 2>&1 < /dev/null &
echo $! > "$HOME/.config/sonder/sonder.pid"
sleep 4

pid="$(cat "$HOME/.config/sonder/sonder.pid")"
if kill -0 "$pid" 2>/dev/null; then
  echo "    running as pid $pid"
else
  echo "    FAILED to stay up; last log lines:" >&2
  tail -20 "$HOME/.config/sonder/logs/sonder.log" >&2 || true
  exit 1
fi
