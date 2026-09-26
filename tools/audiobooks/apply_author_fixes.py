#!/usr/bin/env python3
"""Apply an author-correction manifest by moving book folders.

The manifest is the plan; this is the executor, and it is deliberately dull:
plan by default, move only what the manifest marked safe, and write a receipt
that is also the rollback.

Why a move and not a copy
-------------------------
A copy would leave the original in place, so the scanner would see the same book
twice and the catalog would gain 18 duplicates. The receipt is what preserves
the ability to undo this, and undoing a rename on one filesystem is a rename
back. So the media is never duplicated and never rewritten -- the bytes are the
same bytes, at a different path.

Why the tag cannot do this instead
----------------------------------
applyFileTags only fills an author that is empty, and the parser has already
filled it from the folder. A file sitting at IntelliQuest/Anna Karenina keeps
"IntelliQuest" no matter what its tag says, so moving the folder is the only
lever that exists. (It also means the tag needs no rewriting: for these rows the
tag already names the target author, which is why they were proposed.)

What it will not do
-------------------
Rows marked unsafe are skipped and reported: a destination that exists would
merge two books, and a tag one character from another author's name is a retag
to settle by hand. Neither is a move.
"""

from __future__ import annotations

import argparse
import collections
import json
import os
import pathlib
import sys


def dir_stats(p: pathlib.Path) -> tuple[int, int]:
    """Return (file count, total bytes) for a book folder."""
    files = [f for f in p.rglob("*") if f.is_file() and not f.name.startswith("._")]
    return len(files), sum(f.stat().st_size for f in files)


def preflight(root: pathlib.Path, e: dict) -> str | None:
    """Return a reason to refuse this row, or None if it is safe to move."""
    src = root / e["author_from"] / e["book"]
    dst = root / e["author_to"] / e["book"]
    if not src.is_dir():
        return "source folder is missing"
    if dst.exists():
        return "destination already exists; moving would merge two books"
    if not (e.get("safe_to_move")):
        return e.get("blocker") or "held back by the manifest"
    # Refuse to move a folder into its own subtree, which os.replace would
    # either fail on or turn into something surprising.
    if dst.is_relative_to(src):
        return "destination is inside the source"
    return None


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--manifest", required=True, type=pathlib.Path)
    ap.add_argument("--root", required=True, type=pathlib.Path)
    ap.add_argument("--apply", action="store_true",
                    help="perform the moves (default: plan only)")
    ap.add_argument("--limit", type=int, help="stop after this many moves")
    ap.add_argument("--receipt", type=pathlib.Path,
                    help="where to write the receipt (default: alongside the manifest)")
    a = ap.parse_args()

    manifest = json.loads(a.manifest.read_text())
    receipt_path = a.receipt or a.manifest.with_suffix(".receipt.json")

    planned, refused = [], []
    for e in manifest["entries"]:
        reason = preflight(a.root, e)
        src = a.root / e["author_from"] / e["book"]
        row = {"author_from": e["author_from"], "author_to": e["author_to"],
               "book": e["book"], "from": str(src),
               "to": str(a.root / e["author_to"] / e["book"])}
        if reason:
            refused.append({**row, "reason": reason})
        else:
            n, b = dir_stats(src)
            planned.append({**row, "files": n, "bytes": b})

    if not a.apply:
        print(json.dumps({"mode": "plan", "would_move": len(planned),
                          "refused": len(refused),
                          "total_files": sum(p["files"] for p in planned),
                          "total_bytes": sum(p["bytes"] for p in planned),
                          "refusals": refused,
                          "first_five": planned[:5]}, indent=2))
        return 0

    moved, failed, rolled_back = [], [], []
    for row in planned:
        if a.limit is not None and len(moved) >= a.limit:
            failed.append({**row, "error": "limit_reached"})
            continue
        src, dst = pathlib.Path(row["from"]), pathlib.Path(row["to"])
        created_parent = not dst.parent.exists()
        try:
            dst.parent.mkdir(parents=True, exist_ok=True)
            before = dir_stats(src)
            os.replace(src, dst)
            after = dir_stats(dst)
            if before != after:
                raise RuntimeError(
                    f"post-move check failed: {before} before, {after} after")
            moved.append({**row, "verified_files": after[0],
                          "verified_bytes": after[1]})
        except Exception as exc:  # noqa: BLE001 - reported, not swallowed
            # Put it back if the move itself went through but the check did not.
            if not src.exists() and dst.exists():
                os.replace(dst, src)
                # And take the author folder with it if this run created it, so a
                # failed run leaves the library exactly as it found it rather
                # than accumulating empty author directories.
                if created_parent:
                    try:
                        dst.parent.rmdir()
                    except OSError:
                        pass  # not empty: other books are already there
                rolled_back.append({**row, "error": str(exc)})
            else:
                failed.append({**row, "error": str(exc)})

    receipt = {
        "manifest": str(a.manifest),
        "root": str(a.root),
        "moved": moved,
        "rolled_back": rolled_back,
        "failed": failed,
        "refused": refused,
        "rollback": (
            "for each entry in `moved`, run: "
            "mkdir -p <dirname of from> && mv <to> <from>"),
    }
    receipt_path.parent.mkdir(parents=True, exist_ok=True)
    receipt_path.write_text(json.dumps(receipt, indent=2))
    print(json.dumps({"mode": "apply", "moved": len(moved),
                      "rolled_back": len(rolled_back), "failed": len(failed),
                      "refused": len(refused), "receipt": str(receipt_path),
                      "errors": [f.get("error") for f in (failed + rolled_back)][:5]},
                     indent=2))
    return 1 if (failed or rolled_back) else 0


if __name__ == "__main__":
    sys.exit(main())
