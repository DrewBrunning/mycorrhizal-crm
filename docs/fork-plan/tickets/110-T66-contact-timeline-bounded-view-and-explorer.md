# T66 — Contact timeline: bound the composite + paginated filterable timeline endpoint (backend)

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 — not urgent today, but the underlying fetch is already unbounded and Immich is actively compounding it |
| **Size** | M |
| **Source** | User request, 2026-08-11. Split into backend (this) + web ([T78](122-T78-web-timeline-bounded-view-explorer.md)) on 2026-08-11. |
| **Depends on** | Nothing. [T17](17-T17-change-feeds.md)'s cursor-pagination primitives (`cursorPredicate`/`cursorOrderBy`) already exist and are reused here rather than invented fresh. Blocks [T78](122-T78-web-timeline-bounded-view-explorer.md) — **unblocked 2026-08-12 when this landed**. |
| **Status** | **DONE** (2026-08-12) |

## Why this exists

A contact's timeline can get very large, very quickly — especially once Immich is linked, since
every photo match becomes its own row. Two things compound today, both confirmed against current
source, not assumed:

1. **The merged timeline endpoint fetches everything, unbounded, on every contact-page load.**
   `contact_detail_controller.go`'s `GetContactDetail` (the M4 composite) pulls notes, activities,
   reminder completions, life events, external activities, and gifts with plain `.Find()` calls and
   no `.Limit()` anywhere in the chain — confirmed at `contact_detail_controller.go:78` (completions,
   `Order("completed_at DESC")` with no limit), `:93` (life events, via
   `attachContactDetailLifeEvents`), and `:118` (external activities). A contact with a long history
   pays for the whole thing on every page load, not just when someone asks to see it.
2. **`ContactTimeline.tsx` renders every merged item with no pagination or virtualization.**
   `ContactDetailPage.tsx:932-969` merges all six item types (`note`/`activity`/`completion`/
   `life_event`/`external_activity`/`gift`) into one client-side sorted array and hands the whole
   thing to `ContactTimeline`, which maps over it unconditionally
   (`components/ContactTimeline.tsx:73`).
3. **Immich is the accelerant, not the cause.** Each Immich photo-appearance match lands as its own
   `ExternalActivity` row (`backend/services/immich_service.go:377,451`, type
   `photo-appearance`) — a well-photographed contact can accumulate rows fast, but the underlying
   problem (unbounded fetch + unbounded render) exists independent of Immich and will eventually
   bite active contacts through notes/activities alone.

## The shape requested

- The contact detail page's timeline section shows the **5 most recent** events by default (flat
  merge across all types, sorted by date — matching what the section shows today, just truncated).
- A **"View all"** control opens the full timeline in a modal/dialog on web. (On Android, the
  equivalent should push a new screen rather than open a sheet — noted for whoever picks up the
  Android side later; this ticket is web + backend only.)
- Inside that explorer: filter by **event type** and by **how long ago it happened**.

## Design decisions — settled 2026-08-11

1. **Type filter granularity: the six raw types.** `note`/`activity`/`completion`/`life_event`/
   `external_activity`/`gift`, matching the data model 1:1. Revisit only if a second
   external-activity source ever ships alongside Immich.
2. **Recency filter: fixed buckets, not a range picker.** Last 7 / 30 / 90 days, this year, all
   time. A free-form date-range picker is more UI and more query surface for a filter whose job is
   "recent-ish vs. everything".
3. **Merge strategy: per-table bounded fetch, merged in Go (option b).** The original draft
   recommended (b) but described its downside as "a page split evenly across six
   independently-limited queries can undercount when one type dominates." **That downside is not
   real, provided each table is asked for a full page rather than a share of one.**

   Fetch `N` from each of the six tables (where `N` is the requested page size), merge, take the top
   `N`. This is *exactly* correct, not approximate: if item X belongs in the true global top N but
   wasn't returned, X's own table must have had N items ranked above it — all of which are also
   globally above X, so X was never in the top N. Contradiction. The only cost is fetching up to
   `6N` rows to return `N`, which at a page size of 25 is trivial.

   The failure mode to actually avoid is the tempting one: asking each table for `N/6`. That *does*
   undercount. Write the reasoning into the function's doc comment so nobody later "optimizes" it
   into being wrong.

   The cursor is a normalized `(event_date, type, id)` tuple — `type` is needed in the key because
   two rows in different tables can share a date, and `id` because two rows in the same table can
   too. PK types differ across the six tables (uint vs. UUID-string), so the cursor carries `id` as
   a string and each per-table predicate re-types it, the way `parseCursorID` already does.
4. **Bound the M4 composite's own queries** rather than trimming its output. `.Limit(N)` per type
   with the same N-per-table reasoning as above — with a 5-item preview, 5 per type is provably
   sufficient. Trimming client-side would leave problem #1 (the unbounded fetch) entirely unfixed,
   which is the actual point of this ticket.

## Done when

- `GetContactDetail` (M4 composite) no longer issues unbounded `.Find()` calls for
  timeline-eligible types — each is capped at a small, explicit limit sufficient to build the
  default 5-item preview correctly regardless of type distribution.
- A new paginated timeline endpoint exists, filterable by type and by a recency bucket, using T17's
  cursor-pagination primitives rather than offset pagination.
- A test proves the merge is correct under skew: a contact whose timeline is dominated by one type
  (say 200 `external_activity` rows and 3 notes) still returns a full, correctly-ordered page, and
  paging all the way through returns every item exactly once.
- The composite's response payload for a contact with a long history is measurably smaller than
  before — record the before/after in the landing note.
- OpenAPI spec updated ([T8](16-T8-openapi.md)'s drift test will fail otherwise).
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.

## Landing note

**Shipped 2026-08-12** (branch `feature/T66-contact-timeline-bounded-endpoint`).

**`GET /contacts/:id/timeline`** — a page of the merged timeline across all six types, cursor-
paginated over a normalized `(event_date, type, id)` key, filtered by `?type=` (comma-separated
subset of the six tokens) and `?bucket=` (`last_7_days|last_30_days|last_90_days|this_year|all`).
Merge strategy is exactly design decision 3 option b: each SQL-orderable table is asked for a full
page (N+1, so `next_cursor` presence is exact), merged, sorted, top N returned. The cursor's type
component is what makes the per-table predicates correct — a table ranking below the cursor's type
is entirely before it at a shared date, a table ranking above it must be strictly older; the id is
re-typed per table (uint vs UUID-string) so SQLite's type ordering can't miscompare, and is only
validated/parsed for the one table whose type equals the cursor's (a UUID-string cursor from a
string-PK page boundary must not 400 a mixed page). The proof from the ticket is written into the
controller's doc comment so nobody "optimizes" the per-table N+1 into N/6.

**One deliberate deviation from design decision 3**: life events do NOT participate in the per-table
bounded fetch, because their `PartialDate` is JSON text with no SQL-orderable timestamp — they're
fetched in full per request and their cursor/bucket predicates are applied in Go on a resolved date
(the web's `fullDateFromPartial` semantics, UTC midnight). The bounded-merge proof doesn't cover
them, but the exception is bounded-small (a person's notable life events, not Immich's
photo-appearance flood) and documented in the controller. If life events ever become a large
unbounded table, revisit with a denormalized sortable date column.

**Composite bound** (`buildContactDetail`): the six timeline-eligible blocks are now capped at
`timelinePreviewLimit` (5) each, ordered by the timeline's event-date key. Two ordering fixes the
bound made necessary: external activities gained an `occurred_at DESC` order (were unordered) and
gifts now order by `date DESC` (were `created_at DESC`) so undated ideas sort out of the top 5 —
matching the timeline's "undated ideas are not events" rule. Life events stay on `created_at DESC`
(their PartialDate isn't SQL-orderable in the composite either; the bound is the point there, not
the ordering). The other composite blocks (reminders, agenda, field values, identities, circles,
tags) stay unpaginated per M4 design decision 6.

**Payload measurement (done-when item)**: seeded a contact with 200 external activities + 40
notes/completions/gifts and measured the composite response with the limits removed vs in place
(the same harness, one edit apart) — **122,779 bytes unbounded → 8,407 bytes bounded**, a ~14.6×
reduction (~93% smaller). Pinned by `TestGetContactDetail_PayloadBoundedForLongHistory`, which
asserts the bound and logs the byte count.

**Tests, all hand-verified to fail pre-fix** (`controllers/timeline_controller_test.go`, real
`database.InitDB` schema): the ticket's core merge-under-skew test (200 external activities + 3
notes → full correctly-ordered pages, walk returns all 203 items exactly once), same-date
type-rank/id tiebreaks, the comma-separated type filter, all five recency buckets, life-event date
resolution (full/year-only/yearless/created-at fallback), gift eligibility (given/received-with-date
only), validation 400s (unknown type/bucket, malformed cursor), and cross-user 404. The bounded-
composite tests fail when any `.Limit()` is removed. Four Playwright e2e specs
(`frontend/e2e/timelineEndpoint.spec.ts`) drive the real API end to end: merge+order+paging,
type/bucket filters, unknown-type 400, and the composite bound for data created through the REST
pipeline. OpenAPI updated (route + `TimelineItem`/`TimelineResponse` schemas; drift test green).

**Unblocks [T78](122-T78-web-timeline-bounded-view-explorer.md)**, whose explorer consumes this
endpoint with the exact type/bucket vocabulary it validates.
