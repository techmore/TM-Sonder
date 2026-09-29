# Curated Shelves and Discovery Rails

## What this is

Sonder has a 100-title cross-genre reading queue plus shorter genre and canon
lists. An audit found that several broad shelves—including all-time, American,
British, world literature, nonfiction, poetry, philosophy, and century lists—
had only five titles each. Those are now substantial reading paths, with
dedicated Greek/Roman, Western-tradition, and English-language selections.

The lists are curated entry points, not definitive rankings. Their counts and
missing-title checklists are calculated against the catalog currently open in
Sonder.

Nothing about a list was book-specific — the server has always accepted any item
ID in `POST /api/lists/{id}/items`. The engine now drives every catalog:

| Kinds | Lists | Source |
| --- | --- | --- |
| `ebook`, `audiobook` | 24 | Cross-genre, literary, historical, philosophical, and poetry selections |
| `movie` | 8 | Greatest films, American cinema, Criterion, sci-fi, noir, horror, animation, recent Best Picture winners |
| `tvShow` | 4 | Greatest television, prestige/period, comedy, speculative series |
| `documentary` | 4 | Greats, nature/science/space, history/war, music/performance |

Every movie, show, and documentary shelf is a **selection** rather than a
complete canon, and each one says so in its own description. The new book
descriptions use the same framing.

## How the book selections were reviewed

The Western-tradition path draws on works taught together in Great Books
curricula, including ancient Greek and Roman literature, medieval texts,
Shakespeare, political thought, and later novels. The [University of Dallas
Core reading list](https://udallas.edu/academics/core-curriculum/books.php) is
one concrete reference, not a claim that one college's syllabus defines the
canon. Its [Classics course descriptions](https://udallas.edu/academics/programs/classics/courses.php)
also ground the Greek and Roman shelf in epic, tragedy, history, and Latin
poetry.

The English-language shelf follows the broad period range represented by the
[Norton Anthology of English Literature](https://seagull.wwnorton.com/anthologyofenglishliterature/toc),
from the Middle Ages through the twenty-first century, and extends beyond a
single national tradition to Irish, American, African, Caribbean, and South
Asian writing in English. The [Norton Literature Library](https://seagull.wwnorton.com/NLL)
provides another cross-genre reference point. The [Great Books Foundation's
history](https://www.greatbooks.org/celebrating-75-years/) describes the Great
Books tradition and its later work with women's and immigrant writing; these
lists aim to keep that conversation open rather than freeze it into one roster.

For ancient Roman authors, the [Loeb Classical Library](https://www.hup.harvard.edu/series/loeb-classical-library)
is a reference for the surviving range of Latin writing. Sonder's short list
selects approachable, commonly titled works; it is not a substitute for a
classics syllabus.

## How an entry is written

```
"The Hobbit"                 title only
"The Thing (1982)"           title plus year
```

The year is what makes a film shelf trustworthy. *The Thing* is a 1982 film and
a 2011 prequel; *Dune* spans 1984 and 2021; *It* spans four decades. An entry
that names a year matches a catalog entry carrying the same year. An entry
without one stays title-only, which is all a genre shelf ever needed.

Catalog titles usually *also* carry a year suffix, because media managers name
files `Title (Year)` and the scanner keeps that as the title. Both sides are
stripped before comparison, so those are exact matches rather than fuzzy ones.

## Matching rules

An entry resolves to a catalog item when:

1. Normalized titles are equal (lowercase, alphanumerics only) **and** the years
   are compatible — the same year, or one side has no year recorded.
2. Failing that, one normalized title contains the other, **and both are at
   least 5 characters**.

Rule 2's length floor is load-bearing. Without it, `The Hobbit` matches a book
titled `It`, `The Sound and the Fury` matches `UR`, and `The Shadow of the Wind`
matches `Dow` — all because of short substrings. That is not hypothetical: it
inflated the reported book coverage by ten titles before the floor was added.

Candidate pools are **per kind**. A movie shelf never matches a documentary with
the same title, and the TV pool is built from show groups rather than episodes, so
a TV entry names a series and its rail renders show cards.

## Discovery rails

A shelf becomes a rail once the library already holds **4 or more** of its
titles. That threshold is the point: a shelf you already half own is the
interesting case, because the titles still missing are an obvious queue. A rail
subtitle states the gap directly:

> **Greatest Movies of All Time** — 46 of 59 owned · 13 still to find

Rails appear on every catalog tab, ranked by coverage, deduplicated by content,
and capped at two per tab. Deduplication matters: dozens of the book lists are
the same 100 titles under different publications' names, and three identically
worded "67 of 100 owned" shelves would be noise rather than choice.

A rail keeps the shelf's own ranking rather than the catalog's alphabetical
order — the top of a greatest-films shelf is not its first letter.

The rail's action button saves the shelf as a list, ordered as the list ranks
it, and reads "Saved · open" once it exists.

## The Lists panel

The panel is open on every browsing tab, not just books. The curated shelf
picker only offers lists that can match the tab's catalog, and coverage counts
are measured against that catalog alone. `Export missing .txt` uses each list's
own kinds, so a film's missing list is measured against movies.

## Example coverage snapshot

The counts below came from a 21,753-item catalog snapshot on September 25,
2026, via a headless harness over `library.js`. They illustrate how coverage
works; the live counts vary by catalog and the book shelves have since been
expanded.

| Catalog | Best shelves |
| --- | --- |
| Movies (1,924) | American Cinema Essentials 47/58 · Greatest Movies of All Time 46/59 · Crime, Noir and Thrillers 31/46 |
| TV (13,909 episodes) | Greatest Television 20/40 · Comedy Worth Rewatching 17/40 · Speculative and Genre Series 12/41 |
| Documentaries | no documentary library configured, so no rails |

## Files

- `server/internal/httpapi/web/library.js` — the registry (`CURATED_LISTS`),
  the candidate pools (`listCandidateIndex`), and the rails (`curatedShelves`)
- `server/internal/httpapi/web/library.test.cjs` — the `shelfHarness` suite
