# Browser EPUB reader progress

## Request

Make EPUB books readable in Sonder's browser. The existing ebook page only linked
to `/stream/{id}`, which sent the EPUB ZIP to the browser and did not provide a
reading view.

## Implementation

- `/read/{id}` serves an authenticated reader page for cataloged EPUB items.
- The ebook browser links to **Read in browser** and keeps a separate download
  link.
- The reader fetches the EPUB from Sonder, then renders it with self-hosted
  epub.js 0.3.93 and JSZip 3.10.1. Their licenses are in
  `server/internal/httpapi/web/vendor/`.
- Readers can move page by page or use the EPUB table of contents. The current
  EPUB CFI is saved in browser local storage per book and restored on the next
  visit from that browser.
- Reader sources are embedded in the Go binary with the existing web assets.

## Files changed

- `server/internal/httpapi/server.go`
- `server/internal/httpapi/handlers.go`
- `server/internal/httpapi/webui.go`
- `server/internal/httpapi/web/ebooks.html`
- `server/internal/httpapi/web/ebook-reader.html`
- `server/internal/httpapi/web/vendor/`

## Current release status

Implementation and deployment verification are tracked in the Codex task. Update
this section with the release commit and production smoke-check result before
closing the work.
