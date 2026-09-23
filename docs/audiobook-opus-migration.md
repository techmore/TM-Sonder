# Audiobook Opus migration

## Decision and scope

Retain organized author/title paths and `.m4b` filenames, but encode suitable
books with Opus inside a real MP4 container. AAC is the current codec; M4B is a
filename convention for an audiobook container, not a compression algorithm.
The [Opus MP4 mapping](https://opus-codec.org/docs/opus_in_isobmff.html) describes
the encapsulation. The local FFmpeg build successfully produced and fully
decoded an Opus/MP4 audiobook with chapters and embedded artwork.

This is a custom-player format choice. Do not assume every M4B player can decode
it. Sonder's iOS AVPlayer path successfully played the Opus/MP4 pilot in an
iOS 27 simulator, including advancing playback time. Physical-device behavior,
older supported iOS versions, and browser decoder support still need acceptance
checks before bulk rollout.

The initial target is `/Users/seandolbec/NAS/plex/Audiobooks`. The latest scan
found 933 M4B paths (350.28 GiB), with no other audio extensions. An additional
audit of `/Users/seandolbec/NAS/plex/ebook` checks for stranded audio. Counts are files,
not verified complete books or unique editions. Music, Lectures, quarantine,
and other NAS roots are outside this pass.

## Implemented foundation

- `tools/audiobooks/audit.py`: resumable, read-only codec/container/chapter/cover
  inventory, with errors recorded separately.
- `tools/audiobooks/plan.py`: per-file review queue; avoid re-encoding already
  small files, flag missing covers/chapters, and select pilot candidates.
- `tools/audiobooks/convert.py`: one-file conversion into a separate staging
  tree, including embedded artwork, preserved chapters, common metadata checks,
  full output decode, and SHA-256 receipt. Original files are untouched.
- Go server audiobook optimization jobs: persistent, selected-job queue with
  local source staging, Opus/MP4 conversion, full validation, checksum-verified
  copy-back to a hidden NAS review tree, queue controls, and a reduction receipt.
- `tools/audiobooks/test_convert.py`: actual FFmpeg integration test covering
  chapter/duration preservation, both artwork paths, and overwrite protection.

Working inventories, cover sources, pilot files, and receipts are under
`m4b_work/migration/`, which is already ignored by Git. Commands and constraints
are documented in `tools/audiobooks/README.md`.

## Encoding and artwork policy

Start listening trials at 32 kb/s VBR for mono narration and 48 kb/s VBR for
stereo. Preserve channel count. Compare 24/32/40 kb/s mono and 48/64 kb/s stereo
on representative narration, accents, older noisy recordings, and dramatized
books. Select a higher bitrate when the source needs it. These are trial
settings, not a validated universal quality guarantee.

AAC to Opus is another lossy generation. Prefer a higher-quality original if
available. Keep already-low-bitrate files when savings would be small. Compare
actual staged sizes before promotion; estimates exclude container/artwork and
VBR variation. No bitrate policy proves fidelity or source completeness.

Use existing embedded art first, then visually verified title-local artwork,
then matching publisher/catalog artwork. Match author, title, and collection
contents; record edition/narrator differences. Reuse print-edition cover art as
a clearly labeled fallback where the same work is established. Store downloaded
art and provenance locally before conversion. An image's existence does not
prove it is the correct cover. Leave interviews/ambiguous editions unresolved
rather than using unrelated covers. The staged converter requires artwork.

Keep book identity, author, narrator, series/order, edition, duration, chapter
names/times, and source/output checksums in the migration manifest. Folder names
alone are insufficient to merge multi-part books. For future MP3 track folders,
review exact book membership and track order, detect missing/duplicate tracks,
then implement concatenation and offset chapters. Do not merge an entire author
folder. The current converter deliberately accepts one input file at a time.

## Server and iOS implementation sequence

The existing server already streams `.m4b` over byte ranges with `audio/mp4`,
and the iOS app already routes M4B through AVPlayer. The catalog API now includes
the probed audio codec list. The built-in web player checks browser support for
Opus-in-MP4 and displays a clear message when that codec is unavailable; it does
not yet provide a server-side compatibility transcode.

1. Add a playback capability contract keyed by actual codec and container,
   rather than `.m4b` alone. Include channels, duration, chapters, cover,
   supported delivery variants, and a stable book ID independent of path.
   Preserve listening progress when replacing a file or moving a book.
2. Serve the archived Opus/MP4 file through authenticated byte-range requests
   with `audio/mp4`, Content-Length, range support, ETags, and correct HEAD/206
   handling. Keep the archive file immutable during playback. Serve chapter and
   artwork metadata separately so clients do not need to scan entire files.
3. Test native Opus/MP4 on physical devices across supported iOS versions and
   browsers (the iOS 27 simulator pilot is an initial positive result).
   Validate startup, seeks near beginning/middle/end, chapter jumps, 1–3x speed,
   background/lock-screen controls, interruptions, sleep timer, resume, offline
   downloads, and reconnects. Do not enable an Opus allowlist based on container
   mux support alone.
4. For clients without native support, implement a tested Opus decoder plus MP4
   demuxer (and an audio playback engine) or a server-side delivery variant.
   Browser delivery can remux Opus into a supported container without another
   lossy encode when possible. AAC fallback may support legacy clients, but it
   does not validate the desired direct Opus/offline path. Budget cache storage.
5. Use the server's single-worker queue for individual full-book trials. It
   stages each source locally, checks free space, records failures/retries,
   verifies the returned hash, and never overwrites the original. Listen to the
   staged file and test it on target devices before deciding on promotion. Keep
   stereo/dramatized, multi-part, low-bitrate, and suspect sources as separate
   review categories.
6. After acceptance, perform manifest-driven promotion with a rollback archive
   outside scanned roots, update the catalog atomically, preserve IDs/progress,
   and verify counts and sample playback. Deleting archived originals requires
   a separately established retention policy. No deletion is implemented here.

## Pilot evidence

*The Two Treatises of Government* (John Locke): 9,681.592 seconds, four chapters,
102,514,782 bytes AAC input → 41,813,531 bytes Opus/MP4 output at a 32 kb/s VBR
target: **59.2% smaller**. Duration, chapter names/boundaries, common metadata,
artwork, and a full output audio decode passed. Original and output SHA-256
hashes are recorded in the receipt. Listening and iOS/web device acceptance
remain pending.

An initial combined audio/artwork FFmpeg encode unexpectedly truncated audio.
The implemented converter encodes audio first and attaches artwork in a separate
stream-copy pass; duration checks reject truncated results. This was validated
with both synthetic integration inputs and the full-book pilot.

A missing cover for *The Dispatcher: Murder by Other Means* was downloaded from
[Subterranean Press](https://subterraneanpress.com/the-dispatcher-murder-by-other-means/)
and visually checked. It is a print-edition cover for the same work. Local ebook
artwork candidates are recorded separately with review decisions; one candidate
for *Methuselah's Children* was rejected because it displayed an unrelated essay.

A second full-book pilot, *Murder by Other Means* (John Scalzi), preserved
12,792.384 seconds and 15 chapters while adding the verified publisher cover:
133,131,362 → 51,587,595 bytes, **61.3% smaller**. Full decode passed.

A bounded five-book batch is now staged locally. *The Count of Monte Cristo*
was 1,927,336,539 → 824,466,761 bytes (**57.2% smaller**, one chapter).
Four Witcher audiobooks preserved their chapter counts and embedded artwork:
*Baptism of Fire* 341,819,064 → 237,702,403 bytes (30.5%, 8 chapters), *Blood
of Elves* 311,521,882 → 213,408,577 bytes (31.5%, 7 chapters), *Lady of the
Lake* 579,482,487 → 391,060,291 bytes (32.5%, 26 chapters), and *Season of
Storms* 335,663,114 → 227,187,158 bytes (32.3%, 31 chapters). All five passed
full output decode and probe as Opus in MP4-family containers. Combined, they
save 1,601,997,896 bytes (45.8%). They remain in local staging; no originals
were moved, replaced, or deleted. Eight earlier queue entries were skipped
because they still need verified cover mappings.

A second run used the refreshed cover map and resumed past the five existing
receipts. Three more Witcher books passed the same checks: *Sword of Destiny*
365,142,623 → 250,765,399 bytes (31.3%, 49 chapters), *The Last Wish*
293,325,224 → 199,962,855 bytes (31.8%, 13 chapters), and *The Tower of the
Swallow* 468,659,181 → 328,168,737 bytes (30.0%, 11 chapters). Across all eight
additional staged books, sources total 4,622,950,114 bytes and Opus outputs
total 2,672,722,181 bytes: **1,950,227,933 bytes saved (42.2%)**. Each receipt
confirms full output decode; all outputs retain cover art and chapters. Eight
other entries still lack verified covers and remain skipped.

The current tree contains six author/title folder groups with multiple files
or variants (17 files total). Four have matching filenames in both the compact
wrapper and top-level author folders; three of those pairs have equal sizes,
which is not sufficient to prove identical content. Review these groups before
merging. The proposed final organization is `Author/Title/Book.m4b`, with explicit
edition/part distinctions when needed, without retaining obsolete `80k` wrapper
names. Staged pilots preserve the original relative paths for traceability.

## Inventory results (September 19, 2026)

After the NAS remounted, the refreshed audit recorded 933 `.m4b` paths totaling
350.28 GiB and about 10,526 hours. 932 files probe as AAC inside MP4-family
containers; one zero-byte `.m4b` has no valid MP4 header. The supplemental
stream pass checked all 932 readable files and found 14 with real non-cover
video tracks. Forty-eight lack embedded artwork and a sibling image at scan
time; 31 source paths now have visually verified cover mappings staged locally,
leaving 17 unresolved. Forty-four files lack source chapters. The current plan
has 848 pilot candidates, 20 chapter reviews, 18 book-membership/version reviews,
15 manual reviews, 14 low-bitrate files to retain, and one music album held for
media-kind review.

The separate ebook audit still finds one AAC/MP4 audio track, not an audiobook;
it has a sibling image but no chapters and remains flagged for media-kind review.
No source audio or cover on the NAS has been changed. The current bounded staging
batch writes only to the local migration staging folder and retains originals.
