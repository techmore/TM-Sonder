# TM Sonder HTTP API

This file is the client integration reference for iOS and other Sonder clients. Keep it updated whenever server routes, request bodies, or response shapes change.

### TV browsing identity (additive Go server fields)

Catalog TV items may include `showGroupID` (opaque stable string) and
`showGroupTitle` (source-folder display name). Group show cards by `showGroupID`,
then by season, while retaining individual item IDs for playback and progress.
Fall back to `showTitle` when these optional fields are absent. `showTitle`
remains the original parsed identity and is not overwritten by folder grouping.
Different folders/libraries have distinct group IDs. Grouping itself leaves
media IDs unchanged; folder renames are not part of this operation. Mixed-content diagnostics are
available at `GET /api/library/health`; warnings do not imply safe file merging.

TV items may also carry `showPosterURL` (art from the show folder) and
`seasonPosterURL` (art from the item's season folder, falling back to the
show poster). Show cards must prefer `showPosterURL` over any episode's
`posterURL`, because extras-folder art parses into regular season numbers
and would otherwise hijack the show cover. Season cards must prefer
`seasonPosterURL`. Both are omitted when the item has no group or no art.

> **Reference implementations:** the Go server (`server/` on `main`) is authoritative; the macOS app's embedded server (`xcode-TM-Sonder/SonderHTTPServer.swift`) is legacy and implements a subset. The Go server additionally supports `GET /stream/{id}?transcode=1` (see Streaming below) and the routes listed under [Route index](#route-index); all other routes are wire-compatible with the shapes in this file.

Base URL comes from `/api/discovery` as `localURL` or `lanURL`.

## Auth

- **Loopback peers** (connections from `127.0.0.1` / `::1`) are always allowed without a token so the host Mac keeps working.
- **Non-loopback** requests require `allowLAN == true`.
- The legacy pairing token is accepted on non-loopback requests as:
  - `Authorization: Bearer <token>`, or
  - query `?token=<token>` (discouraged; may appear in logs)
- Auth is based on the **connection peer address**, not the client-controlled `Host` header.
- Product default: server enabled, **LAN off**. Enabling LAN auto-generates a pairing token.
- The first account can be created through `GET /account/setup?token=<pairing-token>` and `POST /api/auth/setup`.
  This owner setup is one-time and requires a 12-character minimum password. The account file is stored with mode `0600`.
- Signed-in users can share the reusable invite link shown in Settings. It opens `GET /account/signup?ref=<invite-code>`;
  submitting the form calls `POST /api/auth/signup` with `username`, `password`, and `inviteCode`. Each new account stores
  the username of the inviter, and each invite owner can read their signup count from `GET /api/auth/invite` using their
  browser session. The response includes `username`, `inviteCode`, and `referralCount`. Invite links do not expire or
  limit the number of signups.
- Browser login uses `GET /account/login`, `POST /api/auth/login`, and the HTTP-only `sonder_session` cookie.
  `GET /api/auth/session` reports setup/authentication state and `POST /api/auth/logout` revokes the browser session.
- Accounts connect only to the person who invited them and the accounts they directly invite. New accounts share
  bookmarks and playback progress with those connections by default. `GET /api/people` returns connected usernames,
  signup dates, and activity each person has chosen to share; `PATCH /api/people/sharing` accepts optional
  `shareBookmarks` and `shareProgress` booleans to change the current account's defaults. The Profile → People panel
  exposes the same controls. The original owner's existing shared bookmarks and progress are migrated into that
  account's private activity store on first use; invited accounts start with their own empty activity.
- Compatibility logins validate an account: Jellyfin `POST /Users/AuthenticateByName` returns an `AccessToken`, and
  Audiobookshelf `POST /login` returns `user.token`. Those tokens are accepted as a Bearer token, Jellyfin
  `Authorization: MediaBrowser ... Token="..."`, or Audiobookshelf `?api_key=...`. Compatibility discovery endpoints
  (`/System/Info/Public`, `/QuickConnect/Enabled`, `/ping`, and `/status`) remain reachable so clients can begin login.
- An optional media-client-only compatibility pair can be supplied through the protected service environment variables
  `SONDER_COMPAT_USERNAME` and `SONDER_COMPAT_PASSWORD`. It is accepted only by the Jellyfin/Audiobookshelf login
  endpoints and issues the same scoped session token; it is not accepted by browser account login. Do not use a weak
  pair when the server is reachable from the public internet.
- Jellyfin primary artwork requests at `/Items/{id}/Images/Primary` are intentionally reachable without a catalog
  token for BookPlayer image loading. The route still resolves a known audiobook and exposes only its poster bytes;
  item metadata, catalog, and downloads remain authenticated.
- Sessions are random, expire after 30 days, and are persisted with the account file so a normal service restart or upgrade
  does not force every client to sign in again. Passwords are never stored in plaintext.

## Network binding and public proxy

`GET /api/network/interfaces` returns only interfaces that are currently up
and have an IPv4 address. Each entry includes its exact ID (`en0`, `bridge0`,
`utun4`, and so on), detected type, IPv4 address, and supported binding modes.
`GET /api/network/status` returns the persisted runtime state, web/API health,
Caddy/public pulse, uptime, and the external prerequisites for public access.

`POST /api/network/rebind` accepts `{ "mode": "wifi/lan", "interfaceID": "" }`
or an exact ID such as `{ "mode": "interface", "interfaceID": "utun4" }`.
The server validates the target before stopping anything, writes
`<dataDir>/runtime-state.json` atomically, restarts the per-user LaunchAgent,
updates and validates Caddy, checks local web/API readiness, and rolls back the
previous state when a post-switch check fails. The response is `202` because
launchd may replace the process while the operation is in flight; poll the
status endpoint for the final state. Caddy is only touched when it is enabled.

`POST` or `PATCH /api/network/exposure` applies interface and port exposure as
one operation. The body may include `mode`, `interfaceID`, `webPort`, and
`apiPort`; omitted ports retain their current values. Ports must be distinct
and within `1..65535`. It returns `202` with a `target` runtime state and a
`statusURL`; clients should poll until `rebinding` is false and both listener
health flags are true. The web Settings panel and menu-bar companion use this
route so a port change is applied live, with automatic reconnection when the
web port changes.

`PUT /api/network/proxy` accepts `enabled`, `publicDomain`,
`publicHealthCheckURL`, and `caddyBindAddress`. It changes only the persisted
proxy state and Caddy configuration. Caddy never configures DNS, router or
firewall forwarding, or WireGuard routes. The API listener defaults to
`127.0.0.1:8097`. The selected web listener defaults to Jellyfin's native
HTTP port `8096` and is a pairing-protected frontend
that proxies to that private listener, so browser and Jellyfin-compatible URLs
remain usable over the LAN without exposing the API process or a database port
directly. The catalog is a local JSON snapshot; no database port is opened by
the server.

## Local media cache

The server has a bounded read-through media cache. It is enabled by default at
`<dataDir>/media-cache` with a 100 GiB limit (`mediaCache.maxBytes`) and a
20 GiB free-space reserve (`mediaCache.minFreeBytes`). A request
that misses the cache continues streaming from the NAS while a complete copy is
created in the background and atomically promoted. Completed entries survive
service restarts. If the NAS is temporarily unavailable, the last completed
local copy can still serve when the catalog has its last-known file size and
modification time.

Eviction is priority-aware: ebooks are highest priority, audiobooks are next,
newer movies are preferred over older movies, and TV/documentary files are
lower priority. The cache is on-demand; it does not silently copy the entire
library at startup. Set `mediaCache.enabled` to `false`, or override
`SONDER_MEDIA_CACHE_ENABLED`, `SONDER_MEDIA_CACHE_DIR`, and
`SONDER_MEDIA_CACHE_MAX_BYTES`/`SONDER_MEDIA_CACHE_MIN_FREE_BYTES` for
host-specific policy.

## Discovery

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/health` | Basic server health, LAN, and pairing flags. |
| GET | `/api/discovery` | Server metadata, capabilities, theme, and endpoint templates. |

### Health response (shape)

```json
{
  "status": "ok",
  "name": "TM Sonder",
  "app": "TM Sonder",
  "id": "tm-sonder",
  "service": "_tmsonder._tcp",
  "library": "/api/library",
  "allowLAN": false,
  "requiresPairing": false
}
```

`allowLAN` and `requiresPairing` are JSON booleans.

### Discovery response (shape)

Includes:

- `serverID`, `app`, `name`, `version` (string), `build`
- `isEnabled`, `allowLAN`, `requiresPairing`, `port`
- `localURL`, `lanURL` (nullable when LAN is off)
- `discoveryMethods`, `tailscaleHint`
- `capabilities` (see below)
- `endpoints` (path templates)
- `theme`

**Capabilities** (all booleans):

| Field | Meaning |
| --- | --- |
| `books` / `ebooks` | Ebook support |
| `audiobooks` | Audiobook catalog/routes |
| `themes` / `themeSync` | Theme payload on discovery/library |
| `progressSync` | Progress/playback update routes |
| `mediaStreaming` / `videoStreaming` | Byte-range `/stream/{id}` |
| `artwork` | Poster/backdrop routes |
| `librarySync` / `remoteCatalog` | Full catalog fetch |

**Endpoints** (path templates):

| Field | Path |
| --- | --- |
| `health` | `/api/health` |
| `library` | `/api/library` |
| `audiobooks` | `/api/audiobooks` |
| `audiobookBrowser` | `/audiobooks` |
| `discovery` | `/api/discovery` |
| `progress` | `/api/progress/{id}` |
| `playback` | `/api/playback/{id}` |
| `playbackTrackRefresh` / `refreshTracks` | `/api/playback/{id}/refresh-tracks` |
| `stream` | `/stream/{id}` |
| `subtitles` | `/subtitles/{id}/{index}` |
| `poster` | `/artwork/poster/{id}` |
| `backdrop` | `/artwork/backdrop/{id}` |
| `movieMetadata` | `/api/movies/{id}/metadata` |
| `cacheStatus` | `/api/cache/status` |

## Library

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/library` | Full library catalog, the signed-in account's progress records, public server settings, activity, and theme. |
| GET | `/api/reading` | The signed-in account's ordered book queue, likes, and audiobook read/session history. |
| GET | `/api/people` | Direct inviter/invitee profiles and only the bookmarks/progress each account shares. |
| PATCH | `/api/people/sharing` | Update the signed-in account's bookmark and/or progress sharing preferences. |
| PATCH/PUT | `/api/reading/{id}` | Set `queued` and/or `liked` for an audiobook or ebook. |
| POST | `/api/reading/queue/reorder` | Set queue order with `{ "itemIDs": ["..."] }`. |
| POST | `/api/reading/{id}/sessions` | Upsert cumulative audiobook session time; body has `sessionID`, `activeSeconds`, `mediaSeconds`, and optional `completed`. |
| GET | `/library.json` | Compatibility alias for `/api/library`. |
| GET | `/api/movies/{id}/metadata` | Lazy, cached movie primer, cast/character portraits, directors, genres, and published ratings for the Rails detail panel. |
| GET | `/api/data/export` | Download a versioned, media-independent Sonder data bundle containing catalog metadata, lists, tags, order, and progress. |
| POST | `/api/data/import` | Restore or merge a data bundle. Accepts `mode` (`replace` or `merge`) and optional `pathMappings`; never starts a scan. |
| GET | `/api/status` | Server/library scan status. |
| GET | `/api/cache/status` | Local media-cache size, queue, and priority policy. |
| GET | `/api/network/interfaces` | Active IPv4-only interface inventory. |
| GET | `/api/network/status` | Runtime bind/API/Caddy health and public prerequisites. |
| POST | `/api/network/rebind` | Validate and asynchronously switch the web interface. |
| POST/PATCH | `/api/network/exposure` | Atomically update interface and web/API ports, then report live listener health. |
| PUT/PATCH | `/api/network/proxy` | Persist Caddy domain/health/bind settings and apply them. |
| GET | `/api/optimization/queue` | Generated recommendations for AAC M4B and selected large H.264 files. |
| GET | `/api/optimization/audiobooks/jobs` | Persistent audiobook job queue and status. |
| POST | `/api/optimization/audiobooks/jobs` | Queue selected recommendation IDs for staged Opus conversion. |
| POST | `/api/optimization/audiobooks/queue/pause` | Pause after the active book completes. |
| POST | `/api/optimization/audiobooks/queue/resume` | Resume queued work. |
| POST | `/api/optimization/audiobooks/jobs/{id}/retry` | Retry a failed, interrupted, or canceled job from a clean local workspace. |
| POST | `/api/optimization/audiobooks/jobs/{id}/review` | Record listening and target-device playback review. |
| GET | `/api/optimization/audiobooks/jobs/{id}/stream` | Range-capable stream for the verified staged output. |
| DELETE | `/api/optimization/audiobooks/jobs/{id}` | Cancel queued or active work; complete outputs cannot be canceled. |

### Portable data bundles

`GET /api/data/export` returns JSON with `format: "tm-sonder-data"`, a bundle
version, the configured library definitions, and a versioned catalog snapshot.
Media files are never included. The snapshot contains catalog metadata,
stable list references, list-specific tags and order, playback progress,
activity history, reading queue/likes, and audiobook listening history.

`POST /api/data/import` accepts the exported document directly. The optional
`mode` defaults to `replace`; `merge` overlays catalog items and newer progress
records while replacing lists with matching IDs. `pathMappings` maps exported
library roots to their current roots, for example:

```json
{
  "mode": "replace",
  "pathMappings": {"/Users/sean/NAS/Audiobooks": "/media/Audiobooks"},
  "format": "tm-sonder-data",
  "version": 1,
  "libraries": [],
  "snapshot": {"schemaVersion": 3, "items": [], "progress": [], "reading": {"queue": [], "records": []}, "lists": []}
}
```

### Book queue and listening history

`GET /api/reading` returns `queue` as ordered catalog item IDs and `records` as
book state. Use `PATCH /api/reading/{id}` with `{"queued":true}` and/or
`{"liked":true}` to change a book's queue or like state. Queue order is
updated through `POST /api/reading/queue/reorder`.

Audiobook clients post cumulative (not per-request delta) session totals to
`POST /api/reading/{id}/sessions`, for example:

```json
{
  "sessionID": "unique-client-session-id",
  "activeSeconds": 1800,
  "mediaSeconds": 2250,
  "completed": false
}
```

`activeSeconds` is real playback time excluding pauses; `mediaSeconds` is the
amount of the recording consumed, so their ratio reflects average playback
speed. Cumulative totals make retries idempotent. Mark `completed` only when
the final part of the book ends. Each audiobook can then retain multiple
completed read runs with start/finish dates and active listening duration.
The current EPUB/PDF view opens the file externally; page-level ebook position
and reading time are not yet tracked by Sonder.

Import is an in-memory/catalog operation followed by an atomic persistence
write. It does not walk the media roots, probe files, or prune missing media.
Run `POST /api/settings/rescan` separately when reconciliation is desired.
Catalog items carry a library-relative stable media key so changing an
absolute mount path does not create a second list/progress identity.

### Storage optimization triage

`GET /api/optimization/queue` recomputes a web-first recommendation list. It returns `jobs`
and `reviews` that need probe or media-integrity review, plus aggregate counts. Audio `estimatedSavingsBytes` and
`estimatedSavingsPct` are bitrate-based estimates, not measured output or
quality claims. AV1 candidates have no estimated output until a sample is
encoded. The response does not contain media filesystem paths.

Queueing an audiobook starts a persistent single-worker job. The server copies
the source into its local `dataDir/audiobook-optimization/work` workspace,
encodes Opus/MP4 there, verifies stream count/codec/container, channels,
duration, chapters, metadata, cover art, and full audio decode, then copies the
output back with a SHA-256 check. The separate result and JSON receipt live
under `<audiobook-library>/.sonder-optimization-staging/` with the original
relative folders. Hidden staging is excluded from library scans. The original
is never replaced or deleted. The configured audiobook library must be writable
and its filesystem must support same-directory hard links for atomic
no-overwrite publication; failures leave the source untouched.

The queue body is `{"itemIDs":["..."],"approvedCatalogCoverIDs":["..."],"bitrateKbps":{"itemID":32}}`.
The cover list explicitly approves available catalog artwork only when the
source has no embedded cover. Mono trial rates are 24/32/40 kb/s; stereo rates
are 48/64 kb/s. A verified job receipt reports actual source and
output bytes, reduction percentage, checksums, encoding bitrate, validation
results, and playback-review status. The stream endpoint supports HTTP Range
through the standard media responder.

Job statuses include `queued`, `copying-source`, `encoding`, `validating`,
`copying-result`, `staged-for-review`, `failed`, `canceled`, and `interrupted`.
The queue snapshot includes `paused`, `activeJobID`, and each job's phase and
progress. The browser UI polls while work is active, streams completed staged
outputs for listening review, and writes the review decision into both the
persisted queue and adjacent receipt.

Queue pause takes effect after the active book finishes; active encodes can be
canceled, which discards local partial work. Failed, interrupted, and canceled
jobs can be retried. A server restart marks in-flight jobs interrupted rather
than resuming a partial encode. AV1 and AAC-to-Opus remain lossy paths. A staged
output still needs listening and target-device playback review. Promotion into
the scanned library and original deletion are deliberately separate and are
not implemented. The web UI exposes these controls; the iOS app does not yet
manage optimization jobs.

### Conditional library fetch (ETag)

`/api/library` returns:

- `ETag: "sonder-library-{generation}"`
- `Cache-Control: private, max-age=0, must-revalidate`

Clients may send `If-None-Match: <etag>`. When the catalog generation is unchanged, the server responds `304 Not Modified` with an empty body.

### Client progress resilience

iOS (and other clients) should:

1. Apply progress optimistically in local state.
2. POST `/api/playback/{id}` (preferred) or `/api/progress/{id}`.
3. On network failure, queue the update durably (last write wins per item) and flush after the next successful library/health sync.

The server remains last-write-wins for progress records.

### Public library contract

`/api/library` returns **public DTOs only**. It never includes:

- pairing tokens
- absolute host filesystem paths
- security-scoped bookmarks
- local poster/backdrop/subtitle file paths

Media items carry `tags` (free-form provider keywords and people names) and,
separately, `genres` (the curated genre list used by the web UI's facets).
`genres` may be absent on rows imported or enriched before it existed; clients
should treat a missing or null `genres` as an empty list and fall back to
`tags` only after filtering out provider markers. `author` and `narrator` are
populated for audiobooks and ebooks.

Media items expose relative artwork URLs instead:

- `posterURL`: `/artwork/poster/{id}` when artwork exists
- `backdropURL`: `/artwork/backdrop/{id}` when artwork exists
- `showPosterURL` / `seasonPosterURL`: TV-only show/season-folder art endpoints
  (same `/artwork/poster/{id}` form, resolved to a representative episode)

`serverSettings` on the wire:

```json
{
  "isEnabled": true,
  "allowLAN": true,
  "port": 8096,
  "version": "0.2.10",
  "themePreset": "earthy",
  "libraryLayout": "rails",
  "hideEmptyLibraries": true,
  "requiresPairing": true
}
```

`version` identifies the running TM Sonder build. `libraryLayout` is the shared
web browser mode: `rails` is the default shelf-based Home and library
experience, while `classic` preserves the existing grid-first browser.
`audiobookLayout` is an independent `rails`/`classic` preference for the
dedicated `/audiobooks` player and is available through `GET/PUT /api/settings`.

`hideEmptyLibraries` defaults to `true`: the web navigation hides media tabs
whose catalog kind has no indexed items, while the configured library remains
available in Settings. Set it to `false` to show empty tabs while configuring a
new library.

Clients must store any pairing token they were given out-of-band (Keychain). The token is never returned by the API.

The admin Settings panel, or `PUT/PATCH /api/settings`, can update the local
cache without a restart:

```json
{
  "mediaCache": {
    "enabled": true,
    "maxBytes": 107374182400,
    "minFreeBytes": 21474836480
  }
}
```

`maxBytes` and `minFreeBytes` are byte counts. Lowering `maxBytes` immediately
evicts inactive lower-priority entries; active streams finish normally. The
cache directory remains managed by the server and is not changed by this
endpoint.

## Playback

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/playback/{itemID}` | Return stream URL, saved progress, selected tracks, subtitles state, and known audio/subtitle tracks. |
| POST | `/api/playback/{itemID}` | Save playback progress and optional track selections, then return the updated playback session. |
| PATCH | `/api/playback/{itemID}` | Same behavior as POST. |
| PUT | `/api/playback/{itemID}` | Same behavior as POST. |
| POST | `/api/playback/{itemID}/refresh-tracks` | Force the server to probe embedded audio/subtitle tracks and sidecar subtitle state, save results, and return the updated playback session. |
| POST | `/api/progress/{itemID}` | Compatibility progress update route. Also accepts optional track selections. |
| GET | `/stream/{itemID}` | Byte-range media stream for AVPlayer or browser playback. |
| GET | `/subtitles/{itemID}/{index}` | Download a sidecar subtitle file returned in `subtitleTracks`. |

### Streaming (Go server)

`GET /stream/{itemID}` direct-plays the original file with full Range/206
semantics. The Go server also supports on-the-fly transcoding to fragmented
MP4 for containers AVPlayer cannot play directly:

| Query param | Values | Meaning |
| --- | --- | --- |
| `transcode` | `1` | Enable the transcode path (fMP4; no Range support — players seek via fragment timestamps). |
| `mode` | `auto` (default), `remux`, `encode` | `auto` remuxes when the probed video codec is H.264, else encodes. |
| `ss` | seconds, e.g. `91.5` | Start position. Requests within 30s of a running session attach to it instead of respawning ffmpeg. |
| `sub` | embedded subtitle index | Burn in `embedded-subtitle:N` during encode. Ignored in remux mode. |
| `audio` | embedded audio index | Map `embedded-audio:N` as the output audio track. Defaults to the first audio track. |

In `remux` mode the video stream is copied for free, but audio is copied only
when the selected track's codec is MP4-safe (AAC, MP3, AC3, EAC3, ALAC).
Codecs that cannot live in MP4 (DTS, TrueHD, FLAC, Vorbis, PCM) are re-encoded
to AAC so the output still plays in AVPlayer.

The `audio` index matches the numeric suffix of the `embedded-audio:N` IDs in
`audioTracks`, so a client can pass the suffix of the user's selected
`audioTrackID`. Sessions are keyed by (path, mode, subtitle, audio), so
switching audio starts a fresh session instead of reusing the wrong one.

Transcode concurrency is bounded by `transcode.maxConcurrent`; disconnecting
clients are detached immediately and their ffmpeg session is reaped after a
short idle grace period (30s) that preserves warm-session seek reuse.

### Playback Update Body

```json
{
  "seconds": 123.4,
  "duration": 1440.0,
  "audioTrackID": "embedded-audio:0",
  "subtitleTrackID": "embedded-subtitle:0",
  "subtitlesEnabled": true
}
```

`seconds` and `duration` are required. Track fields are optional. If an optional track field is omitted, the server keeps the previous saved value. Send `subtitlesEnabled: false` to disable subtitles without forgetting the last subtitle track.

### Playback Session Response

```json
{
  "itemID": "UUID",
  "streamURL": "/stream/UUID",
  "seconds": 123.4,
  "duration": 1440.0,
  "percent": 0.0857,
  "updatedAt": "2026-07-03T12:00:00Z",
  "audioTrackID": "embedded-audio:0",
  "subtitleTrackID": "sidecar:0",
  "subtitlesEnabled": true,
  "audioTracks": [
    {
      "id": "embedded-audio:0",
      "label": "Japanese",
      "languageCode": "ja",
      "kind": "embedded",
      "url": null
    }
  ],
  "subtitleTracks": [
    {
      "id": "embedded-subtitle:0",
      "label": "English",
      "languageCode": "en",
      "kind": "embedded",
      "url": null
    },
    {
      "id": "sidecar:0",
      "label": "Episode 01.en",
      "languageCode": null,
      "kind": "sidecar",
      "url": "/subtitles/UUID/0"
    }
  ]
}
```

Track `kind` values:

| Kind | Meaning | Client Action |
| --- | --- | --- |
| `embedded` | Track inside the media container. | iOS selects locally with `AVPlayerItem.select(_:in:)`. |
| `sidecar` | Separate subtitle file served by Sonder. | Client downloads or attaches the provided `url`. |

## iOS Playback Requirements

The server streams the original media file. It does not rewrite embedded tracks during `/stream/{itemID}`.

iOS should:

1. Call `GET` or `POST /api/playback/{itemID}` before playback.
2. If `audioTracks` and `subtitleTracks` are missing or stale, call `POST /api/playback/{itemID}/refresh-tracks`.
3. Create an `AVPlayerItem` from `streamURL`, attaching the Bearer token via `AVURLAssetHTTPHeaderFieldsKey` when pairing is required.
4. Set `player.appliesMediaSelectionCriteriaAutomatically = false` when manually applying saved selections.
5. Use `.audible` media selection groups for embedded audio.
6. Use `.legible` media selection groups for embedded subtitles.
7. Match server embedded IDs by option index: `embedded-audio:0`, `embedded-subtitle:0`, etc.
8. Apply sidecar subtitle options from `subtitleTracks` entries whose `kind` is `sidecar`.
9. Load artwork (`posterURL` / `backdropURL`) with the same Bearer token.
10. POST progress plus any changed `audioTrackID`, `subtitleTrackID`, or `subtitlesEnabled` back to Sonder.

When no item-specific choice has been saved, the server may return default selections. Current default behavior prefers a Japanese audio track when one is detected and enables the first English subtitle track when available. This supports titles with both English dub and Japanese original audio, such as anime imports. The client should trust non-null returned `audioTrackID`, `subtitleTrackID`, and `subtitlesEnabled` values and apply them to the active `AVPlayerItem`.

For a Japanese-audio, English-subtitle preference, the client should choose the first `audioTracks` entry with `languageCode == "ja"` and the first `subtitleTracks` entry with `languageCode == "en"`, falling back to case-insensitive label matching when language codes are missing. If the server already returned those IDs, apply the returned IDs instead of recalculating.

## Audiobooks

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/audiobooks` | Audiobook catalog, optionally filtered with `?q=`. |
| GET | `/api/audiobooks/{itemID}` | Audiobook detail with chapter/playback snapshot. |
| GET | `/api/audiobooks/{itemID}/chapters` | Embedded chapter markers across every file of the book, with whole-book start times. |
| GET | `/api/audiobooks/{itemID}/download` | Original file download with HEAD/Range support (no transcode). |
| POST | `/api/audiobooks/{itemID}/chapter-lookup` | Look up chapter timings for the recording from the configured external provider (returns a preview to review). |
| POST/DELETE | `/api/audiobooks/{itemID}/chapter-map` | Apply or remove reviewed chapter timings. Owner/administrator only. |
| GET | `/api/audiobooks/{itemID}/conversion` | MP3→M4B conversion eligibility and job state for the book. |
| POST | `/api/audiobooks/{itemID}/conversion` | Start a staged M4B conversion; originals are retained. |
| GET | `/api/audiobooks/{itemID}/conversion/{stream\|download}` | Stream or download the converted M4B for review. |
| GET | `/audiobooks` | Browser audiobook player (rails layout). `/audiobooks-classic` is the list layout; `/audiobooks-beta` is an alias of the rails page. |

Audiobook catalog items also omit host filesystem paths. See
[`docs/audiobook-detail-tools.md`](docs/audiobook-detail-tools.md) for the
conversion and chapter tooling behaviour.

## Ebooks

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/ebooks` | Ebook catalog, optionally filtered with `?q=`. |
| GET | `/ebooks` | Browser ebook interface. |
| GET | `/read/{itemID}` | In-browser EPUB reader for one cataloged item (epub.js, self-hosted at `/epub.js` and `/jszip.js`). |

## Plex-style library root (`kind: "plex"`)

A library entry with kind `plex` is a **meta-library**: instead of being
scanned itself, it is expanded at load time (and on settings updates) into one
child library per recognized Plex-standard subfolder:

| Subfolder name (case/plural tolerant) | Expanded kind |
| --- | --- |
| Movies / Movie / Films | `movie` |
| TV Shows / TV / Shows | `tvShow` |
| Documentaries | `documentary` |
| Audiobooks | `audiobook` |
| Ebooks / Books | `ebook` |

Unrecognized child folders are ignored, so metadata caches and extras never
get misclassified. Child IDs are deterministic (`plex-<kind>-<rootname>`) so
restarts keep library and item identity stable. If no standard folders are
found the server refuses to start (config) or rejects the update (settings)
with a clear error. Example:

```json
{ "id": "plex", "name": "Plex", "path": "/Volumes/NAS/plex", "kind": "plex" }
```

## Artwork

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/artwork/poster/{itemID}` | Poster image. |
| GET | `/artwork/backdrop/{itemID}` | Backdrop image. |
| GET | `/artwork/curated/{itemID}` | Curated poster stored under `<dataDir>/curated-posters`, when one exists for the item. |

Same auth rules as all other routes. Clients must send the Bearer token when pairing is required.

## Route index

Routes registered by the Go server that are not covered by the sections
above. All of them follow the [Auth](#auth) rules unless noted.

### Movies

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/movies/continue` | Movie IDs the account dismissed from the Continue Watching shelf. |
| PATCH | `/api/movies/{id}/continue` | Body `{ "dismissed": true\|false }`; hide or restore a movie on that shelf. |
| GET | `/api/movies/{id}/preparation` | Status, progress, and ready asset URLs for prepared (HLS VOD + fast-start MP4) playback. |
| POST | `/api/movies/{id}/prepare` | Queue or reuse a preparation. |
| GET | `/stream/{id}/vod/{fingerprint}/{asset}` | Prepared playlists, segments, and MP4. |
| GET | `/stream/{id}/hls/{session}/{asset}` | Live HLS session playlists and segments produced by the on-the-fly transcoder. |

See [`docs/movie-preparation.md`](docs/movie-preparation.md).

### Reading lists

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/lists` | The account's lists (name, description, tags, ordered items). |
| POST | `/api/lists` | Create a list; `name` is required. |
| PATCH | `/api/lists/{listID}` | Update name, description, or tags (the `series` tag marks a reading order). |
| DELETE | `/api/lists/{listID}` | Delete a list; catalog items are untouched. |
| POST | `/api/lists/{listID}/items` | Add an item (`itemID`, optional `position`). |
| DELETE | `/api/lists/{listID}/items/{itemID}` | Remove an item. |
| POST | `/api/lists/{listID}/reorder` | Set item order with `{ "itemIDs": [...] }`. |

### Shared reading queue and Google link

| Method | Path | Purpose |
| --- | --- | --- |
| GET/PATCH | `/api/people/public-queue` | Read or set (`{ "enabled": true\|false }`) public sharing of the account's reading queue; the response carries `enabled` and, when on, the share `url`. |
| GET | `/shared/queue/{token}` | **Unauthenticated.** The shared queue page for a token; revoked when sharing is turned off. |
| GET | `/api/auth/invite` | The signed-in account's invite link and referral count. |
| GET | `/oauth2/start`, `/oauth2/link` | Begin Google sign-in / account linking (only when Google OAuth is configured). |
| GET | `/oauth2/callback` | OAuth redirect target. |

### Catalog reports

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/library/insights` | Counts, indexed storage, genres, largest entries, and personal activity for the Catalog insights panel. |
| GET | `/api/library/storage` | Indexed media sizes by directory; never walks the storage roots. |
| GET | `/api/library/health` | Read-only TV folder/identity consistency warnings. |

See [`docs/catalog-insights.md`](docs/catalog-insights.md) and
[`docs/library-health.md`](docs/library-health.md).

### Settings and maintenance

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/settings` | Current server settings (libraries, LAN, theme, layouts, media cache). |
| PUT/PATCH | `/api/settings` | Update settings; library changes trigger a rescan. |
| GET | `/api/settings/browse?path=` | Directory navigator for the add-library flow. |
| POST | `/api/settings/rescan` | Start a library scan (`202`), or report one already running. |
| POST | `/api/settings/enrich` | Fetch missing book descriptions and posters. |
| GET/POST | `/api/settings/subtitles` | Subtitle maintenance status / trigger a pass (owner or loopback only). |
| GET/POST | `/api/settings/youtube` | YouTube subscription state and actions (owner or loopback only). |
| GET | `/api/settings/youtube/permission` | `{ "owner": true }` when the caller may manage subscriptions; the library page uses it to show the YouTube link. |
| POST | `/api/optimization/audiobooks/jobs/{id}/prioritize` | Move a queued job to the front of the queue. |
| POST | `/api/optimization/audiobooks/jobs/{id}/promote` | Replace the original with a staged output that passed review. |

### Compatibility endpoints

Jellyfin-shaped routes used by BookPlayer and similar clients:

| Method | Path |
| --- | --- |
| GET | `/System/Info/Public`, `/QuickConnect/Enabled` (public) |
| POST | `/Users/AuthenticateByName` (public; returns `AccessToken`) |
| GET | `/UserViews`, `/Users/{userID}/Views`, `/Items`, `/Users/{userID}/Items` |
| GET | `/Items/{itemID}`, `/Users/{userID}/Items/{itemID}`, `/Items/{itemID}/Download` |
| GET | `/Items/{itemID}/Images/Primary` (public for known audiobook ids, see Auth) |
| GET | `/Artists/AlbumArtists`, `/Persons` |

Audiobookshelf-shaped routes:

| Method | Path |
| --- | --- |
| GET | `/ping`, `/status` (public) |
| POST | `/login` (public; returns `user.token`) |
| GET | `/api/libraries`, `/api/libraries/{libraryID}/items` |
| GET | `/api/items/{id}`, `/api/items/{id}/cover`, `/api/items/{id}/file/{ino}/download` |

### Browser pages and static assets

Pages: `/`, `/audiobooks`, `/audiobooks-classic`, `/audiobooks-beta`,
`/ebooks`, `/read/{id}`, `/youtube`, `/account/login`, `/account/setup`,
`/account/signup`. Unauthenticated browser requests for protected pages are
redirected to `/account/login?next=…`.

Assets (public, immutable when requested with the hashed `?v=` the page
emits): `/library.css`, `/library.js`, `/shared.js`, `/movie-downloads.js`,
`/video-presentation.js`, `/subtitle-maintenance.js`, `/youtube.js`,
`/youtube-link.js`, `/epub.js`, `/jszip.js`, `/favicon.svg`, `/favicon.png`,
`/favicon.ico`.
