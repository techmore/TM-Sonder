# Automatic subtitle maintenance

Sonder checks every movie, documentary and TV episode from the saved catalog
immediately on startup, again after the startup scan discovers new titles,
then hourly. English (`en`) is the default target. Existing language-tagged
sidecars are attached immediately. Embedded tracks in the requested language
are exported to WebVTT in permanent Sonder storage where possible, making them
available to the browser without burning text into the video. Bitmap subtitles
remain available through the existing burn-in playback path.

Missing text subtitles are searched on the OpenSubtitles REST API when an API
key is configured. File hash matches are preferred. Movie title fallback
requires the exact title and year; episodes require a hash match to avoid
confusing series/remakes. Forced-only, machine-translated, AI-translated and
multipart subtitle results are excluded. Title/year matches may still need
playback timing review because different releases can have different cuts.

Downloaded and extracted subtitles are saved under
`<dataDir>/subtitles/<item ID>/`, using the media filename and a fingerprint of
the exact source revision. They survive restarts and rescans; replaced video
files do not silently reuse captions from a different cut. Existing media
sidecars are never overwritten. The catalog is updated immediately so the
player offers maintained subtitles without a rescan. Production media mounts
are read-only; this storage is on SER8's permanent Sonder data volume and does
not require granting the application write access to the NAS library.

The worker runs sequentially, with a three-minute extraction timeout per track,
bounded HTTP downloads, and a pause between remote searches. Not-found results
retry after seven days; errors, credentials and quota failures retry after 24
hours. Authentication/quota errors stop further provider requests in that pass
while local subtitle extraction continues. New titles join the next hourly
pass. Restarting preserves retry history in `subtitle-maintenance.json` in the
server data directory. Plain sidecars with no language tag remain playable but
are not assumed to be in the requested language.

## Provider configuration

Use your own OpenSubtitles API key. Do not commit credentials to Git.
Set these in the server's systemd environment file:

```
SONDER_OPENSUBTITLES_API_KEY=<your key>
SONDER_OPENSUBTITLES_USERNAME=<optional account username>
SONDER_OPENSUBTITLES_PASSWORD=<optional account password>
SONDER_SUBTITLE_LANGUAGES=en
```

Additional languages use comma-separated two-letter codes, e.g. `en,fr`.
Restart Sonder after changing the environment. Account/provider download quotas
still apply. No external transcription or paid processing is performed.

On production, use `/etc/sonder/subtitles.env` and a systemd service drop-in:

```
[Service]
EnvironmentFile=-/etc/sonder/subtitles.env
```

Protect that environment file with mode 0600. Run `systemctl daemon-reload` and
`systemctl restart sonder` inside the container after setting credentials.

## Owner API

- `GET /api/settings/subtitles`: running state, configured provider, languages,
  counts, and per-title/language status with last check, retry time and error.
- `POST /api/settings/subtitles`: schedule a coverage pass. Existing retry
  cooldowns still apply; configuring a missing API key clears its blocked state
  on the next restart/pass.

Statuses are `pending`, `checking`, `available`, `not_found`, `blocked`, or `error`.
`available` means a text sidecar exists; a blocked entry can still have an
embedded bitmap track usable through burn-in, as explained in its detail.
