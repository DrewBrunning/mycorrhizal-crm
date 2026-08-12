# T78 — Contact timeline: 5-item default + "View all" explorer (web)

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — the user-visible half of T66 |
| **Size** | M — a new dialog with filters and pagination |
| **Depends on** | [T66](110-T66-contact-timeline-bounded-view-and-explorer.md) — the paginated timeline endpoint doesn't exist yet. Blocked until it lands. |
| **Status** | TO BE DONE |
| **Source** | User request, 2026-08-11. Split from T66 on 2026-08-11 so the backend and web halves rank on their own platform lists. |

## Why this exists

`ContactDetailPage.tsx:932-969` merges all six item types (`note`/`activity`/`completion`/
`life_event`/`external_activity`/`gift`) into one client-side sorted array and hands the whole thing
to `ContactTimeline`, which maps over it unconditionally (`components/ContactTimeline.tsx:73`) — no
pagination, no virtualization, no cap. A contact with a long history renders every row it has ever
accumulated, every time the page loads. Immich is the accelerant: each photo-appearance match is its
own `ExternalActivity` row.

[T66](110-T66-contact-timeline-bounded-view-and-explorer.md) fixes the fetch side. This ticket is
the render side and the explorer UI.

## The shape requested

- The timeline section shows the **5 most recent** events by default — flat merge across all types
  sorted by date, exactly what it shows today, just truncated.
- A **"View all"** control opens the full timeline in a modal dialog.
- Inside the explorer: filter by **event type** and by **how long ago it happened**.

Filter vocabulary is settled in T66: the six raw types, and recency buckets of last 7 / 30 / 90
days, this year, all time. Match them exactly — the backend validates against this set.

## What to build

1. Truncate the default timeline section to the 5 most recent merged items, with a "View all"
   control in the `PanelCard`'s `actions` slot (the same slot the other cards use for their "Add"
   buttons — see `ContactDetailPage.tsx:1449`).
2. A timeline explorer dialog: type filter (multi-select across the six types), recency bucket
   select, and a paginated list driven by T66's cursor endpoint. **Paginate inside the explorer
   too** — "View all" must not become a second unbounded fetch, which would move the problem rather
   than fix it.
3. A new `src/api/` module + `src/hooks/use…` pair for the endpoint, following the
   `relationshipEdges.ts` + `useRelationshipEdges.ts` + `RelationshipEdgeDialog/List` shape
   `/CLAUDE.md` names as the most complete example of the pattern.
4. Reuse `ContactTimeline`'s existing row rendering for both surfaces rather than writing a second
   renderer — the explorer is the same list with filters and paging around it.
5. New strings translated in all five locale files (`en`, `de`, `es`, `fr`, `it`) — `/CLAUDE.md`
   frontend trap #5; `src/i18n/locales.test.ts` enforces it.

## Traps

- **Don't nest a `<Chip>` inside `<Typography variant="body2">`** if the explorer rows show type
  badges — invalid HTML, React warns (`/CLAUDE.md` frontend trap #3). Sibling flex `Box`.
- **`afterEach(cleanup)` explicitly** in any new component test file — vitest here has no
  auto-cleanup (`/CLAUDE.md` frontend trap #1).
- **Empty collections**: a contact with no timeline items at all must render the empty state, not
  crash. `/CLAUDE.md` frontend trap #8 is this exact failure mode taking the prep view into the
  ErrorBoundary — check what the new endpoint returns for "no items" and guard the `.length`.

## Done when

- The contact detail page's timeline section shows 5 items by default with a working "View all".
- The explorer's type and recency filters work and are combinable, and its list pages rather than
  fetching everything.
- A contact with 50+ mixed items spanning all six types behaves correctly in both surfaces —
  hand-verified in the browser.
- A contact with zero timeline items renders an empty state in both surfaces.
- New strings translated in all five locales.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
