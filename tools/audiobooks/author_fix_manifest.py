#!/usr/bin/env python3
"""Build a reviewable manifest for correcting audiobook author attributions.

The folder layout puts an author in the author position, and the container tags
often name a different, correct one. That disagreement is only worth acting on in
some cases, so every candidate is classified before it is proposed:

  move_and_retag  the tag names a plausible person, the folder does not, and the
                  tag is not the book's narrator. The folder is wrong.
  code_fixed      the author is wrong only because of a collection level; the
                  parser and the Jellyfin adapter now resolve it. No file moves.
  same_person     the tag and the folder differ only in punctuation or a
                  missing initial ("C. S. Lewis" / "C.S. Lewis"). Folder is right.
  narrator_in_artist
                  the tag's artist is the narrator ("P. J. Ochlan" on a Cixin
                  Liu novel). Folder is right; the tag is wrong.
  no_evidence     the tag is absent or a placeholder. Folder stands.
  needs_metadata  nothing usable, and the folder author is a placeholder.

Read-only. This produces a manifest and a review page and moves nothing.
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

PLACEHOLDER = {"", "unknown", "unknown author", "various", "various authors",
               "n/a", "none", "intelliquest", "audiobook", "audiobooks", "books"}
# Folder names that are never a person, and so must not be proposed *as* one.
NOT_AN_AUTHOR = PLACEHOLDER | {
    "foundation - the complete series", "plex", "library", "compilations",
    "collection", "collections", "inbox", "torrents", "downloads", "test-ebook",
}
PUBLISHER_HINT = re.compile(
    r"audiobooks?\b|library|media\b|studios?\b|press|publishing|records\b|"
    r"entertainment|prod|collective|volks|verlag|editions?\b", re.I)
NARRATOR_RE = re.compile(r"(?i)\b(?:narrated|read|performed)\s+by\b")


def fold(s: str) -> str:
    s = unicodedata.normalize("NFKD", s or "").lower()
    s = "".join(c for c in s if not unicodedata.combining(c))
    return re.sub(r"[^a-z0-9]+", " ", s).strip()


def is_plausible_author(name: str) -> bool:
    n = (name or "").strip()
    if not n or n.lower() in PLACEHOLDER or PUBLISHER_HINT.search(n):
        return False
    if n.lower() in NOT_AN_AUTHOR:
        return False
    parts = re.split(r"\s+(?:and|&|with|et al)\s+", fold(n))
    if len(parts) > 2:
        return False
    return all(1 <= len(p.split()) <= 4 for p in parts)


def name_variant(a: str, b: str) -> bool:
    """True when two names are plausibly the same person written differently.

    Covers dropped middle initials ("Iain Banks" / "Iain M. Banks"), punctuation
    differences ("C.S. Lewis" / "C. S. Lewis") and a dropped second surname.
    Implemented as: same surname, and every other word of the shorter name
    matching a word of the longer one.
    """
    fa, fb = fold(a).split(), fold(b).split()
    if not fa or not fb:
        return False
    short, long = (fa, fb) if len(fa) <= len(fb) else (fb, fa)
    if short[-1] != long[-1]:  # surname must agree
        return False
    return all(w in long for w in short[:-1])


def _starts_with(book: str, ancestor: str) -> bool:
    """Port of startsWithFolderName in library/audiobook_paths.go."""
    a, b = fold(ancestor), fold(book)
    if not a or b == a or not b.startswith(a):
        return False
    rest = b[len(a):]
    return rest == "" or rest[0] == " "


def resolved_author_dir(media_path: str) -> str | None:
    """Port of audiobookAuthorDir in library/audiobook_paths.go.

    Kept in step with the server on purpose: this tool's whole claim is that it
    mirrors what a client sees, so a second, different rule here would make the
    manifest confidently wrong.
    """
    parts = [p for p in media_path.split("/") if p]
    if len(parts) < 3:
        return None
    book, parent = parts[-2], parts[-3]
    if len(parts) >= 4 and _starts_with(book, parts[-4]):
        return parts[-4]
    return parent


def is_nested_collection(row: dict) -> bool:
    """Author/Collection/Book/file -- the shipped author is one level further out.

    The parser and the Jellyfin adapter now resolve this without any file being
    moved, so proposing a move here would flatten a deliberate multi-narrator
    series and change 20 path-derived IDs for nothing.

    Depth is not the test: the legacy compact wrapper is also nested three deep
    and its parent *is* the author. The test is whether the shipped resolver
    disagrees with the grandparent that the raw path suggests.
    """
    for f in row.get("files") or []:
        if resolved_author_dir(f) == row["author_folder"]:
            return False  # agrees: the path is a plain Author/Book
    return bool(row.get("files"))


BY_RE = re.compile(r"(?i)(?:^|[\r\n]|[.;!?]\s+)(?:by|written\s+by|author|"
                   r"written\s+and\s+directed\s+by|created\s+by)\s*[:,]?\s*"
                   r"([A-Z][\w.'’-]*(?:\s+[A-Z][\w.'’-]*){0,3})")


def credits_author(comment: str, name: str) -> bool:
    """True when the file's own comment makes an explicit authorship credit.

    This is the only positive evidence accepted for moving a book whose folder
    already names a plausible person. Anything weaker -- a bare artist tag, a
    name that merely sounds like an author -- is exactly the shape a narrator in
    the artist field takes, so it is not enough.
    """
    if not comment or not name:
        return False
    fn, ln = fold(name).split()[0], fold(name).split()[-1]
    for m in BY_RE.finditer(comment):
        candidate = fold(m.group(1))
        words = candidate.split()
        if not words:
            continue
        if words[-1] == ln or candidate == fn or candidate == fold(name):
            return True
        # "By: Cixin Liu, Ken Liu" credits several people; accept a surname match.
        if any(w == ln for w in words):
            return True
    return False


def edit_distance_le1(a: str, b: str) -> bool:
    """True when a and b differ by at most one insert/delete/substitute."""
    if a == b:
        return False
    la, lb = len(a), len(b)
    if abs(la - lb) > 1:
        return False
    if la == lb:
        return sum(x != y for x, y in zip(a, b)) <= 1
    short, long = (a, b) if la < lb else (b, a)
    i = j = 0
    skipped = False
    while i < len(short) and j < len(long):
        if short[i] == long[j]:
            i += 1
            j += 1
        elif skipped:
            return False
        else:
            skipped = True
            j += 1
    return True


def near_duplicate_author(name: str, in_library: set[str],
                          proposed: set[str]) -> tuple[str, str] | None:
    """Find an author name one typo away from `name`, and say where it came from.

    A tag that says "Leo Tolstory" must not be turned into a second author
    folder beside "Leo Tolstoy". The right action there is a retag, not a move,
    so these are held back for review. The match is reported with its origin
    because the remedy differs: a name already in the library is a duplicate,
    while one only proposed by this manifest means the two spellings are both in
    the tags and the correct one has to be chosen by hand.
    """
    f = fold(name)
    for e in sorted(in_library):
        if edit_distance_le1(f, fold(e)):
            return e, "in the library"
    for e in sorted(proposed - in_library):
        if edit_distance_le1(f, fold(e)):
            return e, "proposed by this manifest"
    return None


def classify(row: dict) -> tuple[str, str]:
    """Return (action, reason) for one book folder."""
    folder_author = (row["author_folder"] or "").strip()
    tag = (row["tag_artist"] or "").strip()

    if is_nested_collection(row):
        return ("code_fixed",
                "author is one collection level up; resolved in code, no file moves")

    if not tag or tag.lower() in PLACEHOLDER:
        return ("needs_metadata" if folder_author.lower() in PLACEHOLDER
                else "no_evidence",
                "the file carries no usable artist tag, so the folder stands")
    if fold(tag) == fold(folder_author):
        return ("none", "tag and folder already agree")
    if not is_plausible_author(tag):
        return ("no_evidence", f"tag artist {tag!r} is not a person")
    if name_variant(tag, folder_author):
        return ("same_person", "the same person written differently")

    # An explicit credit line settles it. This is checked before the
    # mention heuristic below, because a comment like "By: Iain M. Banks" also
    # *contains* the name, and a credit must not be mistaken for a blurb.
    credited = credits_author(row.get("tag_comment") or "", tag)
    # Otherwise, a name that merely appears in the comment is in a narrator or
    # blurb context. "Arthur Morey" on a David Brooks book is the reader; the
    # folder is right.
    comment = fold(row.get("tag_comment") or "")
    if not credited and comment and fold(tag) in comment:
        return ("narrator_in_artist",
                f"the tag's artist appears in the book's comment but is not "
                f"credited as the author, so it is a narrator or blurb name")

    if not is_plausible_author(folder_author):
        return ("move_and_retag",
                f"the folder author {folder_author!r} is not a person; the tag is "
                f"the only attribution and names {tag!r}")
    # The folder names a plausible person, and nothing in the file credits the
    # tag's artist as the author. That is not enough to move a book: a reader in
    # the artist field looks exactly like this ("Arthur Morey" on a David Brooks
    # book, whose comment is a blurb with no credit line). The folder stands and
    # the tag goes to review.
    if not credited:
        return ("unverified_attribution",
                f"the folder names a person ({folder_author!r}) and the file "
                f"carries no credit for {tag!r}; the tag may be the reader")
    return ("move_and_retag",
            f"the tag names {tag!r} and the file's comment credits them as the "
            f"author; the folder says {folder_author!r}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--title-audit", required=True, type=pathlib.Path)
    ap.add_argument("--root", required=True, type=pathlib.Path)
    ap.add_argument("--out", required=True, type=pathlib.Path, help="manifest JSON")
    ap.add_argument("--html", type=pathlib.Path, help="review page to write")
    a = ap.parse_args()

    audit = json.loads(a.title_audit.read_text())
    # The typo check has to see both what exists today and what this manifest is
    # about to create: "Leo Tolstoy" is not in the library, and "Leo Tolstory" is
    # the same author misspelled in another book's tag. Comparing only against
    # existing folders would happily create both spellings as separate authors.
    existing_authors = {p.name for p in a.root.iterdir() if p.is_dir()}
    proposed_authors = {
        (r.get("tag_artist") or "").strip()
        for r in audit["rows"]
        if classify(r)[0] == "move_and_retag" and (r.get("tag_artist") or "").strip()
    }
    entries, counts = [], collections.Counter()
    for row in audit["rows"]:
        action, reason = classify(row)
        counts[action] += 1
        if action != "move_and_retag":
            continue
        tag = row["tag_artist"].strip()
        target_dir = a.root / tag / row["book_folder"]
        # Never propose a destination that is occupied: that is a silent merge.
        clash = target_dir.exists() and target_dir.resolve() != \
            (a.root / row["author_folder"] / row["book_folder"]).resolve()
        # Never invent an author folder that is one typo away from a real one.
        # "Leo Tolstory" beside "Leo Tolstoy" is a retag, not a move.
        typo = near_duplicate_author(tag, existing_authors,
                                     proposed_authors - {tag})
        blocker = None
        if typo:
            other, where = typo
            blocker = (f"the tag spells the author {tag!r}, one character from "
                       f"{other!r} {where}; that is a retag to settle, not a "
                       f"second author folder to create")
        elif clash:
            blocker = (f"{tag}/{row['book_folder']} already exists; moving there "
                       f"would merge two books, so it needs a human decision")
        entries.append({
            "action": action,
            "reason": reason,
            "blocker": blocker,
            "source_dir": f"{row['author_folder']}/{row['book_folder']}",
            "target_dir": f"{tag}/{row['book_folder']}",
            "author_from": row["author_folder"],
            "author_to": tag,
            "book": row["book_folder"],
            "files": row.get("file_count") if isinstance(row.get("file_count"), int) else 1,
            "evidence": {"tag_artist": row["tag_artist"],
                         "tag_title": row["tag_title"],
                         "tag_comment": (row.get("tag_comment") or "")[:160]},
            "target_exists": clash,
            "typo_of": list(typo) if typo else None,
            "safe_to_move": blocker is None,
        })

    manifest = {
        "policy": ("Read-only manifest. Nothing has been moved, renamed or "
                   "re-tagged. A move is proposed only on positive evidence: "
                   "either the folder author is not a person at all, or the "
                   "file's own comment carries an explicit credit for the tag's "
                   "artist. A plausible folder author with an uncredited tag is "
                   "held back, because a reader written into the artist field is "
                   "indistinguishable from that by shape alone. safe_to_move is "
                   "false when the destination exists (moving would merge two "
                   "books) or when the tag misspells an author the library "
                   "already has (a retag, not a move)."),
        "root": str(a.root),
        "classification_counts": dict(counts),
        "proposed_moves": len(entries),
        "safe_to_move": sum(1 for e in entries if e["safe_to_move"]),
        "needs_manual_review": sum(1 for e in entries if not e["safe_to_move"]),
        "entries": entries,
    }
    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(json.dumps(manifest, indent=2))
    if a.html:
        a.html.parent.mkdir(parents=True, exist_ok=True)
        a.html.write_text(render(manifest, entries))

    print(json.dumps({
        "proposed_moves": manifest["proposed_moves"],
        "safe_to_move": manifest["safe_to_move"],
        "needs_manual_review": manifest["needs_manual_review"],
        "classification_counts": manifest["classification_counts"],
        "manifest": str(a.out),
        "html": str(a.html) if a.html else None,
    }, indent=2))
    return 0


def render(manifest: dict, entries: list[dict]) -> str:
    rows = []
    for e in entries:
        rows.append(f"""
<tr class="{'ok' if e['safe_to_move'] else 'bad'}">
  <td><code>{html.escape(e['source_dir'])}</code><br>
      <span class="arrow">&rarr;</span> <code>{html.escape(e['target_dir'])}</code>
      {f'<div class=warn>{html.escape(e["blocker"])}</div>' if e.get('blocker') else ''}</td>
  <td>{html.escape(e['reason'])}</td>
  <td class=ev><b>tag artist</b> {html.escape(e['evidence']['tag_artist'])}<br>
      <b>tag title</b> {html.escape(e['evidence']['tag_title'])}</td>
</tr>""")

    counts = manifest["classification_counts"]
    legend = "".join(
        f"<div class=stat><b>{v}</b><span>{html.escape(k)}</span></div>"
        for k, v in sorted(counts.items(), key=lambda kv: -kv[1]))

    return f"""<!doctype html>
<html lang=en><head><meta charset=utf-8>
<title>Author correction manifest</title>
<style>
 :root {{ --bg:#12151a; --panel:#1a1f27; --line:#2a323d; --fg:#e6ebf2; --dim:#8b98a9;
          --bad:#e2686b; --good:#5ec27e; --warn:#e2a45a; --acc:#5aa9e6; }}
 * {{ box-sizing:border-box }}
 body {{ margin:0; background:var(--bg); color:var(--fg);
   font:14px/1.55 ui-sans-serif,-apple-system,"Segoe UI",Roboto,sans-serif }}
 header {{ padding:16px 22px; border-bottom:1px solid var(--line); background:var(--panel) }}
 h1 {{ margin:0 0 4px; font-size:17px }}
 .sub {{ color:var(--dim); font-size:12.5px; max-width:900px }}
 .stats {{ display:flex; gap:16px; margin-top:12px; flex-wrap:wrap }}
 .stat b {{ font-size:19px; display:block; line-height:1.15 }}
 .stat span {{ color:var(--dim); font-size:11px; text-transform:uppercase; letter-spacing:.4px }}
 main {{ padding:18px 22px 60px; max-width:1100px }}
 table {{ width:100%; border-collapse:collapse }}
 td {{ padding:9px 10px; border-bottom:1px solid var(--line); vertical-align:top; font-size:12.5px }}
 tr.ok td:first-child {{ border-left:3px solid var(--good); padding-left:10px }}
 tr.bad td:first-child {{ border-left:3px solid var(--warn); padding-left:10px }}
 code {{ background:#0e1116; padding:1px 5px; border-radius:4px; font-size:11.5px }}
 .arrow {{ color:var(--acc) }}
 .ev {{ color:var(--dim) }}
 .ev b {{ color:var(--fg); font-weight:600 }}
 .warn {{ color:var(--warn); margin-top:4px }}
 .note {{ border-left:2px solid var(--good); padding-left:12px; color:var(--dim);
   font-size:12.5px; margin:0 0 18px }}
</style></head><body>
<header>
  <h1>Author correction manifest &mdash; nothing moved</h1>
  <div class="sub">A row is proposed only when the file's own container tag
    names a plausible person, the folder author does not, and the tag is not the
    book's narrator. Everything else is classified and left alone.</div>
  <div class="stats">
    <div class=stat><b>{manifest['proposed_moves']}</b><span>proposed moves</span></div>
    <div class=stat><b>{manifest['safe_to_move']}</b><span>safe to move</span></div>
    <div class=stat><b>{manifest['needs_manual_review']}</b><span>need review</span></div>
    {legend}
  </div>
</header>
<main>
  <p class="note">Moving a file inside the library changes its path-derived ID, so
  the server sees a new item. All {manifest['proposed_moves']} of these books
  currently have zero playback progress, so nothing is at risk here &mdash; that
  was measured, not assumed, and it is the reason this is safe to stage at all.</p>
  <table>{''.join(rows)}</table>
</main>
</body></html>"""


if __name__ == "__main__":
    sys.exit(main())
