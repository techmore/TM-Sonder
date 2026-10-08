# SER8 movie detail and public deployment correction

The movie card/search dialog now uses the same detailed renderer as the movie
shelf panel. It includes fixed-ratio cover art, a prominent playback action,
resume progress, synopsis, director/genre credits, cast, source-labelled ratings,
and collapsible playback/file information. Missing online information does not
block playback. Lazy metadata updates only the detail area, preserving browsing
position.

## Active production route

Both `https://sonder.stoverparc.org/` and the legacy
`https://stoverparc.org:8096/` retain their existing Caddy upstream at
`127.0.0.1:8098`. The Incus `sonder` instance now owns that upstream through its
`public-web` proxy device:

- Listen on host: `tcp:127.0.0.1:8098`
- Connect inside container: `tcp:127.0.0.1:8098`
- The private API remains on container loopback port 8097.
- The native user `tm-sonder.service` is stopped and disabled. Do not start it
  alongside Incus: they share the same data directory.
- The existing `sonder-data` mount preserves accounts, progress, lists, catalog,
  and caches at `/var/lib/sonder`.
- A read-only `media-legacy` mount preserves catalog file paths at
  `/home/sdolbec/NAS/plex`. Container library configuration uses those paths so
  future scans retain item identifiers.
- The service's `media-legacy.conf` override uses `ProtectHome=read-only` and the
  protected BookPlayer environment file. Other service restrictions remain.
- Memory limit is 6 GiB, allowing room for the existing catalog and caches.

The old domain served version `0.2.23+browser-epub` even after a successful Incus
GitHub deployment. Deployment verification now compares the publicly served
JavaScript and CSS with the checked-out revision, so this routing mismatch
cannot silently pass again.

Before cutover, configuration and top-level JSON state were backed up under
`/home/sdolbec/.config/sonder/migration-backups/movie-detail-20261008T014003Z`.
The previous container configuration remains at
`/etc/sonder/server.json.before-movie-cutover`.

Verified after cutover: 21,992 catalog items, existing signed-in browser session,
The Ninth Gate metadata, and HTTP 206 range access to its 2,489,018,105-byte file.

## Emergency rollback

Stop the container service and remove the `public-web` device before enabling
and starting the native user service. Never run both against the shared data.
Keep the backup and legacy binary until the container deployment is established.
