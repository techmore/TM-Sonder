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
  EPUB section and page are saved in browser local storage per book and restored
  on the next visit from that browser. Older saved CFI positions are migrated
  to the containing section on the next open.
- Reader sources are embedded in the Go binary with the existing web assets.

## Files changed

- `server/internal/httpapi/server.go`
- `server/internal/httpapi/handlers.go`
- `server/internal/httpapi/webui.go`
- `server/internal/httpapi/web/ebooks.html`
- `server/internal/httpapi/web/ebook-reader.html`
- `server/internal/httpapi/web/vendor/`

## Current release status

The reader was deployed to the browser-facing `tm-sonder` service on SER8. A
smoke check opened “The Republic of Plato (Allan Bloom)” in the browser, rendered
the EPUB, turned to the next section, reloaded the reader, and confirmed it
returned to the saved section. The service health endpoint reported version
`0.2.23+browser-epub`.

The first catalog entry, “StarCraft - It Will End in Fire,” could not be used in
the same check because its NAS file is unreadable by the service account. The
Republic EPUB was readable and verified end to end.

Reader changes are on `main` in commits `f84ecb2` through `9e7d476`. The Deploy
workflow for `9e7d476` passed and confirmed the Incus service is serving; the
broader CI workflow is still running. The host service was built from the
current working tree so it keeps the separate account audit changes already in
progress; its preceding binaries were retained as dated backups on SER8.
