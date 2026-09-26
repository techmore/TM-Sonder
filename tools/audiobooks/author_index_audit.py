#!/usr/bin/env python3
"""Audit the author index a BookPlayer-style client will see.

BookPlayer does not read /api/audiobooks. It reads the Jellyfin compatibility
endpoints, and its author index is built by /Artists/AlbumArtists, which is:

    jellyfinAuthor(item) = item.Author, else filepath.Base(dirname(dirname(path)))
    index                = the distinct set of those, in store order

Three consequences this audit exists to make visible:

1. **No junk filtering.** Whatever is in the author position becomes an author.
   "Unknown Author", "Various" and a publisher name like "IntelliQuest" are
   all indistinguishable from a real person.
2. **The folder is the author of last resort.** When a book's own tags name the
   real author but the folder does not, the folder wins in the index, and the
   tag is never consulted.
3. **The index is not sorted server-side.** It is emitted in store order.

Read-only. It reuses an existing title-audit.json rather than re-probing, and it
writes a report; it never renames, moves, re-tags or deletes anything.
"""

from __future__ import annotations

import argparse
import collections
import html
import json
import pathlib
import re
import sys
import unicodedata

# Folder names that are never a person. These are exactly the values
# jellyfinAuthor() will happily surface as an "author".
NOT_A_PERSON = {
    "unknown author", "unknown", "various", "various authors", "n/a",
    "none", "audiobook", "audiobooks", "books", "book", "library", "plex",
    "intelliquest", "test-ebook", "compilations", "collection", "collections",
    "inbox", "torrents", "downloads", "downloaded",
}
# Names that look like a publisher, imprint or distributor rather than a person.
PUBLISHER_HINT = re.compile(
    r"audiobooks?\b|library|media\b|books?\s+(inc|ltd|llc|group)|"
    r"studios?\b|press|publishing|records\b|entertainment|prod(uctions)?\b|"
    r"collective|volks|verlag|editions?\b", re.I)


def norm(s: str) -> str:
    s = unicodedata.normalize("NFKD", s or "").lower()
    s = re.sub(r"[^a-z0-9]+", " ", s)
    return re.sub(r"\s+", " ", s).strip()


def is_person_name(name: str) -> bool:
    """A weak but explicit shape check, used only to raise a flag."""
    n = norm(name)
    if not n:
        return False
    if n in NOT_A_PERSON or PUBLISHER_HINT.search(name):
        return False
    # Co-authored entries are legitimate authors: "Larry Niven and Jerry
    # Pournelle", "Stephen King and Peter Straub". Counting words to decide
    # "is this a person" flagged all six of them, so a joined pair is split
    # first and each half is judged on its own.
    parts = re.split(r"\s+(?:and|&|with|et al)\s+", n)
    if len(parts) > 2:
        return False
    for part in parts:
        words = part.split()
        if not words or len(words) > 4 or any(len(w) > 18 for w in words):
            return False
    return True


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--title-audit", required=True, type=pathlib.Path,
                    help="output of title_audit.py")
    ap.add_argument("--out", required=True, type=pathlib.Path,
                    help="HTML report to write")
    ap.add_argument("--json-out", type=pathlib.Path, help="also write JSON")
    a = ap.parse_args()

    audit = json.loads(a.title_audit.read_text())
    rows = audit["rows"]

    # A book folder's author in the index is the *parent* of the author folder,
    # i.e. the second-to-last path segment; the deepest folder is the book.
    def index_author(row: dict) -> str:
        parts = row["relative"].split("/")
        return parts[-2] if len(parts) >= 2 else ""

    books: dict[str, list[dict]] = collections.defaultdict(list)
    for r in rows:
        books[index_author(r)].append(r)

    # Cross-author signals that need more than one row to spot.
    series_authors: dict[str, set] = collections.defaultdict(set)
    for r in rows:
        for s in audit.get("series_claims", {}):
            if any(b["relative"] == r["relative"] for b in audit["series_claims"][s]["books"]):
                series_authors[s].add(index_author(r))
    narrator_authors: dict[str, set] = collections.defaultdict(set)
    for r in rows:
        n = r.get("narrator_from_folder")
        if n:
            narrator_authors[n].add(index_author(r))

    report = []
    for author, items in sorted(books.items(), key=lambda kv: -len(kv[1])):
        flags: list[str] = []
        if not author:
            flags.append("no_author_folder")
        elif norm(author) in NOT_A_PERSON:
            flags.append("not_a_person_placeholder")
        elif not is_person_name(author):
            flags.append("looks_like_publisher_or_imprint")

        conflicts = [r for r in items
                     if "tag_artist_differs_from_folder" in r["issues"]]
        if conflicts:
            flags.append("tag_names_a_different_author")
        untitled = [r for r in items if "missing_author_folder" in r["issues"]]
        if untitled:
            flags.append("missing_author_folder")

        # A narrator that also reads for another author is a mis-file signal:
        # Podehl reads five John Brunner books filed under Philip K. Dick.
        narrator_hits = []
        for r in items:
            n = r.get("narrator_from_folder")
            if n and len(narrator_authors.get(n, ())) > 1:
                others = sorted(a2 for a2 in narrator_authors[n] if a2 != author)
                if others:
                    narrator_hits.append({"narrator": n, "also_under": others,
                                          "book": r["book_folder"]})

        report.append({
            "author": author,
            "books": len(items),
            "flags": flags,
            "flag_notes": {
                "tag_names_a_different_author": [
                    {"folder": r["book_folder"], "tag_artist": r["tag_artist"]}
                    for r in conflicts
                ],
                "narrator_shared_with_other_authors": narrator_hits,
            },
            "items": [{"book": r["book_folder"], "relative": r["relative"],
                       "tag_artist": r["tag_artist"],
                       "narrator": r.get("narrator_from_folder", ""),
                       "issues": r["issues"]} for r in items],
        })

    total_books = sum(e["books"] for e in report)
    suspect = [e for e in report if e["flags"]]
    flag_counts = collections.Counter(
        f for e in report for f in e["flags"])

    payload = {
        "policy": ("Read-only audit of the author index a Jellyfin-compatible "
                   "client (BookPlayer) sees. Mirrors jellyfinAuthor(): the "
                   "item author, else the raw author folder, with no junk "
                   "filtering and no server-side sort. Nothing was changed."),
        "index_authors": len(report),
        "books": total_books,
        "authors_with_flags": len(suspect),
        "flag_counts": dict(flag_counts),
        "entries": report,
    }
    if a.json_out:
        a.json_out.parent.mkdir(parents=True, exist_ok=True)
        a.json_out.write_text(json.dumps(payload, indent=2))

    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(render(payload, report))
    print(json.dumps({
        "index_authors": len(report),
        "books": total_books,
        "authors_with_flags": len(suspect),
        "flag_counts": dict(flag_counts),
        "html": str(a.out),
    }, indent=2))
    return 0


def render(payload: dict, entries: list[dict]) -> str:
    cards = []
    for e in entries:
        if not e["flags"]:
            continue
        notes = ""
        conflict = e["flag_notes"]["tag_names_a_different_author"]
        if conflict:
            rows = "".join(
                f"<li><code>{html.escape(c['folder'])}</code> &rarr; tag says "
                f"<b>{html.escape(c['tag_artist'])}</b></li>"
                for c in conflict[:12])
            more = (f"<li class=dim>&hellip; {len(conflict) - 12} more</li>"
                    if len(conflict) > 12 else "")
            notes += (f"<details><summary>{len(conflict)} book(s) whose tag "
                      f"names a different author</summary><ul>{rows}{more}</ul></details>")
        shared = e["flag_notes"]["narrator_shared_with_other_authors"]
        if shared:
            rows = "".join(
                f"<li><code>{html.escape(s['book'])}</code> &mdash; narrator "
                f"{html.escape(s['narrator'])} also reads for "
                f"{html.escape(', '.join(s['also_under']))}</li>"
                for s in shared[:12])
            notes += f"<details><summary>{len(shared)} book(s) share a narrator with another author</summary><ul>{rows}</ul></details>"
        flags = "".join(f'<span class=flag>{html.escape(f)}</span>' for f in e["flags"])
        books = ", ".join(html.escape(i["book"]) for i in e["items"][:6])
        if len(e["items"]) > 6:
            books += f" &hellip; {len(e['items']) - 6} more"
        cards.append(f"""
<section class=author>
  <h2>{html.escape(e['author'] or '(no author folder)')} <span class=count>{e['books']} book(s)</span></h2>
  <div class=flags>{flags}</div>
  <p class=books>{books}</p>
  {notes}
</section>""")

    all_authors = "".join(
        f"<tr><td>{html.escape(e['author'] or '(none)')}</td>"
        f"<td class=n>{e['books']}</td>"
        f"<td>{' '.join(f'<span class=flag>{html.escape(f)}</span>' for f in e['flags'])}</td></tr>"
        for e in sorted(entries, key=lambda x: -x["books"]))

    return f"""<!doctype html>
<html lang=en><head><meta charset=utf-8>
<title>Author index audit</title>
<style>
 :root {{ --bg:#12151a; --panel:#1a1f27; --line:#2a323d; --fg:#e6ebf2; --dim:#8b98a9;
          --bad:#e2686b; --warn:#e2a45a; --acc:#5aa9e6; }}
 * {{ box-sizing:border-box }}
 body {{ margin:0; background:var(--bg); color:var(--fg);
   font:14px/1.55 ui-sans-serif,-apple-system,"Segoe UI",Roboto,sans-serif }}
 header {{ position:sticky; top:0; z-index:5; background:var(--panel);
   border-bottom:1px solid var(--line); padding:14px 22px }}
 h1 {{ margin:0 0 4px; font-size:17px }}
 .sub {{ color:var(--dim); font-size:12.5px }}
 .stats {{ display:flex; gap:18px; margin-top:10px; flex-wrap:wrap }}
 .stat b {{ font-size:20px; display:block; line-height:1.2 }}
 .stat span {{ color:var(--dim); font-size:11.5px; text-transform:uppercase; letter-spacing:.4px }}
 main {{ padding:18px 22px 60px; max-width:1100px }}
 h2 {{ font-size:15px; margin:0 0 4px }}
 .count {{ color:var(--dim); font-weight:400; font-size:12.5px }}
 section.author {{ border:1px solid var(--line); border-left:3px solid var(--bad);
   border-radius:8px; padding:12px 14px; margin-bottom:10px; background:var(--panel) }}
 .flag {{ display:inline-block; background:#2a1c1d; color:var(--bad);
   border:1px solid var(--bad); border-radius:999px; padding:0 8px;
   font-size:11px; margin:2px 4px 0 0 }}
 .books {{ color:var(--dim); font-size:12.5px; margin:6px 0 0 }}
 details {{ margin-top:8px }}
 summary {{ cursor:pointer; color:var(--acc); font-size:12.5px }}
 ul {{ margin:6px 0 0; padding-left:18px }}
 li {{ margin:2px 0; font-size:12.5px }}
 code {{ color:var(--fg); background:#0e1116; padding:1px 5px; border-radius:4px }}
 .dim {{ color:var(--dim) }}
 table {{ width:100%; border-collapse:collapse; margin-top:6px }}
 td {{ padding:5px 8px; border-bottom:1px solid var(--line); font-size:12.5px }}
 td.n {{ text-align:right; color:var(--dim); font-variant-numeric:tabular-nums }}
 .note {{ border-left:2px solid var(--warn); padding-left:12px; color:var(--dim);
   font-size:12.5px; max-width:900px; margin:0 0 18px }}
</style></head><body>
<header>
  <h1>Author index audit &mdash; what BookPlayer sees</h1>
  <div class="sub">Mirrors <code>jellyfinAuthor()</code>: the item author, else the
    raw author folder. No junk filtering, no server-side sort. Read-only.</div>
  <div class="stats">
    <div class=stat><b>{payload['index_authors']}</b><span>index entries</span></div>
    <div class=stat><b>{payload['books']}</b><span>books</span></div>
    <div class=stat><b>{payload['authors_with_flags']}</b><span>authors flagged</span></div>
    {"".join(f'<div class=stat><b>{v}</b><span>{html.escape(k)}</span></div>' for k, v in sorted(payload['flag_counts'].items()))}
  </div>
</header>
<main>
  <p class="note">An entry is flagged when the name in the author position is a
  placeholder or looks like a publisher rather than a person, when the book's
  own container tag names a <em>different</em> author, or when a narrator it
  shares also reads for another author (which is how five John Brunner books
  filed under Philip K. Dick surface). The index is faithful to the data &mdash;
  the data is what is wrong, so nothing here can be fixed in code alone.</p>
  {"".join(cards) if cards else '<p>No authors flagged.</p>'}
  <h2 style="margin-top:26px">Full index</h2>
  <table>{all_authors}</table>
</main>
</body></html>"""


if __name__ == "__main__":
    sys.exit(main())
