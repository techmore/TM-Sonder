# Refactor Checklist

Track the maintainability refactor in small, reviewable steps.

- [x] Add this checklist to the Xcode workspace.
- [x] Split standalone models out of `ContentView.swift`.
- [x] Split persistence/storage code out of `ContentView.swift`.
- [x] Split parsing code out of `ContentView.swift`.
- [x] Split scan/import helper types out of `ContentView.swift`.
- [x] Split HTTP helper types out of `xcode_TM_SonderApp.swift`.
- [x] Extract embedded web UI out of `SonderHTTPServer`.
- [x] Wire LAN pairing-token authorization through the HTTP routing path.
- [x] Add focused unit coverage for parser, store sanitization, byte ranges, and server settings.
- [x] Build the project after refactor.

Deferred larger follow-ups:

- [x] Break `SonderLibrary` into smaller services.
- [x] Extract pure derived-data building from `SonderLibrary`.
- [x] Extract scan/import orchestration from `SonderLibrary`.
- [x] Extract playback/conversion commands from `SonderLibrary`.
- [x] Move `NSOpenPanel` and `NSWorkspace` interactions behind UI/app services.
- [x] Replace repeated library mutation/save/cache patterns with a single mutation helper.
- [x] Move library snapshot and HTTP cache support types out of `ContentView.swift`.

Clean-code roadmap:

- [x] Split SwiftUI feature views out of `ContentView.swift`.
- [x] Move shared SwiftUI components and wrapping layout out of `ContentView.swift`.
- [x] Move app root shell and sidebar out of `ContentView.swift`.
- [x] Move library and continue-watching views out of `ContentView.swift`.
- [x] Move movie/audiobook/book browser views out of `ContentView.swift`.
- [x] Move TV Shows views out of `ContentView.swift`.
- [x] Move Collections view out of `ContentView.swift`.
- [x] Move media detail/card views out of `ContentView.swift`.
- [x] Move About view out of `ContentView.swift`.
- [x] Move Server dashboard out of `ContentView.swift`.
- [x] Move `SonderLibrary` into its own file.
- [x] Extract library folder detection and bounded concurrency helpers out of `SonderLibrary`.
- [x] Extract progress record clamping/upsert logic out of `SonderLibrary`.
- [x] Extract media-library root directory detection out of `SonderLibrary`.
- [x] Extract managed import item construction out of `SonderLibrary`.
- [x] Extract conversion candidate and job planning out of `SonderLibrary`.
- [x] Extract search, audiobook, and priority asset query helpers out of `SonderLibrary`.
- [x] Extract scan progress snapshot builders out of `SonderLibrary`.
- [x] Extract scan/import orchestration from `SonderLibrary` into a coordinator/service.
- [x] Extract playback/progress/conversion commands from `SonderLibrary`.
- [x] Move `SonderHTTPServer` out of `xcode_TM_SonderApp.swift`.
- [x] Move HTTP response DTOs out of `xcode_TM_SonderApp.swift`.
- [x] Move metadata enrichment into its own file.
- [x] Move audiobook import into its own file.
- [x] Rename remaining Plex import context to `SonderPlexImport.swift`.
- [x] Add route/service tests for audiobook search, progress updates, malformed HTTP IDs, and scan deduplication.

Audiobook feature follow-ups:

- [x] Add a dedicated audiobook browser route at `/audiobooks`.
- [x] Add searchable audiobook API support with `/api/audiobooks?q=...`.
- [x] Add a pure chapter-aware playback snapshot model.
- [x] Surface current chapter, chapter jumping, resume, and progress sync in the web browser.
- [x] Add richer audiobook metadata matching against an external Audnexus-compatible provider.
