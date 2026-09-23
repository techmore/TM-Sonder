#!/usr/bin/env python3
"""Regenerate m4b_work/retag-dashboard.html from retag-full.log + job state.

Run once, or loop every 30s:
  python3 tools/audiobooks/retag_dashboard.py --loop
"""
import argparse
import html
import json
import os
import re
import subprocess
import time
from datetime import datetime

ROOT = "/Users/seandolbec/Projects/TM-Sonder"
LOG = os.path.join(ROOT, "m4b_work", "retag-full.log")
OUT = os.path.join(ROOT, "m4b_work", "retag-dashboard.html")
SEEN = os.path.join(ROOT, "m4b_work", "retag-seen.json")
PLAN = os.path.join(ROOT, "m4b_work", "migration", "standardize-plan.json")

LINE_RE = re.compile(r"^\[(\d+)/(\d+)\]\s+(SKIP-OK|DONE|FAIL|MISSING)\s+(.*)$")


def parse_log():
    total = 228
    try:
        with open(PLAN) as f:
            total = len(json.load(f).get("retags", [])) or 228
    except OSError:
        pass
    done = skip = fail = 0
    current = None
    last, fails, entries = [], [], []
    finished = False
    try:
        with open(LOG) as f:
            lines = f.read().splitlines()
    except OSError:
        return {"total": total, "done": 0, "skip": 0, "fail": 0,
                "current": None, "last": [], "fails": [],
                "finished": False, "log_missing": True}
    for line in lines:
        m = LINE_RE.match(line)
        if not m:
            if line.startswith("RETAG-DONE"):
                finished = True
            continue
        idx, tot, status, rest = m.groups()
        total = int(tot)
        current = (int(idx), rest, status)
        last.append(line)
        if status == "DONE":
            done += 1
        elif status == "SKIP-OK":
            skip += 1
        else:
            fail += 1
            fails.append(line)
    return {"total": total, "done": done, "skip": skip, "fail": fail,
            "current": current, "last": last[-15:], "fails": fails,
            "finished": finished, "log_missing": False}


def ffmpeg_active():
    try:
        p = subprocess.run(["ps", "aux"], capture_output=True, text=True, timeout=10)
        for line in p.stdout.splitlines():
            if "sonder-retag.m4b" in line and "ffmpeg" in line:
                m = re.search(r"Audiobooks/(.+?\.m4b)\.sonder-retag", line)
                return m.group(1) if m else "ffmpeg active"
        return None
    except Exception:
        return None


def load_seen():
    try:
        with open(SEEN) as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def save_seen(seen):
    try:
        with open(SEEN, "w") as f:
            json.dump(seen, f)
    except OSError:
        pass


def stamp_seen(entries):
    """entries: list of (idx, status, rest). Returns {idx: first_seen_str}."""
    seen = load_seen()
    now = datetime.now()
    changed = False
    for idx, _status, _rest in entries:
        if str(idx) not in seen:
            seen[str(idx)] = now.isoformat(timespec="seconds")
            changed = True
    if changed:
        save_seen(seen)
    return seen


def render(s):
    total = s["total"]
    processed = s["done"] + s["skip"] + s["fail"]
    pct = (100.0 * processed / total) if total else 0
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    active = ffmpeg_active()
    if s["finished"]:
        banner = "COMPLETE"
    elif active or (processed and processed < total):
        banner = "RUNNING"
    elif s["log_missing"]:
        banner = "WAITING FOR LOG"
    else:
        banner = "IDLE"
    cur = ""
    if s["current"]:
        idx, rest, st = s["current"]
        cur = f"[{idx}/{total}] {html.escape(st)} {html.escape(rest)}"
    last_html = "<br>".join(html.escape(l) for l in s["last"]) or "<i>no lines yet</i>"
    fails_html = "<br>".join(html.escape(l) for l in s["fails"]) or "<i>none</i>"
    active_html = html.escape(active) if active else "<i>no ffmpeg process seen</i>"
    return f"""<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<meta http-equiv="refresh" content="30" />
<title>Retag dashboard — {banner}</title>
<script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
<link rel="preconnect" href="https://fonts.googleapis.com" />
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
<link href="https://fonts.googleapis.com/css2?family=Instrument+Serif:ital@0;1&display=swap" rel="stylesheet" />
<link href="https://fonts.googleapis.com/css2?family=Inter:ital,opsz,wght@0,14..32,100..900;1,14..32,100..900&display=swap" rel="stylesheet" />
<style type="text/tailwindcss">
@theme {{
  --font-sans: "Inter", system-ui, sans-serif;
  --font-display: "Instrument Serif", serif;
  --color-olive-100: oklch(96.6% .005 106.5);
  --color-olive-200: oklch(93% .007 106.5);
  --color-olive-300: oklch(88% .011 106.6);
  --color-olive-400: oklch(73.7% .021 106.9);
  --color-olive-500: oklch(58% .031 107.3);
  --color-olive-600: oklch(46.6% .025 107.3);
  --color-olive-700: oklch(39.4% .023 107.4);
  --color-olive-800: oklch(28.6% .016 107.4);
  --color-olive-950: oklch(15.3% .006 107.1);
}}
</style>
</head>
<body class="bg-olive-100 text-olive-950 antialiased dark:bg-olive-950 dark:text-white">
<main class="mx-auto max-w-4xl px-6 py-16">
<p class="text-sm font-medium text-olive-600 dark:text-olive-400">TM Sonder · Audiobook cleanup</p>
<h1 class="font-display text-5xl tracking-tight text-balance mt-2">Retag <em>dashboard</em>
<span class="inline-block rounded-full bg-olive-950 px-3 py-1 align-middle font-sans text-xs font-semibold text-white dark:bg-olive-300 dark:text-olive-950">{banner}</span></h1>
<p class="mt-4 text-sm text-olive-600 dark:text-olive-400">Updated {now} · auto-refreshes every 30s · source <code>retag-full.log</code></p>

<div class="mt-6 rounded-xl border border-olive-950/10 bg-olive-950/[0.025] p-6 dark:border-white/10 dark:bg-white/5">
<div class="h-8 overflow-hidden rounded-full bg-olive-950/10 dark:bg-white/10">
<div class="h-full bg-gradient-to-b from-olive-600 to-olive-800 text-center text-sm font-medium leading-8 whitespace-nowrap overflow-hidden text-white" style="width:{pct:.1f}%">{processed}/{total} ({pct:.1f}%)</div>
</div>
<div class="mt-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
<div class="rounded-xl border border-olive-950/10 bg-olive-950/[0.025] p-4 text-center dark:border-white/10 dark:bg-white/5">
<div class="font-display text-2xl">{s["done"]}</div><p class="text-sm text-olive-600 dark:text-olive-400">rewritten</p></div>
<div class="rounded-xl border border-olive-950/10 bg-olive-950/[0.025] p-4 text-center dark:border-white/10 dark:bg-white/5">
<div class="font-display text-2xl">{s["skip"]}</div><p class="text-sm text-olive-600 dark:text-olive-400">already correct</p></div>
<div class="rounded-xl border border-olive-950/10 bg-olive-950/[0.025] p-4 text-center dark:border-white/10 dark:bg-white/5">
<div class="font-display text-2xl">{s["fail"]}</div><p class="text-sm text-olive-600 dark:text-olive-400">failed</p></div>
<div class="rounded-xl border border-olive-950/10 bg-olive-950/[0.025] p-4 text-center dark:border-white/10 dark:bg-white/5">
<div class="font-display text-2xl">{total - processed}</div><p class="text-sm text-olive-600 dark:text-olive-400">remaining</p></div>
</div>
</div>

<h2 class="font-display text-2xl tracking-tight mt-10">Current</h2>
<div class="mt-2 rounded-xl border border-olive-950/10 bg-olive-950/[0.025] p-6 dark:border-white/10 dark:bg-white/5">
<p class="font-mono text-sm">{cur or "—"}</p>
<p class="mt-2 text-sm text-olive-600 dark:text-olive-400">ffmpeg: {active_html}</p>
</div>

<h2 class="font-display text-2xl tracking-tight mt-10">Recent lines</h2>
<pre class="mt-2 overflow-x-auto rounded-xl bg-olive-950 p-6 font-mono text-sm leading-7 text-olive-100">{last_html}</pre>

<h2 class="font-display text-2xl tracking-tight mt-10">Failures</h2>
<pre class="mt-2 overflow-x-auto rounded-xl bg-olive-950 p-6 font-mono text-sm leading-7 text-olive-100">{fails_html}</pre>

<h2 class="font-display text-2xl tracking-tight mt-10">What the tags do</h2>
<div class="mt-2 rounded-xl border border-olive-950/10 bg-olive-950/[0.025] p-6 dark:border-white/10 dark:bg-white/5">
<p><code>title</code>/<code>album</code> → Book name, <code>artist</code>/<code>album_artist</code> → Author. Labels only; audio, chapters, and covers are verified unchanged after each rewrite.</p>
</div>
</main>
</body>
</html>"""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--loop", action="store_true")
    args = ap.parse_args()
    while True:
        with open(OUT, "w") as f:
            f.write(render(parse_log()))
        print(f"dashboard -> {OUT}", flush=True)
        if not args.loop:
            return
        time.sleep(30)


if __name__ == "__main__":
    main()
