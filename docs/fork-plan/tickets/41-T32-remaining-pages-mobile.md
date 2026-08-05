# T32 — Mobile layout: Network, Settings, User Management

| | |
|---|---|
| **Rating** | 3 — real friction, but these are lower-traffic pages than the contact detail view T28 fixed |
| **Size** | M |
| **Depends on** | — |
| **Alpha** | n/a — real data exists; this is display-only, no schema change |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

[T28](21-T28-mobile-contact-layout.md) fixed mobile layout on the contact detail page
specifically. Three other pages were found broken the same way during real-world testing and were
out of that ticket's scope:

- **Network** (the relationship graph view) doesn't scale down to mobile layout.
- **Settings** doesn't scale down to mobile layout.
- **User Management** (`UsersPage.tsx`) doesn't scale down to mobile layout.

Grouped as one ticket because they're the same class of fix repeated across three pages, not
three different problems — same discipline T28 used for the contact page.

## What to build

For each of the three pages, apply the same fixes T28 established as the pattern for this
codebase:

- **Network**: the graph visualization itself has real constraints a simple reflow won't solve
  (it's inherently a wide 2D layout) — at minimum, ensure the graph is contained in a
  horizontally-scrollable/pannable viewport rather than overflowing the page, and that any
  surrounding controls (filters, depth selector, legend) reflow to single-column below `sm`.
  Consider whether a reduced default zoom/scale on mobile viewport-open is warranted so the
  initial view isn't unusable.
- **Settings**: audit for multi-column layouts or fixed-width elements that don't reflow below
  `sm`/`600px`. Section navigation (if any) should collapse the way T28's action-button pattern
  did, rather than requiring horizontal scroll.
- **User Management**: `UsersPage.tsx`'s table is the likely culprit — a wide table with several
  columns (username, email, role, created date, actions) does not fit 360–390px. Either make the
  table horizontally scrollable within its own container (never the page), or switch to a stacked
  card-per-user layout below `sm`, following the "reflow to single column" precedent from T28.

## Traps

- Match T28's actual technique choices rather than reinventing — `variant="scrollable"` +
  `scrollButtons="auto"` for any tab-like strips, `overflow-wrap: anywhere` for long values
  (emails especially, on User Management), action-button collapsing into a `<Menu>` at the
  smallest widths.
- Test against 360px width specifically (T28's own minimum), not just "looks fine at 768px."
- The Network page's graph library may have its own viewport/zoom API — check what's already
  exposed before building custom pan/zoom scaffolding.

## Done when

- At 360px, 390px, and 414px viewport widths, none of the three pages overflow the page
  horizontally.
- User Management's user list is usable (readable, actionable) at 360px.
- Settings sections are all reachable and usable at 360px.
- The Network graph is viewable (even if panning/zooming is required) without page-level
  horizontal overflow.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Visually verified on responsive-design mode at 360px, 390px, and 414px for all three pages.

**Done, 2026-08-05.** Network, Settings and User Management no longer overflow horizontally at
360-414px. Root cause shared across all three: the app's `<main>` flex item defaulted to
`min-width: auto`, so any wide child (timeline, network controls, settings tables) forced the
whole page to scroll — fixed with `minWidth: 0` so each child contains its own overflow
(committed with T33). Network additionally grew on phone widths (the fixed page height clipped
the graph card) and the graph got an explicit 65vh card; User Management reflows to stacked
card-per-user below `sm`. New `e2e/mobileLayout.spec.ts` pins the no-overflow invariant and
the stacked user list.
