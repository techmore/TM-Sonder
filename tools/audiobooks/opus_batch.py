#!/usr/bin/env python3
"""Convert single-file MP3 audiobooks to Opus-in-MP4 .m4b (background batch).

For each target: probe channels -> convert.py (32k mono / 48k stereo,
cover embedded, chapters/metadata verified, full decode checked) ->
place as <Book>.m4b beside the original -> re-verify -> remove the MP3.
Originals are replaced only after the new file verifies; every step is
logged with receipts. One missing/disappeared book can't stop the rest.
"""
import json
import os
import shutil
import subprocess
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)),
                                "..", ".."))
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__))))
from convert import convert as opus_convert

LIB = os.environ.get("NAS_AUDIOBOOKS", "/Users/seandolbec/NAS2/plex/Audiobooks")
STAGING = os.environ.get("NAS_STAGING", "/Users/seandolbec/NAS2/plex/.sonder-staging")
CAND = "/Users/seandolbec/Projects/TM-Sonder/m4b_work/migration/covers/candidates"

TARGETS = [
    ("Atul Gawande", "The Checklist Manifesto", "atul-gawande-the-checklist-manifesto.jpg"),
    ("Blake Crouch", "Recursion", "blake-crouch-recursion.jpg"),
    ("Cixin Liu", "Death's End", "cixin-liu-death-s-end.jpg"),
    ("Dennis E. Taylor", "We Are Legion", "dennis-e-taylor-we-are-legion.jpg"),
]


def probe(path):
    p = subprocess.run(
        ["ffprobe", "-v", "error", "-show_format", "-show_streams",
         "-show_chapters", "-of", "json", path],
        capture_output=True, text=True, timeout=180)
    if p.returncode:
        raise ValueError(p.stderr[-300:])
    return json.loads(p.stdout)


def main():
    os.makedirs(STAGING, exist_ok=True)
    for author, book, cover_name in TARGETS:
        bdir = os.path.join(LIB, author, book)
        print(f"--- {author} / {book} ---", flush=True)
        try:
            try:
                names = [e.name for e in os.scandir(bdir)]
            except OSError as e:
                print(f"VANISHED {author}/{book}: {e}", flush=True)
                continue
            audio = sorted(n for n in names
                           if not n.startswith("._") and not n.startswith(".")
                           and n.lower().endswith(".mp3"))
            if len(audio) != 1:
                print(f"SKIP {author}/{book}: {len(audio)} mp3s, needs review",
                      flush=True)
                continue
            src = os.path.join(bdir, audio[0])
            ch = next(s for s in probe(src)["streams"]
                      if s.get("codec_type") == "audio")["channels"]
            bitrate = 32 if int(ch) == 1 else 48
            cover = os.path.join(CAND, cover_name)
            import pathlib
            receipt = opus_convert(pathlib.Path(src), pathlib.Path(LIB),
                                   pathlib.Path(STAGING), bitrate,
                                   pathlib.Path(cover))
            staged = receipt["output"]
            final = os.path.join(bdir, book + ".m4b")
            if os.path.exists(final):
                raise ValueError(f"refusing to overwrite existing {final}")
            # Cross-filesystem move (local staging -> NFS), then re-verify
            # in place, then retire the MP3.
            shutil.move(staged, final)
            got = probe(final)
            if not any(s.get("disposition", {}).get("attached_pic")
                       for s in got["streams"]):
                raise ValueError("cover missing after move")
            if abs(float(got["format"]["duration"]) - receipt["duration"]) > 0.25:
                raise ValueError("duration drift after move")
            os.unlink(src)
            print(f"DONE {author}/{book}: {audio[0]} -> {book}.m4b "
                  f"(opus {bitrate}k, receipt kept)", flush=True)
        except Exception as e:
            print(f"FAIL {author}/{book}: {e}", flush=True)


if __name__ == "__main__":
    main()
