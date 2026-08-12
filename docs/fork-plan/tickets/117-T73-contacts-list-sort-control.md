# T73 — Contacts list can only be sorted by most-recently-edited (backend)

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 — real UX gap, has a live workaround (search/filter) |
| **Size** | S–M — migration + query param + a second cursor shape |
| **Depends on** | Nothing. Blocks [T77](121-T77-web-contacts-list-sort-control.md), the web control that consumes it. |
| **Status** | TO BE DONE |
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

## Done when

- `GET /contacts?sort=name` returns contacts ordered by the denormalized key, in both directions.
- Paging fully through a name-sorted list yields every contact exactly once — verified against a
  seeded dataset with sort-key ties and missing lastnames.
- `?since=` behavior with `sort` present is explicit and tested.
- OpenAPI drift test passes.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
