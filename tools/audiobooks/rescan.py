#!/usr/bin/env python3
"""Trigger a TM-Sonder library rescan and wait for it to finish.

The server exposes ``POST /api/settings/rescan``, which starts a scan in the
background and returns immediately. This wraps it so the caller gets the result
instead of having to poll: it waits for ``/api/status`` to report the scan as
finished, with a bounded timeout, and prints the log tail either way.

Read-only apart from the rescan itself. The token is read from the server config
and never printed.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys
import time
import urllib.error
import urllib.request

DEFAULT_CONFIG = pathlib.Path.home() / ".config/sonder/server.json"


def load_token(config: pathlib.Path) -> tuple[str, str]:
    cfg = json.loads(config.read_text())
    token = cfg.get("pairingToken", "")
    if not token:
        raise SystemExit(f"no pairingToken in {config}")
    port = cfg.get("apiPort", 8097)
    return f"http://127.0.0.1:{port}", token


def get_status(base: str, token: str, timeout: int = 30) -> dict:
    req = urllib.request.Request(f"{base}/api/status?token={token}")
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read())


def start_rescan(base: str, token: str, timeout: int = 30) -> tuple[int, str]:
    req = urllib.request.Request(
        f"{base}/api/settings/rescan?token={token}", method="POST", data=b"")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")


def log_tail(config: pathlib.Path, lines: int) -> str:
    cfg = json.loads(config.read_text())
    log_dir = cfg.get("logDir") or str(
        pathlib.Path.home() / "Library/Application Support/TM-Sonder-Server/logs")
    log = pathlib.Path(log_dir) / "launchd-err.log"
    if not log.exists():
        return f"(no log at {log})"
    text = log.read_text(errors="replace").splitlines()
    keep = [l for l in text[-400:] if "scan complete" in l or "scan " in l
            and "error" in l.lower()]
    return "\n".join(keep[-lines:]) or f"(no scan lines in the tail of {log})"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--config", type=pathlib.Path, default=DEFAULT_CONFIG)
    ap.add_argument("--timeout", type=int, default=900,
                    help="seconds to wait for the scan (default 900)")
    ap.add_argument("--wait", action="store_true", default=True,
                    help="block until the scan finishes (default)")
    ap.add_argument("--no-wait", dest="wait", action="store_false",
                    help="start the scan and return immediately")
    a = ap.parse_args()

    base, token = load_token(a.config)

    before = get_status(base, token)
    print(f"before: items={before.get('itemCount')} scanning={before.get('scanning')}")

    status, body = start_rescan(base, token)
    if status == 409:
        print("a scan is already running; waiting for it instead")
    elif status not in (200, 202):
        print(f"rescan request failed: {status} {body}", file=sys.stderr)
        return 1
    else:
        print("rescan started")

    if not a.wait:
        return 0

    deadline = time.time() + a.timeout
    last = None
    while time.time() < deadline:
        st = get_status(base, token)
        if st.get("scanning") != last:
            last = st.get("scanning")
            print(f"  scanning={last} items={st.get('itemCount')}")
        if not st.get("scanning"):
            after = st
            print(f"\ndone: items={after.get('itemCount')}")
            print(log_tail(a.config, 5))
            return 0
        time.sleep(3)

    print(f"\ntimed out after {a.timeout}s; the scan is still running",
          file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
