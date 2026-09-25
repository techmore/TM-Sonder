#!/usr/bin/env python3
"""Propose author attributions for audiobooks the layout planner could not place.

This is a **proposal generator, not a mover**. It queries a metadata provider
(read-only HTTP GETs), matches each unattributed book, and writes a reviewable
JSON proposal. It never renames, moves, or writes tags; applying a proposal is
a separate, explicit step that a human reviews first.

Confidence is deliberately conservative:

  high     the provider returned exactly one plausible author, and the
           normalized title matches the folder's book name
  medium   one author returned, but the title only approximately matches
  low      several candidate authors, or the title match is weak
  none     no candidate found; the folder stays unattributed

Only `high` and `medium` rows carry a proposed author. Everything else is
reported so a human can decide, which is what the layout planner needs too.
"""

from __future__ import annotations

import argparse
import collections
import difflib
import json
import pathlib
import re
import sys
import time
import unicodedata
import urllib.error
import urllib.parse
import urllib.request

OPEN_LIBRARY_SEARCH = "https://openlibrary.org/search.json"
USER_AGENT = "tm-sonder-audiobook-audit/1.0 (local library organization tool)"

# Parenthetical narrator/year credits that appear in many folder names:
# "VALIS (Gigante) (1981)" -> narrator Gigante, year 1981. These are NOT the
# author, so they are stripped before matching against a provider.
CREDIT_RE = re.compile(r"\(([^)]{1,40})\)")


def normalize_title(s: str) -> str:
    s = unicodedata.normalize("NFKD", s)
    s = "".join(c for c in s if not unicodedata.combining(c))
    s = s.lower()
    s = CREDIT_RE.sub(" ", s)
    s = re.sub(r"[^a-z0-9]+", " ", s)
    return re.sub(r"\s+", " ", s).strip()


def fetch(url: str, timeout: int = 30) -> dict:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read().decode("utf-8", "replace"))


def search_open_library(title: str, limit: int = 5) -> list[dict]:
    q = urllib.parse.urlencode({
        "title": CREDIT_RE.sub("", title).strip(),
        "limit": limit,
        "fields": "title,author_name,first_publish_year,key",
    })
    try:
        return fetch(f"{OPEN_LIBRARY_SEARCH}?{q}").get("docs", []) or []
    except (urllib.error.URLError, json.JSONDecodeError, TimeoutError, OSError):
        return []


def strip_leading_index(s: str) -> str:
    """'03 - The State of the Art' -> 'The State of the Art'."""
    return re.sub(r"^\s*\d{1,3}[a-z]?\s*[-_ ]\s*", "", s).strip() or s


def score(book_title: str, doc_title: str) -> float:
    a, b = normalize_title(book_title), normalize_title(doc_title)
    if not a or not b:
        return 0.0
    return difflib.SequenceMatcher(None, a, b).ratio()


def resolve(title: str, delay: float) -> dict:
    title = strip_leading_index(title)
    docs = search_open_library(title)
    if not docs:
        return {"confidence": "none", "candidates": [], "proposed_author": ""}

    ranked = sorted(
        ((score(title, d.get("title", "")), d) for d in docs),
        key=lambda t: t[0], reverse=True,
    )
    best_score, best = ranked[0]
    authors = [a for a in (best.get("author_name") or []) if a]
    if not authors:
        return {"confidence": "none", "candidates": [], "proposed_author": ""}

    # Several distinct authors at a high match means an anthology or an
    # ambiguous record; a human should choose.
    distinct = {a.strip().lower() for a in authors}
    if best_score >= 0.85 and len(distinct) == 1:
        confidence = "high"
    elif best_score >= 0.85:
        confidence = "low"
    elif best_score >= 0.6 and len(distinct) == 1:
        confidence = "medium"
    else:
        confidence = "low"

    proposed = authors[0].strip() if confidence in ("high", "medium") else ""
    time.sleep(delay)
    return {
        "confidence": confidence,
        "title_match": round(best_score, 3),
        "provider_title": best.get("title", ""),
        "provider_year": best.get("first_publish_year", 0),
        "all_candidates": authors,
        "proposed_author": proposed,
    }


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("plan", type=pathlib.Path)
    p.add_argument("--reason", default="unknown_author_folder")
    p.add_argument("--out", required=True, type=pathlib.Path)
    p.add_argument("--delay", type=float, default=1.0,
                   help="seconds between provider requests (default 1.0)")
    a = p.parse_args()

    plan = json.loads(a.plan.read_text())
    folders = [f for f in plan["folders"] if a.reason in f["reasons"]]
    # The same book can appear twice (a wrapper copy and a top-level copy);
    # resolve it once.
    unique = sorted({f["book"] for f in folders if f["book"]})
    print(f"{len(folders)} folders, {len(unique)} distinct books", file=sys.stderr)

    rows = []
    for i, book in enumerate(unique, 1):
        res = resolve(book, a.delay)
        rows.append({"book": book, **res})
        print(f"[{i}/{len(unique)}] {book} -> {res['confidence']}: "
              f"{res.get('proposed_author') or res.get('all_candidates')}",
              file=sys.stderr)

    counts = collections.Counter(r["confidence"] for r in rows)
    by_author = collections.Counter(
        r["proposed_author"] for r in rows if r["proposed_author"])

    report = {
        "policy": ("Read-only metadata lookup. Nothing was renamed, moved, or "
                   "written. high/medium rows carry a proposed author; low and "
                   "none rows must be decided by a human."),
        "reason": a.reason,
        "provider": "openlibrary.org",
        "distinct_books": len(rows),
        "confidence_counts": dict(counts),
        "proposed_by_author": dict(by_author.most_common()),
        "rows": rows,
    }
    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(json.dumps(report, indent=2))

    print(json.dumps({
        "distinct_books": len(rows),
        "confidence_counts": dict(counts),
        "proposed_by_author": dict(by_author.most_common(12)),
        "out": str(a.out),
    }, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
