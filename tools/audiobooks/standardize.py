#!/usr/bin/env python3
"""Plan (and, only on explicit request, execute) audiobook layout standardization.

Target layout for every book:

    <root>/<Author>/<Book>/<Book>.m4b

The unit of decision is the book *folder*, not the file: a folder holding one
audio file is a candidate for a safe rename, while a folder holding several
files may be a multi-part book, a series collection, or two competing editions.
Those are different problems, so they are reported separately and never guessed
at. The planner is strictly read-only unless ``--apply`` is passed, and even
then it only renames a single unambiguous file to a free path -- it never
deletes, overwrites, merges, re-encodes, or re-tags anything.

Usage:
    standardize.py <root> --out plan.json            # plan only (default)
    standardize.py <root> --out plan.json --apply    # apply safe renames only
    standardize.py <root> --out plan.json --limit 50  # cap the applied renames
"""

from __future__ import annotations

import argparse
import collections
import json
import os
import pathlib
import re
import sys
import unicodedata

WRAPPER_PARTS = ("M4B Forge Compact", "compact-m4b-80k")
UNKNOWN_AUTHOR = {"unknown author", "unknown", "various", "various authors", ""}
AUDIO_EXT = {".m4b", ".m4a", ".mp3", ".ogg", ".opus", ".flac", ".wav", ".aax", ".aaxz"}

# Trailing tool/edition noise that is safe to drop from a canonical stem.
#
# Bracketed text is NOT unconditionally noise: "[64kbps]" is technical junk,
# but "[Unabridged]" or "[Full Cast]" is a real edition distinction. Merging
# those would collapse two genuinely different recordings into one book, so
# only clearly technical brackets are stripped.
_TECHNICAL_BRACKET = re.compile(
    r"\[\s*"
    r"(?:\d{2,4}\s*k(?:bps)?|[\d.]+\s*[kmg]b|"
    r"audible|amazon|aax|abridged\s*-\s*\d+\s*h|"
    r"\d+\s*h(?:rs?|ours?)?\s*m?(?:in)?|source|r\d+|retag|"
    r"converted|reencoded|fixed|test)"
    r"\s*\]",
    re.I,
)
_EDITION_MARKERS = re.compile(
    r"\[\s*(unabridged|abridged|full cast|single[- ]narrator|"
    r"audiobook|audio book|graphic audio)\s*\]",
    re.I,
)

NOISE_PATTERNS = [
    (re.compile(r"\{[^}]*\}"), ""),                        # {audible-B08G9PBSFV}
    (_TECHNICAL_BRACKET, ""),                               # [64kbps], [417mb]
    (re.compile(r"\(\s*\d{2,4}\s*kbps\s*\)", re.I), ""),    # (64kbps)
    (re.compile(r"\s[-_]?\d{2,4}k\b", re.I), ""),            # " 128k", "-64k"
]

# Names that indicate one volume of a book delivered as many files.
PART_PATTERNS = [
    re.compile(r"\bpart\s*\d+\b", re.I),
    re.compile(r"\bpt\s*\d+\b", re.I),
    re.compile(r"\bcd\s*\d+\b", re.I),
    re.compile(r"\bdisc\s*\d+\b", re.I),
    re.compile(r"\bvol(?:ume)?\.?\s*\d+\b", re.I),
    re.compile(r"^\s*\d{1,3}\s*[-_ ]"),                     # "03 - Title"
    re.compile(r"[-_ ]\d{2}\s*[-_]\s*\d{2}\b"),            # "…-cd01-01"
    re.compile(r"[-_ ]\d{2,3}$"),                          # "… 05"
    re.compile(r"^\s*[a-z]{2,4}\d{1,2}[-_ ]?\d{1,2}\b", re.I),   # "PWE01-10", "cd2-01"
]

TEMP_MARKER = re.compile(r"\.sonder-retag\b", re.I)


def clean_name(name: str) -> str:
    """Normalize a book title into a stable folder/file stem."""
    out = unicodedata.normalize("NFC", name).strip()
    for pattern, repl in NOISE_PATTERNS:
        out = pattern.sub(repl, out)
    # An edition marker is meaningful and its brackets are kept, but the
    # spacing is normalized so "Dune[Unabridged]" and "Dune [Unabridged]"
    # resolve to the same name.
    out = _EDITION_MARKERS.sub(lambda m: f" [{m.group(1)}]", out)
    out = re.sub(r"\s+", " ", out).strip(" -_. ")
    return out or name.strip()


def is_unknown_author(name: str) -> bool:
    return unicodedata.normalize("NFC", name).strip().lower() in UNKNOWN_AUTHOR


def looks_like_parts(name: str) -> bool:
    return any(p.search(name) for p in PART_PATTERNS)


def under_wrapper(parts: tuple[str, ...]) -> bool:
    return parts[: len(WRAPPER_PARTS)] == WRAPPER_PARTS


def scan_files(root: pathlib.Path) -> list[pathlib.Path]:
    """Read-only walk. Dot files/dirs (AppleDouble ._Name.m4b, .Trashes) are skipped."""
    found: list[pathlib.Path] = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = sorted(d for d in dirnames if not d.startswith("."))
        for name in sorted(filenames):
            if name.startswith("."):
                continue
            if pathlib.Path(name).suffix.lower() in AUDIO_EXT:
                found.append(pathlib.Path(dirpath) / name)
    return found


def classify_folder(root: pathlib.Path, folder: pathlib.Path,
                    files: list[pathlib.Path], nested: bool) -> dict:
    """Decide what should happen to one book folder. Never touches the disk."""
    rel_folder = folder.relative_to(root)
    parts = rel_folder.parts
    wrapped = under_wrapper(parts)
    effective = parts[len(WRAPPER_PARTS):] if wrapped else parts

    reasons: list[str] = []
    if not wrapped and parts and parts[0] in WRAPPER_PARTS:
        # A half-applied unwrap: the wrapper directory name survived.
        reasons.append("partial_legacy_wrapper_path")
    if not effective:
        reasons.append("file_directly_under_library_root" if not wrapped
                       else "stray_file_under_wrapper")
    else:
        author = effective[0]
        if len(effective) == 1:
            reasons.append("file_directly_under_author_folder")
        if is_unknown_author(author):
            reasons.append("unknown_author_folder")
        if len(effective) > 2:
            # Author/Book/<collection>/: one folder level deeper than the target
            # layout, i.e. a series collection rather than a single book.
            reasons.append("series_collection_folder")

    entries = []
    for f in files:
        try:
            size = f.stat().st_size
        except OSError as exc:
            size = -1
            reasons.append(f"stat_failed: {exc.strerror or exc}")
        entries.append({
            "source": str(f),
            "relative_path": str(f.relative_to(root)),
            "size_bytes": size,
            "stem": clean_name(f.stem),
            "suffix": f.suffix.lower(),
            "looks_like_parts": looks_like_parts(f.stem),
            "temp_artifact": bool(TEMP_MARKER.search(f.name)),
        })
    if any(e["size_bytes"] == 0 for e in entries):
        reasons.append("zero_byte_file")

    # A multi-file book folder is a different problem from a rename. Classify
    # why it holds several files so the review list is actionable.
    group = "single_file"
    if "series_collection_folder" in reasons or nested:
        group = "series_collection"
    elif len(entries) > 1:
        if all(e["temp_artifact"] or e["size_bytes"] == 0 for e in entries[1:]):
            group = "single_book_plus_leftovers"
        elif all(e["looks_like_parts"] for e in entries):
            group = "multi_part_book"
        elif len({e["size_bytes"] for e in entries if e["size_bytes"] > 0}) == 1:
            group = "identical_size_duplicates"
        elif any(e["looks_like_parts"] for e in entries):
            group = "mixed_parts_and_extras"
        else:
            group = "multiple_editions"

    effective_book = clean_name(effective[1]) if len(effective) > 1 else ""
    want_name = f"{effective_book or 'book'}.m4b" if effective_book else None
    sole = entries[0] if len(entries) == 1 else None

    # The layout requires the book folder and the file inside it to share a
    # name, and players (and the parser) show the folder name, so a noisy
    # folder has to move too -- renaming only the file would leave the
    # canonical title wrong.
    book_dir_name = clean_name(folder.name) if not wrapped else effective_book
    folder_needs_move = bool(effective_book) and folder.name != (
        effective_book if not wrapped else folder.name)

    action = "keep"
    if reasons:
        action = "review"
    elif group == "multi_part_book":
        # Already in the right place; the layout rule simply does not apply.
        action = "keep_multi_part"
    elif group != "single_file":
        action = "review"
    elif folder_needs_move:
        action = "rename_book"
    elif want_name and sole and pathlib.Path(sole["source"]).name != want_name:
        action = "rename"

    # Where this book belongs in the canonical layout, whether or not a change
    # is currently proposed. Two folders claiming the same canonical directory
    # are competing copies and must not be silently merged.
    canonical = None
    if effective_book and not reasons:
        canonical = str(root / effective[0] / effective_book)
    target_dir = canonical if action == "rename_book" else None
    target_file = str(root / effective[0] / effective_book / want_name) if action == "rename" else None

    return {
        "folder": str(folder),
        "relative_folder": str(rel_folder),
        "author": effective[0] if effective else "",
        "book": effective_book,
        "book_dir_name": book_dir_name,
        "wrapped_in_legacy_wrapper": wrapped,
        "group": group,
        "action": action,
        "reasons": sorted(set(reasons)),
        "canonical_path": canonical,
        "target": target_dir or target_file,
        "target_dir": target_dir,
        "target_file": target_file,
        "files": entries,
    }


def build_plan(root: pathlib.Path) -> dict:
    grouped: dict[pathlib.Path, list[pathlib.Path]] = collections.defaultdict(list)
    for f in scan_files(root):
        grouped[f.parent].append(f)

    # A folder that directly contains further media folders is a collection, not
    # a single book; those need a different decision than a rename.
    def has_book_children(folder: pathlib.Path) -> bool:
        for other in grouped:
            if other != folder and folder in other.parents:
                return True
        return False

    folders = [classify_folder(root, folder, files, has_book_children(folder))
               for folder, files in grouped.items()]
    folders.sort(key=lambda x: x["relative_folder"])

    # Two folders that both claim the same canonical path (a wrapper copy beside
    # its already-unwrapped twin, or two editions of one book).
    by_canonical: dict[str, list[dict]] = collections.defaultdict(list)
    for f in folders:
        if f["canonical_path"]:
            by_canonical[f["canonical_path"]].append(f)
    collisions = {t: [x["folder"] for x in srcs]
                  for t, srcs in by_canonical.items() if len(srcs) > 1}
    for canonical, srcs in by_canonical.items():
        if len(srcs) < 2:
            continue
        for f in srcs:
            f["action"] = "review"
            f["target"] = f["target_dir"] = f["target_file"] = None
            f["reasons"] = sorted(set(f["reasons"] + ["target_collision"]))

    groups = collections.Counter(f["group"] for f in folders)
    return {
        "policy": (
            "Target layout <root>/<Author>/<Book>/<Book>.m4b, decided per book folder. "
            "Planning is read-only. Applying only renames a single unambiguous file to a "
            "free path; nothing is deleted, overwritten, merged, re-encoded or re-tagged. "
            "Multi-part books, competing editions, identical-size duplicates, unknown "
            "authors and zero-byte files are left in place for human review."
        ),
        "root": str(root),
        "summary": {
            "folders": len(folders),
            "files": sum(len(f["files"]) for f in folders),
            "actions": dict(collections.Counter(f["action"] for f in folders)),
            "groups": dict(groups),
            "reasons": dict(collections.Counter(r for f in folders for r in f["reasons"])),
            "collision_targets": len(collisions),
        },
        "collision_targets": collisions,
        "folders": folders,
    }


def apply_plan(plan: dict, limit: int | None) -> dict:
    """Apply only the unambiguous single-file changes. Returns a receipt.

    ``rename``      renames the file inside an already-correctly-named folder.
    ``rename_book`` moves the whole book folder to its canonical name, and then
                    renames the file inside it to match, so the folder and the
                    file agree on one title.
    """
    receipt: list[dict] = []
    skipped: list[dict] = []
    for f in plan["folders"]:
        if f["action"] not in ("rename", "rename_book"):
            continue
        if limit is not None and len(receipt) >= limit:
            skipped.append({"folder": f["folder"], "reason": "limit_reached"})
            continue

        src_folder = pathlib.Path(f["folder"])
        if not src_folder.is_dir():
            skipped.append({"folder": f["folder"], "reason": "source_missing"})
            continue

        if f["action"] == "rename_book":
            dst_folder = pathlib.Path(f["target_dir"])
            if dst_folder.exists():
                skipped.append({"folder": f["folder"], "reason": "target_exists"})
                continue
            dst_folder.parent.mkdir(parents=True, exist_ok=True)
            os.replace(src_folder, dst_folder)
            # The single file inside must adopt the new folder name.
            sole = f["files"][0]
            src_file = dst_folder / pathlib.Path(sole["source"]).name
            dst_file = dst_folder / f"{f['book']}{pathlib.Path(sole['source']).suffix.lower()}"
            if src_file.is_file() and not dst_file.exists() and src_file != dst_file:
                os.replace(src_file, dst_file)
            receipt.append({
                "action": "rename_book", "from": str(src_folder), "to": str(dst_folder),
                "file": str(dst_file) if src_file.is_file() else None,
                "author": f["author"], "book": f["book"],
            })
            continue

        src = pathlib.Path(f["files"][0]["source"])
        dst = pathlib.Path(f["target_file"])
        if not src.is_file():
            skipped.append({"folder": f["folder"], "reason": "source_missing"})
            continue
        if dst.exists():
            skipped.append({"folder": f["folder"], "reason": "target_exists"})
            continue
        before = src.stat().st_size
        os.replace(src, dst)
        receipt.append({
            "action": "rename", "from": str(src), "to": str(dst), "bytes": before,
            "author": f["author"], "book": f["book"],
        })
    return {"applied": receipt, "skipped": skipped}


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("root", type=pathlib.Path)
    p.add_argument("--out", required=True, type=pathlib.Path)
    p.add_argument("--apply", action="store_true",
                   help="rename the unambiguous single-file books (default: plan only)")
    p.add_argument("--limit", type=int,
                   help="with --apply, stop after this many renames")
    p.add_argument("--receipt", type=pathlib.Path,
                   help="with --apply, where to write the rename receipt")
    a = p.parse_args()

    if not a.root.is_dir():
        p.error(f"root is not a directory: {a.root}")

    plan = build_plan(a.root)
    if a.apply:
        plan["receipt"] = apply_plan(plan, a.limit)
        plan["summary_after"] = build_plan(a.root)["summary"]

    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(json.dumps(plan, indent=2))
    if a.apply and a.receipt:
        a.receipt.parent.mkdir(parents=True, exist_ok=True)
        a.receipt.write_text(json.dumps(plan["receipt"], indent=2))

    print(json.dumps({
        "mode": "apply" if a.apply else "plan",
        "out": str(a.out),
        "summary": plan["summary"],
        "applied": len(plan.get("receipt", {}).get("applied", [])),
        "skipped": len(plan.get("receipt", {}).get("skipped", [])),
    }, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
