# Movie deduplication audit — 2026-09-24

Source: `/api/library` from the running TM-Sonder server.

## Summary

- Movies audited: **1,924**
- Multi-file movie groups found: **31**
- Files represented by those groups: **62**
- Duplicate cards removed by the corrected matcher: **31**
- Corrected unique movie cards represented by the catalog: **1,893**
- Shared metadata IDs found among these groups: **0**

The groups below are likely alternate encodes, quality releases, punctuation/casing variants, release tags, or minor filename typos of the same movie. All files remain available in the movie detail panel's version picker.

## Likely duplicate groups

| Movie titles found | Year | Notes |
|---|---:|---|
| 007 Jame Bond - Die Another Day / 007 James Bond-Die Another Day | 2002 | Typo/punctuation variant |
| Alaska - Silence & Solitude / Alaska: Silence & Solitude | 2005 | Punctuation variant |
| Blade Runner / Blade Runner XviD | 2049 | Codec suffix; exact reported case |
| Contact / Contact 1080p Remux | 1997 | Quality/release suffix |
| Dracula Dead and Loving It / Dracula Dead And Loving It | 1995 | Case/punctuation variant |
| Dungeons and Dragons Honor Among Thieves / dungeons and dragons honor among thieves web | 2023 | Case/web release suffix |
| Escaflowne - The movie / Escaflowne: The Movie | 2000 | Punctuation/casing variant |
| Fat Sick Nearly Dead / fat sick and nearly dead xvid | 2010 | Title wording/codec suffix |
| Four Lions / Four Lions a | 2010 | Likely junk/alternate suffix; review |
| From Here To Eternity / From Here To Eternity [1080p] [YTS AG] | 1953 | Quality/release suffix |
| Get him the the Greek / Get Him to the Greek | 2010 | Typo/article variant |
| Ghandi / Ghandi [1080p] | 1982 | Quality suffix; spelling likely filename typo |
| Good Will Hunting / Good Will Hunting V2 | 1997 | Re-encode version |
| Harry Potter and the Half Blood Prince / Harry Potter and the Half-Blood Prince | 2009 | Punctuation variant |
| Her / Her (1080p BluRay x265 HEVC 10bit AAC 5 1 afm72) | 2013 | Quality/release suffix |
| Knocked Up / Knocked Up[][Unrated Edition] | 2007 | Edition suffix |
| Legend Of The Fist Return Of Chen Zhen / Legend of the Fist The Return of Chen Zhen | 2010 | Wording/casing variant |
| Looper / Looper [1080p] | 2012 | Quality suffix |
| Moon / Moon [1080p] | 2009 | Quality suffix |
| Pat Garrett & Billy The Kid / Pat Garrett & Billy The Kid [1080p] [WEBRip] [YTS LT] | 1973 | Quality/release suffix |
| Predators / Predators | 2010 | Two encodes |
| Predestination / Predestination [1080p] | 2014 | Quality suffix |
| Quiz Show / Quiz Show [1080p] [BluRay] [5 1] [YTS MX] | 1994 | Quality/release suffix |
| The Strangers / The Strangers (3) | 2026 | Likely copy/alternate filename marker |
| The Terminator / The Terminator GAZ | 1984 | Release suffix |
| The Twilight Samurai / The Twilight Samurai | 2002 | Two encodes |
| Tucker The Man And His Dream / Tucker: The Man and His Dream | 1988 | Punctuation variant |
| Umberto D / Umberto D [1080p] [BluRay] [YTS MX] | 1952 | Quality/release suffix |
| Underworld Rise of the Lycans / Underworld: Rise of the Lycans | 2009 | Punctuation variant |
| Watchmen / Watchmen[]DvDrip[Eng] | 2009 | Release/language suffix |
| the biggest little farm / the biggest little farm -lpd | 2018 | Release suffix |

## False merges found and fixed

The first audit pass found seven groups that the old fuzzy matcher incorrectly treated as versions:

- `Ghibli Batch Film 02` through `Ghibli Batch Film 23`: these are separate numbered movies, not versions. The old numeric-tail guard was reading the year suffix instead of the title suffix.
- `Predator` / `Predator 2`: sequel guard failed for the same reason.
- `Storyboard Comparisons - The Parade Scene` / `The Ruins Scene`: these are different bonus-feature videos, not alternate movie encodes.
- `Merlin Part I Gopo` / `Merlin Part II Gopo`: split parts.
- `Life Stinks` / `Life Stinks (2of2)`: split parts.
- `Indiana Jones and the Kingdom of the Crystal Skull` / `... -cd1`: disc part.
- `The Twelve Chairs` / `The Twelve Chairs (2of2)`: split parts.

The matcher now reads numeric/plural guards from the normalized title, detects common part/disc markers, and keeps comparison/bonus-feature videos separate.

## Selection behavior

For each retained group, the default representative is chosen by:

1. Existing playback progress, so a partially watched copy resumes correctly.
2. Highest known/probed resolution.
3. Runtime metadata when resolution is equal.
4. Browser-friendly format when otherwise equal.

No source files are deleted or hidden permanently; the alternate files remain selectable in the detail panel.
