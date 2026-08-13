# T87 — Android parity: fold search into the contact list, retire the placeholder search route

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — delivers M13's capability in the IA web is moving to |
| **Size** | S–M — one `ApiClient` method, one section, one route deletion |
| **Depends on** | [T85](129-T85-contacts-list-fts-search.md) — the embedded list search only becomes FTS-quality when the backend param does. |
| **Supersedes** | [M13](95-M13-android-real-search.md) — same capability, different destination. See below. |
| **Status** | BLOCKED on [T85](129-T85-contacts-list-fts-search.md) |
| **Source** | Filed 2026-08-12 alongside [T86](130-T86-web-fold-search-into-contacts.md), so the Contacts/Search fold doesn't land on web and leave Android on the old IA. |

## Why this supersedes M13

[M13](95-M13-android-real-search.md) specifies building a separate global search screen that mirrors
`SearchPage.tsx`, and says explicitly to *leave the contact-list search bar as-is*.
[T86](130-T86-web-fold-search-into-contacts.md) **deletes `SearchPage.tsx`**. Building M13 as written
would mean Android ships a screen that no longer exists anywhere else, one ticket after web removed it.

The capability M13 exists for is not in dispute — server-side FTS5 across contacts, notes and
activities, with relation-synonym resolution. This ticket delivers exactly that payload into the
contact list instead of into a new screen. M13's endpoint contract, test cases and conventions are
still correct and are carried over below.

Android also gets the easier half of this for free: its list search already calls
`GET /contacts?search=` (`ContactListViewModel.kt:77` → `ApiClient.listContacts`), so
[T85](129-T85-contacts-list-fts-search.md) upgrades its contact matching to FTS with **no Android
change at all**. What remains is the cross-entity half.

## Current state

- `MycorrhizalApp.kt:104` registers a `search` drawer destination; `MycorrhizalApp.kt:453` wraps it in
  `PlaceholderScreen`. It has never done anything.
- `ContactListScreen.kt:154-165` is a real, working search field: online it hits
  `GET /contacts?search=`, offline it falls back to `ContactRepositoryImpl.searchLocal` over the Room
  FTS4 mirror (`CachedContactDao.searchFts`), contacts only.
- `ContactListViewModel.kt:155-162` already debounces via a cancellable `searchJob`.
- `ApiClient` has **no** method for `GET /search` — it is the one endpoint this ticket must add.

## What to build

1. **Add `ApiClient.search(q: String, limit: Int? = null, householdId: String? = null)`** for
   `GET /api/v1/search`, parsing the grouped response (contacts / notes / activities, each with
   `snippet`) plus the top-level `resolved_relation`.
2. **A "matches in notes and activities" section in `ContactListScreen`**, below the contact list,
   collapsed by default with a count in its header. Fired in parallel with the list query from the
   same debounced query, same two-character gate.
   - **Discard the response's `contacts` group.** The list is the authority for contacts — rendering
     both would show two disagreeing contact lists on one screen. Same call T86 makes.
   - Note hits show the contact chip and the "unfiled" state and navigate to the contact; activity
     hits navigate to theirs.
   - Surface `resolved_relation` the way web does — it is the only visible proof the synonym half
     works.
3. **Delete the `search` drawer destination and its placeholder route** (`MycorrhizalApp.kt:104`,
   `:453`). The contact list is the search surface now.
4. **Offline: hide the section, don't error.** This is the Android-only decision web doesn't have.
   `/search` is server-side FTS5 with no local mirror — when the list is being served from cache
   (`ContactListViewModel.kt:99-112`'s fallback path), the cross-entity section has no data source.
   Hide it entirely and leave the cached contact results alone. Do **not** render an error, and do
   **not** route it into `CachedContactDao.searchFts` — that index covers cached *contact* rows only
   ([T76](120-T76-android-local-fts-phone-search.md) covers its own separate bugs; different
   mechanism, don't conflate them).

## Traps

- **Three search mechanisms now, and they are not interchangeable.** Server FTS5 via
  `GET /contacts?search=` (contacts, filterable, paginated), server FTS5 via `GET /search`
  (cross-entity, top-N), and the local Room FTS4 mirror (offline, cached contacts). Route each
  deliberately; the temptation to collapse them in the ViewModel is how the offline path breaks.
- **Android's list has no circle filter or archived toggle** — those are
  [M23](105-M23-android-contact-list-bulk-breadth.md). So "search composes with the filters," a
  headline outcome on web, is partly untestable here until M23 lands. Don't add filters as scope
  creep; do make sure the search state won't obstruct them (keep it in the same `uiState` the list
  params are built from, `ContactListViewModel.kt:130`).
- **One debounce, two requests.** Reuse the existing `searchJob` cancellation rather than adding a
  second timer, or rapid typing fires two independent request streams that resolve out of order.
- **`ApiClient.listContacts` has no `sort` param** (`ApiClient.kt:125-139` carries `cursor`, `limit`,
  `search`, `includeArchived` only). Out of scope here — noted so it isn't mistaken for something
  this ticket broke.

## Test cases

1. **Grouped response parses** — MockWebServer: notes and activities populate with snippets, and
   `resolved_relation` is surfaced for a synonym query ("mom").
2. **Contacts group is discarded** — a response whose `contacts` group is non-empty must not add rows
   to the contact list; the list's own query is the only source.
3. **Two-character gate** — a one-character query issues **no** `/search` request, matching the
   backend's gate rather than relying on it.
4. **Debounce** — rapid typing produces one request per endpoint, not one per keystroke.
5. **Offline** — with the API failing and the cache populated, contact results still render from
   `searchLocal` and the notes/activities section is absent, not errored.
6. **Empty result** — a query matching nothing renders an empty state, not an error.

## Gate

- `./gradlew testDebugUnitTest`, `./gradlew lintDebug`, `./gradlew assembleDebug` — the exact three
  steps `.github/workflows/android-tests.yml` runs.
- Every new user-facing string in all five locales (`values`, `values-de/es/fr/it`).

### Test conventions (this repo, not generic)

JUnit4 + MockK (`mockk`/`coEvery`) + Turbine + `runTest` with `MainDispatcherRule`. ViewModel tests
mock the repository — `feature/contacts/.../ContactListViewModelTest.kt` is the reference. New
`ApiClient` methods get a MockWebServer test in `core/network` — `ApiClientTest.kt` is the reference.
Hand-verify per `/CLAUDE.md`: break the code, confirm the new test fails, restore.

## Done when

- The `search` drawer entry and its placeholder are gone.
- The contact list's search field returns FTS-quality contact matches (free from T85) and surfaces
  note/activity matches for the same query.
- A note-body-only match is findable on the phone, and navigates to the right contact.
- A synonym query shows its resolved relation.
- Airplane-mode search still returns cached contacts with no error and no empty cross-entity section.
- Gate green; five locales.

## Landed 2026-08-12

Implemented as scoped. New `ApiClient.search(q, limit, householdId)` for `GET /search`; a new
`Search.kt` model deliberately has no `contacts` property at all (not just an unused one) — the
response's `contacts` group is discarded structurally, Moshi ignores the JSON key, there's no
Kotlin field it could leak into. `ContactListViewModel` gained `apiClient: ApiClient` as a direct
dependency (mirroring `DashboardViewModel`'s precedent for a composite endpoint with no
single-entity repository home) and a `searchResult: SearchResult?` state field, populated by
`loadCrossEntitySearch` — fired from the *same* debounced `searchJob` as the contact-list fetch
(not a second timer), gated at two characters client-side, and set to `null` (never a distinct
error) on any `/search` failure so the section simply disappears offline rather than showing its
own error surface. That "never errors" property is structural — the function has no code path
that writes `ContactListUiState.error` at all — not something a runtime assertion reliably pins,
since `loadContacts` unconditionally resets `error` at the start of its own next run in the same
tick, which would mask a regression regardless of where the assertion sits (see the doc comment
on `loadCrossEntitySearch`).

`ContactListScreen.kt` gained a `SearchNotesActivitiesSection` — collapsed by default,
re-collapses on every new query (`remember(query)`), shows a resolved-relation banner
independently of hit count, and renders note/activity rows with contact-chip (or "Unfiled")
navigation. Started with `AnimatedVisibility` for the expand/collapse per the design pass; dropped
it for a plain `if (expanded)` after Compose's animation clock made the Robolectric tests
non-deterministic without `mainClock.advanceTimeBy` plumbing — simpler and untested-motion-free
beats chasing that. The `search` drawer destination and its `PlaceholderScreen` route are deleted;
`nav_search` removed from all five locale files as dead weight, matching web's T86 cleanup.

Two real findings, both from following the ticket's own instructions to verify rather than assume:

- **The ticket's third listed gap doesn't exist.** "Passes unsanitized input to MATCH" was already
  fixed by prior hardening work in `ContactRepositoryImpl.searchLocal` (character-stripping +
  boolean-keyword removal + a try/catch LIKE fallback) that landed after this ticket was filed.
  Confirmed by reading the current source before touching it; left as-is.
- **A weak version of the "no cross-entity error" test would pass even with a real regression.**
  Traced through the actual coroutine ordering (see above) before trusting an assertion on the
  shared `error` field — it wasn't reliable, so the test asserts the structural guarantee instead.

Test coverage: `ApiClientTest` (grouped-response parsing incl. snippets/resolved_relation, the
`contacts` group's structural non-leak, query-param omission), `ContactListViewModelTest` (a note
match populating state, the two-character gate firing zero requests, offline hiding the section,
an empty result not being an error, debounce collapsing rapid typing to one request, clearing the
query clearing the result), `ContactListScreenTest` (header count + collapsed-by-default +
expand-on-tap, contact-chip navigation, the unfiled chip, the resolved-relation banner independent
of hit count). Hand-verified per `/CLAUDE.md`: broke the two-character gate, confirmed exactly the
discriminating test failed. Separately, injected an extra `error =` write into the offline branch
to check the "never errors" claim, traced why no assertion caught it (the coroutine-ordering issue
above), and rewrote the test against the structural guarantee instead of the flaky one.

**Hand-verified on a real device** (Pixel 8a) across two installs: app launches cleanly with no
crashes on real production data (400+ unit tests all green beforehand); the drawer's "Search" entry
is confirmed gone (screenshot); the contact list's own `?search=` FTS matching (free from T85)
works correctly against real contacts. Several real search terms ("an", "call", "email") were tried
live but none happened to match this account's actual note/activity content, so the cross-entity
section's *populated* rendering was not directly observed on-device — it is covered instead by the
dedicated `ContactListScreenTest` cases listed above, run against the exact same composable.

Landed via [PR #106](https://github.com/DrewBrunning/mycorrhizal-crm/pull/106).
