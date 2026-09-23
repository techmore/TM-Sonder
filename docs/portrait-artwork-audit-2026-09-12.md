# Portrait artwork audit

## Findings

Image-header audit of the saved catalog found 151 distinct local TV images,
including 83 non-portrait images and one other flagged image. Of 1,331 local
movie images, 70 were flagged for aspect ratio, dimensions, or decoding.
Generated thumbnails were counted separately, not as valid local posters.
These are unique image-file counts, not show/title counts.

The web used `object-fit: cover`, cropping wide title cards into unreadable
fragments. It now uses `contain`; the iOS poster view likewise fits artwork
inside its fixed frame. Aspect ratio alone is not evidence of a wrong match.

## Reviewed replacements

Nine Wiki-hosted season-one covers were visually reviewed and activated for
personal-library display: American Gods, American Horror Story, Lost, Mom,
My Name Is Earl, Arrested Development, Better Call Saul, Boardwalk Empire,
and Breaking Bad. These are season covers used for the series card, not claims
of complete-series editions. Sources label these images Fair use; they are not
public-domain assets. The user explicitly requested their inclusion for a
personal library. No blanket redistribution rights are claimed.

Images and source/license/credit records live under
`~/.config/sonder/data/curated-posters/manifest.json`. The original NAS images
and catalog artwork paths remain unchanged. Remove an entry and restart Sonder
to restore its previous cover. Timestamped manifest backups are retained.

## Repeatable tools

- `go run ./cmd/sonder-poster-audit -snapshot PATH -kind tvShow` (from server/)
  emits a JSON dimension audit; use `-kind movie` for movies.
- `node tools/wiki-portrait-audit.mjs AUDIT.json REVIEW_DIR tvShow` stages
  conservative Wiki candidates with provenance. It requires explicit title
  associations and portrait dimensions, uses sequential paced requests, stops
  on HTTP 429, and resumes without re-fetching completed entries.
- `node tools/activate-reviewed-posters.mjs REVIEW_DIR DATA_DIR 'Show (year)'`
  activates only explicitly named, visually reviewed TV candidates. Restart the
  server to invalidate its payload cache. It never changes source media.

The first Wiki runs encountered throttling; no further requests were attempted
after the updated importer observed HTTP 429. The default pace was increased
to six seconds per metadata request for future runs. The audit is incomplete:
many titles need manual matching or another appropriate licensed source;
no movie replacements were approved in this pass.

Validation: Go race tests, Node tests, vet, server build and iOS simulator Release
build passed. Regression tests cover override rollback and path traversal.
