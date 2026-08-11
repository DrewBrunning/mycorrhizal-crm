# T66 — Contact timeline: bounded default view + full-timeline explorer with filters

| | |
|---|---|
| **Rating** | 3 — not urgent today, but the underlying fetch is already unbounded and Immich is actively compounding it |
| **Source** | User request, 2026-08-11 |
| **Depends on** | Nothing. [T17](17-T17-change-feeds.md)'s cursor-pagination primitives (`cursorPredicate`/`cursorOrderBy`) already exist and are reused here rather than invented fresh. |
| **Status** | Scoped, not started. No defined vision yet — see the design questions below before implementing. |

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

## Design questions to settle before implementing

Flagging these rather than guessing, per the "no defined vision yet" starting point:

1. **Type filter granularity.** The six raw types (`note`/`activity`/`completion`/`life_event`/
   `external_activity`/`gift`) mirror the backend model, but some may deserve grouping or splitting
   in the filter UI — e.g. is "external activity" one filter option, or does it eventually need to
   distinguish Immich from a future second integration? Recommend starting with the six raw types
   (cheapest, matches the data model 1:1) and revisiting only if a second external-activity source
   ships.
2. **Recency filter buckets.** Not specified by the request — pick concrete buckets during
   implementation (e.g. last 7/30/90 days, this year, all time) rather than a free-form date-range
   picker, unless a range picker turns out to be just as cheap given whatever backend query shape
   gets built.
3. **Cursor design across heterogeneous tables.** T17's existing cursor pagination
   (`cursorPredicate`/`cursorOrderBy`) is built for a single table's `(updated_at, id)` ordering. A
   merged timeline spans six tables with different PK types (uint vs. UUID-string) and different
   date columns (`date`, `completed_at`, `occurred_at`, partial dates on life events). Decide the
   merge strategy up front — likely candidates: (a) a real `UNION`-backed query keyed on a
   normalized `(event_date, type, id)` tuple, or (b) fetch a bounded page from each table
   independently and merge/truncate in Go. (b) is simpler and reuses T17's per-table primitives
   as-is; (a) is more correct at the boundary (a page split evenly across six independently-limited
   queries can undercount when one type dominates). Recommend (b) to start, since it's a much
   smaller change and the failure mode (a slightly short page when one type dominates) is cheap to
   detect and fix later if it matters in practice.
4. **Does the 5-item default preview need its own bounded backend call, or is trimming the M4
   composite's existing arrays to 5-per-type-then-merge-and-slice sufficient?** The latter doesn't
   fix problem #1 above (the composite still fetches everything, just displays less) — worth doing
   properly: either cap the composite's underlying queries at a small multiple of 5 (e.g. `.Limit(20)`
   per type, which safely covers "flat top 5 across 6 types" even in a worst-case single-type
   skew), or have the composite call the same bounded-timeline logic the explorer uses. Prefer
   actually bounding the composite's queries — the point of this ticket is fixing the unbounded
   fetch, not just hiding it behind a shorter render.

## Done when

- `GetContactDetail` (M4 composite) no longer issues unbounded `.Find()` calls for
  timeline-eligible types — each is capped at a small, explicit limit sufficient to build the
  default 5-item preview correctly regardless of type distribution.
- A new paginated, filterable timeline endpoint (or equivalent query shape) exists, filterable by
  type and by a recency bucket, using T17's cursor-pagination primitives rather than offset
  pagination.
- Contact detail page shows the 5 most recent timeline items by default, with a "View all" control.
- "View all" opens a modal/dialog with working type and recency filters, paginated (not one giant
  unbounded fetch even inside the explorer).
- Hand-verified against a contact with 50+ mixed timeline items spanning all six types, confirming
  both the default view and the explorer behave correctly and the network payload for a normal
  contact-page load is visibly smaller than before.
- New strings translated in all five locales.
