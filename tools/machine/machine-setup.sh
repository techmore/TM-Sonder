#!/usr/bin/env bash
# Mac-side wrapper: run the in-machine setup script by path.
#
# The indirection is not decoration. `container machine run` forwards arguments
# correctly but silently discards the script given to `sh -c` / `bash -lc`, so an
# inline multi-line command here would appear to succeed and do nothing. A file
# invoked by absolute path works.
set -euo pipefail

MACHINE="${SONDER_MACHINE:-sonder}"
REPO="${SONDER_REPO:-$HOME/Projects/TM-Sonder}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# `container machine create` returns before the machine can actually run a
# command, and the first attempt fails with "Operation not supported by device".
# Retrying is the fix; a sleep before the first attempt is not, because the delay
# is unbounded on a cold image.
echo "==> waiting for $MACHINE to accept commands"
for attempt in $(seq 1 30); do
  if container machine run -n "$MACHINE" -- /bin/true >/dev/null 2>&1; then
    echo "    ready after ${attempt} attempt(s)"
    break
  fi
  if [[ "$attempt" == 30 ]]; then
    echo "machine $MACHINE never became runnable" >&2
    exit 1
  fi
  sleep 2
done

# The Mac home directory is visible inside the machine at its macOS path
# (/Users/<you>) on container 1.0.0, and at /home/<you> on some builds. Rather
# than guess, link it to a stable location inside $HOME so the in-machine script,
# the config and the log paths never hardcode either form.
container machine run -n "$MACHINE" -- /bin/bash "$here/inside-link.sh"

container machine run -n "$MACHINE" -- /bin/bash "$here/inside-setup.sh"

cat <<EOF

ready.

  web   http://localhost:8096
  api   http://localhost:8097
  logs  make machine-logs
  stop  make machine-stop

EOF
