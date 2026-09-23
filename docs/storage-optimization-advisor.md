# Storage optimization advisor and audiobook jobs

Sonder's library page produces recommendations through `GET /api/optimization/queue`. It identifies AAC-in-M4B files with enough measured audio bitrate for a trial target, and large H.264 video that may merit AV1 sample encoding. The AV1 list has no size estimate. HEVC and VP9 are not treated as automatic wins.

Audiobook estimate math uses duration multiplied by 32 kb/s mono or 48 kb/s stereo, plus a 5% allowance. This is an audio-oriented screening estimate, not measured whole-file output or an audible-quality claim. Actual savings are documented only after a staged full-book job finishes.

## Audiobook job flow

The web UI lets a user select audiobook recommendations and create jobs. Jobs are persisted atomically under `dataDir/audiobook-optimization/queue.json`; local temporary media lives under `dataDir/audiobook-optimization/work/`. The single worker performs:

1. Confirm the source size and modification time still match the queued catalog version.
2. Copy the source to local scratch while calculating SHA-256, then confirm it was not changed during the copy.
3. Encode the local copy as Opus in an MP4-family `.m4b`, preserving chapters and common metadata. Embedded artwork is retained; if absent, the UI can explicitly approve existing catalog artwork. No artwork is silently fetched or assumed to be exact.
4. Validate Opus codec/container, channel count, total and audio duration, chapter names/times, common metadata, attached cover, and a complete FFmpeg audio decode.
5. Hash the encoded output, copy it to a same-directory temporary file under `<audiobook-library>/.sonder-optimization-staging/<original-relative-folders>/`, verify its copied checksum, then publish without overwriting an existing output. Write a JSON receipt beside it.
6. Remove local media scratch. Keep the original library file untouched.

The review receipt records source/output size, bytes and percent saved, source/output SHA-256, target bitrate, container, codec, channels, duration, chapter count, cover source, validation results, review status, and the relative staging path. The web UI can stream the staged result with HTTP Range and record whether listening and target-device playback passed. If listening indicates that the encode needs revision, the user can select a different bitrate and stage a separate variant. AV1 and AAC-to-Opus are lossy; automated checks establish file integrity and metadata preservation, not subjective quality.

The queue can pause after the current book, resume, cancel active or queued jobs, retry failed/interrupted jobs, and preserve state across server restarts. A restart does not resume partial FFmpeg output; in-flight records become retryable `interrupted` jobs. Source promotion and deletion are not part of this workflow. A verified output remains separate in the hidden NAS review folder until a future deliberate promotion process.

The configured audiobook library must be writable for output transfer. The server checks local and destination free space, and output publication requires same-directory hard-link support to guarantee no-overwrite behavior. Unsupported or read-only NAS mounts fail the job while leaving the source unchanged. Output folders are hidden and excluded by the scanner.

`tools/audiobooks/` remains the offline audit/planning/batch workflow. It is useful for inventory-wide migration work and verified cover research; the web job runner is for user-selected individual conversions and keeps its own persistent job records and receipts.
