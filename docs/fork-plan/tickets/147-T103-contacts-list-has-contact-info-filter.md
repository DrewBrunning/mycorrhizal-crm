# T103 — Contacts should default to showing contacts you can actually contact

| | |
|---|---|
| **Platform** | Backend + Web |
| **Rating** | 4 — the list is the app's front door and real data has made it noisy |
| **Size** | M |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-13 — see the landing note) |
| **Source** | Beta testing note, 2026-08-13: *"Filter to contacts with contact info by default — pets, children, and people created just for relationships/connections show in the contacts list which makes it less useful as a Contacts list. We can should filter down to Contacts with at least one email, phone number, or URL/link and have a toggle to show all."* |

## Landing note (2026-08-13)

Implemented as specified. What shipped:

- **Backend** (`backend/controllers/contact_controller.go`): `?has_contact_info=true|false` on
  `GET /contacts`, applied as a `query.Where(contactInfoClause)` beside the `circle` clause. The
  clause reads the flat `email`/`phone` scalars (trimmed, non-empty, `COALESCE`d against NULL — see
  the review-pass note below) *and* `json_each` over the `emails`/`phones`/`urls` arrays — the
  arrays cover the contactable case a scalar-only path would miss (a contact whose only email
  arrived without populating the flat column). Any value other than `true`/`false` is a 400,
  matching `sort`'s treatment. The `?vcard_uid=` batch path returns before the parse and never sees
  the param; the `?since=` feed returns after but ignores the flag — a feed is sync state and must
  carry every row (a malformed value still 400s there, since the parse is shared).
- **`hidden_count`**: present only while the filter is active — how many contacts matched the other
  filters (archive/search/circle) but were excluded by the predicate. Counted on a `Session()` clone
  of the query *before* the resume cursor, so it reflects the whole filtered set, not the page. The
  browse path was restructured (filters → count → cursor) to make that possible; WHERE clauses are
  ANDed so the reordering is semantically inert. Web renders "N contacts without contact info are
  hidden" so the default-on filter never reads as silently lost data.
- **Web** (`ContactsPage.tsx`): a "Show all" `Switch` beside the archived toggle. The filter defaults
  on (`showAll` is the URL's `has_contact_info` absent-or-`true` = filtered, `false` = all); the
  toggle writes the param via T77's `setSearchParams` pattern, so the choice survives reload and a
  shared link. `has_contact_info` is threaded through `GetContactsParams`/`getContacts`/`useContacts`
  and was added to the selection-clearing dependency array (the ticket's trap). Six new strings
  (`showAll` + `hiddenContactable` `_one`/`_other`) in all five locales.
- **Query-plan measurement** (the ticket's trap, with numbers): against a 20k-row real migrated
  schema, the predicate adds ~120µs to the page query (453µs vs 336µs baseline) and the hidden-count
  `COUNT(*)` is ~18ms. The denormalized `has_contact_info` column escape hatch was deliberately **not**
  reached for — the measurement is the reason it isn't needed.
- **E2E fallout**: seven existing specs search for / create contacts with no contact info and expected
  them on the list, which the new default hides. They now pass `has_contact_info=false` explicitly
  (bulkOperations, contactSortControl, importExport's circle-chips check, t79's apartment search,
  search.spec's FTS/address/soft-delete tests), and the `/search?q=` redirect spec gained a
  contactable fixture since the redirect target can't carry the param. Two of those assertions
  (`search.spec`'s surname-match ones) had been passing **vacuously** via the "No results for …" line
  once the card was hidden — worth remembering: `getByText(new RegExp(searchTerm))` matches the
  no-results message, so a hidden contact is not a failed assertion.

**Not done / deferred**: the web's list still shows the filter switch only; the Android client's
contacts list has no `has_contact_info` surface (out of scope — this ticket is Backend + Web, the
M-series has its own tickets).

### Review pass (2026-08-13)

An independent review of the branch found one real bug and several untested branches; all fixed.

- **NULL-scalar `hidden_count` undercount (fixed)**: the flat `email`/`phone` columns are nullable
  (raw-SQL/legacy rows can store NULL where GORM writes `''`), and the original clause's
  `length(trim(email)) > 0` evaluates to NULL for a NULL column. In the visible-list predicate that
  correctly excludes the row, but the hidden-count query's `NOT (clause)` turned that NULL into NULL
  too — the row vanished from the list *without* being counted as hidden, so the "N contacts hidden"
  disclosure under-reported. Fixed by `COALESCE`ing both scalars to `''`; the regression test
  (`TestGetContacts_ContactInfoFilterNullScalars`, which inserts a NULL-scalar row via raw SQL)
  fails pre-fix, verified.
- **Branches that were untested and now are**: cursor pagination over the filtered set
  (`TestGetContacts_ContactInfoFilterPagesFilteredSet` — walks pages, asserts every contactable row
  exactly once, stubs never, and that each page reports the whole-set `hidden_count`, not the
  per-page count); `sort=name` + filter (`TestGetContacts_ContactInfoFilterNameSort`);
  `includes=notes` + filter, i.e. the `ContactSummaryWithRelations` response branch
  (`TestGetContacts_ContactInfoFilterIncludesRelations`); and explicit `has_contact_info=false`
  returning every row with no `hidden_count` (extended the off-by-default test). The pagination and
  includes assertions were hand-verified to fail when their code path is broken.
- **E2E additions**: two more Playwright specs — a fresh load of a shared `?has_contact_info=false`
  link reproduces Show-all with no toggle ever touched, and toggling Show-all clears an in-progress
  bulk selection (the ticket's trap, previously unit-test-only).
- Full gates re-run green: backend build/vet/gofmt/test, `tsc` + 673 vitest, 174 Playwright (4 of
  them this spec). One pre-existing flaky vitest run (`LifeEventDialog.test.tsx` fetches the real
  `localhost:8080`, unmocked) was observed and is unrelated to this change.

## Why this exists

The graph features work by creating contact rows for entities that are not people you contact: pets
([T37](46-T37-pet-relationship-kind-default.md) gives them an animal `CRM.Kind`), children, and
relationship-only stubs created to hang a `RelationshipEdge` off. `Household` members and
[T40](49-T40-household-suggestions-shared-address.md)'s address-derived suggestions add more. Every one of
them lands in the same flat list as the people you actually phone.

On real production data that has made `/contacts` a graph dump rather than an address book.

The list endpoint already composes several filters — `GetContacts`
(`backend/controllers/contact_controller.go:162-448`) handles `sort` (`:222`), cursor/limit/since (`:229`),
`include_archived`/`archived` (`:319-320`, applied `:332-339`), `search` (`:364-366`) and `circle`
(`:368-374`) — so a "has contact info" predicate slots in as one more `query.Where(...)` beside `circle`.

The columns it needs already exist and are already maintained by `Contact.BeforeSave`: the flat `email` and
`phone` scalars, `phones_normalized` ([T69](113-T69-phone-search-tokenization.md)), and the JSON `emails`,
`phones` and `urls` arrays. `applyContactSearch:110-116` shows the working `json_each(contacts.emails)`
shape to copy.

## What to build

### Backend

1. A `has_contact_info` boolean query param on `GET /contacts`, applied as a `query.Where(...)` beside the
   `circle` clause at `:368`. The predicate is **at least one of**: non-empty flat `email`, non-empty flat
   `phone`, a non-empty entry in `json_each(contacts.emails)`, a non-empty entry in
   `json_each(contacts.phones)`, or a non-empty entry in `json_each(contacts.urls)`.
   - Reading the flat scalars *and* the JSON arrays is deliberate: the flat columns are derived from
     `[0]` of each array, so they cover the common case cheaply, but a contact whose only email arrived
     via a path that didn't populate the scalar must still count.
2. An unknown or malformed value is a 400, matching how `sort` is handled at `:222` — not a silent
   fallback.
3. `backend/openapi.yaml` gains the param (the drift test enforces it).

### Web

4. A "Show all" `Switch` in the `ContactsPage` filter row (`frontend/src/ContactsPage.tsx:286-343`), beside
   the existing archived toggle at `:315-325`.
5. **The filter defaults on** — the list shows only contactable contacts on first load, and the switch
   turns it off. Persist the choice in the **URL** as a search param, via the same `setSearchParams`
   pattern [T77](121-T77-web-contacts-list-sort-control.md) established at `:128-136`, so a shared or
   bookmarked link reproduces what the sender saw.
6. Thread it through `GetContactsParams` (`frontend/src/api/contacts.ts:714-727`), `getContacts`
   (`:729-762`) and `useContacts`, and add it to the `contactParams` memo at `ContactsPage.tsx:142-149`.
7. Show a count of what's hidden — "showing 340 of 512" or an explicit "N contacts without contact info are
   hidden" line — so the default is discoverable rather than mysterious. A user who imported 500 contacts
   and sees 340 must be able to tell that nothing was lost.
8. New strings translated in all five locale files (`/CLAUDE.md` frontend trap #5).

## Traps

- **Add the new filter to the selection-clearing dependency array** at `ContactsPage.tsx:109-111`, which
  currently lists `[searchQuery, selectedCircle, showArchived]`. Without it a bulk selection made before
  toggling can act on contacts that are no longer visible — an archive or delete applied to rows the user
  can't see.
- **This changes what the app shows by default on real production data.** Nothing is deleted and the toggle
  is one click, but it must be obvious — item 7 is not optional polish.
- The `circle` filter is *not* URL-persisted today (`selectedCircle`, `:47`) and neither is `showArchived`
  (`:93`). Do not "fix" those here; this ticket adds one param, consistently with T77's precedent.
- The two `json_each` scans plus the flat comparisons run on every list request. Check the query plan on a
  realistic row count before landing — if it degrades, a denormalized `has_contact_info` column maintained
  by `BeforeSave` (the same technique as `sort_name` and `phones_normalized`) is the escape hatch, but that
  needs a migration and a backfill on real data, so only reach for it with a measurement in hand.
- `/CLAUDE.md` backend trap #1: test against `database.InitDB`, not `AutoMigrate`.

## Done when

- A fresh load of `/contacts` shows only contacts with at least one email, phone or URL.
- The "Show all" toggle reveals the rest, and the choice survives a reload and a shared link.
- A contact whose only email lives in the `emails` JSON array but not the flat `email` column is treated as
  contactable.
- A pet or relationship-stub with no contact fields is hidden by default and visible with the toggle on.
- The hidden count is shown when the filter is active.
- Toggling the filter clears any bulk selection.
- New strings translated in all five locales.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green, plus a Playwright spec covering the default,
  the toggle, and the URL round trip.
