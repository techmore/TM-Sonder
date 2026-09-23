# Library consistency checks

`GET /api/library/health` is a read-only endpoint using the server's existing
authentication policy. The web library requests it on load and after a catalog
refresh. Expand **Library consistency** to inspect warnings.

The audit compares TV show identities against the first directory under each
configured library root. It reports split show folders, folder/name mismatches,
and excess identities. Case, punctuation, and parenthesized years are ignored
for folder/name comparisons. Multiple quality files with one show identity do
not produce split warnings. Unmapped items are counted explicitly.

These are review signals, not proof of corruption: aliases, collections, and
libraries rooted at a single show may require a different folder convention.
The report counts folders represented by catalog media; it does not enumerate
empty folders or prove the NAS is mounted. It never moves files, merges shows,
downloads artwork, or rewrites metadata. No absolute media paths are exposed.

Current scope is server/web. iOS and menu-bar presentation of these warnings,
physical disk inventory, and persistent acknowledged exceptions are follow-ups.

## Stable browsing identity

TV catalog items now carry optional `showGroupID` and `showGroupTitle` fields.
For configured TV libraries with Show/Season/File layout, these identify the
top-level source folder, independently of filename-derived `showTitle`.
The ID is an opaque hash of library ID and folder name; remakes in different
folders stay separate. Flat/unmapped files fall back to parsed identity.
No files, playback IDs, progress, or original show metadata are merged/deleted.
The web and updated iOS client use this identity for show cards.

`browsingGroupCount` describes the intended number of show cards; `showCount`
continues to count original parsed names. Mixed-content warnings deliberately
remain visible after display grouping is repaired. Episode copies with distinct
parsed show identities are not combined solely because they share a folder.
