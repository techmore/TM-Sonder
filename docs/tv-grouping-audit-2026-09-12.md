# TV grouping audit

## Root causes

- Parsed filenames doubled as top-level show identity: punctuation, case,
  differing years, release tags, extras, and episode-title parsing created
  separate cards inside one source folder.
- The All page previously rendered TV episodes directly.
- Unknown-number TV copy keys omitted show identity and could combine unrelated
  same-title extras across shows.
- Stable JavaScript URLs were cached for five minutes, keeping old grouping
  behavior visible even after a server restart and ordinary reload.
- Source folders genuinely contain mixed content. Samurai Jack includes
  Samurai Champloo; this is not safely solved by deleting files or pretending
  every same-number episode is an alternate encode.

## Corrections

The server emits additive folder-based browsing IDs and titles, preserving
original parsed identities and all media IDs. Web and updated iOS grouping use
those IDs. The web retains separate episode sets for different parsed shows in
one folder, while simple case/punctuation/year variants share episode copies.
Unknown-number copy keys now include their show boundary. Remake folders stay
separate. Stable web assets revalidate with ETags on reload; this release changes
the script URL once to bypass already-cached old policy.

The health report distinguishes browsing groups from parsed names and continues
to flag mixed-content metadata. It must not claim all metadata is repaired just
because top-level navigation is correct.

Live API validation after the grouping deployment: 20,881 total items,
13,789 TV files, 146 browsing groups, zero unmapped TV files. No source media
was moved or deleted. Full Go race tests, Node grouping tests, vet, Swift shared
package tests, and the iOS Release simulator build passed. iOS source changes
require installation of a new client build to appear on a physical phone.

Browser acceptance: TV page rendered exactly 146 show cards; Samurai search
returned one card; opening it showed four season cards; selecting Season 01
showed episode rows and an All Seasons back control. Initial browser validation
still displayed 310 cards despite the updated API, exposing the stale-script
cache defect; the cache fix resolved that discrepancy.

Additional web-only copy grouping recognizes packed episode filename `-1`
(or another numeric suffix) when an unsuffixed sibling exists in the same
show/season. This preserves both files in the version picker; unmatched names
and distinct titles remain separate. This is a grouping heuristic, not a claim
of byte-identical content. iOS still lists individual files inside seasons;
its new folder grouping fixes top-level show duplication only.
