# T86 — Fold the Search page into Contacts: one search field, one list

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — IA cleanup, not a missing capability |
| **Size** | M — one page grows a field and a section, one page is deleted, three surfaces collapse to one |
| **Depends on** | [T85](129-T85-contacts-list-fts-search.md) — the merged field is only worth building on the FTS-backed `search=` param. Blocked until it lands. |
| **Interacts with** | [T77](121-T77-web-contacts-list-sort-control.md) — same filter row, same URL-param question. See Sequencing. |
| **Status** | BLOCKED on [T85](129-T85-contacts-list-fts-search.md) |
| **Source** | Testing notes, 2026-08-11: "no real UX benefit to separating contacts-browsing from search as distinct activities." Sat in Deferred → Feature ideas until the 2026-08-12 design pass; ranked here now that its decisions are made. |

## Why this exists

Web has **three** search surfaces, not two:

| Surface | Backend | What it can do |
|---|---|---|
| AppBar `Autocomplete` ([App.tsx:106](../../../frontend/src/App.tsx)) | `GET /contacts?search=` | jump-to-contact dropdown; Enter navigates to `/search?q=` |
| [`SearchPage.tsx`](../../../frontend/src/SearchPage.tsx) | `GET /search` | grouped contacts/notes/activities, snippets, `resolved_relation`; no pagination |
| [`ContactsPage.tsx:65`](../../../frontend/src/ContactsPage.tsx) | `GET /contacts?search=` | circle filter, archived toggle, bulk select, cursor pagination, load-more, T77 sort |

Two of them are the same query rendered differently, and the third is a whole page reachable from a
nav entry. The Contacts page can already display a search result — it reads `?search=` from the URL
([ContactsPage.tsx:36-37](../../../frontend/src/ContactsPage.tsx)) — but **nothing on the page can
write it**. The only way to fill it is the AppBar box, and the only feedback is an undeletable-looking
chip ([ContactsPage.tsx:216-233](../../../frontend/src/ContactsPage.tsx)). That is the actual defect:
Contacts is already a search-results page with no search field.

## What to build

### 1. A search field on Contacts, owning the `?search=` param

Full-width `TextField` on **its own row above** the existing filter row, not inside it — that row
already overflows on phone widths ([T71](115-T71-mobile-circles-tags-add-row-overflow.md)) with four
controls in it. Search icon start-adornment, clear-button end-adornment; copy `SearchPage.tsx:73-96`'s
shape before deleting it.

- **Search-as-you-type, 300ms debounce, two-character minimum** — matching both `SearchPage`'s
  existing debounce ([SearchPage.tsx:54-63](../../../frontend/src/SearchPage.tsx)) and the backend's
  own two-rune gate. Below two characters the field is inert and the list shows its normal unfiltered
  default; **an empty query is the full contact list**, so Contacts keeps its browse identity.
- The field **owns `?search=`** in the URL, seeded from it at mount. `SearchPage` already solved the
  "did the URL change because we wrote it, or because something else navigated here" problem with
  `ownWriteRef` ([SearchPage.tsx:38-52](../../../frontend/src/SearchPage.tsx)) — that comment
  explains a real bug; port the mechanism, don't rediscover it.
- Drop the search chip from the chip row. A visible field showing the query makes a second chip
  echoing it redundant; the circle chip stays.

### 2. Notes and activity hits, below the list

Fire `GET /search` in parallel with the list query and render **only its `notes` and `activities`
groups** in a collapsible section *below* the contact cards, headed with a count
("N matches in notes and activities"), collapsed by default.

- **Discard `/search`'s `contacts` group.** Both endpoints return contacts; rendering both would show
  two disagreeing contact lists on one page. The list endpoint is the authority for contacts here —
  it is the one with the filters, the sort and the cursor.
- Keep the `resolved_relation` info line ([SearchPage.tsx:103-107](../../../frontend/src/SearchPage.tsx))
  — it is the only visible proof T11's synonym half works, and it has no other consumer.
- **Say that this section ignores the filters.** Circle and archived have no meaning for a note hit,
  so a filtered contact list next to an unfiltered notes section is inconsistent unless labelled.
  One line of secondary text in the section header; do not silently apply or silently ignore.

### 3. Collapse the other two surfaces

- **`/search` route redirects** to `/contacts?search=<q>`, preserving the query. Old bookmarks and
  the e2e suite's deep links keep working. `?search=` is canonical; `?q=` survives only as the thing
  the redirect reads.
- **Delete** `SearchPage.tsx`, `SearchPage.test.tsx`, and the `/search` nav entry
  ([App.tsx:148](../../../frontend/src/App.tsx)) and lazy route ([App.tsx:489](../../../frontend/src/App.tsx)).
  `hooks/useSearch.ts` and `api/search.ts` **stay** — the notes/activities section is their new
  consumer. Also update `PRIMARY_NAV_PATHS` ([App.tsx:70](../../../frontend/src/App.tsx)), which
  lists `/search`.
- **The AppBar box stays**, retargeted. It answers a different question — "take me to this person"
  from anywhere — rather than "filter my list", and its `Autocomplete` navigates straight to a
  contact. Change only `handleSearchSubmit` ([App.tsx:134-139](../../../frontend/src/App.tsx)) to
  navigate to `/contacts?search=` instead of `/search?q=`. Its clear handler already has a
  `/contacts`-specific branch ([App.tsx:395](../../../frontend/src/App.tsx)) — re-check it against
  the new param ownership.

### 4. i18n

The `search.*` namespace is `title, placeholder, clear, contacts, notes, activities, noResults,
unfiled, resolvedRelation, minLengthHint`. `title` and `contacts` become dead; the rest move to
whatever namespace the merged page uses. All five locales, real translations — `locales.test.ts`
enforces key-set parity in both directions, so a key removed from `en` must be removed from four
others.

## Traps

- **The selection-clearing effect must key off the debounced value, not the raw input.**
  [ContactsPage.tsx:58-60](../../../frontend/src/ContactsPage.tsx) clears `selectedUids` whenever
  `searchQuery` changes — deliberate, so a bulk delete can't run against contacts the user can no
  longer see. With as-you-type search, wiring it to the raw input clears the user's selection on
  every keystroke.
- **Changing the query must reset the cursor**, not append. Same failure mode T77 documents for
  sort: the cursor is opaque and query-specific, so a new query has to start a fresh list rather
  than append a differently-filtered page onto the old one.
- **Two requests per query, one loading state.** The list query and the `/search` query resolve
  independently; don't block the contact cards on the notes call, and don't let a `/search` failure
  render an error over a contact list that loaded fine.
- **`e2e/search.spec.ts` is nine `page.goto('/search?q=…')` navigations** plus three direct API
  calls. The API calls (lines 265, 275, 282) stay valid untouched; the page navigations must be
  rewritten against the Contacts page. `dataSettingsImport.spec.ts:46` and
  `relationshipEdges.spec.ts:150` also hit `/search` — both are **API** calls, unaffected.

## Sequencing with T77

Both tickets add a control to the same filter row and both need view state in the URL. T77 is S and
already unblocked; land it **first** and this ticket inherits its URL-param pattern rather than
inventing a second one. If they land in the other order, T86 owns the pattern and T77 conforms —
what must not happen is two different persistence mechanisms in one filter row.

## Done when

- Contacts has a working search field; typing filters the list; clearing restores the full list.
- Search composes with the circle filter, the archived toggle and T77's sort in one request —
  hand-verified in the browser, not just unit-tested.
- Load-more pages correctly through a searched result set, no duplicates or skips, against a dataset
  larger than one page.
- A note-body-only match is still findable from the merged page (T11's "otherwise the feature is
  invisible" requirement survives the fold), and a relation synonym still surfaces its
  `resolved_relation` line.
- `/search?q=x` redirects to the Contacts page with the query applied.
- Selecting contacts, then typing, does not leave a stale selection.
- New/moved strings translated in all five locales.
- `cd frontend && npx tsc --noEmit && npx vitest run` green; `search.spec.ts` rewritten and passing.
