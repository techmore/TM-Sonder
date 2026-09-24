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
`port`/`webPort`, `apiPort`, `dataDir`, `libraries[]`, `allowLAN`, `safeScan`,
`probeWorkers`, `thumbWorkers`, `transcode`, and optional Caddy paths. Every
field also has a `SONDER_*` environment override (see the generated template).

Requirements: the FFmpeg package, which provides both `ffmpeg` and `ffprobe`,
on `PATH` (or configured paths) for track probing, thumbnails, chapters, and
transcoding. Prepare a native host with:

```bash
make prepare-install
```

The preflight detects Homebrew, apt, dnf, apk, or pacman, installs the FFmpeg
package when needed, and verifies both executables. The server can start
without them, but those media features will be unavailable.

The web listener defaults to Jellyfin's native HTTP port `8096`; the private
readiness/API listener uses `8097` on `127.0.0.1`. Local-only until LAN is enabled; enabling LAN
auto-generates a pairing token (persisted in `dataDir/pairing-token`). For a hosted
instance, Caddy can terminate HTTPS on the same external `:8096` port while
Sonder stays on a private local web port; the API and catalog storage remain
private. The active interface selection is stored separately in
`<dataDir>/runtime-state.json`, so changing adapters does not rescan or alter
the catalog.
The LAN-facing web listener is a pairing-protected frontend over the private
API listener, so existing browser and Jellyfin-compatible routes keep working
without binding the API process or catalog storage to the network. The first
browser account is created once from `/account/setup?token=<pairing-token>`;
after that, browsers use an HTTP-only session cookie and Jellyfin/Audiobookshelf
clients authenticate with the same account and receive a session token.

### Operations

```bash
make test                   # go test -race ./...
make vet
make fmt
make prepare-install       # install/check ffmpeg + ffprobe
make install-launchd        # build + install + bootstrap + health check
make container-build && make container-run

# Ubuntu host migration: keep public Jellyfin/BookPlayer HTTPS on :8096
sudo ./deploy/enable-ubuntu-https8096.sh

sonder app interfaces --json
sonder app status --json
sonder app proxy status
sonder app restart wifi       # also: ethernet, vpn, loopback, public, en0, utun4
sonder app proxy enable --domain books.example.com --health https://books.example.com/api/health
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
- **Network binding:** `sonder app interfaces --json` lists only active IPv4
  interfaces. Use `sonder app restart loopback`, `wifi`, `ethernet`, `vpn`,
  `public`, or an exact ID such as `en0`/`utun4`. Each swap validates first,
  keeps the API on loopback, updates Caddy when enabled, and rolls back on a
  failed health check. Live port exposure can be changed with
  `sonder app exposure --web-port 8897 --api-port 8898` or the web Settings
  panel; `POST /api/network/exposure` applies mode, interface, and ports as
  one operation and reports listener health while they reconnect. The
  `sonder app status --json` and `sonder app proxy status` commands expose the
  same state used by the menu-bar companion.
- **Menu-bar updates:** `make install-status-app` installs the per-user status
  companion. Its menu can check the `tm-sonder` Homebrew formula and install an
  update with visible progress through download, restart, and version
  verification. It only restarts a Homebrew-managed server; manually launched
  instances are left safe and report when a manual restart is needed.
- **Public exposure:** enabling Caddy only manages Sonder's reverse-proxy
  configuration. DNS records, router/firewall TCP 80/443 forwarding, and
  WireGuard routes remain explicit external prerequisites. The catalog is a
  local JSON snapshot in `dataDir`; no database listener is opened or exposed.
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
  The app container is named `tm-sonder` so it is easy to distinguish from
  Apple's internal `buildkit` builder helper in container-management UIs.

### Releases and Homebrew

`VERSION` is the source of truth for the server, container labels, and macOS
app marketing version. Run `make release-check`, commit the result, and tag it
as `vX.Y.Z`; `.github/workflows/release.yml` verifies the tag, runs the release
checks, builds arm64 macOS/Linux archives, and publishes SHA256 checksums.

The native Homebrew formula lives in [`Formula/tm-sonder.rb`](Formula/tm-sonder.rb)
and can be tested as a local tap using the instructions in
[`packaging/homebrew/README.md`](packaging/homebrew/README.md). It installs
the same `sonder` binary and configuration model as the native and container
paths; Homebrew is optional and Orchard is not required.

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
