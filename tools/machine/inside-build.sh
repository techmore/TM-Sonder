#!/usr/bin/env bash
# Runs INSIDE the container machine: cross-compile the shipping binary.
#
# The machine builds for the *deployment* target, not for itself. ser8 is
# x86_64, so an arm64 binary built on an Apple Silicon Mac is not the artifact
# that runs there, and "it worked on my machine" is otherwise a real category of
# bug rather than a figure of speech.
set -euo pipefail

repo="$HOME/TM-Sonder"
version="${1:-dev}"
build="$(git -C "$repo" rev-parse --short HEAD 2>/dev/null || echo unknown)"

cd "$repo/server"
mkdir -p "$repo/bin"

ldflags="-s -w"
ldflags="$ldflags -X main.version=$version"
ldflags="$ldflags -X tm-sonder/server/internal/httpapi.Version=$version"
ldflags="$ldflags -X tm-sonder/server/internal/httpapi.Build=$build"

for target in "linux amd64 sonder-linux-amd64" "linux arm64 sonder-linux-arm64" "darwin arm64 sonder-darwin-arm64"; do
  set -- $target
  os="$1" arch="$2" out="$3"
  echo "building $out"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$ldflags" \
    -o "$repo/bin/$out" ./cmd/sonder
done

echo
ls -l "$repo/bin"/sonder-linux-* "$repo/bin"/sonder-darwin-*
echo
echo "deploy with:"
echo "  scp $repo/bin/sonder-linux-amd64 sdolbec@100.127.99.74:~/TM-Sonder/bin/"
