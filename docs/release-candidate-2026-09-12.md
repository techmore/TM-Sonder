# Release candidate — September 12, 2026

Status: implemented and automated checks passing; **not approved for distribution**
until the physical-device acceptance steps below pass. No service deployment,
device installation, commit, push, or NAS media rewrite was performed.

## Included changes

- iOS background downloads synchronously persist media before URLSession releases
  its temporary file. Transfer descriptions retain media and server identity so
  completion can save a manifest after an OS background relaunch without a live
  view callback. Foreground refresh reconnects to active transfers.
- Download manifests retain item metadata so offline items remain accessible when
  the server catalog changes. Downloads opens those items directly.
- PDF, EPUB, and M4B are downloadable. PDF restores its page. EPUB has a built-in
  local reader following OPF spine order, section selection, text sizing, and
  automatic section/scroll-position bookmarks. Reading bookmarks are local to
  the device; EPUB font encryption/DRM is not supported.
- Audiobook chapter metadata and covers are saved alongside downloads when
  available. Existing downloads should be downloaded again to populate metadata
  newly added by this release.
- EPUB extraction rejects path escapes and symlinks, bounds entry count and
  expanded size, disables XML external-entity resolution, and rejects encryption.
  The web reader blocks scripts, remote resources, and navigation outside the book.
- ZIPFoundation 0.9.20 is pinned through SwiftPM. Its MIT license and privacy
  resources remain in the dependency. Lockfile changes are intentional.
- Go catalog compatibility: Swift library identifiers now accept opaque strings;
  nil collection fields decode as empty arrays; client dates accept fractional
  ISO-8601 timestamps.

**Swift source compatibility change:** `libraryID` on public media items and
`id`/`libraryID` on public directories are Strings instead of UUIDs. The legacy
macOS host adapters were updated. The HTTP representation remains JSON strings.

## Verified

- `make test`: four Node version-grouping tests and Go race tests pass.
- `make vet`: passes.
- Shared SonderAPI package: five tests pass.
- Client SwiftPM harness: five tests pass with live-server and real-EPUB inputs.
  The real server's 20,881-item payload decodes. The NAS DNS and BIND EPUB opens
  with 32 readable sections. Fixture checks cover reading order, unsafe paths,
  offline bytes, manifest recreation, server separation, and removal errors.
- iOS Release simulator build, unsigned device archive, and macOS Release build
  pass. Go macOS arm64 release binary builds.
- Modified-file diff whitespace checks pass. There is a pre-existing trailing
  blank-line warning in `.github/workflows/ci.yml` outside this pass.

Reproduce the client acceptance tests:

```sh
cd IOS_Client_Xcode/TM_Sonder_Client
SONDER_TEST_URL=http://127.0.0.1:8096 \
SONDER_TEST_EPUB='/Users/seandolbec/NAS/plex/ebook/dnsandbind_5thedition.epub' \
swift test
```

Without those environment values, the two integration checks skip explicitly.
The only dependency warning observed is ZIPFoundation's legacy watchOS platform
declaration under Xcode 27. No warnings or tests were suppressed.

## Required device acceptance

1. Install a signed candidate on the paired Techmore iPhone and connect to LAN.
2. Download one PDF, EPUB, and M4B; compare completed byte counts with server files.
3. Pause/resume a large transfer. Background the app during another transfer,
   allow completion, and reopen. Verify both progress and durable completion.
4. Enable airplane mode; relaunch and open all three downloads from Downloads.
   Check EPUB text/images, section selection, and restored position; check PDF
   restored page; check M4B chapters and playback position.
5. Lock the phone during M4B playback. Check pause/play and ±15-second controls,
   route interruptions, and position after returning. Lock-screen remote command
   behavior is not certified by the automated tests.
6. Remove a download and verify it no longer appears or consumes its media space.
7. Reconnect and verify queued audiobook progress sync.

## Normalization and remaining scope

The engine scans configured roots, normalizes catalog naming and classification,
groups quality variants in the web UI, and chooses local/Plex artwork. Original
files remain intact. Automatic bulk conversion, filename rewrites, and moving
NAS files are not part of this candidate. Unsupported containers require an
explicit derived-copy conversion workflow; client-ready normalization of every
format is not claimed. Native iOS quality grouping still needs parity with the
web grouping corrections. Missing/thumbnail covers remain as documented in the
NAS audit. EPUB navigation currently presents spine sections rather than a
full semantic TOC or annotation system.

The nested iOS repository still uses a gitlink without `.gitmodules`, so clean
checkout/release packaging remains a repository-level release blocker.
