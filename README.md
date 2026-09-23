# TM Sonder

A personal media library stack: a **headless Go server** that catalogs and
streams movies, TV, audiobooks, and ebooks, plus **Swift clients** for macOS
and iOS.

The Go server is the authoritative implementation of the wire contract in
[`API.md`](API.md). The macOS app is a library-management and playback GUI; it
still contains a legacy embedded HTTP server that the Go server supersedes.

## Layout

| Path | Role | Status |
| --- | --- | --- |
| `server/` | Go media server (catalog, scan, probe, transcode, web UI, HTTP API) | **Authoritative** |
| `Packages/SonderAPI/` | Shared Swift wire DTOs and route constants | Shared by both clients |
| `IOS_Client_Xcode/` | iOS client | Active, but see note below |
| `xcode-TM-Sonder/` | macOS app (library manager + legacy embedded server) | Active GUI; embedded server is legacy |
| `API.md` | HTTP contract for clients | Keep updated with route changes |
| `deploy/` | launchd install + Apple `container` image | Ops |
| `docs/` | Audits and design notes | Reference |

> **Repo caveat:** `IOS_Client_Xcode/TM_Sonder_Client` is currently committed as
> a bare gitlink (submodule pointer) with **no `.gitmodules` entry**, so a fresh
> clone cannot obtain the iOS sources. Either add a proper submodule or vendor
> the directory into this repo. See [`docs/review-2026-09.md`](docs/review-2026-09.md).

## Run the Go server

```bash
make mac                    # → bin/sonder-darwin-arm64
./bin/sonder-darwin-arm64 -config ~/.config/sonder/server.json
```

On first run the server writes a commented config template. Edit `libraries`
to point at your media roots, then restart. Key config fields:
`port`, `dataDir`, `libraries[]`, `allowLAN`, `safeScan`, `probeWorkers`,
`thumbWorkers`, `transcode`. Every field also has a `SONDER_*` environment override (see the
generated template).

Requirements: `ffprobe` and `ffmpeg` on `PATH` (or configured paths) for track
probing, thumbnails, chapters, and transcoding. The server runs fine without
them, minus those features.

Default port: `8797`. Local-only until LAN is enabled; enabling LAN
auto-generates a pairing token (persisted in `dataDir/pairing-token`).

### Operations

```bash
make test                   # go test -race ./...
make vet
make fmt
make install-launchd        # build + install + bootstrap + health check
make container-build && make container-run
```

- **Portable data:** the catalog is persisted to `<dataDir>/library.json` as a
  versioned, atomically replaced snapshot. It includes catalog metadata,
  reading lists, list tags/order, and user activity. `GET /api/data/export`
  downloads a media-independent bundle; `POST /api/data/import` restores or
  merges it without scanning or modifying media. Use `pathMappings` when
  moving between native paths and container mounts. Playback progress is written to a small
  `<dataDir>/progress.json` sidecar on each heartbeat and overlaid on load, so
  progress updates do not rewrite the full catalog. `safeScan` (default on)
  refuses to prune a library that suddenly yields zero files while the catalog
  still holds items, so an unmounted NAS share cannot wipe the catalog. Set
  `"safeScan": false` to let a deliberately emptied library prune normally.
- **Bonjour:** advertised as `_tmsonder._tcp` when LAN is enabled.
- **`/debug/pprof/*`:** loopback-only.
- **`?transcode=1`:** on-the-fly fragmented-MP4 transcode for containers
  AVPlayer cannot play directly. See `API.md` for `mode`, `ss`, and `sub`.
- **Web player:** the library page has a built-in persistent player. Audio
  (including `.m4b` audiobooks) and browser-safe video stream directly; other
  video containers are transcoded on demand. Playback continues in a docked
  bottom controller while you browse, with speed control, ±30s skips, Media
  Session (lock-screen/OS) controls, and progress sync.
- **Containers are optional:** the OCI image in
  [`deploy/Containerfile`](deploy/Containerfile) can run under Apple
  Container/Orchard, Docker, or another compatible runtime. Mount media
  read-only and persist `/data`; Orchard is not required by the server or API.

The intended native Homebrew distribution is `brew install <tap>/sonder`
followed by `brew services start sonder`. The Homebrew and container paths use
the same configuration, API, and portable data bundle.

For scripted backups or migrations, the server binary also supports catalog
maintenance without starting the web service:

```bash
sonder -config ~/.config/sonder/server.json -export-data sonder-backup.json
sonder -config ~/.config/sonder/server.json -import-data sonder-backup.json
# When the media root changed:
sonder -config ~/.config/sonder/server.json \
  -import-data sonder-backup.json \
  -import-path-map '/Users/sean/NAS/Audiobooks=/media/Audiobooks'
```

Imports replace catalog data by default, accept `-import-mode merge`, and do
not scan media. Run the normal rescan command only when you want to reconcile
the restored catalog with the current filesystem.

## Run the clients

```bash
# macOS app
open xcode-TM-Sonder.xcodeproj

# iOS app
open IOS_Client_Xcode/TM_Sonder_Client/TM_Sonder_Client.xcodeproj
```

Both clients point at the server URL + pairing token shown in the server
dashboard or config.

## Tests

```bash
# Go server (authoritative)
cd server && go test -race -count=1 ./...

# Shared Swift package
cd Packages/SonderAPI && swift test

# macOS host unit tests
xcodebuild test -project xcode-TM-Sonder.xcodeproj -scheme xcode-TM-Sonder \
  -destination 'platform=macOS' -only-testing:xcode-TM-SonderTests
```

CI runs the Go format/vet/test/cross-compile, the SonderAPI package tests, and
the macOS unit tests (`.github/workflows/ci.yml`).

## Documentation

- [`API.md`](API.md) — HTTP contract, auth, streaming, playback, artwork.
- [`docs/review-2026-09.md`](docs/review-2026-09.md) — architecture review and
  prioritized improvement roadmap.
- [`docs/performance-audit.md`](docs/performance-audit.md) — scan/stream hot
  paths.

## Delegate from OpenCode to Pi

This repo includes an OpenCode MCP entry for Pi. Start OpenCode from the repo
root, then ask it to use the `pi_delegate` tool for an independent coding or
review pass. The bridge starts a fresh, non-persistent Pi RPC session in the
current project for each call.

Set `PI_BIN` if `pi` is not on OpenCode's `PATH`:

```bash
PI_BIN="$HOME/.local/bin/pi" opencode
```
