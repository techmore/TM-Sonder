#!/usr/bin/env bash
# Runs INSIDE the container machine: report whether the server is up.
set -euo pipefail

pidfile="$HOME/.config/sonder/sonder.pid"
if [[ -f "$pidfile" ]] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
  echo "process: running (pid $(cat "$pidfile"))"
else
  echo "process: NOT running"
fi

echo -n "api:     "
curl -sS --max-time 10 http://127.0.0.1:8097/api/status 2>&1 | head -c 400 || true
echo
echo -n "web:     "
curl -sS -o /dev/null -w 'HTTP %{http_code}\n' --max-time 10 http://127.0.0.1:8096/ 2>&1 || true

echo -n "library: "
curl -sS --max-time 20 http://127.0.0.1:8097/api/library 2>/dev/null \
  | grep -o '"kind":"audiobook"' | wc -l | tr -d ' ' || true
echo " audiobook rows"
