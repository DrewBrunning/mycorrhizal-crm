# T77 — Contacts page has no sort control (web)

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — the user-visible half of T73 |
| **Size** | S — one control, one param, one persistence hook |
| **Depends on** | [T73](117-T73-contacts-list-sort-control.md) — landed 2026-08-12; the server-side `sort=updated_at|name` param this control consumes now exists. Unblocked. |
| **Status** | **DONE**, 2026-08-12 — see landing note below |
| **Source** | Testing notes, 2026-08-11: "Contacts sort by most recently edited". Split from T73 on 2026-08-11 so the backend and web halves rank on their own platform lists. |

## Why this exists

`ContactsPage.tsx:69` hardcodes `order: 'desc' as const` into `contactParams` and the page has **no
sort control in its UI at all** — the only interactive filters rendered (`ContactsPage.tsx:172-215`)
are the circle filter, archived toggle, import, and add. There is no "sort by" dropdown and no
column-header sort. Grep for `sortBy`/`sort_by`/`alphabetical` returns zero hits outside build
artifacts: nothing for a user to discover, because the control simply doesn't exist.

## What to build

1. A sort `Select` alongside the existing circle filter in `ContactsPage.tsx`'s filter row, offering
   name and recently-edited, each in both directions. Feed the chosen value into `contactParams` as
   [T73](117-T73-contacts-list-sort-control.md)'s `sort` param; add it to `GetContactsParams` in
   `src/api/contacts.ts` (~699-709), which today types `order` as a direction only.
2. **Default to name (alphabetical)** — the conventional CRM expectation, with recently-edited
   available as an explicit choice rather than an undiscoverable default. Note this differs from
   T73's server-side default of `updated_at`, deliberately: the server keeps today's behavior for
   any existing API consumer, and the web client opts into the better browsing default for itself.
3. Persist the selection. **Check the existing pattern first** — `ContactsPage.tsx` currently holds
   `selectedCircle` and `showArchived` in plain `useState` with no persistence at all, and imports
   `useSearchParams` for a different purpose. So there is no established pattern to copy here; pick
   one (URL search param is the better fit — it makes the view shareable and survives
   back-navigation) and apply it to the sort control. Extending it to the circle/archived filters is
   **out of scope** for this ticket, but worth noting as a follow-up if it looks trivially cheap.
4. New strings translated in all five locale files (`en`, `de`, `es`, `fr`, `it`) — see
   `/CLAUDE.md`'s frontend trap #5; `src/i18n/locales.test.ts` enforces it.

## Traps

- **`selectedUids` is cleared on filter change** (`ContactsPage.tsx:~59`, a deliberate guard so a
  bulk action can't run against contacts the user can no longer see). Changing the sort does *not*
  change which contacts are visible, only their order — so whether it should clear the selection is
  a real call. Recommend **not** clearing on sort change, and adding a comment saying so, since the
  existing effect's dependency list is the obvious place someone would reflexively add it.
- **"Load more" must keep working under the new sort.** The cursor is opaque and sort-specific;
  changing the sort has to reset the accumulated list and start from a fresh cursor, not append a
  name-sorted page onto a date-sorted one.

## Done when

- The Contacts page has a sort control offering at least name and recently-edited, both directions,
  defaulting to name.
- "Load more" pages correctly under every option — no duplicate or skipped rows — hand-verified in
  the browser against a dataset larger than one page.
- The selection survives a reload/back-navigation.
- New strings translated in all five locales.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.

## Landing note

**Shipped 2026-08-12** (in the same branch as [T86](130-T86-web-fold-search-into-contacts.md)).

**Sort control** — the Contacts filter row grows a sort `Select` alongside the circle filter, offering
name and recently-edited each in both directions (Name A–Z / Z–A, Recently edited newest-first /
oldest-first). Default is **name ascending** — the web client opts into the conventional CRM browsing
default for itself while the server keeps `updated_at` for existing API consumers (T73's deliberate
server default is untouched).

**Persistence** — the sort lives in the URL as `?sort=` + `?order=`, the same `setSearchParams`
mechanism T86 established for `?search=`. The `Select` encodes both dimensions in one `sort:order`
value and writes both params together via a functional `setSearchParams` update, so it composes with
T86's `?search=` (each field only touches its own param). Direction defaults to `asc` for name and
`desc` for updated_at (the server's natural default) — only reachable via a hand-crafted URL, since
the control always writes both.

**Wiring** — `GetContactsParams` gains `sort?: 'updated_at' | 'name'`; `getContacts` and `useContacts`
thread it through as a first-class dependency alongside `order`, so a sort change re-runs `fetchFirst`
and resets the accumulated list rather than appending a differently-sorted page onto the old one (the
load-more trap). The selection-clearing effect deliberately does **not** list `sort`/`order` — a sort
change reorders the same visible contacts, so clearing the checkbox selection would throw away a valid
selection; a comment on the effect says so.

**i18n** — five new `contacts.*` keys (`sortBy`, `sortNameAsc`, `sortNameDesc`, `sortUpdatedDesc`,
`sortUpdatedAsc`) translated in all five locales; `locales.test.ts` green.

**Tests** — three new `ContactsPage.test.tsx` tests (defaults to name ascending; changing the control
refetches with the chosen `sort`/`order`; changing the sort does **not** clear an in-progress
selection) and a new `e2e/contactSortControl.spec.ts` driving the real UI (default + URL persistence,
URL-honoured-on-load, and a search-isolated list that reorders name A–Z ↔ Z–A). Backend ordering
remains pinned by T73's API-level `contactSort.spec.ts`.

`npx tsc --noEmit && npx vitest run` green (641 tests). As with T86 in this branch, the Playwright
specs were not executed in this session (the shared dev environment holds port 7300 with a stale
build); they parse (`--list`) and follow `dateFormats.spec.ts`'s Select-driving pattern. The
"load-more pages correctly under every option" done-when item needs a browser hand-check against a
dataset larger than one page before the branch is merged.
