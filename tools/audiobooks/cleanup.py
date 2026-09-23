#!/usr/bin/env python3
"""Audiobook library cleanup + missing-cover finder (NFS-safe).

READDIR-only by default: uses os.scandir (no sort, no stat storms, no ffprobe),
so it stays fast on the Synology NFS mount where `ls -la`/stat/write ops hang.
ffprobe embedded-art check is opt-in via --probe-embedded.

Standard enforced (Plex + Jellyfin + Audiobookshelf compatible):
  Audiobooks/<Author>/<Book>/<Book>.m4b
  + cover.jpg (Audiobookshelf) + folder.jpg (Jellyfin/Kodi) + poster.jpg (Plex)
  The three sidecars should exist with identical bytes; the script never
  overwrites existing art, it only copies poster.jpg -> missing siblings.

Usage:
  # dry-run whole library (read-only, safe):
  python3 tools/audiobooks/cleanup.py --root /Users/seandolbec/NAS/plex/Audiobooks

  # one author (folder-by-folder):
  python3 tools/audiobooks/cleanup.py --root .../Audiobooks --author "Adolf Hitler"

  # JSON report:
  python3 tools/audiobooks/cleanup.py --root .../Audiobooks --out m4b_work/cleanup-report.json

  # apply safe fixes (needs healthy NAS writes; one author at a time):
  python3 tools/audiobooks/cleanup.py --root .../Audiobooks --author "Adolf Hitler" --apply

  --apply currently does, per book, each step guarded so one hung file
  can't kill the run:
    - delete AppleDouble `._*` files
    - copy existing poster.jpg/cover.jpg/folder.jpg -> missing siblings
      (never overwrites, never invents art)
  It never renames books, never deletes audio, never touches the obsolete
  `M4B Forge Compact/compact-m4b-80k` wrapper (flagged only).
"""
import argparse
import json
import os
import shutil
import sys

AUDIO = {".m4b", ".m4a", ".mp3", ".opus", ".ogg", ".flac", ".wav", ".aac", ".mp4", ".wma"}
SIDECAR_NAMES = ("cover.jpg", "folder.jpg", "poster.jpg",
                 "cover.png", "folder.png", "poster.png",
                 "cover.webp", "folder.webp")
CANON = ("cover.jpg", "folder.jpg", "poster.jpg")


def scan_book(book_path):
    """Single scandir; no follow-up stats. Returns dict."""
    try:
        entries = list(os.scandir(book_path))
    except OSError as e:
        return {"error": f"scandir: {e}"}
    names = [e.name for e in entries]
    audio = sorted(n for n in names
                   if not n.startswith("._") and not n.startswith(".")
                   and os.path.splitext(n)[1].lower() in AUDIO)
    sidecars = sorted(n for n in names
                      if not n.startswith("._")
                      and (n in SIDECAR_NAMES or n.lower() in ("cover.jpeg", "folder.jpeg")))
    appledouble = sorted(n for n in names if n.startswith("._"))
    hidden = sorted(n for n in names if n.startswith(".") and not n.startswith("._"))
    other = sorted(n for n in names
                   if n not in audio and n not in sidecars
                   and n not in appledouble and n not in hidden
                   and n not in (".", ".."))
    book = os.path.basename(os.path.normpath(book_path))
    stems = sorted({os.path.splitext(a)[0] for a in audio})
    return {
        "audio": audio,
        "sidecars": sidecars,
        "appledouble": appledouble,
        "hidden": hidden,
        "other": other,
        "stem_matches_book": bool(audio) and all(s == book for s in stems),
        "multi_edition": len(stems) > 1 or len(audio) > 3,
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", required=True, help="Audiobooks root (Author/Book/...)")
    ap.add_argument("--author", default=None, help="Only this author folder")
    ap.add_argument("--out", default=None, help="Write JSON report here")
    ap.add_argument("--apply", action="store_true",
                    help="Apply safe fixes (delete ._*; copy sidecar siblings). Default is dry-run.")
    ap.add_argument("--probe-embedded", action="store_true",
                    help="ffprobe each audio file for embedded art (SLOW on NFS, opt-in).")
    args = ap.parse_args()

    root = args.root
    try:
        with os.scandir(root) as it:
            authors = sorted(e.name for e in it if e.is_dir(follow_symlinks=False))
    except OSError as e:
        sys.exit(f"Root unavailable: {root}: {e}")
    if args.author:
        if args.author not in authors:
            sys.exit(f"Author not found under root: {args.author!r}")
        authors = [args.author]
    # Obsolete wrapper lives *inside* Audiobooks on this NAS; flag, never descend+modify.
    wrapper = "M4B Forge Compact" in authors
    authors = [a for a in authors if a != "M4B Forge Compact"]

    report = {"root": root, "mode": "apply" if args.apply else "dry-run",
              "authors": {}, "totals": {}}
    n_books = n_missing = n_clutter = n_misnamed = 0

    for ai, author in enumerate(authors, 1):
        adir = os.path.join(root, author)
        try:
            with os.scandir(adir) as it:
                books = sorted(e.name for e in it if e.is_dir(follow_symlinks=False))
        except OSError as e:
            report["authors"][author] = {"error": f"scandir: {e}"}
            print(f"[{ai}/{len(authors)}] {author}: UNREADABLE ({e})", flush=True)
            continue
        a_missing, a_clutter, a_misnamed = [], [], []
        for book in books:
            bdir = os.path.join(adir, book)
            row = scan_book(bdir)
            n_books += 1
            if "error" in row:
                a_missing.append({"book": book, "issue": row["error"]})
                continue
            has_art = bool(row["sidecars"])
            if args.probe_embedded and not has_art and len(row["audio"]) == 1:
                import subprocess
                try:
                    p = subprocess.run(
                        ["ffprobe", "-v", "error", "-show_streams", "-of", "json",
                         os.path.join(bdir, row["audio"][0])],
                        capture_output=True, text=True, timeout=90)
                    streams = json.loads(p.stdout).get("streams", [])
                    if any(s.get("disposition", {}).get("attached_pic") for s in streams):
                        has_art = True
                        row["embedded_cover"] = True
                except Exception as e:
                    row["probe_error"] = str(e)[:200]
            if not has_art and row["audio"]:
                n_missing += 1
                a_missing.append(book)
            if row["appledouble"]:
                n_clutter += len(row["appledouble"])
                a_clutter.append({"book": book, "files": row["appledouble"]})
            if not row["stem_matches_book"] and row["audio"]:
                n_misnamed += 1
                a_misnamed.append({"book": book, "audio": row["audio"]})
            if args.apply:
                apply_book(bdir, row)
        report["authors"][author] = {
            "books": len(books), "missing_covers": a_missing,
            "appledouble": a_clutter, "misnamed": a_misnamed}
        print(f"[{ai}/{len(authors)}] {author}: books={len(books)} "
              f"missing={len(a_missing)} clutter={sum(len(c['files']) for c in a_clutter)} "
              f"misnamed={len(a_misnamed)}", flush=True)

    report["totals"] = {"authors": len(authors), "books": n_books,
                        "missing_covers": n_missing,
                        "appledouble_files": n_clutter,
                        "misnamed_books": n_misnamed,
                        "wrapper_present": wrapper}
    print(f"DONE authors={len(authors)} books={n_books} missing={n_missing} "
          f"appledouble={n_clutter} misnamed={n_misnamed} wrapper={wrapper} "
          f"mode={report['mode']}", flush=True)
    if args.out:
        with open(args.out, "w") as f:
            json.dump(report, f, indent=1)
        print(f"Report -> {args.out}", flush=True)


def apply_book(bdir, row):
    """Best-effort safe fixes; every op guarded. No renames, no audio deletes."""
    for n in row.get("appledouble", []):
        try:
            os.unlink(os.path.join(bdir, n))
            print(f"  rm {bdir}/{n}", flush=True)
        except OSError as e:
            print(f"  KEEP (unlink failed) {bdir}/{n}: {e}", flush=True)
    have = [n for n in CANON if n in row.get("sidecars", [])]
    if have:
        src = os.path.join(bdir, have[0])
        for n in CANON:
            if n not in row.get("sidecars", []):
                try:
                    shutil.copyfile(src, os.path.join(bdir, n))
                    print(f"  cover-sibling {bdir}/{n} (from {have[0]})", flush=True)
                except OSError as e:
                    print(f"  SKIP (copy failed) {bdir}/{n}: {e}", flush=True)


if __name__ == "__main__":
    main()
