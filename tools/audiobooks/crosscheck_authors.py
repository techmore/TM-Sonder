#!/usr/bin/env python3
"""Cross-check proposed audiobook authors against the ebook library.

The ebook library and the audiobook library are independent acquisitions of
overlapping books, and the ebooks follow a ``Title - Author`` filename
convention. That makes the ebook title an *independent* witness to a proposed
audiobook author, which matters because a fuzzy metadata lookup alone produced
plausible-looking but wrong answers ("Cycle of the Werewolf" -> Stephen King
rather than Philip K. Dick).

Read-only: this only reads the library API and the proposal JSON. It never
renames, moves, or writes anything. Output is a review artifact.

Agreement rules:
  confirmed   the ebook and the proposal name the same author
  contradicted the ebook names a different author (a near-certainty the
              proposal is wrong)
  unsupported the ebooks have nothing to say, so the proposal stands alone
"""

from __future__ import annotations

import argparse
import collections
import difflib
import json
import pathlib
import re
import sys
import unicodedata
import urllib.request

# "A Maze of Death - Philip K  Dick" / "Mein Kampf - Adolf Hitler"
AUTHOR_SUFFIX = re.compile(r"^(?P<title>.+?)\s+[-–—]\s+(?P<author>[^-–—]{2,60})$")


def normalize(s: str) -> str:
    s = unicodedata.normalize("NFKD", s or "")
    s = "".join(c for c in s if not unicodedata.combining(c))
    s = re.sub(r"\([^)]*\)", " ", s.lower())
    s = re.sub(r"[^a-z0-9]+", " ", s)
    return re.sub(r"\s+", " ", s).strip()


def similar(a: str, b: str) -> float:
    na, nb = normalize(a), normalize(b)
    if not na or not nb:
        return 0.0
    return difflib.SequenceMatcher(None, na, nb).ratio()


def same_author(a: str, b: str) -> bool:
    if not a or not b:
        return False
    if similar(a, b) >= 0.9:
        return True
    # "Philip K. Dick" vs "Philip Dick" vs "Philip K Dick"
    return normalize(a).replace(" ", "") == normalize(b).replace(" ", "")


def load_ebooks(api_base: str, token: str) -> list[dict]:
    url = f"{api_base}/api/library?token={token}"
    with urllib.request.urlopen(url, timeout=300) as r:
        data = json.loads(r.read())
    items = data.get("items", data)
    out = []
    for it in items:
        if it.get("kind") != "ebook":
            continue
        title = (it.get("title") or "").strip()
        author = (it.get("author") or "").strip()
        if author in ("", "pdf", "HTML", "html", "txt", "epub", "djvu"):
            author = ""
        m = AUTHOR_SUFFIX.match(title)
        if m:
            # "Title - Author" is the library's own convention and is more
            # reliable than the parsed author field, which is often the file
            # extension.
            title, author = m.group("title").strip(), m.group("author").strip()
        # A leading "- " is a leftover sort prefix on some files.
        title = re.sub(r"^[-–—]\s*", "", title).strip()
        out.append({"title": title, "author": author})
    return out


def crosscheck(ebooks: list[dict], book: str) -> list[dict]:
    """Return the ebook records that describe the same book.

    The threshold is deliberately strict. A loose one produces confident
    nonsense: at 0.72 "State of Fear" matched the ebook "State of the Art" and
    "Cycle of the Werewolf" matched "Curse of the Werewolf", which turns a
    cross-check into a source of false contradictions. Short titles need near
    identity, so the bar rises as the title gets shorter.
    """
    nb = normalize(book)
    # Require near-identity for short titles, where one wrong word changes the
    # book entirely, and slightly less for long descriptive titles.
    threshold = 0.92 if len(nb.split()) <= 4 else 0.85
    hits = []
    for e in ebooks:
        r = similar(book, e["title"])
        if r >= threshold and e["author"]:
            hits.append({"title": e["title"], "author": e["author"],
                         "match": round(r, 3)})
    hits.sort(key=lambda h: h["match"], reverse=True)
    return hits[:3]


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("proposal", type=pathlib.Path, help="author-proposal.json")
    p.add_argument("--api", default="http://127.0.0.1:8097")
    p.add_argument("--token", default="", help="pairing token (or set SONDER_TOKEN)")
    p.add_argument("--out", required=True, type=pathlib.Path)
    a = p.parse_args()

    token = a.token
    if not token:
        cfg = pathlib.Path.home() / ".config/sonder/server.json"
        token = json.loads(cfg.read_text()).get("pairingToken", "") if cfg.exists() else ""
    if not token:
        p.error("no API token: pass --token or ensure ~/.config/sonder/server.json exists")

    prop = json.loads(a.proposal.read_text())
    ebooks = load_ebooks(a.api, token)
    print(f"loaded {len(ebooks)} ebooks", file=sys.stderr)

    rows, verdicts = [], collections.Counter()
    for r in prop["rows"]:
        hits = crosscheck(ebooks, r["book"])
        proposed = r.get("proposed_author", "")
        ebook_authors = sorted({h["author"] for h in hits})
        if not hits:
            verdict = "unsupported"
        elif proposed and any(same_author(proposed, ea) for ea in ebook_authors):
            verdict = "confirmed"
        elif proposed and ebook_authors:
            verdict = "contradicted"
        else:
            verdict = "unsupported"
        verdicts[verdict] += 1
        rows.append({
            "book": r["book"],
            "provider_confidence": r["confidence"],
            "proposed_author": proposed,
            "ebook_authors": ebook_authors,
            "ebook_titles": [h["title"] for h in hits],
            "verdict": verdict,
        })

    report = {
        "policy": ("Read-only cross-check. Nothing was renamed, moved, or "
                   "written. 'contradicted' means the ebook library names a "
                   "different author and the proposal must not be used."),
        "proposal": str(a.proposal),
        "verdict_counts": dict(verdicts),
        "rows": rows,
    }
    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(json.dumps(report, indent=2))

    print(f"\nverdicts: {dict(verdicts)}\n")
    for r in rows:
        if r["verdict"] != "confirmed":
            print(f"  [{r['verdict']:12}] {r['book']}")
            if r["proposed_author"]:
                print(f"       provider said: {r['proposed_author']}")
            if r["ebook_authors"]:
                print(f"       ebooks say:    {r['ebook_authors']}  "
                      f"(from {r['ebook_titles']})")
    print(f"\nconfirmed: "
          f"{sum(1 for r in rows if r['verdict'] == 'confirmed')}/{len(rows)}")
    print(f"wrote {a.out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
