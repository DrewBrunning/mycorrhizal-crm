# T73 — Contacts list can only be sorted by most-recently-edited (backend)

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 — real UX gap, has a live workaround (search/filter) |
| **Size** | S–M — migration + query param + a second cursor shape |
| **Depends on** | Nothing. Blocks [T77](121-T77-web-contacts-list-sort-control.md), the web control that consumes it. |
| **Status** | DONE (2026-08-12) |
| **Source** | Testing notes, 2026-08-11: "Contacts sort by most recently edited" |

## Why this exists

Confirmed real and permanent, not a mis-observation of transient refetch behavior:

- `GetContacts` (`backend/controllers/contact_controller.go:110`) orders via
  `cursorOrderBy(query, "contacts", desc)`, which in `backend/controllers/helpers.go:265-271` always
  does `.Order("contacts.updated_at " + dir).Order("contacts.id " + dir)`. There is no `sort`/
  `order_by` param at all — not even reachable by a hand-crafted API request. The sort field is
  compile-time fixed.
- `order` is a *direction* only, typed `'asc' | 'desc'`, never a field selector.
- `Contact.UpdatedAt` is bumped by GORM on every save, so editing any field on a contact moves it to
  the front of the list on the next fetch.

This is a side effect of [T17](17-T17-change-feeds.md)'s cursor-pagination redesign — `(updated_at,
id)` was chosen for sync-cursor stability (a stable total order under concurrent writes,
`helpers.go:250-260`), not as a deliberate "browse by recently edited" product decision.

## The design — decided 2026-08-11

### The sort key is a denormalized `sort_name` column, not an expression

The obvious `ORDER BY lastname, firstname` has two problems this codebase can't wave away. First,
`Firstname`/`Lastname` are declared `COLLATE NOCASE` (`models/contact.go:73-75`), and a cursor's
row-value predicate must compare under exactly the same collation as its `ORDER BY` or pagination
silently skips and repeats rows at page boundaries. Second, a personal CRM has many first-name-only
contacts, whose empty `lastname` would clump them all at one end.

Both go away with a denormalized key, which is also the idiom this codebase already uses for
derived search columns (`addresses_flat`, T38):

```
sort_name = lower(trim(lastname))  when non-empty
          = lower(trim(firstname)) otherwise
```

Maintained in `Contact.BeforeSave` beside `AddressesFlat` (`models/contact.go:242`), backfilled by
the migration, and indexed. Pre-lowercased, so no collation ambiguity; never empty for a valid
contact, since `Firstname` is required.

### `sort` applies to `?cursor=` only — never to `?since=`

This is the constraint that keeps T17 intact. The `?since=` change feed is *sync state*, not
browsing: `CheckFeedCursorAge` (`helpers.go:~240`) compares `cursor.UpdatedAt` against the purge
retention window to decide whether to return `410 Gone`. A name-ordered feed has no `UpdatedAt` to
check and no meaningful replay order.

**Decided: `?since=` combined with `sort` is a `400`**, not a silent fallback to `(updated_at, id)`.
Silently ignoring a parameter the caller explicitly set is the worse failure — a sync client would
believe it had a name-ordered feed and quietly diverge. This also matches how the same handler treats
an unrecognized `sort` value (item 3 below).

### A second cursor shape, not a generalized one

`Cursor` (`helpers.go:133-136`) is `{UpdatedAt time.Time; ID string}`, encoded as
`RFC3339Nano|id`. Rather than generalizing it — which would touch every list handler that
pagination — add a name-sorted variant carrying `{SortName string; ID string}` encoded the same way,
used only when `sort=name`. Everything else, including every other list endpoint, keeps today's
shape untouched.

## What to build

1. Migration `000021`: `sort_name TEXT NOT NULL DEFAULT ''` on `contacts`, backfilled in SQL with
   the rule above, plus an index on `(user_id, sort_name, id)`. Matching `.down.sql`.
2. `Contact.SortName` populated in `BeforeSave`.
3. `sort` query param on `GetContacts`: `updated_at` (default, today's behavior) | `name`. Anything
   else is a `400` via `apperrors.ErrInvalidInput`, not a silent fallback.
4. Cursor variant + predicate/order-by for the name sort. `order` (`asc`/`desc`) keeps working for
   both sorts.
5. `?since=` handling per the decision above.
6. OpenAPI spec updated — [T8](16-T8-openapi.md) has a drift test that will fail otherwise.
7. Tests against a `database.InitDB` schema (trap #1): paging all the way through a name-sorted
   fixture returns every contact exactly once, with **no duplicates or skips at page boundaries**,
   including a fixture with several contacts sharing a `sort_name` (the case the `id` tiebreak
   exists for) and several with no lastname. Hand-verify per `/CLAUDE.md`.

## Traps

- **Don't break T17's pagination-cursor stability guarantee.** Read T17's ticket before touching
  `cursorOrderBy`/`GetCursorParams`. The tiebreak-by-`id` is not decoration — without it, contacts
  sharing a sort key straddle page boundaries unpredictably.
- **`sort_name` changes when a contact is renamed**, so a page cursor taken before a rename can skip
  or repeat that one row. This is inherent to keyset pagination on a mutable key and is acceptable
  for browsing — it is exactly why `?since=` must not use it.
- **Keep `updated_at` as the default.** This ticket adds a missing control; changing the default for
  existing users is [T77](121-T77-web-contacts-list-sort-control.md)'s decision to make on the web
  side, and only for new sessions.

### Landing note (2026-08-12)

Built exactly to the decided design, with no deviations: migration `000021` adds
`contacts.sort_name` (`lower(trim(lastname))` else `lower(trim(firstname))`, `COALESCE`-guarded so a
NULL lastname — legal in the schema — falls back to the firstname instead of writing NULL into the
NOT NULL column and aborting the migration), backfilled in SQL and indexed on
`(user_id, sort_name, id)` mirroring `idx_contacts_feed`. `models.DeriveSortName` keeps the Go-side
derivation in lockstep (the same testable-function pattern as `FlattenPhones`/`FlattenAddresses`),
populated by `Contact.BeforeSave` after the projection block so it reflects the final
Firstname/Lastname.

The controller adds `?sort=updated_at|name` (anything else is a 400 via `ErrInvalidInput`, not a
silent fallback) with a **second cursor shape** — `NameCursor` encoded as
`base64url("sort_name|id")` — used only when `sort=name`. `GetCursorParams` was refactored into a
shared `cursorParams(c, kind)` with `GetCursorParams` (time shape, byte-identical behavior for every
existing list endpoint) and the new `GetCursorParamsForSort` (name shape for the contacts list).
`?since=` is always decoded time-shaped, and `sort=name` + `?since=` is an explicit 400 per the
decision — the change feed is sync state, never name-ordered. Cross-shaped cursors fail loudly in
both directions (`DecodeNameCursor` rejects a timestamp-shaped payload; the time decoder already
rejects a name-shaped one), so a cursor minted under one sort can never be silently misapplied under
the other.

Tests: `TestDeriveSortName` + `TestBeforeSave_DerivesSortName` (models), migration shape + data-safe
backfill tests incl. the NULL-lastname row (`database/migrate_test.go`), `NameCursor` round-trip /
malformed / cross-shape decode tests, and a controller suite (`contact_sort_test.go`) covering both
directions, the exhaustive paging-total-order proof (three-way `sort_name` tie + two no-lastname
contacts straddling page boundaries at `limit=2`/`limit=3`), the unknown-sort 400, the since+sort
400, the cross-shape-cursor 400, sort composing with `search`, and a `database.InitDB` real-migrated
schema test (trap #1). Four Playwright e2e tests (`contactSort.spec.ts`) drive the API through the
real stack: name order both directions, a full name-sorted page walk returning every created contact
exactly once, and the two 400 paths. All hand-verified to fail against broken implementations
(uppercase sort key, `sort_name` removed from the ORDER BY, backfill stripped to `''`).

One near-miss caught by the full-suite gate during development: the hand-verification `sed` that
restored `nameCursorOrderBy`'s `sort_name` also rewrote `cursorOrderBy` to order by `sort_name`,
which would have broken **every** other list endpoint (activities, notes, circles, …) — caught by
`TestGetActivities` in the full `go test ./...` run before any commit. The whole backend suite,
`npx tsc --noEmit`, `npx vitest run`, and the full Playwright suite are green; the only full-run
e2e failures were two `immich.spec.ts` tests that pass in isolation (a pre-existing shared-config
race between parallel workers, unrelated to this change).

## Done when

- `GET /contacts?sort=name` returns contacts ordered by the denormalized key, in both directions.
- Paging fully through a name-sorted list yields every contact exactly once — verified against a
  seeded dataset with sort-key ties and missing lastnames.
- `?since=` behavior with `sort` present is explicit and tested.
- OpenAPI drift test passes.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
