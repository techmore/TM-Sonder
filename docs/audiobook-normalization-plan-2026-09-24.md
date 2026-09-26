# Audiobook normalization plan — 2026-09-24

## Goal

Make the audiobook library use the common `Author/Book/file.m4b` convention supported by Plex, Jellyfin, Audiobookshelf, and Sonder, without losing progress or deleting uncertain material.

This is a staged migration. The first pass is read-only and produces a manifest. No media files are moved or retagged until the manifest and code safeguards are reviewed.

## Target layout

```text
Audiobooks/
└── Canonical Author/
    └── Canonical Title/
        ├── Canonical Title.m4b
        └── cover.jpg
```

For distinct narrator or edition variants:

```text
Audiobooks/
└── Andy Weir/
    └── The Martian (Read by Wil Wheaton) [Unabridged]/
        ├── The Martian (Read by Wil Wheaton) [Unabridged].m4b
        └── cover.jpg
```

Series belongs primarily in metadata tags, not in a universal `Author/Series/Book` path. Audible/ASIN identifiers should be retained in metadata and may be included in an edition folder only when needed to disambiguate copies.

## Current risks

- Multipart MP3 books currently become one Sonder item per file.
- M4B and MP3 files in one book folder can represent one book or distinct editions.
- Author and series fields were historically conflated.
- Renaming or merging files changes path-derived item IDs.
- Inbox, test, wrapper, and interrupted retagging files are currently visible to the scanner.
- Some chapter markers extend beyond the media duration.
- Duplicate detection must not rely on title or file size alone.

## Phase 1 — inventory and manifest

Run the read-only audit tool:

```bash
python3 tools/audiobooks/audit.py \
  /home/sdolbec/NAS/plex/Audiobooks \
  --out /home/sdolbec/.config/sonder/data/audiobook-audit-2026-09-24 \
  --workers 4
```

Outputs:

- `inventory.jsonl` — resumable per-file probe cache
- `inventory.json` — complete inventory
- `migration-manifest.json` — non-destructive canonical path and review plan
- `summary.json` — counts and issue totals

The manifest assigns each file one of:

- `keep` — already fits the target layout
- `review_multipart` — multiple files appear to belong to one book
- `quarantine` — inbox, test, wrapper, or working file

The target action is only a recommendation. It does not move, merge, retag, or delete anything.

## Phase 2 — quarantine obvious non-library content

After review, move the following outside the scanned root while retaining the original path manifest:

- `_INBOX-torrents-2026-09-22`
- `M4B Forge Compact/compact-m4b-80k`
- `Test-ebook`
- `*.sonder-retag.*`
- `*.wcqr*`
- `*.partial`, `*.tmp`, and `*.download`
- torrent source/banner text and other non-audio reference files

Do not delete uncertain duplicates. Use a dated archive outside the library root.

## Phase 3 — classify book membership

Review every `review_multipart` group and assign one outcome:

1. Keep one complete M4B.
2. Merge verified tracks into one M4B.
3. Keep as an ordered track folder.
4. Quarantine an incomplete salvage.
5. Quarantine an exact duplicate.
6. Keep a distinct narrator, abridged, dramatized, or other edition.

Checksums, duration, stream membership, metadata, and cover provenance are required before duplicate or edition decisions.

## Phase 4 — deploy Sonder book/track support

Before moving or merging library files, Sonder needs:

- Book-level catalog identity with ordered child tracks
- Combined duration and chapter offsets
- Mixed M4B/MP3 membership handling
- Embedded metadata precedence
- Author, narrator, series, publisher, edition, and ASIN exposure
- Chapter validation against total duration
- Embedded cover extraction when no local cover exists
- Audiobookshelf responses with one book and multiple audio files
- A migration manifest that transfers progress and list references from old file IDs to the new book identity

Until this exists, a multipart book should not be physically merged in the live library.

## Phase 5 — staged retag and artwork normalization

For each retained book:

1. Verify the correct cover.
2. Write one canonical `cover.jpg`.
3. Embed the verified cover in the final M4B when possible.
4. Retag a staged copy with title, author, narrator, series/order, date, publisher, description, edition, language, and ASIN where known.
5. Preserve chapters and audio streams.
6. Full-decode and compare duration.
7. Atomically replace only after validation.
8. Keep the original in the archive until playback and metadata are accepted.

AAC-in-M4B remains the interoperability baseline. Opus conversion is a separate approved project.

## Phase 6 — controlled rescan

Only after the book/track support and migration manifest are ready:

1. Confirm no scan is active.
2. Confirm the root contains no inbox, test, or working files.
3. Run one explicit rescan.
4. Compare catalog results to the manifest.
5. Verify book counts, track counts, metadata, covers, chapters, and progress.
6. Test Plex, Jellyfin, Audiobookshelf, Sonder web, and mobile playback on representative books.

## Already implemented in Sonder

The current code now supports:

- Canonical author/book folder fallback
- Author and series separation
- Embedded audiobook tag extraction
- Chapter duration validation
- Scanner exclusion of known inbox/test/working paths
- Stable movie poster fallback in grouped API responses

The next implementation milestone is book/track identity and progress migration.
