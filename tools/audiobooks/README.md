# Audiobook migration tools

These commands require Python 3, ffprobe, and FFmpeg with libopus. They do not
modify or remove source media. Run from the repository root.

## Audit

```sh
python3 tools/audiobooks/audit.py /Users/seandolbec/NAS/plex/Audiobooks --out m4b_work/migration
```

Records actual container, audio codec/profile/bitrate/channels, duration,
chapter names/times, embedded artwork, sibling image candidates, and errors.
`inventory.jsonl` is an append-only probe cache; `inventory.json` and
`summary.json` describe the completed scan. Failed probes are retried.
Artwork detection establishes presence, not correctness or image quality.
This is a header audit, not a full decode of every original.

## Plan

```sh
python3 tools/audiobooks/plan.py m4b_work/migration/inventory.json \
  --covers m4b_work/migration/covers/verified-covers.json \
  --overrides m4b_work/migration/plan-overrides.json \
  --out m4b_work/migration/plan.json
```

Optionally pass `--covers m4b_work/migration/covers/verified-covers.json` to
include staged, visually checked artwork. Proposed paths remove the obsolete
`M4B Forge Compact/compact-m4b-80k` wrapper; multiple files landing in the same
book folder are flagged for track/edition review, not merged automatically.
The converter itself preserves its selected source-root-relative paths.

This creates a review queue; it does not execute conversions. 32 kb/s mono and
48 kb/s stereo are trial settings. Low-bitrate sources are retained because
another lossy encode may provide little benefit. Cover and chapter gaps are
reported even for retained files. Files are not assumed to be complete books:
multi-file books need reviewed membership, order, and combined chapter timing
before a joining implementation is used.

`--overrides` applies exact-path manual classifications after the automatic
rules. The current local override keeps Ursula K. Le Guin and Todd Barton's
*Music and Poetry of the Kesh* out of the audiobook bitrate queue until a
music-appropriate encoding target is chosen. The override file stays with the
local ignored migration artifacts.

## Stage a full-book pilot

```sh
python3 tools/audiobooks/convert.py '/absolute/source/Author/Book/Book.m4b' \
  --root /absolute/source \
  --output-root /absolute/separate-staging \
  --bitrate 32
```

Supply `--cover /absolute/verified-cover.jpg` if the input lacks artwork, or to
replace it in the staged output. The converter preserves paths relative to the
source root and writes a real MP4 file with an Opus audio stream and `.m4b`
extension. It accepts other FFmpeg-readable single audio files too. It refuses
existing outputs, multiple audio tracks, more than two channels, existing Opus,
and files without supplied or embedded artwork. It does not concatenate tracks.

Audio encoding and cover attachment are separate passes because the installed
FFmpeg build truncated an initial combined encode. Before publication, verify:
codec/container, duration (250 ms tolerance), channel count, all chapter names
and boundaries (100 ms tolerance), common metadata fields, attached artwork,
and full output audio decoding. The receipt records SHA-256 checksums and sizes.
Files without source chapters do not gain invented chapter boundaries. Inspect
unusual/custom metadata separately; original metadata remains in the audit.
Publication uses an exclusive hard link within the staging filesystem; the
filesystem must support hard links. No cleanup/promotion command is included.

Automated validation does not prove subjective quality, source completeness,
correct artwork identity, or real-device playback. Keep originals until those
checks pass and a separate backup/rollback policy has been established.

## Standardize the folder layout

```sh
python3 tools/audiobooks/standardize.py /Users/seandolbec/NAS/plex/Audiobooks \
  --out m4b_work/migration/standardize-plan.json
```

Plans the move to the target layout `Audiobooks/<Author>/<Book>/<Book>.m4b`,
decided **per book folder** rather than per file. A folder holding one audio
file is a rename candidate; a folder holding several is classified instead
(`multi_part_book`, `multiple_editions`, `identical_size_duplicates`,
`single_book_plus_leftovers`, `mixed_parts_and_extras`, `series_collection`)
and left alone, because merging tracks or picking an edition is a decision a
human has to make. Zero-byte files, unknown-author folders, half-applied
unwraps, and two folders claiming one canonical path are all reported as
reviews rather than resolved by guesswork.

Planning never touches the disk. `--apply` only renames unambiguous single-file
books and moves unambiguous book folders; it refuses to overwrite an existing
path, and never deletes, merges, re-encodes, or re-tags. `--limit` bounds the
batch and `--receipt` records every applied and skipped path.

Note that the file name inside a book folder is what the layout normalizes, but
the *folder* name is what the server parser and most players display as the
book title. A book whose folder name is noisy therefore gets a `rename_book`
folder move, not just a file rename.

Dot-prefixed files (`._Name.m4b` AppleDouble sidecars from the NAS) and
dot-directories are skipped, so the planner's file counts exclude them.

## Tests

```sh
python3 -m unittest discover -s tools/audiobooks -p 'test_*.py' -v
```

Exercises actual FFmpeg encoding/remuxing, duration/chapter preservation,
embedded and sidecar artwork, refusal to overwrite, and unchanged originals.
The standardization planner's tests additionally cover folder-level
classification, collision detection, and the guarantee that planning never
modifies the library.

## Bounded batches

```sh
python3 tools/audiobooks/batch.py m4b_work/migration/plan.json \
  --output-root /absolute/separate-staging --limit 3
```

This prints a dry run. Add `--execute` to stage the selected files. Only
`pilot_candidate` jobs are selected; jobs missing an explicit verified sidecar
mapping are skipped. Runs are sequential and stop at the first failure. The
runner checks free disk space, records results in `batch.jsonl`, and resumes
outputs whose source fingerprint, encoding settings, cover choice, size, and
output hash match their receipt. It never promotes or deletes originals.
Source fingerprints use size/mtime for resume; receipts also record the full
original checksum, which can be compared independently for archival validation.

## Supplemental checks and reports

`check_video.py` supplements old inventories with MP4 stream-header checks,
using limited packet analysis to avoid reading long media segments. New audits
already record non-cover video. Such files are withheld from audio-only
conversion so actual video is not discarded silently.

```sh
python3 tools/audiobooks/check_video.py m4b_work/migration/inventory.json \
  --out m4b_work/migration/video-checks.jsonl --workers 8
python3 tools/audiobooks/report.py m4b_work/migration/inventory.json \
  --covers m4b_work/migration/covers/verified-covers.json \
  --video-checks m4b_work/migration/video-checks.jsonl
```

The report updates inventory/summary, missing-cover, and probe-error files.
Regenerate the plan after applying new probe results or verified covers.
