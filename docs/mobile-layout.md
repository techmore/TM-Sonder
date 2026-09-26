# The mobile (phone) layout

The web UI is the phone UI. The iOS client is committed as a bare gitlink with
no sources, so the browser is the only touch surface Sonder actually has.

## What was wrong

Measured on an iPhone 14 Pro (393×852) with real layout numbers, not guesses:

| | Before | After |
| --- | --- | --- |
| Sideways page scroll | 704px in a 393px viewport | none |
| Header height | 216px (5 rows) | 169px (2 rows) |
| Facet filter row | 484px (14 chip rows) | 154px (2 rows) |
| Chrome before the first poster | ~700px of 852px | ~323px |
| Interactive targets under 32px tall | 10+ | 0 |

The tab bar was 567px wide inside a 393px viewport and was not a scroll
container, so **the entire page panned sideways** and Books, Storage, and
Optimize were physically off-screen. Genre chips wrapped one per row, so the
first poster appeared about 1400px down.

## The changes

**Docked bottom tab bar.** `nav.tabs` becomes `position: fixed` at the bottom,
where a thumb reaches it. It stays the same `<nav>` element, so the active
state, click handling, and the hidden-empty-tab logic are untouched.

> `backdrop-filter` on the header had to be dropped on phones. A filtered
> ancestor becomes the containing block for `position: fixed` descendants, which
> would have trapped the fixed tab bar inside the sticky header. The header gets
> an opaque `--panel` background instead.

**`.header-tools` wrapper.** The search field and the filter selects are wrapped
in a `div.header-tools` that is `display: contents` on desktop — so the desktop
header is byte-for-byte the same single flex row — and a single swipeable row on
a phone.

**Facets as one swipeable row.** The chip wall becomes a horizontal scroller,
with the same edge-fade affordance as the shelves.

**Shelves snap.** `scroll-snap-type: x proximity` plus a mask that fades the
trailing 34px, so a rail reads as "there is more this way" rather than a hard
cut, and cards land one at a time.

**Movie detail as a bottom sheet.** This was a real dead end: the detail panel
rendered *below* the entire page, so tapping a poster appeared to do nothing. It
is now a fixed sheet, max 84vh, opaque. Opaque matters — a palette's translucent
panel gradient is fine for a side rail but let shelf art bleed through the
synopsis. Collapsed now means "no sheet" instead of the 54px desktop sliver.

**44px touch targets** across buttons, selects, inputs, and chips.

**Stacked rows.** Episode rows, edition rows, and reading-list entries were
four-to-six inline columns and crushed at 393px; they reflow to two-column grids.
Storage and Optimize metric grids go to 2 columns.

**Safe-area insets** on the docked bar, the sheet, and the now-playing
controller, so nothing hides behind the home indicator.

## Verifying it

The layout is verified by driving real headless Chrome over the public URL with
iPhone device metrics, then reading back layout numbers — not by eyeballing
screenshots. `document.body.scrollWidth` versus
`document.documentElement.clientWidth` is the sideways-scroll test.

One trap worth knowing: cache-busting by appending `&cb=<ts>` to a URL that ends
in `#fragment` puts the parameter *inside the fragment*, the URL never changes,
and Chrome silently serves a cached page. That produced a whole round of
confidently wrong measurements before it was caught. Append to the query, or
add the hash back afterwards.

## Still not done

`audiobooks.html`, `audiobooks-beta.html`, and `ebooks.html` are standalone
pages with their own inline stylesheet hardcoded to the Earthy dark palette.
They ignore both the Settings palette and everything in this document. They are
linked from the header and are a separate mobile experience. Unifying them onto
`library.css` tokens is the remaining work, and it is the same job as the theme
gap in [`../THEME.md`](../THEME.md).
