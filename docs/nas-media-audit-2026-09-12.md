# NAS media audit — September 12, 2026

The mounted share is `/Users/seandolbec/NAS`, backed by
`192.168.222.188:/volume2/14tb`. No NAS files were modified.

The running Go server has four configured roots under `plex`: `movies`,
`tv_shows`, `Audiobooks`, and `ebook`. Its scan completed with 20,881 entries.
The persisted snapshot contains 20,881 file entries; every referenced media
file and every referenced poster file passed a filesystem existence check.
Existence does not establish media decodability or visual cover correctness.

| Snapshot media kind | Files | Poster references | No poster reference | Generated thumbnail posters |
| --- | ---: | ---: | ---: | ---: |
| Movie | 1,655 | 1,616 | 39 | 233 |
| TV episode | 13,789 | 13,750 | 39 | 587 |
| Ebook | 4,527 | 3,660 | 867 | 8 |
| Audiobook | 910 | 862 | 48 | 0 |

Counts by media kind differ slightly from configured-library counts because
classification is determined per file. There are 993 entries without a poster
reference and 828 using generated thumbnails. These still need a cover review;
no guessed online artwork was installed by this audit.

The Plex tree also includes Comics, Lectures, music, photos, Library, and other
folders outside the four configured roots. They are not covered by the catalog
completeness result. Ebook-Share was empty at inspection. Unsupported ebook
formats such as CHM and MOBI are present in the configured ebook tree, so the
catalog count is not a count of every filesystem document.

## Corrections

- Web version identity now includes release year and split part. TV identity
  uses show, season, and episode rather than repeated episode titles.
- Fuzzy grouping no longer bypasses its year guard by adding a shared zero.
- Group membership is rebuilt after all kinds are merged.
- TV season lists use one selected representative per episode version group.
- Automatic ranking preserves the version with progress, then prefers higher
  resolution. It does not measure available network bandwidth.
- Local artwork lookup checks case variations in the title folder before
  looking in the parent folder, addressing case-sensitive NFS cover priority.

Applied to the snapshot, the updated web grouping yields 1,598 movie groups,
10,408 TV episode groups, and 1,238 multiple-version groups across all kinds.
These are heuristic groups, not a verified count of unique creative works.

## Verification and rollout

Four Node regression tests cover quality ranking, resume preference, remakes,
split parts, and TV episode grouping. The Go artwork regression verifies that
title-local cover art wins over parent art. Go race tests and vet pass.
`make test` includes the Node tests.

Changes are source-only: the running service was not rebuilt/restarted or
deployed, and existing user changes were preserved. Web grouping changes do
not automatically change the native iOS grouping implementation. A deployed
build and client UI checks are still required before claiming the live
experience is corrected. Full visual artwork review and unsupported-format
handling remain follow-up work.
