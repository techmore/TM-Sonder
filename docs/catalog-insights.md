# Catalog insights

Open the chart button in the library header, Settings → Catalog insights, or `/#storage`.
The header shortcut starts with the current media type. Library and media filters
apply to counts, storage, genres, largest entries/files, personal activity, and folders.
Refresh updates the report; Scan libraries refreshes the catalog from disk.

## What the numbers mean

- Catalog entries group multipart audiobooks within each library. Film versions,
  TV episodes, and other indexed media files remain separate entries.
- Indexed media size sums source file sizes recorded by the scanner. This is not
  filesystem capacity or free space and excludes caches, conversion staging, and
  unindexed files. Files with missing sizes are called out.
- Largest entries aggregate audiobook parts; individual file details include the
  filename without exposing filesystem paths. Both rankings show the top ten.
- Genres use curated genre metadata, not arbitrary tags. Case variants count once
  per entry. Entries may have multiple genres, so genre counts overlap. The view
  shows the top fifteen and the full unique genre count, plus missing metadata.
- Most accessed uses the signed-in account’s saved listening/reading sessions.
  Repeated checkpoints and sessions spanning book parts are deduplicated. Stream
  and range requests are not visits. Earlier activity cannot be reconstructed.
  Time spent is active wall-clock time, rather than speed-adjusted media time.

## Performance measurements

Sonder uses an in-memory catalog persisted to JSON files, rather than a SQL database.
Catalog read latency shows measured count, average, maximum, and last duration
for single-entry reads, public catalog reads, internal snapshots, and progress reads.
Measurements include lock wait, copying, and sorting where applicable. They exclude
HTTP transport, disk persistence, and playback. Counters are server-wide, independent
of the selected library, and reset on restart. Unsampled operations show a dash.

The fixed-size counters retain no per-request samples or item identifiers. Personal
activity is account-scoped; pairing-token requests cannot retrieve an account’s
listening history. The API response uses `Cache-Control: private, no-store`.

## API

`GET /api/library/insights?libraryID=<id>&kind=<kind>` follows existing API authentication.
Omit filters for all libraries/media. Unknown libraries or media kinds return 400.
The browser keeps a snapshot for fifteen seconds to avoid redundant navigation
requests; explicit refresh and filtering always fetch a new report. Stale overlapping
responses cannot replace a newer selection.
