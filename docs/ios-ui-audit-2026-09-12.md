# iOS UI/UX standardization

Direction: neutral surfaces, one restrained accent, content first, and secondary
details disclosed on demand. The existing six-destination bottom navigation is
retained as a product requirement.

Implemented corrections:

- Replaced fixed beige/translucent surfaces and secondary text with semantic
  iOS colors that adapt to appearance. Standard panel radius is 16 points and
  input radius is 12; removed repeated panel outlines.
- Reduced oversized section/detail headings to title2. Playback/read actions
  precede descriptions; descriptions and technical details are expandable.
- Removed video-style time controls from ebook details. PDF/EPUB readers own
  their reading positions.
- Poster titles reserve two scalable lines instead of a 34-point text box;
  cover frames remain fixed. Ebook/audiobook artwork fits inside the bounds.
- Bottom labels use caption2 instead of shrinking 9-point text. Selection has
  both weight and a capsule indicator. Accessibility text sizes use a scrollable
  bottom row rather than clipping six enlarged destinations.
- Playback transport and track controls have 44-point touch targets.
- Paused downloads no longer show a busy spinner; active transfers include an
  approximate remaining time. Removing a download from Details asks confirmation.
- Collapsed secondary Settings information/diagnostics. Kept Library Audio/Books
  selection when visiting another bottom destination.

Reference: Apple Human Interface Guidelines, Accessibility and Typography:
https://developer.apple.com/design/human-interface-guidelines/accessibility
https://developer.apple.com/design/human-interface-guidelines/typography

Remaining review: screen-by-screen VoiceOver ordering, landscape and iPad layouts,
contrast measurements on physical devices, and EPUB content-specific typography.
The custom six-item bar remains denser than a five-item native bar; accessible
sizes deliberately trade one-screen visibility for readable labels.

This pass changes presentation and interaction affordances. It does not certify
background downloads, playback, or release distribution. See the release report.

Validation: the final Release simulator build passed. The offline core suite
passed three tests; two environment-dependent live acceptance tests were skipped
in this run. The nested iOS working-tree whitespace check passed. Simulator
installation stalled, so rendered-screen acceptance is not complete. The build
reported only the App Intents metadata extraction warning (no framework dependency).
