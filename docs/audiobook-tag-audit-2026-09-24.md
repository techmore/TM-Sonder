# Audiobook tag audit — 2026-09-24 pass 2

Source: `/home/sdolbec/NAS/plex/Audiobooks`

This was a read-only audit after the first quarantine/cleanup pass. No files were renamed, moved, retagged, merged, or deleted.

## Scope

- **1,277 files discovered**
- **1,186 files successfully probed**
- **91 files unreadable by ffprobe**
- **368.32 GiB**
- **10,231.51 hours**
- **1,028** files already fit the proposed `Author/Book/file` shape
- **234** files across multipart candidates require membership review
- **15** files remain flagged for quarantine/review because they are in the permission-blocked Forge wrapper or are interrupted working files

## Existing embedded tag coverage

Counts below are among the **1,186 successfully probed files**:

| Field | Present | Missing | Assessment |
|---|---:|---:|---|
| Title/album | 1,082 | 104 | Mostly usable; folder fallback is still needed |
| Author/artist | 1,076 | 110 | Mostly usable |
| Genre | 1,067 | 119 | Strong coverage |
| Publication date | 294 | 892 | Needs folder/provider enrichment |
| Description/comment/lyrics | 171 | 1,015 | Sparse and inconsistent |
| Publisher | 10 | 1,176 | Almost entirely missing |
| Explicit narrator | 0 | 1,186 | No standardized narrator field |
| Series | 0 | 1,186 | Missing |
| Series order | 0 | 1,186 | Missing |
| Edition | 0 | 1,186 | Missing |
| Audible/ASIN | 0 | 1,186 | Missing |

### Important narrator finding

At least **58 files** contain comments such as:

```text
Narrated by Ray Porter
```

The current Go parser does not yet extract narrator names from this pattern. These should be converted to a dedicated narrator tag during the staged retag pass.

### Other observed tag conventions

- `artist` is usually the author.
- `album_artist` is present on many M4B files.
- `album` is commonly the book title.
- `comment` sometimes contains the narrator.
- `lyrics` or `description` sometimes contains the synopsis.
- `track` and `disc` exist on some files but are not consistently used for book-level identity.
- No files currently use a standardized `series`, `series-part`, `edition`, or `audible_id` tag.

## Content issues

- **41 files** have no embedded or local cover.
- **342 files** have no chapters. This is expected for many individual MP3 tracks but is not acceptable as a book-level result without review.
- **12 files** contain a non-cover video stream.
- **91 files** could not be probed due NAS permission errors or invalid partial M4B data.

## Folder/layout result

The active tree is mostly already close to the desired cross-client format:

```text
Audiobooks/Author/Book/Book.m4b
```

The remaining work is not a blanket rename. The correct next actions are:

1. Keep the 1,028 already-canonical files in place.
2. Review the 13 multipart book groups before merging or grouping tracks.
3. Resolve the 91 unreadable files with NAS permissions or quarantine decisions.
4. Create a staged metadata manifest for files needing retagging.
5. Write standardized tags only to staged copies.
6. Preserve the original path, size, mtime, checksum, chapters, and progress identity.
7. Promote validated files atomically.
8. Rescan and compare book/file counts, metadata, covers, chapters, and progress.

## Target embedded metadata

For each retained book, the target M4B/MP4 atom set is:

- `title`
- `artist`
- `album_artist`
- `album`
- `narrator`
- `series`
- `series-part`
- `date`
- `genre`
- `description`
- `publisher`
- `edition`
- `language`
- `audible_id` when known

For MP3, these should map to the corresponding ID3 frames where practical:

- `TIT2` / `TALB` — title
- `TPE1` / `TPE2` — author
- `COMM` — narrator/description
- `TCON` — genre
- `TDRC` — date
- `TLAN` — language
- `TRCK` — track/order

## Decision before physical retagging

The tag audit is complete, but physical retagging should wait until the multipart and unreadable-file groups are classified. Retagging 1,000+ files before that would create avoidable progress and identity risk.

The safe next deliverable is a **staged retag manifest** containing, for each file:

- source path
- destination/staging path
- current checksum and size
- current tags
- proposed title/author/narrator/series/date/description/publisher/edition
- confidence/source for each proposed field
- cover decision
- chapter decision
- progress identity to preserve
- validation result
