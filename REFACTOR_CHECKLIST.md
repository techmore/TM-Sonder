# Refactor Checklist

Track the maintainability refactor in small, reviewable steps.

- [x] Add this checklist to the Xcode workspace.
- [ ] Split standalone models out of `ContentView.swift`.
- [ ] Split persistence/storage code out of `ContentView.swift`.
- [ ] Split parsing code out of `ContentView.swift`.
- [ ] Split scan/import helper types out of `ContentView.swift`.
- [ ] Split HTTP helper types out of `xcode_TM_SonderApp.swift`.
- [ ] Extract embedded web UI out of `SonderHTTPServer`.
- [ ] Wire LAN pairing-token authorization through the HTTP routing path.
- [ ] Add focused unit coverage for parser, store sanitization, byte ranges, and server settings.
- [ ] Build the project after refactor.

Deferred larger follow-ups:

- [ ] Break `SonderLibrary` into smaller services.
- [ ] Move `NSOpenPanel` and `NSWorkspace` interactions behind UI/app services.
- [ ] Replace repeated library mutation/save/cache patterns with a single mutation helper.
