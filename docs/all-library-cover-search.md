# All-library cover search

The resumable Wiki search now accepts a mixed queue of movies, documentaries,
TV groups, ebooks and audiobooks. It alternates kinds, includes missing-cover
items (the old image-only audit could not see them), and records item IDs,
expected year and author for review. Audiobook candidates may be square.

Current queue: `~/.config/sonder/data/all-library-cover-queue.json`.
Candidate images and source/license records:
`~/.config/sonder/data/wiki-review-all/report.json`.

Initial live-catalog queue contains 1,305 targets: 345 movies, 78 TV groups,
46 audiobooks, and 836 ebooks. Counts are deduplicated search targets, not
missing media files. Movie/TV geometry flags were included. Additional book
geometry audits are separate; square audiobook covers must not be treated as
defective merely for not being portrait.

Build/extend the queue:

```
node tools/cover-search-queue.mjs OUTPUT.json AUDIT.json ...
node tools/wiki-portrait-audit.mjs OUTPUT.json REVIEW_DIR all
```

Search results are candidates only: film years, authors, audiobook narrators,
editions, and title collisions still require verification. Missing or throttled
results are not proof that artwork does not exist. The importer stops on HTTP
429 and can resume from its report. It does not activate results, modify the NAS,
or restart the server because searching alone changes no running application
assets. Existing reviewed overrides remain intact.
