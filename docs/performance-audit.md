# TM Sonder Performance Audit

Scope: `server/` Go backend (~6,400 LOC non-test), live server on NAS mount.
Method: code review of hot paths + benchmark run + live payload measurement.

---

## FINDINGS (ranked by impact)

### P1 — Probe queue blocks on ebook/audio files that gain nothing from ffprobe
**File:** `server/internal/library/scanner.go` (`scanLibraryInto`, ~line 320)
Every media-ish file gets a probeJob — including 4,548 ebooks (.epub/.pdf/.mobi)
and 2,979 audio files. ffprobe runs against each over the NAS mount (~1s/file).
Ebooks always fail probe; audio probes yield no thumbnail.
**Impact:** ~7.5k wasted ffprobe invocations ≈ 2+ hours of the first-scan time.
**Fix:** skip probe jobs when `lib.Kind == "ebook"`, or filter by extension:
```go
if api.FormatForExtension(ext) == api.FormatUnknown || lib.Kind == "ebook" {
    // register item but skip probe job
}
```

### P2 — Probe worker count hardcoded to 2
**File:** `cmd/sonder/main.go:150` — `scanner.SetProber(..., 2)`.
ffprobe is I/O-bound on the NAS (0.25–2s per file); 2 workers can't saturate
the pipe. 6–8 workers would cut first-scan wall time ~3-4x on typical NAS
links. Make it configurable (`config.ProbeWorkers`, default = NumCPU/2).

### P3 — Thumbnail generation serializes with probing inside each worker
**File:** `scanner.go:238-244`. Each worker runs ffprobe then ffmpeg
(another NAS read + decode) synchronously. A movie file costs probe+thumbnail
back-to-back in one worker slot.
**Impact:** doubles effective per-file latency for new video files.
**Fix:** decouple: push thumbnail jobs onto a second channel consumed by a
separate worker pool sized independently (thumbs are CPU-heavy, probes are
I/O-heavy — they scale differently).

### P4 — `countSidecars` does an extra directory listing per file during scan walk
**File:** `scanner.go:546` / `findSidecars` — `os.ReadDir(folder)` runs for
every candidate file just for the unchanged-check. For TV folders with many
episodes this re-lists the same dir once per episode file.
**Fix:** cache the last dir listing (dir path → entries) in the scanner with
a 1-entry memo; TV walks hit the same folder consecutively. Cheap win:
~13k redundant ReadDir calls eliminated.

### P5 — `Upsert` bumps generation and clones per item, called in tight loop
**Files:** `store.go` (`Upsert`), `scanner.go:360`.
During scan, each of 23k items triggers gen++ and a full deep clone
(slices copied). Then `rebuildLibraryPayload` clones+sorts again on next
request. Not a bottleneck today (payload cached by gen), but the clone-per-
Upsert doubles GC pressure during scans.
**Fix:** batch API: `UpsertBatch(items []*Item)` that bumps gen once.

### P6 — `/api/library` payload is 16 MB uncompressed, rebuilt on any mutation
**File:** `handlers.go:139-171`. Cached by generation — good — but every
probe Upsert bumps gen (23k times during a scan), so any client polling
`/api/library` mid-scan forces a full rebuild (clone 23k items + sort +
JSON encode + gzip ≈ 7ms CPU each poll, plus 16MB alloc). With multiple
clients this compounds.
**Fix:** rate-limit rebuilds (min interval 1-2s), or serve stale-with-ETag
during scanning. The gzip cache already exists — good — but consider
caching the sorted item slice separately from encoding.

### P7 — Snapshot write is 20MB JSON on every debounced save
**File:** `snapshot.go`. During scans, mutations arrive continuously →
debounced saves fire every 500ms, each marshaling 23k items (~20MB alloc +
write to NAS). This is likely why snapshot mtime updates lag and CPU stays
warm during scans.
**Fix:** lengthen DefaultSaveDelay during active scans (e.g. 30s), or write
atomically to local disk instead of the NAS.

---

## NON-ISSUES (verified OK)
- `/api/library` gzip + ETag caching — correct and effective
- `InternalItems()` sort — O(n log n), fine at this scale
- Progress last-write-wins logic — no lock contention risk
- Activity log capped at 200 — bounded
- BenchmarkLibrary10k: 7ms/op — healthy
- ffmpeg thumbnail args (`-ss` before `-i`) — correct fast-seek form
- Artwork `Generate()` skips existing outputs — good idempotency

## MEASURED BASELINE
- /api/library payload: 16.3 MB, 150ms cold, served from cache otherwise
- BenchmarkLibrary10k: 7.05 ms/op (M1 Pro)
- ffprobe on NAS file: 0.25s (movies) — network-bound
- First full-scan estimate: 21k files × ~1s ÷ 2 workers ≈ 3h
- With P1+P2 applied: est. ≤45 min

## RECOMMENDED ORDER
1. P1 (ebook probe skip) — trivial diff, biggest single win
2. P2 (worker count config) — one-line change + config field
3. P4 (sidecar dir cache) — small diff, big constant-factor win on TV walks
4. P7 (save throttle during scan) — small diff
5. P3/P5/P6 — worthwhile but lower urgency
