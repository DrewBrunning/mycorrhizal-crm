# T111 — Contacts: search, filters and bulk actions scroll away; pin them above the list

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 4 — the bulk actions are unreachable once you scroll past the first screen, which defeats N5 |
| **Size** | M |
| **Depends on** | Nothing (T110 edits the same toolbar — land T110's gap fix together or first) |
| **Status** | **DONE** (2026-08-15, same branch as T110 — landed together) |
| **Source** | Testing note: *"Search, buttons, and bulk actions should be fixed above the scrolling content rather than scrolling off screen (makes it hard to take actions)."* |

## Why this exists

`ContactsPage.tsx` renders everything inside one scrollable `Box` (`:319`):

1. `Typography` title (`:320-322`)
2. the search `TextField` (`:323-347`)
3. the filter/toolbar `Stack` (`:348-426`)
4. the hidden-count caption (`:430-434`), the active-circle chip (`:435-443`)
5. the `BulkActionsBar` (`:444-461`)
6. the scrolling contact list + "load more" (`:462-538`)

None of 2–5 is `position: sticky`, so the moment the list scrolls, the search field, the filter controls and
the bulk-action bar scroll out of view — and the bulk bar is only visible *after* you select rows, so taking
a bulk action requires scrolling back to the top to find it. This is the core N5 workflow.

## What to build

Make the search field, the toolbar row, and the `BulkActionsBar` a sticky header above the list. Concretely:

- Wrap items 2–5 above in a single sticky container (`position: 'sticky'`, `top: 0`, a high `zIndex` below
  the AppBar's, and an opaque `bgcolor: 'background.paper'` so scrolling cards don't show through). The
  AppBar is `position: fixed` at height 64 (plus the T31 jump-nav pattern already uses `top: 64`), so use
  `top: 64` (and `top: 56` on `xs` where the AppBar is smaller) rather than `0`.
- The `BulkActionsBar` must be part of the sticky block so the bulk actions stay reachable while the list
  scrolls. Confirm it still only renders (or only is meaningful) when `selectedCount > 0` — see
  `BulkActionsBar.tsx` — so an empty-selection sticky bar doesn't take up space.
- The list (`:462-538`) remains the only scrolling content. The `SearchNotesActivities` section (`:539-545`)
  and the dialogs stay below as today.

## Traps

- A sticky container that spans the full page width must not break the `maxWidth: 1200, mx: 'auto'` centering
  of the page (`:319`) — keep the sticky block inside the existing `maxWidth` `Box` so it stays centered and
  doesn't edge-to-edge against the drawer.
- The AppBar is `zIndex: drawer + 1`; the sticky block must sit below it or it will cover the AppBar's search
  on scroll. Mirror the `top`/`zIndex` numbers `ContactDetailPage`'s `ContactJumpNav` already uses (`:218-267`).
- `ScrollToTop` (`App.tsx:87-95`) scrolls on route change only; the sticky block must not re-introduce a jump
  when the search debounce commits a new `?search=` (that state update does not remount the page — verify no
  scroll reset on typing).
- Coordinate with T110: its `gap` fix is inside the same toolbar; whichever lands second must preserve the
  other.

## Done when

- Scrolling the contact list leaves the search field, filter controls, and (once rows are selected) the
  bulk-action bar visible and pinned above the list.
- Bulk actions (add/remove circle/tag, archive/unarchive, merge, delete) can be taken without scrolling back
  to the top.
- At 390px and 1440px nothing overlaps the AppBar and the sticky block stays centered with the page.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec scrolling the list and
  asserting the bulk bar is still on screen.

## Landing note (2026-08-15)

The search field, T110's toolbar row, the hidden-count caption, the active-circle chip and the
`BulkActionsBar` were wrapped in a single `position: 'sticky'` container (`top: { xs: 56, sm: 64 }`,
`zIndex: 10`, `bgcolor: 'background.paper'`) inside the page's existing `maxWidth: 1200` box, so it stays
centered and clears the fixed AppBar. The title stays above it and the list scrolls beneath the opaque
band. Landed with T110 in the same commit (they edit the same region). All 23 ContactsPage tests pass,
including a **new T111 regression test** added in the review pass that climbs from the search field to its
nearest `position: sticky` ancestor via `getComputedStyle` (MUI's `sx` emits a class, so no attribute
selector can find it) and asserts the bulk bar's "Select all" lives in the *same* sticky container while
the contact cards do not — hand-verified to fail with the sticky removed. Browser-level scroll
verification still outstanding (no browser in the build env).

