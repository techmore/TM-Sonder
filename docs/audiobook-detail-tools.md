# Audiobook details and recording tools

The audiobook shelf now fixes every cover to a 2:3 frame independently of the
source image's natural dimensions. Artwork is contained, preserving the whole
cover. Detail covers use 220 × 330 px on desktop and smaller fixed frames at
mobile breakpoints.

The individual book page separates the story, chapter timeline and series
context from recording information and tools. Tags are deduplicated. Chapters
show whole-book start times and durations; clicking a row starts the correct
source part at its local offset. Books without chapter markers show file order,
explicitly labeled as tracks rather than verified chapters. Related books come
from the existing local library; missing narrator/summary data is shown honestly.

## MP3 to M4B

For books containing MP3, the page checks conversion eligibility. Only a complete,
unambiguous ordered MP3 source set can be converted. The owner can create a
staged AAC M4B; original files are retained. Available catalog art requires
explicit approval; embedded source art is retained automatically.

One worker processes conversions on SER8. Files decode to normalized lossless
PCM and receive one AAC encode for the whole book, avoiding per-track encoder
priming and cumulative chapter drift. PCM workspace, output and reserve space
are checked before work starts; intermediates are removed after verification.
Long books can require substantial temporary space. AAC is 128 kb/s; conversion
cannot improve source quality. Mono stays mono unless a source part is stereo.

Existing embedded chapters are preserved. Reviewed chapter maps take precedence;
otherwise source-file boundaries receive the source file names. Verification
checks runtime, timestamps, titles, metadata, artwork when available, and full
audio decode. The finished M4B is available for listening review and download.
It does not automatically replace the catalog entry. Jobs interrupted by a
restart are marked for a manual retry. State, outputs and receipts live under
`DataDir/audiobook-m4b`; include that directory in application data backups.

## Chapter timing lookup

Open **Recording tools → Find chapter timings**, enter the edition's Audible
ASIN and region, then look up its metadata and timing table via
[Audnexus](https://audnex.us/). Lookup sends only ASIN/region, not audio files.
The preview compares the provider runtime with the local recording and shows
its author/narrator. Runtime similarity does not establish edition identity;
the owner must explicitly confirm the title, narrator and edition before applying.
No timestamps are silently scaled. Ordered markers outside the local runtime
are rejected. Approximate provider timings are identified in the preview.

Reviewed maps are atomic sidecars under `DataDir/audiobook-chapter-maps`.
They overlay playback without rewriting source files. Source ID/path, runtime,
size and modification time invalidate stale maps. **Use original chapter markers**
removes the overlay. Include sidecars in backups alongside existing account data.
Manual aligned imports are supported by the same chapter-map API; automatic
Whisper alignment is not implemented in this pass.

Shared conversion and chapter changes require the library owner, a pairing
credential, or guarded direct loopback access. Ordinary authenticated users can
read timelines and verified outputs. Upstream lookup uses a fixed host,
redirect rejection, strict ASIN/region checks, timeouts and response-size bounds.

## Checks

- Web tests cover cover frames, MP3 detection, tag deduplication, edition review,
  runtime mismatch and exact chapter selection across multipart audio.
- Generated FFmpeg fixtures cover staged conversion and original retention.
  An 18-track fixture checks decoded chapter offsets within 2 ms and that each
  marker points at the expected audio, including different source sample rates.
- HTTP tests cover lookup bounds, import validation, persistence, stale-source
  fallback and conversion route authorization.
- No existing production audiobook is converted or retagged by deployment.
