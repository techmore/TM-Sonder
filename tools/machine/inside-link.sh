#!/usr/bin/env bash
# Runs INSIDE the container machine: link the Mac home directory to a stable
# path. Invoked by tools/machine/machine-setup.sh on the Mac.
#
# container mounts the Mac home at its macOS path (/Users/<you>) in 1.0.0, while
# the Apple docs describe /home/<you>. Both are in play across builds, and
# hardcoding either makes the setup script break on the other. So the repo is
# linked to $HOME/TM-Sonder, which is stable, and everything else refers to
# that.
set -euo pipefail

name="TM-Sonder"
for candidate in "/Users/$(id -un)/Projects/$name" "$HOME/Projects/$name"; do
  if [[ -d "$candidate" ]]; then
    ln -sfn "$candidate" "$HOME/$name"
    echo "repo: $candidate -> \$HOME/$name"
    exit 0
  fi
done

echo "could not find the repository inside the machine; looked in:" >&2
echo "  /Users/$(id -un)/Projects/$name" >&2
echo "  \$HOME/Projects/$name" >&2
exit 1
