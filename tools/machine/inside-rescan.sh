#!/usr/bin/env bash
curl -sS -X POST "http://127.0.0.1:8097/api/settings/rescan" >/dev/null; sleep 6; echo rescanned
