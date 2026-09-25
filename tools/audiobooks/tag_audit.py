#!/usr/bin/env python3
"""Read embedded container tags for audiobook folders the layout planner could
not attribute to an author.

Read-only: this only calls ffprobe and prints a report. It never writes tags,
renames, or moves anything. Its purpose is to find out whether the missing
author is already present in the files, in which case the catalog can be fixed
by *reading* the tag rather than by guessing a folder name.
"""

from __future__ import annotations

import argparse
import collections
import json
import pathlib
import subprocess
import sys

FFPROBE = "/opt/homebrew/bin/ffprobe"

# A tag value equal to one of these is a placeholder, not a real credit.
PLACEHOLDERS = {"", "unknown", "unknown author", "various", "various authors",
                "audiobook", "n/a", "none", "null", "untagged"}


def probe_tags(path: pathlib.Path) -> dict:
    try:
        out = subprocess.run(
            [FFPROBE, "-v", "quiet", "-print_format", "json",
             "-show_format", "-show_entries", "format_tags", str(path)],
            capture_output=True, timeout=120, check=True).stdout
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError):
        return {}
    try:
        tags = json.loads(out).get("format", {}).get("tags", {}) or {}
    except json.JSONDecodeError:
        return {}
    return {k.lower().lstrip("©"): v.strip() for k, v in tags.items()}


def resolve_author(tags: dict, folder_hint: str) -> tuple[str, str]:
    """Return (author, how) using album_artist, then artist, then composer."""
    for key in ("album_artist", "artist", "composer"):
        val = tags.get(key, "")
        if val.lower() in PLACEHOLDERS:
            continue
        if val:
            return val, key
    return "", ""


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("plan", type=pathlib.Path,
                   help="standardize-plan JSON from standardize.py")
    p.add_argument("--reason", default="unknown_author_folder",
                   help="only report folders carrying this review reason")
    p.add_argument("--out", type=pathlib.Path, help="write a JSON report here")
    a = p.parse_args()

    plan = json.loads(a.plan.read_text())
    folders = [f for f in plan["folders"] if a.reason in f["reasons"]]
    print(f"folders with reason {a.reason!r}: {len(folders)}", file=sys.stderr)

    rows = []
    for f in folders:
        for fe in f["files"]:
            tags = probe_tags(pathlib.Path(fe["source"]))
            author, how = resolve_author(tags, f["author"])
            rows.append({
                "source": fe["source"],
                "relative_folder": f["relative_folder"],
                "book": f["book"],
                "author_folder": f["author"],
                "tag_author": author,
                "tag_author_field": how,
                "tag_album": tags.get("album", ""),
                "tag_narrator_comment": tags.get("comment", "")[:200],
                "tag_genre": tags.get("genre", ""),
                "tag_date": tags.get("date", ""),
            })

    resolvable = [r for r in rows if r["tag_author"]]
    unresolved = [r for r in rows if not r["tag_author"]]
    by_field = collections.Counter(r["tag_author_field"] for r in resolvable)

    print(f"\nresolvable from tags: {len(resolvable)} / {len(rows)}")
    print(f"by tag field: {dict(by_field)}")
    print(f"NOT resolvable from tags: {len(unresolved)}\n")
    for r in unresolved:
        print(f"  ? {r['relative_folder']}  ({r['tag_album'] or 'no album tag'})")
    print()
    for r in resolvable:
        print(f"  + {r['relative_folder']}")
        print(f"      author={r['tag_author']} (from {r['tag_author_field']})")

    if a.out:
        a.out.parent.mkdir(parents=True, exist_ok=True)
        a.out.write_text(json.dumps({
            "policy": ("Read-only ffprobe inspection. Nothing was written, "
                       "renamed, or moved."),
            "reason": a.reason,
            "resolvable": len(resolvable),
            "unresolved": len(unresolved),
            "by_field": dict(by_field),
            "rows": rows,
        }, indent=2))
        print(f"\nwrote {a.out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
