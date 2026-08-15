# T114 — Collapse the Connections panel to its top five connections

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — a contact with a deep network renders dozens of chains and buries the useful ones |
| **Size** | S |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-15) |
| **Source** | Testing note: *"Collapse Connections to top five?"* |

## Why this exists

`ConnectionsPanel.tsx` renders **every** returned chain with no bound: `connections.chains.map(renderChain)`
(`:171-173`). `GET /graph/connections` returns every reachable contact within the selected depth, so a
well-connected contact at depth 3 can produce a very long list. The timeline section solved the same problem
in [T78](122-T78-web-timeline-bounded-view-explorer.md) — preview the 5 most recent and offer "View all" —
but Connections has no such bound.

The note ends in a `?`, so the shape is a design decision, but the parity argument is strong: every other
unbounded collection on this page (timeline via T66/T78, external activities, the composite's six blocks)
is already capped at 5 with a "view more" affordance. Connections is the odd one out.

## What to build

- Render at most the 5 most relevant chains by default, with a "View all" affordance that reveals the rest.
  The backend has no order guarantee worth relying on for "relevance" (`chains` is a BFS traversal), so
  prefer the first 5 in server order for the preview and mark the rest as the expansion — do not invent a
  client-side ranking the endpoint never promised.
- Decide the reveal mechanism: the simplest parity with T78 is a "View all (N)" button in the PanelCard's
  actions slot that toggles a `showAll` local state in `ConnectionsPanel` (the panel is already self-
  contained), rather than a new endpoint — the data is already in hand, so a second fetch is wasted work.
- Keep the "N connections" count (`connections.results`, `:166-169`) accurate for the *full* set even while
  previewing 5, so the truncation reads as a preview, not as missing data.
- Add strings for the toggle in all five locales (e.g. `connections.viewAll` / `connections.showLess`).

## Traps

- `connections.chains` arrives only after `visible` flips true (the panel's IntersectionObserver deferral,
  `:51-79`); the `showAll` toggle must not trigger a refetch — it only reveals already-loaded chains.
- Empty state (`:155-160`) and the relation filter (`:135-149`) must behave identically whether truncated or
  expanded — a filter that matches 2 of 50 chains should not be forced to expand to show 2.
- Do not change the default depth here — that is a separate testing note already fixed (default 1 hop). This
  ticket is only about capping the *rendered* chains.
- If "top five" should mean "five most connected" rather than "first five", that is a backend ordering
  change to `/graph/connections`, not a frontend slice — flag it rather than guessing; the recommendation
  above is first-five-in-server-order because the endpoint makes no relevance promise.

## Done when

- A contact whose Connections list has more than 5 chains shows 5 by default with a "View all (N)" toggle;
  clicking it reveals the rest, clicking again re-collapses.
- The "N connections" count reflects the full set at all times.
- The relation filter and empty state still work at both truncation states.
- New strings in all five locales; `cd frontend && npx tsc --noEmit && npx vitest run` green, with a
  `ConnectionsPanel.test.tsx` case asserting only 5 render before expanding.

## Landing note (2026-08-15)

`ConnectionsPanel` renders the first 5 chains by default with a `connections.viewAll` / `connections.showLess`
toggle (×5 locales) that reveals the rest, and re-collapses on every new result set (an effect keyed on the
`connections` object), so a depth/relation change never re-opens a long list. The full "N connections" count
keeps reflecting the whole set. A new test seeds 7 chains and asserts only 5 render, Contact 6/7 absent,
then expands. One test-authoring trap: each chain renders its target name *and* its own step link, so a
contact name appears twice — `getByText` failed with "multiple elements" and the assertions use
`getAllByText`/`queryAllByText` instead. ConnectionsPanel's 6 tests pass.

