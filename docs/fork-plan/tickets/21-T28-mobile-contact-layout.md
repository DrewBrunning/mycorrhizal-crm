# T28 — Mobile contact view layout fixes

| | |
|---|---|
| **Rating** | 5 — the contact detail page is the most-used surface and is broken on mobile |
| **Size** | M |
| **Depends on** | — |
| **Alpha** | **before** — a broken primary view should not ship to alpha |
| **Source** | T23 UI polish pass |

## Why this exists

The contact detail page (`ContactDetailPage.tsx`) was designed against desktop-first breakpoints
and does not handle small viewports correctly. Specific problems observed:

1. **Elements get cut off** at narrower widths — content overflows its container rather than
   reflowing.
2. **The tab bar (Information / Relationships / Life Events / Preferences) does not scroll
   horizontally** — tabs that don't fit simply disappear off-screen with no way to reach them.
3. **Layout breakpoints are not aggressive enough** — the horizontal→vertical reflow happens
   too late (or not at all) for phone-sized viewports.
4. **Actions at the top of the contact view** (export, merge, delete, edit) take up too much
   horizontal space at small widths.
5. **Minimum viewport is larger than actual phone widths** — the app assumes more horizontal
   space than real mobile devices provide (360–390px CSS width on common phones).

## What to build

1. **Tab bar horizontal scroll**: add `variant="scrollable"` with `scrollButtons="auto"` to the
   MUI `<Tabs>` component in `ContactDetailPage.tsx` so tabs that overflow the viewport become
   horizontally scrollable with arrow indicators.

2. **Aggressive layout reflow**: audit the contact header + detail layout for breakpoints.
   The current `md` breakpoint (900px) for horizontal→vertical transitions should probably
   be `sm` (600px) for the contact layout specifically, or use container queries. The
   information fields that currently sit in a multi-column grid should stack to single-column
   at smaller widths.

3. **Action collapsing**: at the smallest viewport sizes, collapse the action buttons
   (export, merge, delete, edit) into a single `<Menu>` behind an "Actions" overflow button
   (MUI `<IconButton>` + `<Menu>` or a `<SpeedDial>`). Show the 1–2 most critical actions
   as standalone buttons if space permits.

4. **Minimum viewport respect**: test against 360px width (iPhone SE / common Android) and
   ensure no element overflows the container horizontally. Use `overflow-wrap: anywhere` on
   long single-word values (emails, URLs).

5. **Alternative tab navigation**: for the smallest viewports, consider replacing the
   horizontal tabs with a vertical `<List>` or a dropdown `<Select>` that allows access
   to all sections without scrolling.

## Done when

- At 360px viewport width, all contact detail content is visible without horizontal
  scrolling of the page itself.
- The tab bar scrolls horizontally or shows an alternative navigation when tabs overflow.
- Action buttons collapse into an overflow menu at ≤400px.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Visually verified on a real phone or responsive-design mode at 360px, 390px, and 414px widths.
