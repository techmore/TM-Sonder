#!/usr/bin/env python3
"""Verify embedded covers (and duration) for finished books. Read-only.

  python3 tools/audiobooks/verify.py [--author "Name"] [--log m4b_work/remux-log.jsonl]

Prints OK / NO-COVER / ERROR per book. Exit nonzero if anything is missing.
"""
import argparse
import json
import os
import subprocess
import sys

LIB = os.environ.get("NAS_AUDIOBOOKS", "/Users/seandolbec/NAS2/plex/Audiobooks")
AUDIO = (".m4b", ".m4a", ".mp3", ".mp4")


def probe(path):
    p = subprocess.run(
        ["ffprobe", "-v", "error", "-show_format", "-show_streams",
         "-of", "json", path],
        capture_output=True, text=True, timeout=180)
    if p.returncode:
        raise ValueError(p.stderr[-300:])
    return json.loads(p.stdout)


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--author", default=None)
    ap.add_argument("--log", default="/Users/seandolbec/Projects/TM-Sonder/m4b_work/remux-log.jsonl")
    args = ap.parse_args()
    try:
        with open(args.log) as f:
            entries = [json.loads(l) for l in f if l.strip()]
    except OSError:
        entries = []
    # Fall back to the staged-cover list if no remux log yet.
    if not entries:
        fetch = "/Users/seandolbec/Projects/TM-Sonder/m4b_work/migration/covers/fetch-results.json"
        entries = [{"author": r["author"], "book": r["book"]}
                   for r in json.load(open(fetch)) if r.get("status") == "staged"]
    bad = 0
    for e in entries:
        author, book = e["author"], e["book"]
        if args.author and author != args.author:
            continue
        bdir = os.path.join(LIB, author, book)
        try:
            names = [x.name for x in os.scandir(bdir)]
            audio = sorted(n for n in names
                           if not n.startswith("._") and not n.startswith(".")
                           and n.lower().endswith(AUDIO))
            if len(audio) != 1:
                print(f"REVIEW {author}/{book}: {len(audio)} audio files", flush=True)
                bad += 1
                continue
            d = probe(os.path.join(bdir, audio[0]))
            has_art = any(s.get("disposition", {}).get("attached_pic")
                          for s in d["streams"])
            dur = float(d["format"].get("duration", 0))
            if has_art and dur > 0:
                print(f"OK {author}/{book} ({dur/3600:.1f}h)", flush=True)
            else:
                print(f"NO-COVER {author}/{book}", flush=True)
                bad += 1
        except Exception as ex:
            print(f"ERROR {author}/{book}: {ex}", flush=True)
            bad += 1
    print(f"VERIFY-DONE bad={bad}", flush=True)
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
