#!/bin/sh
set -eu

config="${SONDER_CONFIG:-/data/server.json}"
if [ ! -f "$config" ]; then
  mkdir -p "$(dirname "$config")"
  cp /etc/sonder/server.json "$config"
fi

exec /sonder -config "$config" "$@"
