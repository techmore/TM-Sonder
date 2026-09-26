#!/usr/bin/env python3
"""Audit audiobook titles and credits across a whole library.

Sorting and display were fixed in the server, but two distinct problems remain
that no amount of parser work can fix, because the data itself is wrong:

1. **Titles the layout cannot normalize away.** A leading series parenthetical
   is now handled, but a folder called "(Culture 1) Race and Culture" still
   reads badly, and anything the rules do not cover needs a human.
2. **Mis-filed books.** Iain M. Banks' Culture novels sit in a folder named
   *Thomas Sowell* and carry ``artist=Thomas Sowell`` in their tags, so the
   parser faithfully reports the wrong author. This is a retag defect and it is
   invisible unless you compare the folder, the tag and the title against each
   other.

Read-only. It only reads the catalog snapshot and calls ffprobe; it never
renames, moves, re-tags or deletes. The output is a review artifact.
"""

from __future__ import annotations

import argparse
import collections
import concurrent.futures
import json
import os
import pathlib
import re
import subprocess
import sys
import unicodedata

FFPROBE = "/opt/homebrew/bin/ffprobe"

# Text that should not survive into a displayed title.
LEADING_JUNK = re.compile(r"^\s*[-–,;:.]")
PAREN_NO_NUMBER = re.compile(r"^\s*\([^)]*\)\s*")
DIGIT_RUN = re.compile(r"\b\d{1,4}[a-z]?\b")


def norm(s: str) -> str:
    s = unicodedata.normalize("NFKD", s or "").lower()
    s = re.sub(r"[^a-z0-9]+", " ", s)
    return re.sub(r"\s+", " ", s).strip()


def probe_tags(path: pathlib.Path) -> dict:
    try:
        out = subprocess.run(
            [FFPROBE, "-v", "quiet", "-print_format", "json",
             "-show_format", "-show_entries", "format_tags", str(path)],
            capture_output=True, timeout=120, check=True).stdout
        tags = json.loads(out).get("format", {}).get("tags", {}) or {}
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError,
            json.JSONDecodeError):
        return {}
    return {k.lower().lstrip("©"): (v or "").strip() for k, v in tags.items()}


def title_issues(title: str) -> list[str]:
    """Problems a reader would notice in a displayed title."""
    issues = []
    t = title or ""
    if not t.strip():
        return ["empty_title"]
    if LEADING_JUNK.match(t):
        issues.append("leading_separator")
    if "  " in t:
        issues.append("double_space")
    if t != t.strip():
        issues.append("surrounding_whitespace")
    if re.search(r"\b\d{2,4}k\b|\{\}|\[\]", t, re.I):
        issues.append("technical_tag_in_title")
    m = PAREN_NO_NUMBER.match(t)
    if m and norm(t[len(m.group(0)):]) != norm(t):
        # A leading parenthetical that is a series marker should already have
        # been stripped; if it is still here and the rest differs, it is noise.
        if not re.search(r"\d", m.group(0)):
            issues.append("leading_paren_without_position")
    if re.match(r"^\s*\d{4}\s*[-–—_]", t):
        issues.append("leading_year")
    if re.match(r"^\s*\d{1,3}[a-z]?\s*[-–—_\s]", t) and not re.match(r"^\d{4}", t):
        issues.append("leading_ordinal")
    if len(t) > 120:
        issues.append("very_long_title")
    return issues


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", required=True, type=pathlib.Path,
                    help="audiobook library root")
    ap.add_argument("--snapshot", type=pathlib.Path,
                    help="catalog snapshot JSON; walked directly if omitted")
    ap.add_argument("--out", required=True, type=pathlib.Path)
    ap.add_argument("--workers", type=int, default=8)
    a = ap.parse_args()

    # Collect one row per book folder: the folder's own name is the title source.
    folders: dict[str, dict] = {}
    for dirpath, dirnames, filenames in os.walk(a.root):
        dirnames[:] = [d for d in sorted(dirnames) if not d.startswith(".")]
        media = [f for f in sorted(filenames)
                 if not f.startswith(".")
                 and pathlib.Path(f).suffix.lower() in
                 (".m4b", ".m4a", ".mp3", ".ogg", ".opus", ".flac")]
        if not media:
            continue
        folder = pathlib.Path(dirpath)
        rel = folder.relative_to(a.root)
        parts = rel.parts
        if len(parts) >= 2:
            author_folder, book_folder = parts[-2], parts[-1]
        else:
            author_folder, book_folder = "", parts[-1] if parts else ""
        folders[str(folder)] = {
            "path": str(folder),
            "relative": rel.as_posix(),
            "author_folder": author_folder,
            "book_folder": book_folder,
            "file_count": len(media),
            "files": [str(folder / m) for m in media],
        }

    print(f"{len(folders)} book folders", file=sys.stderr)

    paths = [f for v in folders.values() for f in v["files"][:1]]
    tagmap: dict[str, dict] = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=a.workers) as ex:
        for p, tags in zip(paths, ex.map(
                lambda f: probe_tags(pathlib.Path(f)), paths)):
            tagmap[p] = tags

    rows = []
    counts = collections.Counter()
    for v in folders.values():
        tags = tagmap.get(v["files"][0], {})
        tag_title = tags.get("title", "")
        tag_album = tags.get("album", "")
        tag_artist = tags.get("album_artist") or tags.get("artist", "")
        issues = list(title_issues(v["book_folder"]))
        if tag_title and norm(tag_title) != norm(v["book_folder"]):
            issues.append("tag_title_differs_from_folder")
        if tag_artist and tag_artist != v["author_folder"] \
                and norm(tag_artist) != norm(v["author_folder"]):
            issues.append("tag_artist_differs_from_folder")
        if not tag_artist and not v["author_folder"]:
            issues.append("no_author_anywhere")
        if not v["author_folder"]:
            issues.append("missing_author_folder")
        for i in issues:
            counts[i] += 1
        rows.append({**v, "tag_title": tag_title, "tag_album": tag_album,
                     "tag_artist": tag_artist, "tag_comment": tags.get("comment", ""),
                     "tag_narrator": tags.get("narrator", ""),
                     "issues": issues})

    rows.sort(key=lambda r: (len(r["issues"]) == 0, r["issues"], r["relative"]))

    # A mis-filed book cannot be detected locally: Iain M. Banks' Culture
    # novels sit in "Thomas Sowell/" and carry artist=Thomas Sowell in their
    # tags, so the folder and the tag agree and both are wrong. What *is*
    # checkable is which author folders claim each series. A series that only
    # one author claims, while a better-fitting author exists elsewhere in the
    # library, is the shape of a mis-filing -- and it takes seconds to eyeball.
    series_claims: dict[str, list[dict]] = collections.defaultdict(list)
    narrator_claims: dict[str, list[dict]] = collections.defaultdict(list)
    for r in rows:
        for series in series_markers(r["book_folder"]):
            series_claims[series].append({
                "author_folder": r["author_folder"],
                "book": r["book_folder"],
                "relative": r["relative"],
            })
        n = narrator_credit(r["book_folder"])
        if n:
            narrator_claims[n].append({
                "author_folder": r["author_folder"],
                "book": r["book_folder"],
                "relative": r["relative"],
            })
            r["narrator_from_folder"] = n

    def table(claims: dict[str, list[dict]]) -> dict:
        return {k: {"claimed_by": sorted({c["author_folder"] for c in v}),
                    "books": v}
                for k, v in sorted(claims.items())}

    series_table = table(series_claims)
    narrator_table = table(narrator_claims)
    # A series claimed by exactly one author folder is the reviewable case.
    single_claim = {s: v for s, v in series_table.items() if len(v["claimed_by"]) == 1}

    report = {
        "policy": ("Read-only audit. Nothing was renamed, moved, re-tagged or "
                   "deleted. 'tag_artist_differs_from_folder' means the tag and "
                   "the folder disagree, so one of them is wrong. The series and "
                   "narrator tables are the mis-filing review surface: a series "
                   "claimed by one author, or a narrator recurring under "
                   "unrelated authors, is the shape of a book filed under the "
                   "wrong author. Both are hints for the eye, not verdicts."),
        "root": str(a.root),
        "book_folders": len(rows),
        "issue_counts": dict(counts),
        "folders_with_issues": sum(1 for r in rows if r["issues"]),
        "single_author_claimed_series": len(single_claim),
        "series_claims": series_table,
        "narrator_credits": narrator_table,
        "rows": rows,
    }
    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(json.dumps(report, indent=2))
    print(json.dumps({
        "book_folders": len(rows),
        "folders_with_issues": report["folders_with_issues"],
        "issue_counts": dict(counts),
        "out": str(a.out),
    }, indent=2))
    return 0


def series_markers(book_folder: str) -> list[str]:
    """Series names a folder name claims, e.g. "(Culture 1) X" -> ["Culture"]."""
    out = []
    m = re.match(r"^\s*\(\s*([^)]{1,40}?)\s*(?:\d{1,4}[a-z]?)?\s*\)\s*", book_folder)
    if m:
        name = m.group(1).strip().rstrip(".")
        if name and not re.fullmatch(r"\d+", name):
            out.append(name)
    m2 = re.match(r"^\s*(\d{1,3})[a-z]?\s*[-–—_\s]+", book_folder)
    if m2:
        out.append("(unnumbered position)")
    return sorted(set(out))


def narrator_credit(book_folder: str) -> str:
    """A trailing "(Name)" or "(Read by Name)" is a narrator, not a series.

    These are the library's version markers: one folder per reading, which is
    why "The Lathe of Heaven" appears six times. Grouping by narrator is what
    makes a mis-filing obvious, because the same narrator recurs across
    unrelated authors.
    """
    m = re.search(r"\(\s*(?:read by\s+|narrated by\s+|narrator:\s*)?"
                  r"([A-Z][\w'’.-]{1,20}(?:\s+[A-Z][\w'’.-]{1,20})?)\s*\)\s*$",
                  book_folder)
    return m.group(1).strip() if m else ""


if __name__ == "__main__":
    sys.exit(main())
