#!/usr/bin/env python3
"""Lossless remux: embed staged covers into audiobooks (dry-run default).

For each book with a staged cover (from fetch_covers.py results):
  ffmpeg -i <audio> -i <cover> -map 0 -map 1 -c copy \
    -disposition:v attached_pic <book>.sonder-remux.m4b
then verify (duration match, chapter count match, attached_pic present)
and only then atomically replace the original. No re-encode, no quality loss.

  # dry-run one author (prints planned ops, touches nothing):
  python3 tools/audiobooks/remux.py --author "Blake Crouch"

  # apply one author (folder-by-folder, each book guarded):
  python3 tools/audiobooks/remux.py --author "Blake Crouch" --apply

Books with >1 audio file (multi-part MP3 sets, duplicate editions) are
SKIPPED for manual review -- joining needs confirmed track order first.
"""
import argparse
import json
import os
import subprocess
import sys

ROOT = "/Users/seandolbec/Projects/TM-Sonder"
LIB = os.environ.get("NAS_AUDIOBOOKS", "/Users/seandolbec/NAS2/plex/Audiobooks")
REPORT = os.path.join(ROOT, "m4b_work", "cleanup-report.json")
FETCH = os.path.join(ROOT, "m4b_work", "migration", "covers", "fetch-results.json")
LOG = os.path.join(ROOT, "m4b_work", "remux-log.jsonl")


def probe(path):
    p = subprocess.run(
        ["ffprobe", "-v", "error", "-show_format", "-show_streams",
         "-show_chapters", "-of", "json", path],
        capture_output=True, text=True, timeout=120)
    if p.returncode:
        raise ValueError(p.stderr[-500:])
    return json.loads(p.stdout)


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--author", default=None)
    ap.add_argument("--apply", action="store_true")
    args = ap.parse_args()

    staged = {(r["author"], r["book"]): r["cover"]
              for r in json.load(open(FETCH)) if r.get("status") == "staged"}
    if not staged:
        sys.exit("No staged covers in fetch-results.json yet.")
    print(f"staged covers: {len(staged)}", flush=True)

    items = [(a, b) for (a, b) in staged if not args.author or a == args.author]
    print(f"mode={'apply' if args.apply else 'dry-run'} books={len(items)}", flush=True)

    for author, book in sorted(items):
        bdir = os.path.join(LIB, author, book)
        cover = staged[(author, book)]
        try:
            names = [e.name for e in os.scandir(bdir)]
        except OSError as e:
            print(f"SKIP {author}/{book}: unreadable ({e})", flush=True)
            continue
        audio = sorted(n for n in names
                       if not n.startswith("._") and not n.startswith(".")
                       and os.path.splitext(n)[1].lower()
                       in (".m4b", ".m4a", ".mp3", ".mp4"))
        if len(audio) != 1:
            print(f"SKIP {author}/{book}: needs review ({len(audio)} audio files: "
                  f"{', '.join(audio[:3])})", flush=True)
            continue
        src = os.path.join(bdir, audio[0])
        tmp = src + ".sonder-remux.m4b"
        print(f"{'APPLY' if args.apply else 'PLAN'} {author}/{book}: "
              f"{audio[0]} + {os.path.basename(cover)}", flush=True)
        if not args.apply:
            continue
        try:
            if os.path.exists(tmp):
                os.unlink(tmp)
            is_mp3 = src.lower().endswith(".mp3")
            if is_mp3:
                tmp = src + ".sonder-remux.mp3"
                if os.path.exists(tmp):
                    os.unlink(tmp)
                cmd = ["ffmpeg", "-v", "error", "-y", "-i", src, "-i", cover,
                       "-map", "0:a", "-map", "1", "-c", "copy",
                       "-id3v2_version", "3",
                       "-metadata:s:v", "title=Album cover",
                       "-metadata:s:v", "comment=Cover (front)", tmp]
            else:
                cmd = ["ffmpeg", "-v", "error", "-y", "-i", src, "-i", cover,
                       "-map", "0:a", "-map", "1",
                       "-map_chapters", "0", "-map_metadata", "0",
                       "-c", "copy",
                       "-disposition:v", "attached_pic", tmp]
            p = subprocess.run(cmd, capture_output=True, text=True, timeout=900)
            if p.returncode:
                raise ValueError(f"ffmpeg: {p.stderr[-500:]}")
            s_old, s_new = probe(src), probe(tmp)
            d_old = float(s_old["format"].get("duration", 0))
            d_new = float(s_new["format"].get("duration", 0))
            if abs(d_old - d_new) > 0.25:
                raise ValueError(f"duration drift {d_old:.2f} -> {d_new:.2f}")
            if len(s_old.get("chapters", [])) != len(s_new.get("chapters", [])):
                raise ValueError("chapter count changed")
            if not any(s.get("disposition", {}).get("attached_pic")
                       for s in s_new.get("streams", [])):
                raise ValueError("no attached_pic in output")
            os.replace(tmp, src)
            print(f"  DONE {author}/{book}", flush=True)
            with open(LOG, "a") as f:
                f.write(json.dumps({"author": author, "book": book,
                                    "status": "remuxed"}) + "\n")
        except Exception as e:
            print(f"  FAIL {author}/{book}: {e}", flush=True)
            try:
                if os.path.exists(tmp):
                    os.unlink(tmp)
            except OSError:
                pass


if __name__ == "__main__":
    main()
