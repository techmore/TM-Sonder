#!/usr/bin/env bash
# Runs INSIDE the container machine: the full check suite, in the same Linux
# environment the release binary is built and shipped from.
set -euo pipefail

cd "$HOME/TM-Sonder/server"

echo "==> gofmt"
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  echo "gofmt needed on:" >&2
  echo "$unformatted" >&2
  exit 1
fi
echo "    clean"

echo "==> go vet"
go vet ./...

echo "==> go test"
go test -count=1 ./...

cd "$HOME/TM-Sonder"
echo "==> web client tests"
node --test server/internal/httpapi/web/library.test.cjs
