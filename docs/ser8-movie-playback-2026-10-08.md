# iPhone Safari playback repair — 2026-10-08

The user reported that The Ninth Gate's Play button changed to Pause while
video never started and other phone audio kept playing. The recent production
request started a fragmented MP4 encoder, despite the current iPhone client
having an HLS delivery path. The exact state of that phone's loaded page was
not available.

Apple user agents now receive HLS for legacy `?transcode=1` requests as well.
An explicit delivery parameter still selects its requested format. Legacy
Safari probes reuse the same encoder, scoped by hashed authentication,
browser, movie, offset, and selected tracks. Seeking replaces that encoder.
The web client also recognizes iOS when MIME capability probes return empty.

Startup and buffering have visible status messages. The media session remains
paused during buffering; the player labels its pending action as "Pause loading
video". Both prepared HLS and live HLS allow 25 seconds for initial buffering.

Before deployment, production supplied a complete prepared copy of The Ninth
Gate. Browser playback from that copy reached readyState 4, decoded 1280×544
video, and advanced from the saved position without a media error. This verifies
the prepared copy and browser path; confirmation on the user's physical iPhone
remains a separate check.

Regression checks cover an iPhone request without delivery hints, duplicate
Safari probes with one encoder slot, seek replacement, account isolation, an
empty MIME capability response, and the startup/buffering state transitions.
