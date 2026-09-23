# Media ops — organize / collect / expand / convert

Date: 2026-09-20. Library: `/Users/seandolbec/NAS/plex` (NFS `192.168.222.188:/volume2/14tb`).
Staging (small local disk!): `~/Downloads/Torrents`. Client: Harbor 1.7.8
(`co.hapy.harbor`, aria2 backend). Mover: `bin/harbor-mover.py` (live loop
`bin/harbor-mover.sh`, 300s, log `/tmp/harbor-mover.log`).

## 1. Organize (Plex layout)

- Movies: `movies/Title (Year)/Title (Year).ext` (+ poster.jpg, .nfo sidecars kept).
- TV: `tv_shows/Show (Year)/Season 01/Episode files`.
- Audiobooks: `Audiobooks/Author/Book/file.m4b` (+ cover.jpg). New: Andy Weir x7,
  Jefferson Fisher, Larry Niven/Ringworld, Andrzej Sapkowski x8 (Witcher),
  Ringworld-Book-1-MP3-ingest, Turner Diaries 68-track (partial salvages —
  re-run when torrents complete: 4.39h/10.19h, 1.09h/6.2h, chapters drift).
- Music: `music/Artist/Album (Year)/NN - Title.flac`. Done: Third Eye Blind /
  Blue (1999), 13 tracks, regenerated Blue.m3u.
- Comics: `Comics/Series/` (Green Lantern pending).
- Loose-file fix: Baywatch was a loose mp4 in movies/ (dup of Documents copy;
  Documents copy deleted). Man from Earth kept under YIFY name (mover skips
  dupe instead of doubling).
- Typo guard: mover once made `Se7en ( (1995)` — fixed to `Se7en (1995)`.

## 2. Collect (Harbor)

- Harbor handles `magnet:` URLs; `harbor://download?url=` is HTTP(S) only.
  `open magnet:` from agent context silently fails — inject into
  `~/Library/Application Support/Harbor/downloads.json` (backup first) with
  Harbor stopped, then `pkill aria2-next`, `open -a Harbor`.
- Entry fields that matter: `sourceURL` (magnet + trackers), `metadataName`,
  `torrentFingerprint` (lowercase hash), `destinationFolderPath`
  (`/Users/seandolbec/Downloads/Torrents`), `status: queued`.
- `max-concurrent-downloads` is 10; new magnets land queued/pause.
- Queue hygiene (2026-09-20): removed 2 Vanity Fair dupes (BBC edition already
  in Plex) + 2 IntelliQuest curly-apostrophe dupes; kept straight-apostrophe
  94% (stalled — dead torrent, leave to resume if seeds appear).
- Counts 2026-09-20: 72 entries (taste 49 + 6 priority gaps + 33 batch-3 +
  strays). Mover drains completed to Plex.

## 3. Expand (recommendation lists)

- Source: `plex/1.recommend/` (movies-recommended Wave 1 = 112, Wave 2 = 294,
  tvshows 500, taste-based 49).
- Taste 49: all queued/fetched (Prospect, Another Earth, Sound of My Voice,
  Stalker in Plex; Prisoners, No Country, Se7en landed; anime/docs/action/
  prestige/horror batch-3 in flight). Skipped: Man from Earth (dupe),
  Rolling Red Go (no hash found).
- Priority gaps queued 2026-09-20: Godfather, Godfather II, Vertigo, Parasite,
  EEAAO, Seven Samurai (2001/Alien/Blade Runner already owned — list stale).
- Wave 1: 112 total, 87 missing, 79 fresh to queue (8 overlap Harbor).
  Queuing in ~15-title batches (local disk guard: 10 concurrent x ~2G).
- TV strays: Taxi S1-5 (114 eps, 9.5G) moved to `tv_shows/Taxi (1978)/`.
  Beast Machines / Outsourced local copies were sparse-corrupt stubs — deleted,
  need Harbor redownload. Escaflowne local deleted (Plex holds superior 12G).

## 4. Convert (density)

- Video → AV1 (ffmpeg libsvtav1 preset 8 crf 32, audio → Opus 128k):
  movies 84% non-AV1 (~1,300 candidates), TV 100% non-AV1.
  Pilots: DS9 68G→~30G, Westworld 65G→~29G (56% savings); movie top-10 ~13.4G
  back (Ben-Hur 6.87G h264 first). Career BONE file was a sparse partial —
  deleted, re-sourced via Harbor. Use ≤5 parallel ffprobes on NAS; earlier
  36/195 "permission denied" was NFS throttle, not file modes (0 fixed).
- Audiobooks → Opus/MP4 per `docs/audiobook-opus-migration.md` (NOT .opus):
  Opus inside MP4 mapping, iOS AVPlayer pilot passed on simulator; physical
  device + browser acceptance still pending. Library is 933 M4B / 350 GiB,
  mostly AAC ~80k mono (already compact). Staged pilots save 30-61% with
  chapters + art preserved (`m4b_work/migration/`). Do NOT bulk-rollout before
  acceptance; MP3 backlogs convert to compact M4B AAC 80k at ingest instead.
- Ebooks/posters: TM Sonder server :8096 enrich fills summaries + Plex
  `poster.jpg` for official art only; home-video `no match` gets frame-grab
  thumbnails. Rescan after every move batch.

## Disk guard

Local `/System/Volumes/Data` swings 13–84G; Torrents staging 6–72G. NAS has
3.6T free — space pressure is local only. Mover (single instance! double-start
caused a copy race on Weathering With You) clears completed every 300s.
If NAS unmounts (`/Users/seandolbec/NAS` shows local fallback), remount from a
user shell: `osascript -e 'mount volume "nfs://192.168.222.188/volume2/14tb"'`
(agent mount fails -5014). Pause Harbor while NAS is down — mover can't offload.
