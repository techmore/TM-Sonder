# Audiobook library — state and decisions (2026-09-25)

Regenerated against the live library with committed tools, so every number here
is reproducible rather than hand-counted.

- Root: `/Users/seandolbec/NAS/plex/Audiobooks` (`~/NAS` → `/Volumes/14tb`, NFSv3)
- Target layout: `Audiobooks/<Author>/<Book>/<Book>.m4b`
- Plan: `m4b_work/migration/standardize-plan-2026-09-25.json`

## Inventory correction

The earlier "1,575 `.m4b`" figure was a raw `find` count that included
AppleDouble sidecars. Corrected:

| Kind | Count |
| --- | --- |
| AppleDouble sidecars (`._Name.m4b`) | 510 |
| Real audio files | 1,065 |
| Real audio under the legacy `M4B Forge Compact/compact-m4b-80k` wrapper | 12 |
| Zero-byte `.m4b` | 2 |

The scanner and the planner both skip dot-prefixed files, so 510 of those are
noise that should never appear in an inventory count.

## Layout plan: nothing left to apply

The 6 unambiguous changes were applied (receipt:
`m4b_work/migration/standardize-receipt-2026-09-25.json`). The regenerated plan
now reports **0 renames** and **971 keep / 4 keep-multi-part / 79 review**.

Applied:

| Change | Kind |
| --- | --- |
| `David Brooks/How to Know a Person/` — file renamed to match the folder | rename |
| `Isaac Asimov/Foundation and Earth/` — file renamed | rename |
| `Isaac Asimov/The Robots of Dawn/` — file renamed | rename |
| `James S. A. Corey/Leviathan Wakes/` — file renamed | rename |
| `Miguel de Cervantes/Don Quixote [AmazonClassics Edition] (Classics) (2020)/` → `Don Quixote (Classics) (2020)/` | folder move |
| `Philip K. Dick/The Three Stigmata of Palmer Eldritch (Weiner) 128k 07.31.44 {417mb}/` → `… (Weiner) 07.31.44/` | folder move |

Note on the Don Quixote move: it was planned under an earlier, looser noise
rule that stripped every bracketed token. The rule has since been tightened
(see below) so `[Unabridged]` and similar edition markers survive. The
`[AmazonClassics Edition]` marker was dropped by that earlier run; the book is
still unambiguously identified, and no other book in the library shares the
name, so nothing was made ambiguous by it.

## Two planner rules that were wrong and are now fixed

**Bracketed text is not automatically noise.** The first version stripped every
`[...]` token, which merged genuine editions: `Jurassic Park` and
`Jurassic Park [Unabridged]` were reported as two folders competing for one
canonical path. They are two different recordings. Only clearly technical
brackets are stripped now (`[64kbps]`, `[417mb]`, `[audible-…]`), while
`[Unabridged]`, `[Abridged]`, `[Full Cast]` and similar are preserved. This
removed a false collision and dropped the real collision count from 4 to 3.

**A plan's unit of decision is the book folder, not the file.** Deciding per
file produced 305 "review" entries for what is really a few dozen folder-level
decisions, because every part of a multi-part book collided with every other
part. The folder-level model classifies instead of guessing.

## Collisions: 3 remaining, all resolved by content

A collision is a *suspicion*, not a finding. `tools/audiobooks/compare_books.py`
settles them by decoding sampled audio to raw PCM and hashing it. Container
bytes cannot answer the question, because the retag pass rewrites metadata
without touching a single audio sample — two of these pairs differ byte-for-byte
while being the same recording.

| Folders | Verdict | What to do |
| --- | --- | --- |
| `Iain M. Banks/The Player of Games` + its `M4B Forge Compact` twin | `same_recording_different_container` | Keep the top-level copy; the wrapper copy is a pre-retag duplicate |
| `Iain M. Banks/The State of the Art (BBC Adaptation)` + its wrapper twin | `same_recording_different_container` | Same |
| `Larry Niven/Ringworld` + `M4B Forge Compact/…/Larry Niven/Ringworld/Ringworld (Unabridged).m4b` | `different_books` | Genuinely different: 10.93 h Opus 49 kb/s vs 10.93 h AAC 129 kb/s. Decide which to keep; do not merge |
| `Michael Crichton/Jurassic Park` + `Jurassic Park [Unabridged]` | `different_books` (no longer a collision) | Two different recordings, 15.17 h vs 13.84 h. Keep both |

Evidence: `m4b_work/migration/collision-comparison.json`.

## Remaining reviews (79 folders)

- **47 unknown author** — see below
- **20 series collection** — `Isaac Asimov/Foundation - The Complete Series/`,
  one subfolder per book with three narrators each (Jack Fox, Scott Brick,
  William Hope). This is a deliberate multi-narrator set, not a layout error.
  The planner reports it and leaves it alone. It needs no fix; it needs a
  decision about whether multi-narrator sets should be a supported layout
  everywhere or flattened to one book per narrator.
- **10 multi-file folders** — 4 genuine multi-part books (already correct), 3
  mixed parts-plus-extras, 2 single-book-plus-leftovers (each with a 0-byte
  file, one a `.sonder-retag.m4b` temp artifact), 1 identical-size duplicate
  pair (`Iain M. Banks/The State of the Art` has `03 - …` and `…` at 96.3 MB
  each)
- **2 zero-byte files** — `Black Library/…/Shadows of Treachery…wcqr1ulr.m4b`
  and `Unknown Author/…/….m4b.sonder-retag.m4b`
- **1 stray** — `compact-m4b-80k/Test-ebook/Test-ebook.m4b`, a test fixture
  left at the library root with a half-applied unwrap

## The 47 unknown-author books

This was attempted automatically and **rejected**. Of 45 distinct books:

- Embedded tags: only 1 of 48 files carried any author credit at all. The
  retag pass never reached these files.
- Open Library lookup: returned 29 "high confidence" answers, of which several
  were plainly wrong — "Cycle of the Werewolf" → **Stephen King** (it is Philip
  K. Dick), "The Aeneid" → "Publius Vergilius Maro", "Threshold" → Bill Myers,
  "Influence" → Robert Cialdini, "The Broom of the System" → David Foster
  Wallace. Folder names here are too short and too generic for fuzzy matching
  to be evidence.
- Cross-check against the 4,641-book ebook library: confirmed 2, and
  manufactured a false contradiction ("State of Fear" matched the ebook
  "State of the Art"). The matching threshold was then raised, which removed the
  false contradiction but also left the cross-check with almost nothing to say.

So the automated pipeline is recorded as a dead end, and the result is a curated
proposal in `m4b_work/migration/author-curation.json`: 30 attributions with a
stated reason, 15 marked `needs-verification`, and 7 left explicitly
`UNIDENTIFIED` (they may not be books at all — several look like lectures or
radio interviews).

Nothing has been moved. Applying that curation is a separate, reviewed step.

## Server-side fixes

All three were found by comparing the on-disk layout with the catalog.

1. **The audiobook parser ignored the folder layout.** It took the title from
   the filename and never derived an author. It now uses the book folder as the
   title and its parent as the author. Author coverage went from 2 of 1,279 to
   1,278 of 1,279.
2. **The rebuild merge erased the author.** `item.Author = prev.Author` ran
   unconditionally, so a previous nil author overwrote the freshly parsed one on
   every rebuild. It now only lets a prior value win when non-empty.
3. **ffprobe already received the tags; the parser discarded them.**
   `ffprobeOutput` had no `Tags` field at all, so the author and narrator
   written by the retag pass were on hand and thrown away. Tags are now parsed
   (with alias resolution for ffprobe's lowercase MP4 atoms, uppercase
   FFMETADATA1 names, and ID3 `©` forms) and folded into empty catalog fields
   only, so a metadata provider still outranks them.

A fourth issue was found while testing the third: an audiobook with no narrator
tag would be re-probed on *every* scan, forever. `ProbedTagsRead` makes tag
reading a one-shot marker rather than a standing condition.

**Bump `ParserVersion` whenever parser or scanner output changes.** The
incremental scan skips files whose parse version already matches, so without
the bump a catalog fix appears to have no effect. This exact trap produced one
misleading "the fix didn't work" result during this work.

## Still open

- Narrator coverage was still climbing when this was written (0 → 128 of 1,279
  and draining); many files genuinely carry no narrator tag.
- The 3 remaining collisions need a keep/delete decision.
- 15 curated attributions need verification; 7 are unidentified.
- 510 AppleDouble sidecars remain on the NAS. Harmless to the scanner, but they
  should be excluded from any future count and are worth deleting at the share
  level.
- The Foundation multi-narrator set needs a series-level organization decision.
