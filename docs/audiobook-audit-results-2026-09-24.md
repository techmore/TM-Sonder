# Audiobook audit results — 2026-09-24

Source: `/home/sdolbec/NAS/plex/Audiobooks`

The audit is read-only. No files were moved, renamed, retagged, merged, or deleted.

## Summary

- **1,569 audio files probed**
- **375.97 GiB**
- **11,221.15 hours**
- **1,028 files** already fit the proposed `Author/Book/file` shape
- **234 files across 25 multipart book candidates** require membership review
- **307 files** are recommended for quarantine from the active library

### Containers/codecs

- M4A/M4B-compatible containers: **1,067**
- MP3: **491**
- Opus streams: **114**
- NSV: **2**
- M4V: **1**
- Probe errors after the cached rerun: **8**

The current Go server recognizes M4B, MP3, and M4A. The audit deliberately inventories additional formats so unsupported or suspicious files are not silently lost.

## Quarantine recommendations

### 305 files: inbox/wrapper content

- `_INBOX-torrents-2026-09-22`
- `M4B Forge Compact/compact-m4b-80k`
- `Test-ebook`

These are not reliable canonical library content. Keep them in a dated archive outside the scanned audiobook root until their contents are reviewed.

### 2 working files

- `*.wcqr*`
- `*.sonder-retag.*`

One `.wcqr` M4B and one `.sonder-retag` M4B also fail probing with `moov atom not found`, so they should not be treated as playable books.

## Multipart book candidates

The largest groups requiring review are:

- *Complications: A Surgeon's Notes on an Imperfect Science*: 148 files
- *Ringworld Engineers*: 36 tracks
- *Echopraxia*: 11 files
- *The Road to Character*: 10 tracks
- *Being Mortal*: 9 tracks
- *Better*: 7 files
- *The Screwtape Letters — John Cleese*: 6 parts

The `Complications`, `Better`, and `Echopraxia` folders include both a complete M4B and MP3 tracks. These must be classified as one book, a distinct edition, or a duplicate set after duration/checksum/metadata review. They must not be merged based on folder membership alone.

## Other notable findings

- **316 files** have neither embedded art nor a recognized local cover.
- **639 files** have no chapters. This is expected for many individual MP3 tracks but needs book-level review.
- **17 files** contain a non-cover video stream and need manual review. Fourteen are outside the quarantine candidates, including several older M4B books.
- **11 files** have invalid audio according to ffprobe. These are concentrated in the inbox and should be quarantined rather than imported.
- Eight files still cannot be read because of NAS permission errors. They require a permissions check, not metadata cleanup.

## Next decision

The next migration pass should classify each multipart folder as:

1. Keep a complete M4B
2. Merge verified MP3 tracks into a new M4B
3. Keep as an ordered multipart book
4. Quarantine incomplete salvage
5. Quarantine exact duplicate
6. Keep a distinct narrator/edition

No physical migration should happen until the book/track model and progress migration are implemented.
