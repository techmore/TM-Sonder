# Reusable movie playback

The movie detail page offers **Prepare movie**. Preparation creates a complete H.264 Main / AAC stereo HLS VOD and a fast-start MP4, using six-second segments aligned to forced keyframes. Safari uses native HLS. Browsers without native HLS use the prepared MP4 with HTTP range requests. Prepared playback preserves the full movie timeline for seeking and resume; unprepared playback retains the live-stream fallback.

Authenticated endpoints:

- `GET /api/movies/{id}/preparation`: status, progress and ready asset URLs.
- `POST /api/movies/{id}/prepare`: queue or reuse preparation.
- `GET /stream/{id}/vod/{fingerprint}/{asset}`: authenticated playlists, segments and MP4.

The fingerprint includes the source path, size, modification time and encoding profile. Completed assets survive service restarts. Incomplete encodes are disposable and can be retried. Source media is never removed by this cache.

## Server storage and resource limits

Assets live under `DataDir/movie-vod-cache`. The default combined HLS and MP4 budget is 20 GiB; set `SONDER_VOD_CACHE_MAX_BYTES` on the service to change it (minimum 1 GiB). Preparation is serial, with at most eight queued jobs and a six-hour deadline. It uses the existing shared encoder limit and a low CPU scheduling priority.

Before encoding, space is reserved for both representations. Preparation rejects movies larger than the budget. Old assets are removed in least-recently-used order as space is needed; assets unused for seven days are also eligible. Assets requested within 30 minutes are protected. Cleanup runs when another preparation starts. Encoding is canceled if available disk space falls below 2 GiB or the cache budget is exceeded. Queue, storage and encoding failures are shown in the detail page, with retry available.

## Device downloads

**Download MP4 to Files** is the simplest durable offline option. Optional **Keep on this device** saves an account-scoped copy in IndexedDB with incremental 1 MiB writes, quota checks and a 2 GiB limit. Interrupted copies are removed and never shown as playable. Storage persistence is requested, but browsers can deny it or evict saved data. Browser playback builds a Blob from stored chunks and can need considerable memory for large titles. Saved copies are usable from an already loaded page without a connection; cold offline website startup is not implemented.

The native iOS app uses Apple's managed HLS download session, including background transfers and pause/resume. See [native HLS behavior](../IOS_Client_Xcode/TM_Sonder_Client/HLS_OFFLINE.md). Deploying the website does not install the updated app on phones.
