# T78 — Contact timeline: 5-item default + "View all" explorer (web)

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — the user-visible half of T66 |
| **Size** | M — a new dialog with filters and pagination |
| **Depends on** | [T66](110-T66-contact-timeline-bounded-view-and-explorer.md) — **landed 2026-08-12**; the paginated, filterable timeline endpoint it specifies is now live, and the composite's timeline blocks are bounded at 5. Unblocked. |
| **Status** | **DONE** (2026-08-12) |
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

## Landing note

**Shipped 2026-08-12** (branch `feature/t78-web-timeline-bounded-view-explorer`).

The timeline section's preview now truncates to the **5 most recent** merged events via
`timelineItems.slice(0, 5)` — T66 already capped each composite block at 5, so this is literally
"what it showed before, just cut off". A "View all" button in the `PanelCard`'s actions slot opens
the explorer regardless of history size, so the empty state is reachable in both surfaces.

**The explorer (`components/TimelineExplorerDialog.tsx`)** is an `AppDialog` over T66's
`GET /contacts/:id/timeline`:
- **Type filter**: a six-option multi-select (checkboxes), defaulting to all; the API layer omits
  `?type=` when the full set is selected (the backend default), and an empty selection reads back as
  "All types".
- **Recency filter**: a single-select over the five T66 buckets (`last_7_days` … `all`).
- **Pagination**: a 25-item first page plus a "Load more" button that appends the `next_cursor`
  page — "View all" is a second bounded fetch, never an unbounded one.
- **Rows**: `ContactTimeline` is reused verbatim, so edit/delete on note/activity/completion rows
  works identically to the preview. `ContactTimeline` gained two small things: an optional
  `emptyText` (the default "no notes or activities yet" is wrong for a filtered view that matched
  nothing), and aria-labels on its edit/delete icon buttons (they had none, which also made them
  untargetable by the e2e edit flow).
- **Consistency**: the page passes a `revision` counter bumped by `refreshNotesAndActivities`; when a
  note/activity edit or completion delete lands through the page-level dialogs, the explorer
  refetches its own list instead of showing stale rows.

**New modules** follow the `relationshipEdges` pattern the ticket names: `api/timeline.ts`
(`getTimeline` + `TIMELINE_TYPES`/`TIMELINE_BUCKETS`, hardcoded mirrors of
`backend/models/timeline.go` per frontend trap #4) and `hooks/useTimeline.ts` (owns the page,
filters, cursor; `refresh` is memoized on the filters, `loadMore` appends).

**Concurrency** (review pass): `useTimeline` uses the repo's request-epoch guard
(`useActivities`/`useAudit`/`useGraph` all do) so a stale fetch — a "Load more" that was in flight
when the filter changed, or a fetch started for a previous contact — is discarded rather than
appended over fresh rows; and it clears its page when `contactId` changes, so navigating between
contacts can't briefly render the previous contact's rows (the dialog stays mounted across contact
navigation). The "Load more" button is disabled during a refresh too, since its cursor belongs to
the page it was returned with.

**Deliberate decisions**: the "View all" button is always visible (not gated on >5) so the explorer
and its empty state are reachable for small/empty histories.

**Tests, all hand-verified to fail pre-fix where a pre-fix existed**:
- 9 `api/timeline.test.ts` cases (URL contract: comma-joined type subset, full-set/empty omission,
  bucket/cursor passthrough, 400 error propagation, registry mirrors).
- 2 `useTimeline.test.ts` cases (the stale load-more discard, and page clear on contact switch —
  both verified to fail against the pre-guard hook).
- 7 `TimelineExplorerDialog.test.tsx` cases (fetch on open, no fetch while closed, type/bucket
  filter refetch, load-more append, empty state, edit/delete passthrough).
- 7 Playwright specs in `e2e/timelineExplorer.spec.ts`: preview truncation to 5 + full explorer,
  all-six-types mixed preview/explorer, type filter isolation, bucket filter + type combination
  (with the filtered-empty state), cursor paging past 25, zero-item empty states in both surfaces,
  and editing a note from within the explorer (stacked dialogs + the revision-triggered refresh).
  All driven against the real shipped artifact (rebuilt `docker-compose.test.yml` stack); the full
  e2e suite is green. (`e2e/reminders.spec.ts` also gained the `waitForLoading`/`stableClick` the
  other contact-page specs use — it was intermittently failing under parallel load without them.)

**Unblocks nothing** — this was the final web ticket. The Web platform list on the board is now
empty; the remaining open work is the Android list (T67/T81/M21/…, see the board).
