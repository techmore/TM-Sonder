# TM Sonder HTTP API

This file is the client integration reference for iOS and other Sonder clients. Keep it updated whenever server routes, request bodies, or response shapes change.

> **Reference implementations:** the macOS app (`xcode-TM-Sonder/SonderHTTPServer.swift`) and the Go server (`server/`, branch `go-port`). The Go server additionally supports `GET /stream/{id}?transcode=1` (see Streaming below); all other routes are wire-compatible with the shapes in this file.

Base URL comes from `/api/discovery` as `localURL` or `lanURL`.

## Auth

- **Loopback peers** (connections from `127.0.0.1` / `::1`) are always allowed without a token so the host Mac keeps working.
- **Non-loopback** requests require `allowLAN == true`.
- When a pairing token is configured, non-loopback requests must send it as:
  - `Authorization: Bearer <token>`, or
  - query `?token=<token>` (discouraged; may appear in logs)
- Auth is based on the **connection peer address**, not the client-controlled `Host` header.
- Product default: server enabled, **LAN off**. Enabling LAN auto-generates a pairing token.

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

## Library

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/library` | Full library catalog, progress records, public server settings, activity, and theme. |
| GET | `/library.json` | Compatibility alias for `/api/library`. |
| GET | `/api/status` | Server/library scan status. |

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

Media items expose relative artwork URLs instead:

- `posterURL`: `/artwork/poster/{id}` when artwork exists
- `backdropURL`: `/artwork/backdrop/{id}` when artwork exists

`serverSettings` on the wire:

```json
{
  "isEnabled": true,
  "allowLAN": true,
  "port": 8797,
  "themePreset": "earthy",
  "requiresPairing": true
}
```

Clients must store any pairing token they were given out-of-band (Keychain). The token is never returned by the API.

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

Transcode concurrency is bounded by `transcode.maxConcurrent`; disconnecting
clients kill their ffmpeg process group within ~1s.

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
| GET | `/audiobooks` | Browser audiobook interface. |

Audiobook catalog items also omit host filesystem paths.

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

Same auth rules as all other routes. Clients must send the Bearer token when pairing is required.
