# TM Sonder Theme Notes

TM Sonder now uses a selectable earthy palette intended to feel warm, archival, and mac-native.

## Current Theme

`Earthy Tones`

This palette is designed around gentle sage as the base tone, with the other colors acting as supporting permutations:

- `#B0C4B1` Ash Grey, the base tone
- `#F7E1D7` Powder Petal, a light supporting surface
- `#DEDBD2` Dust Grey, a quiet neutral layer
- `#EDAFB8` Cherry Blossom, a limited accent
- `#4A5759` Iron Grey, the contrast anchor

## How It Is Used

- `background`: sage-tinted canvas wash that establishes the base mood
- `sidebar`: slightly stronger sage for the navigation rail
- `surface`: warm cream for cards and panels
- `surfaceDeep`: taupe for depth, overlays, and quieter structural areas
- `border`: sage-leaning line color for separators and outlines
- `accent`: charcoal for the strongest UI actions and labels
- `accentStrong`: same as accent for maximum contrast
- `accentMuted`: neutral support color for secondary surfaces
- `text`: readable charcoal for primary content
- `textLight`: softer charcoal for secondary copy
- `darkText`: light text on darker chips or labels

## Design Intent

- Ash Grey should set the overall mood and visual base
- Powder Petal and Dust Grey should support surfaces, not dominate them
- Cherry Blossom should remain a limited accent, not the primary brand color
- Iron Grey should anchor text, contrast, and the strongest interactive moments

## Migration Guidance

- Use the earthy palette for all primary surfaces before introducing any new accent colors
- Keep the base sage tone visible in the canvas, sidebar, and navigation treatment
- Reserve Cherry Blossom for action emphasis, not large areas
- Keep cards and panels soft, not high-contrast
- Prefer the gray-green tones for borders and neutral controls
- If another theme is added later, preserve the same token names so the UI can swap palettes without rewriting views
