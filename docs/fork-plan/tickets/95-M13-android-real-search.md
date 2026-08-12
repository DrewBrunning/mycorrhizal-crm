# M13 — Real full-text search on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 |
| **Size** | S–M — 1 endpoint, one screen, and a debounce |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — T11's FTS5 `/search` endpoint already exists and serves web today |
| **Status** | TO BE DONE |

`MycorrhizalApp.kt:453` wraps the `"search"` drawer route in `PlaceholderScreen`. The audit found
Android does have *a* search box — embedded in `ContactListScreen.kt:154-165` — but it's a
different, weaker mechanism: it hits `GET /contacts?search=`, a naive SQL `LIKE` scan
(`backend/controllers/contact_controller.go:93-99`), not T11's FTS5 `/search` endpoint that web's
`SearchPage.tsx` uses (relation-synonym resolution — "brother" → `sibling_of` — plus notes and
activities coverage, not just contacts). There's also a third, unrelated local FTS4 mirror
(`CachedContactFts.kt`) that only backs offline fallback for cached contact rows — don't confuse
it with this ticket's scope.

## Scope (mirrors `SearchPage.tsx`)

- Replace the placeholder with a real search screen calling the T11 `/search` endpoint.
- Grouped results: Contacts, Notes (with contact chip click-through and "unfiled" indicator),
  Activities.
- Relation-synonym resolution surfaced the way web shows it (`resolved_relation`,
  `SearchPage.tsx:103-107`).
- No-results / min-query-length hints.
- Leave the existing contact-list-embedded naive search bar as-is — it's a reasonable quick filter
  for a page you're already on; this ticket is about giving the *global* search a real backend.

## Done when

- `search` drawer route hits the same FTS5 endpoint and returns the same grouped, synonym-resolved
  results web does for an identical query.
- Notes/Activities results navigate to the right contact.
- Hand-verified on-device with a synonym query (e.g. a relation term) and confirmed against web's
  result set for the same query on the same account.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Full-text search | `GET /search?q=&limit=` | **No.** Add `search`. |

Optional `household_id` scopes contact hits to one household (T11's household-scoped half).

### The distinction this ticket exists for

The contact-list search bar uses Room's local FTS (`CachedContactDao.searchFts`) — offline, cached
rows, contacts only. This screen is T11's server-side FTS5 across **contacts, notes and activities**
with snippets and relation-synonym resolution. They are different mechanisms with different results;
do not quietly route one into the other.

### Test cases

1. **Grouped response parses** — MockWebServer: contacts, notes and activities each populate, with
   snippets, and `resolved_relation` is surfaced when the query is a relation synonym ("mom").
2. **Two-character gate** — a one-character query issues **no** request, matching the backend's own
   gate rather than relying on it.
3. **Debounce** — rapid typing produces one request, not one per keystroke.
4. **Empty result** — a query matching nothing renders an empty state, not an error.
5. See [T76](120-T76-android-local-fts-phone-search.md) for the phone-number tokenization bug in the
   *local* index — different screen, different bug, don't conflate them.

### Gate

- `./gradlew testDebugUnitTest`, `./gradlew lintDebug`, `./gradlew assembleDebug` — the exact three
  steps `.github/workflows/android-tests.yml` runs. CI has been green since M1's review pass; keep it.
- Every new user-facing string in all five locales (`values`, `values-de/es/fr/it`). M1's review pass
  had to retrofit ~80 unlocalized strings — don't rebuild that debt.

### Test conventions (this repo, not generic)

JUnit4 + MockK (`mockk`/`coEvery`) + Turbine + `runTest` with `MainDispatcherRule`. ViewModel tests
mock the repository — `feature/contacts/.../ContactListViewModelTest.kt` is the reference. New
`ApiClient` methods get a MockWebServer test in `core/network` — `ApiClientTest.kt` is the reference.
Hand-verify per `/CLAUDE.md`: break the code, confirm the new test fails, restore.
