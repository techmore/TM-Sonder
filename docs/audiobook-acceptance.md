# Audiobook Opus/MP4 acceptance checklist

Source policy: `docs/audiobook-opus-migration.md` §68–69. No bulk rollout or
promotion until this matrix passes. Simulator pilot (iOS 27, AVPlayer, time
advances) is done — everything below is still open.

## Staged cohort (8 books, `m4b_work/migration/staging/`)

- [ ] *The Count of Monte Cristo* (Dumas) — 57.2% smaller, 1 chapter
- [ ] *Baptism of Fire* (Sapkowski) — 30.5%, 8 chapters
- [ ] *Blood of Elves* (Sapkowski) — 31.5%, 7 chapters
- [ ] *Lady of the Lake* (Sapkowski) — 32.5%, 26 chapters
- [ ] *Season of Storms* (Sapkowski) — 32.3%, 31 chapters
- [ ] *Sword of Destiny* (Sapkowski) — 31.3%, 49 chapters
- [ ] *The Last Wish* (Sapkowski) — 31.8%, 13 chapters
- [ ] *The Tower of the Swallow* (Sapkowski) — 30.0%, 11 chapters

Each receipt already confirms: full output decode, duration match, chapters,
embedded art. (~2G saved across the 8.)

## Device matrix

- [ ] Physical iPhone, current iOS — install staged file via Sonder, play
- [ ] Physical iPhone, oldest supported iOS — same
- [ ] iOS simulator (done — re-verify after any encoder change)
- [ ] Safari (Opus-in-MP4 support check; web player shows message if absent)
- [ ] Chrome / Firefox — same check

## Per-device test cases (each book spot-check ≥1 long + ≥1 heavily-chaptered)

- [ ] Cold startup, time advances within 2s
- [ ] Seeks near beginning / middle / end
- [ ] Chapter jumps (first, middle, last; verify names/times)
- [ ] Speeds 1x, 1.5x, 2x, 3x (pitch + chapter events sane)
- [ ] Background + lock-screen controls (play/pause/seek, artwork shows)
- [ ] Interruption (call/notification) → resume at correct position
- [ ] Sleep timer ends playback cleanly
- [ ] Kill + relaunch → resume position preserved
- [ ] Offline download → airplane-mode playback
- [ ] Reconnect mid-book → no position loss, no re-download loop

## Promotion gate (only after all boxes above)

1. Manifest-driven promotion per migration doc §100: immutable archive,
   atomic catalog update, IDs/progress preserved, sample playback verified.
2. Originals move to rollback archive OUTSIDE scanned roots (separate
   retention policy required before any deletion — none implemented).
3. Re-run the 2 partial MP3 salvages (Ringworld 4.39h/10.19h, Turner
   1.09h/6.2h) once torrents complete; chapters currently drifted.
