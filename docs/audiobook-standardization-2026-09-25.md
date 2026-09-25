# Audiobook layout standardization — current state (2026-09-25)

Regenerated against the live library with the committed planner
(`tools/audiobooks/standardize.py`), so this plan is reproducible from source
control rather than hand-built.

- Root: `/Users/seandolbec/NAS/plex/Audiobooks` (`~/NAS` → `/Volumes/14tb`, NFSv3)
- Plan: `m4b_work/migration/standardize-plan-2026-09-25.json` (read-only run)
- Target layout: `Audiobooks/<Author>/<Book>/<Book>.m4b`

## Inventory correction

The previous status (933 `.m4b`) was a *filtered* count. The current tree holds
1,575 `.m4b` paths, of which:

| Kind | Count |
| --- | --- |
| AppleDouble sidecars (`._Name.m4b`) | 510 |
| Real audio files | 1,065 |
| Real audio under the legacy `M4B Forge Compact/compact-m4b-80k` wrapper | 12 |
| Zero-byte `.m4b` | 2 |

So the real library is 1,065 books, not 1,575. The AppleDouble files are a NAS
artifact: they are dot-prefixed, the scanner already skips them, and the
planner skips them. They are pure noise in `find`-based counts and should not be
counted as books.

The planner groups by *book folder* rather than by file, because a folder
holding several files is a different problem from a rename:

| Group | Folders |
| --- | --- |
| `single_file` | 1,024 |
| `series_collection` | 20 |
| `multi_part_book` | 4 |
| `mixed_parts_and_extras` | 3 |
| `single_book_plus_leftovers` | 2 |
| `identical_size_duplicates` | 1 |

| Action | Folders |
| --- | --- |
| `keep` | 963 |
| `keep_multi_part` | 4 |
| `review` | 81 |
| `rename` | 4 |
| `rename_book` | 2 |

1,054 folders / 1,277 files were examined. Nothing on the NAS was modified.

## Proposed changes (6, all unambiguous)

Renames inside an already-correctly-named folder:

- `David Brooks/How to Know a Person/David Brooks - How to Know a Person.m4b` → `How to Know a Person.m4b`
- `Isaac Asimov/Foundation and Earth/Isaac Asimov - Foundation 07 - Foundation and Earth [Hope-2023].m4b` → `Foundation and Earth.m4b`
- `Isaac Asimov/The Robots of Dawn/The Robots of Dawn The Robot, Book 3.m4b` → `The Robots of Dawn.m4b`
- `James S. A. Corey/Leviathan Wakes/The Expanse, Book 1 - Leviathan Wakes.m4b` → `Leviathan Wakes.m4b`

Folder moves, because the parser and players display the *folder* name as the
book title — renaming only the file would leave the catalog title wrong:

- `Miguel de Cervantes/Don Quixote [AmazonClassics Edition] (Classics) (2020)/` → `Miguel de Cervantes/Don Quixote (Classics) (2020)/`
- `Philip K. Dick/The Three Stigmata of Palmer Eldritch (Weiner) 128k 07.31.44 {417mb}/` → `Philip K. Dick/The Three Stigmata of Palmer Eldritch (Weiner) 07.31.44/`

## Review required (81 folders)

### Two folders claiming one canonical path (4 targets, 8 folders)

Never merge these without comparing audio content; equal byte size is not proof.

- `Iain M. Banks/The Player of Games` vs `M4B Forge Compact/compact-m4b-80k/Iain M. Banks/The Player of Games` — both 173.5 MB, likely the same file
- `Iain M. Banks/The State of the Art (BBC Adaptation)` — canonical and wrapper copy, both 27.3 MB
- `Larry Niven/Ringworld` — canonical 244.7 MB vs wrapper `Ringworld (Unabridged).m4b` 635.5 MB, two different editions
- `Michael Crichton/Jurassic Park` 433.5 MB vs `Michael Crichton/Jurassic Park [Unabridged]` 518.9 MB, two different editions

### Series collection (20 books)

`Isaac Asimov/Foundation - The Complete Series/` nests one subfolder per book
with three narrators each (Jack Fox, Scott Brick, William Hope). This is a
deliberate multi-narrator set, not a layout error. It needs a series-level
policy, not per-file renames.

### Multi-file folders (10)

Correctly left alone, classified so the intent is recorded:

- Multi-part books, already in the right place: `Atul Gawande/Being Mortal` (9), `C. S. Lewis/The Screwtape Letters` (6), `David Brooks/The Road to Character` (10), `Larry Niven/The Ringworld Engineers` (36)
- Mixed parts plus an extra: `Atul Gawande/Better` (7), `Atul Gawande/Complications` (148), `Peter Watts/Echopraxia` (11) — each has a cover track or similar extra that the part heuristic does not recognize
- A real book plus leftovers: `Black Library/Shadows of Treachery` (2, one 0-byte) and `Unknown Author/Selected Stories of Philip K. Dick` (2, one 0-byte `.sonder-retag.m4b` — a retag temp artifact)
- Identical-size duplicates: `Iain M. Banks/The State of the Art` has `03 - The State of the Art.m4b` and `The State of the Art.m4b`, both 96.3 MB

### Unknown author (47 folders)

Mostly Philip K. Dick, Iain M. Banks/Robertsonian, Robert A. Heinlein, and
various anthologies whose author is recoverable from the title. These need
metadata resolution, not a folder move; the planner refuses to guess.

### Wrapper remnants (2)

- `M4B Forge Compact/compact-m4b-80k/` still holds 12 books; 8 are canonical
  duplicates of top-level copies, 3 collide (see above), and 1 is
  `Unknown/Change and Quality`
- `compact-m4b-80k/Test-ebook/Test-ebook.m4b` — a stray test fixture at the
  library root with a half-applied unwrap

## Two server bugs fixed alongside this

Both made the *catalog* disagree with a correct on-disk layout:

1. `server/internal/library/parser.go` — the audiobook parser ignored the
   `Author/Book/Book.m4b` structure entirely, taking the title from the filename
   and never deriving an author. It now prefers the book folder as the title and
   its parent as the author, and still works inside the legacy wrapper. Junk
   directory names (`audiobooks`, `audio book`, …) are not mistaken for authors.
2. `server/internal/library/scanner.go` — the rebuild merge assigned
   `item.Author = prev.Author` unconditionally, so a previous entry with a nil
   author erased the newly parsed author on every rebuild. It now only lets a
   prior value win when that value is non-empty.

`ParserVersion` is bumped so a rebuild actually happens; without the bump the
incremental scan skips unchanged files and the nil rows survive.

**Do not skip the version bump when parser or scanner output semantics change.**
An earlier run of exactly this bug looked fixed but was not, because the second
restart matched the already-bumped parse version and skipped the rebuild.

## Applying

```bash
cd tools/audiobooks
python3 standardize.py /Users/seandolbec/NAS/plex/Audiobooks --out plan.json            # plan
python3 standardize.py /Users/seandolbec/NAS/plex/Audiobooks --out plan.json --apply \
  --receipt m4b_work/migration/receipt.json --limit 6                                    # apply
```

`--apply` only renames the unambiguous single-file books and moves the two
unambiguous book folders. It refuses to overwrite an existing path, never
deletes, never merges, and never re-encodes. `--limit` bounds the batch; the
receipt records every applied and skipped path.

## Still open

- Narrator is still empty for all 1,279 cataloged audiobooks. Authors were
  already written into the `.m4b` tags by the retag pass, but nothing reads
  them back. This needs an ffprobe-based tag read during probe.
- 47 unknown-author books and 4 canonical-path collisions need human decisions.
- The 20-book Foundation series needs a series-level organization policy.
- 510 AppleDouble sidecars remain on the NAS. They are harmless to the scanner
  but should be excluded from any future inventory counts.
