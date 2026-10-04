# Audiobook lifecycle audit

Code review: 2026-10-03. Scope: Go server, current browser audiobook controller,
and the checked-out iOS client. Findings are code observations; this is not an
on-device certification of iOS/macOS 27. Simulator/device and Safari testing on
installed OS versions remains necessary.

## What already works

- Server book grouping identifies recordings separately from alternate complete
  copies, assigns part indexes, and sorts filenames naturally (`2` before `10`).
  The browser honors the derived order instead of treating each file as a book.
- Browser playback uses a persistent media element outside refreshed library
  regions. Closing details and changing shelves do not discard audio.
- Multipart progress is written against the actual playing file. Whole-book
  progress sums the parts; resume chooses the most recently heard unfinished part.
- Browser checkpoints are serialized per item, retained locally until acknowledged,
  timestamped monotonically, and replayed after reconnect. Stale library responses
  do not overwrite newer in-memory checkpoints.
- Browser hidden/pagehide handlers checkpoint progress. `pageshow` restores media
  controls. A play attempt preserves file offset/rate, checks actual advancement,
  and retries a stalled source once. Media Session actions and optional Safari
  audio-session support are feature detected.
- Direct server streams use `http.ServeContent`, with byte ranges, last-modified
  and seek support. Transcoding is a separate fragmented stream without ordinary
  range semantics. Artwork supports conditional/cacheable responses; static
  versioned assets are immutable, unversioned assets revalidate.
- iOS explicit downloads use Application Support, an atomic manifest, retained
  item metadata, chapters, and artwork. They are distinct from disposable caches.
  An OS background URLSession reconnects via the app delegate and supports pause
  with resume data. Download requests carry authorization headers.
- Native progress has a durable last-write-per-item queue, bounded to 200 entries.
  Local playback can start without a network request when a saved file exists.

## Highest-value remaining work

1. **Native player lifetime and interruption behavior.** The iOS player belongs to
   a view and `onDisappear` pauses and releases it. Move audiobook playback into
   an application-owned controller, then connect a persistent mini player,
   explicit audio-session interruption/route observers, and Now Playing/remote
   commands. Background audio mode alone does not establish these behaviors.
   No explicit `MPRemoteCommandCenter`/`MPNowPlayingInfoCenter` implementation was
   found in this checkout. Test calls, unplugged headphones, lock-screen controls,
   Bluetooth route changes, and closing the detail view.
2. **Native multipart parity.** The reviewed native player constructs one
   AVPlayerItem. Book-level ordered play/advance and download-all-parts need a
   shared book queue that uses server grouping/indexes. Do not label a single
   downloaded segment as a complete offline book.
3. **Browser offline expectations.** No service worker or durable browser media
   download store was found. Browser buffering and server media cache do not mean
   an audiobook is available offline. Keep native “Saved on this device” wording
   distinct from browser streaming. A browser download feature needs a durable
   manifest, range-aware local playback, storage quota/eviction handling, and
   explicit readiness for every part before displaying “Available offline”.
4. **Native download resilience.** Resume files are currently keyed only by item
   ID, while completed downloads are scoped by server URL. Scope resume state by
   server identity too. If the OS rejects stale resume data, retry the authorized
   original request instead of leaving users in a repeated resume failure loop.
   Retain an existing completed file until its replacement moves successfully;
   current save removes the old destination before moving the new file.
5. **Native startup latency.** `startPlayback` refreshes tracks when either audio
   or subtitle tracks are empty. Audiobooks ordinarily have no subtitles; this
   condition can unnecessarily request refresh/probing. Use media-kind-aware
   conditions and benchmark cold/warm chapter fetch plus first audible sample.
6. **Freeze and termination validation.** Browser checkpoints cannot guarantee an
   event runs when a tab/process is killed. Confirm the existing periodic save
   interval limits lost position, while preserving queued checkpoints. Test
   elapsed sleep timers across suspend; a suspended page cannot promise a timer
   will fire at its deadline. Device background playback requires device testing.

## Acceptance matrix

| Scenario | Expected result |
| --- | --- |
| Desktop library refresh/details close | Audio remains audible; mini player remains accessible |
| Narrow mobile viewport | Cover/primary Play action visible; seek and chapter controls usable by touch |
| Part `2` vs part `10`, alternate complete recording | Natural part order; copies remain separate |
| Resume midway through later part | Same file and exact saved offset; total book progress accurate |
| Lock/unlock, tab restore, system interruption | Position/rate retained; actionable recovery rather than silent Play state |
| Wi-Fi to mobile data and back | Recover stream; queued progress syncs without rewinding |
| Offline native launch | Saved media, metadata, artwork, and chapters usable; writes queue locally |
| Pause/restart download and app relaunch | Transfer reconnects; byte progress accurate; stale resume data recoverable |
| Low storage/server switch | No false offline readiness; scoped cache/resume state; existing good file preserved |

## Repeatable local bot preview

Run from the repository:

```sh
python3 tools/audit-preview.py --port 8768
```

Open `http://127.0.0.1:8768/audiobooks`. This serves the working tree's assets
with real production catalog, audiobook details, chapters, and artwork, using
existing SSH access to SER8 and its existing Incus Sonder pairing credential.
It requires neither a copied library nor a new public credential. `Ctrl-C`
stops the preview and SSH tunnel. The optional host/container/address flags
support a changed deployment; the default address is `10.96.131.52`.

The preview binds only loopback, rejects foreign Host/Origin/browser fetch-site
subrequests, and forwards only an explicit GET/HEAD route allowlist. Top-level
navigation to static preview pages is permitted. Settings,
exports, optimization jobs, unknown queries, transcoding, and all mutations are
blocked. The pairing credential stays in process memory and an upstream Bearer
header; it is never put in browser URLs, cookies, responses, or access logs.
Upstream redirects and Set-Cookie headers are not forwarded. Media is streamed
in 64 KiB chunks with Range/conditional headers; metadata is capped at 64 MiB.
Local assets and upstream data use `no-store` to avoid stale preview assets.

This is an audit of real library display and direct playback, **not a sandbox
for live account changes**. Progress/bookmark/queue writes deliberately return
403, so checkpoint persistence and cross-device state must be validated through
a normal signed-in production session or a dedicated writable test instance.
The pairing credential represents the library's default profile rather than the
signed-in user's personal reading state. Ordinary upstream reads can populate
server artwork/chapter/media caches, but cannot trigger explicit administrative
mutation routes. Any trusted local process can access the preview while it runs;
stop it after the audit.

Security checks:

```sh
python3 -m unittest discover -s tools -p test_audit_preview.py
```

## Implemented in this pass

- Current main library audiobook view prioritizes compact Continue listening,
  bookmarked titles, and a single full catalog. Metadata filters open on demand;
  mobile uses the same Filters control. Artwork retains its original proportions.
- Book details put Resume before progress and chapter diagnostics. Missing series
  metadata is a collapsed setup option rather than a large empty panel.
- Original-file downloads use an authenticated audiobook-only endpoint with
  attachment disposition, HEAD and byte Range support. Multipart download links
  follow the server's playback order. These are ordinary file downloads to an
  external player, not a browser offline library or resumable native transfer UI.
- Alternate classic/rails pages now checkpoint every ten seconds during playback,
  on pause, and when hidden/leaving. Previously a continuously cancelled 500 ms
  debounce could postpone saves throughout playback. Resume responses cannot
  seek a detached player, a different part/source, or playback already started.
  Automatic part advancement starts at zero rather than replaying old progress.
- Remaining alternate-page debt: chapter rendering still uses detail-page chapter
  data rather than the canonical whole-book timeline; its resume selection lacks
  newest-part timestamps and its progress writer lacks the main controller's
  durable queue. The main library controller is the recommended playback surface.
- Brave desktop and a narrow viewport were visually reviewed against real catalog
  data. Automated web tests cover recency/grouping, download part order, primary
  action order, periodic checkpoints, and existing main-controller lifecycle
  behavior. HTTP tests cover downloads, ranges, HEAD, and remote authentication.
  No physical iPhone/iPad/macOS sleep, call interruption, or browser kill was tested.

## Apple behavior references

Use documented API capabilities and feature detection rather than assuming a
particular iOS/macOS version changes suspension rules. The downloaded file,
progress checkpoint, network connection, and player controller have different
lifetimes and must be tested independently.

- [Apple: Downloading files in the background](https://developer.apple.com/documentation/foundation/downloading-files-in-the-background): reconnect background sessions using the same session identifier on relaunch.
- [Apple: AVAssetDownloadURLSession](https://developer.apple.com/documentation/avfoundation/avassetdownloadurlsession): dedicated background HLS asset downloads; original M4B/MP3 files use URLSession instead.
- [WebKit: Safari 26 features](https://webkit.org/blog/17333/webkit-features-in-safari-26-0/): published browser capabilities; no unverified iOS/macOS 27 guarantees are asserted here.

## Native client follow-up (2026-10-03)

The iOS source is now part of this repository, with pinned dependency versions
and unsigned Xcode 27 CI coverage. Its previous embedded Git metadata and local
source were backed up before conversion.

- A model-owned audiobook player survives dismissing the player screen and
  navigating the library. A mini player reopens it without restarting playback.
- Ordered multipart playback, current-part canonical chapters, sleep timers,
  headphones/interruption handling, and lock-screen commands are implemented.
- Checkpoints are persisted before network awaits, scoped to the selected
  server, and replayed with timestamps. Stale acknowledgements cannot remove
  newer pending updates; cached library loading merges pending local progress.
- Original-file background downloads retain server-scoped resume data, retry
  stale resume data once, and replace existing saved files atomically. Multipart
  books are only labeled fully offline when every part is present.

Persistence tests and an unsigned local build cover code and file behavior.
Real-device calls, lock-screen controls, suspension, sleep/wake, airplane-mode
playback, and OS-driven background download recovery have not been tested.
This change publishes source; it does not install a new app on a device.
