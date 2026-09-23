#!/usr/bin/env python3
"""Install staged poster-queue covers onto the NAS (Plex local assets).

Idempotent: skips books/movies that already have art, records applied work in
applied.json so reconnect runs only do the remainder. Exits 0 with message
when the NAS is unreachable (the sync wrapper retries later).
"""
import json
import os
import re
import shutil
import sys

ROOT = "/Users/seandolbec/Projects/TM-Sonder/m4b_work/poster-queue"
NAS_MOVIES = "/Users/seandolbec/NAS/plex/movies"
NAS_TV = "/Users/seandolbec/NAS/plex/tv_shows"


def nas_ok():
    try:
        with os.scandir("/Users/seandolbec/NAS/plex") as it:
            return any(True for _ in it)
    except OSError:
        return False


def norm(s):
    return re.sub(r"[^a-z0-9]", "", (s or "").lower())


def find_dir(root, title):
    want = norm(title)
    try:
        cands = [e.name for e in os.scandir(root) if e.is_dir(follow_symlinks=False)]
    except OSError:
        return None
    for c in cands:
        if c.startswith("."):
            continue
        if norm(c).startswith(want) or want in norm(c):
            return os.path.join(root, c)
    return None


def install(d, cover):
    made = []
    for side in ("poster.jpg", "folder.jpg"):
        dst = os.path.join(d, side)
        if not os.path.exists(dst):
            shutil.copyfile(cover, dst)
            made.append(side)
    return made


def main():
    if not nas_ok():
        print("NAS unreachable, will retry on reconnect")
        return 0
    res_path = os.path.join(ROOT, "fetch-results.json")
    if not os.path.exists(res_path):
        print("nothing queued yet")
        return 0
    res = json.load(open(res_path))
    ap_path = os.path.join(ROOT, "applied.json")
    applied = json.load(open(ap_path)) if os.path.exists(ap_path) else {}
    q = json.load(open(os.path.join(ROOT, "queue.json")))
    titles = {f"{m['title']} ({m.get('year', 0)})": ("movie", m["title"])
              for m in q["movies"]}
    for s in q["shows"]:
        titles[f"{s['title']} ({(s['years'] or [0])[0]})"] = ("show", s["title"])
    for key, r in res.items():
        if r.get("status") != "staged" or key in applied:
            continue
        kind, title = titles.get(key, ("movie", key.split(" (")[0]))
        root = NAS_TV if kind == "show" else NAS_MOVIES
        d = find_dir(root, title)
        if not d:
            print(f"NOMATCH-DIR {key}")
            continue
        made = install(d, r["cover"])
        applied[key] = {"dir": d, "installed": made or ["already-present"]}
        print(f"{'INSTALLED ' + '+'.join(made) if made else 'PRESENT'} {key}")
    json.dump(applied, open(ap_path, "w"), indent=1)
    print(f"APPLY-DONE applied={len(applied)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
