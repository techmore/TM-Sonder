#!/usr/bin/env bash
# Generate a small, real media fixture for the container machine.
#
# Why this exists: a container machine mounts $HOME and nothing else, so it
# cannot see /Volumes/14tb. That is a feature for the build-and-test loop -- the
# machine runs the real deployment shape against data it cannot accidentally
# mutate -- but it means the machine needs its own library to serve. Scanning
# the real NAS from a machine is not possible, and the plain app container
# (`make container-run`) remains the tool for that.
#
# Everything here is synthesised with ffmpeg: a few seconds of real, decodable
# media per file. It is deliberately tiny, and it deliberately includes a
# multi-file book, because "one card per book, plays through in order" is the
# behaviour most likely to regress and cannot be tested without a book made of
# several files.
#
# Usage: tools/machine/make-fixtures.sh [dest]
set -euo pipefail

dest="${1:-media/fixtures}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dest="$root/$dest"

command -v ffmpeg >/dev/null || { echo "ffmpeg is required" >&2; exit 1; }

# A quiet tone plus an audible pitch per file, so a part is identifiable by ear
# and by waveform if a player ever advances to the wrong one.
tone() { # out duration freq label
  ffmpeg -hide_banner -loglevel error -y \
    -f lavfi -i "sine=frequency=$3:duration=$2:sample_rate=44100" \
    -f lavfi -i "anullsrc=r=44100:cl=mono" -filter_complex "[0:a]volume=0.15[a]" \
    -map "[a]" -c:a aac -b:a 64k -movflags +faststart \
    -metadata title="$4" -metadata artist="$5" -metadata album="$4" \
    -metadata comment="by $5" "$1"
}

echo "building fixtures in $dest"

# --- Audiobooks -----------------------------------------------------------
# A single-file book, and a three-file book. The multi-file one is the point:
# it is what a 147-part recording looks like after a scale-down.
mkdir -p "$dest/Audiobooks/Ursula K. Le Guin/The Dispossessed"
tone "$dest/Audiobooks/Ursula K. Le Guin/The Dispossessed/The Dispossessed.m4b" 6 220 \
     "The Dispossessed" "Ursula K. Le Guin"

# A multi-file book is ONE folder holding several files -- that is how the real
# library is laid out, and it is what the server groups on. One folder per part
# would be a different shape entirely: three single-file books, and no
# multi-part behaviour to test at all.
mkdir -p "$dest/Audiobooks/Atul Gawande/Complications"
for n in 1 2 3; do
  tone "$dest/Audiobooks/Atul Gawande/Complications/Complications - Part $n.m4b" \
       5 $((300 + n * 120)) "Complications" "Atul Gawande"
done

# --- Movies ---------------------------------------------------------------
mkdir -p "$dest/Movies/Arrival (2016)"
ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc=size=320x180:rate=12:duration=4" \
  -f lavfi -i "sine=frequency=440:duration=4" -c:v libx264 -preset ultrafast -pix_fmt yuv420p \
  -c:a aac -b:a 64k -movflags +faststart -metadata title="Arrival" \
  "$dest/Movies/Arrival (2016)/Arrival (2016).mp4"

# --- TV -------------------------------------------------------------------
mkdir -p "$dest/TV Shows/Foundation/Season 01"
for ep in 1 2; do
  ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc=size=320x180:rate=12:duration=3" \
    -f lavfi -i "sine=frequency=$((500 + ep * 100)):duration=3" \
    -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -b:a 64k -movflags +faststart \
    -metadata title="Episode $ep" -metadata show="Foundation" \
    "$dest/TV Shows/Foundation/Season 01/Foundation - S01E$ep.mp4"
done

# --- Ebooks ---------------------------------------------------------------
mkdir -p "$dest/Books/Frank Herbert/Dune"
printf 'Dune fixture. Not the real text.\n' > "$dest/Books/Frank Herbert/Dune/Dune.epub"

echo
echo "fixtures ready:"
find "$dest" -type f ! -name '._*' | sed "s#^$dest/#  #" | sort
echo
echo "books, by file count (the multi-file one must be 3):"
find "$dest/Audiobooks" -mindepth 2 -maxdepth 2 -type d | while read -r d; do
  printf '  %-46s %s files\n' "$(basename "$d")" "$(find "$d" -type f ! -name '._*' | wc -l | tr -d ' ')"
done
