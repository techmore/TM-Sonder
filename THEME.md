# TM Sonder Theme Notes

TM Sonder uses selectable palettes intended to feel warm, archival, and mac-native.

## Palettes

| Preset | Name | Mood |
| --- | --- | --- |
| `earthy` | Earthy (default) | Warm archival sage on a dark canvas |
| `dark` | Dark | Plain system dark, no decorative treatment |
| `techmore` | Techmore Olive | Light parchment-and-olive editorial, serif headings |
| `bunny` | Space Bunny | Deep-space indigo with aurora mint and nebula violet |

The palette name shown in Settings is the preset key. Presets are registered in
three places, and all three must agree or the first paint disagrees with the
page after it hydrates:

- `server/internal/httpapi/web/library.css` — the token block
- `server/internal/httpapi/web/library.html` — the Settings option
- `server/internal/httpapi/handlers.go` (`themeFor`) and
  `server/internal/httpapi/webui.go` (`libraryPageForThemeAndLayout`)

## Token Architecture

A palette is a block of CSS custom properties on `body[data-theme="…"]`. Every
surface, control, and card reads those properties, so adding a palette means
adding a token block, not rewriting views.

| Token | Role |
| --- | --- |
| `--bg` | Page canvas |
| `--panel` | Cards, panels, dialogs, the sticky detail rail |
| `--panel2` | Secondary surfaces: chips, secondary buttons, inactive tabs |
| `--line` | Borders and separators |
| `--text` | Primary content |
| `--text-soft` | Body copy: long-form summaries, one step down from `--text` |
| `--muted` | Secondary labels, metadata, counts |
| `--accent` | Strongest interactive moment: active tab, focus ring, primary action |
| `--accent-dark` | Text/icon color that sits *on* `--accent` |
| `--accent-2` | Optional second accent, for decorative gradients only |
| `--gold` | Progress bars and the "N% LEFT" resume badge, nothing else |
| `--chrome` | Translucent sticky-header background |
| `--frame-from` / `--frame-to` | Poster-placeholder gradient stops |
| `--radius` | Corner radius |

Two rules the tokens exist to enforce:

- **`--gold` means progress.** A resume bar and a "42% LEFT" badge read the
  same in every palette, so "how far did I get" never depends on the theme.
- **`--accent-2` is decoration.** It appears in gradients and glows, never as
  the sole carrier of meaning.

### `color-scheme`

Declared per palette, not globally. A palette that is light must set
`color-scheme: light` so form controls and scrollbars follow.

## Browser Layout Is Not a Palette

`data-library-layout` (`rails` or `classic`) selects structure only. The Rails
block deliberately declares **no** color tokens.

This was a real bug. The Rails block used to redeclare the whole palette, and
because `body[data-library-layout="rails"]` and `body[data-theme="…"]` have equal
specificity and the layout block came later, the layout silently won — so
selecting Techmore changed nothing on the main library page. The token blocks
now live at body level and every palette works in both layouts.

## Adding a Palette

1. Copy an existing `body[data-theme="…"]` block in `library.css` and change the
   token values. Keep the structure; drop decorative rules you do not need.
2. Add the option to `#themeSel` in `library.html`.
3. Add a `case` to `themeFor` in `handlers.go` (8-digit `RRGGBBAA` hex for the
   clients) and allow the key in `libraryPageForThemeAndLayout` in `webui.go`.
4. Add a test asserting the layout block declares no palette tokens, so the
   override bug cannot come back.

## Space Bunny

A near-black indigo canvas with two soft radial glows, aurora mint for actions,
and nebula violet for decoration. Starlight gold stays reserved for progress.

- Deep space reads as calm rather than clinical, and the mint accent is
  distinguishable from both the Earthy green and the Techmore olive.
- Gradients carry `--accent` into `--accent-2` on the active tab, the primary
  action, selected chips, and the play button, so the two accents always meet
  somewhere deliberate.
- Card hover lifts the poster and adds a violet ring, which keeps the shelves
  feeling like a physical stack on a dark surface.

## Known Gap

`audiobooks.html`, `audiobooks-beta.html`, and `ebooks.html` each carry their own
inline stylesheet hardcoded to the Earthy dark palette. They ignore the Settings
palette entirely. Unifying them onto `library.css` tokens is the remaining
theming work.
