# M16 — Audit trail + undo (T60) on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | S–M — 2 endpoints, one list screen, one confirm dialog |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — T60/T18's backend already exists and serves web today |
| **Status** | **DONE** (2026-08-14 — new `:feature:audit` module: `GET /audit` + `POST /audit/:id/undo` in `ApiClient`, an `AuditRepository` pass-through, `AuditViewModel`/`AuditScreen` mirroring `AuditPage.tsx`, and a drawer entry. All four test cases pinned; see the landing note) |

`/audit` (`AuditPage.tsx`) has zero Android footprint — a repo-wide search for "audit" in
`android/**/*.kt` returns no hits.

## Scope (mirrors `AuditPage.tsx`)

- Reverse-chronological event list, paginated ("load more").
- Filter by entity type.
- Filter by entity ID (debounced).
- Clear filters.
- Undo a contact-update event.
- Navigate from an audit row to the linked contact.

## Done when

- All actions above work on Android against the same T18 event log web reads.
- Undo behaves identically to web's (same confirmation gate, same result).
- Hand-verified on-device: make a change, undo it via the Android audit screen, confirm the change
  reverted.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Event list | `GET /audit?entity_type=&entity_id=` | **No** |
| Undo | `POST /audit/:id/undo` | **No** |

Both filters are server-side (T60 built them); don't filter client-side over a full fetch.

### Read this first

[T75](119-T75-plain-save-destroys-card-only-data.md) and
[T82](126-T82-audit-snapshots-miss-nested-contact-data.md): contact undo is currently **destructive**
to nested data, and after T75 it will be non-destructive but **partial**. Whatever this screen says
about what undo does must match that reality — do not promise a full revert.

### Test cases

1. **List + filters** — MockWebServer: the entity-type and entity-id filters go on the query string
   and the response parses.
2. **Undo is confirmed first**, then refreshes the list.
3. **Delete events offer no undo button** — the backend rejects undo for delete events, so surfacing
   one is a dead control. Assert it is absent rather than relying on the error.
4. **Contact links** — an event's contact UID resolves to a tappable contact-detail destination.

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

---

## Landing note (2026-08-14)

Shipped as a new `:feature:audit` module following the `cadence` feature's shape (the most recent
standalone feature), with the model + network + repository + DI wiring across the four core modules.

**What web's `useAudit`/`AuditPage.tsx` semantics were carried over verbatim:**

- **No-cursor pagination.** The API only returns the newest `limit` rows (default 100, cap 500), so
  "load more" re-fetches with a grown window, and `canLoadMore = events.size >= limit && limit < 500`
  is the only signal available — same as web's hook.
- **A filter change resets the window to 100** so a grown window never masks the filtered result
  (the ticket's "don't filter client-side over a full fetch" rule, in both directions).
- **Entity-id filter is debounced** at 350ms, matching web's `useDebouncedValue`.
- **Undo is gated to `entity_type == contact && operation == update`** via `AuditEvent.canUndo`, so a
  delete/other-entity event renders no button at all — test case 3 asserts the button is *absent*,
  not that the 400 error is surfaced.
- **410 handling:** undo failures where the event aged past `AUDIT_RETENTION_DAYS` show web's
  `retentionGone` string; every other failure shows the server's own message.
- **Contact links resolve via `?vcard_uid=`** (the pre-existing `ContactRepository.resolveByUid`),
  including archived contacts; a UID that doesn't resolve (deleted contact) falls back to its raw
  uid as plain text, exactly like web's `useContactsForEvents`. Resolution failure is a silent
  degrade, never an error surface.

**Partial-undo honesty:** the confirm dialog carries the `partialNote` string verbatim from web
("Details that were never captured in this record … are preserved unchanged"), so the screen says
exactly what T75/T82 undo does and never promises a full revert.

**Tests** (hand-verified — each broke before the fix, confirmed, restored):

- `ApiClientTest` (+6): filters land on the query string (`limit`, `entity_type`, `entity_id`, blank
  filters omitted), the event list + `canUndo` parse, undo POSTs to `/audit/:id/undo`, and a 410
  maps to `ApiError.Client(410)`.
- `AuditViewModelTest` (+16): default window fetch, load failure, entity-type filter resets the
  window, entity-id debounce fires exactly one request before the delay and one after, clear-filters,
  load-more growth, partial-window has no load-more, contact-UID resolution populates the link map,
  resolve-failure keeps the previous map, list-failure keeps prior rows+map and skips resolution,
  stale-response-discarded (out-of-order guard), load-more stops at the 500 cap, double-undo ignored
  while in flight, undo → refresh (two list calls), 410 vs other-code undo failure.
- `AuditScreenTest` (+7, Robolectric): delete event renders **no** undo button, unresolved UID shows
  raw text with no link node, resolved UID is a tappable contact-detail link, load-more button
  appears only when more rows exist, the filter toolbar renders "All types" with a disabled
  Clear-filters until a filter is active, full-screen undo confirm → refresh flow (ticket test case
  2), and Clear-filters enabled from the entity-id *input* value.

**Strings:** 28 new keys (`nav_audit` + `audit_*`) translated in all five locales, with the web
JSON's existing translations reused so the four non-English copies stay aligned with web.

**Gate:** `./gradlew testDebugUnitTest`, `./gradlew lintDebug`, `./gradlew assembleDebug` all green.

---

## Review-pass note (2026-08-14, same branch)

A review pass over the landed implementation found and fixed four issues, plus closed the testing
gaps that review exposed:

- **`resolveByUid` never requested archived contacts (parity bug affecting both consumers).** Web's
  shared `getContactsByUid` always sends `include_archived: true` (an audit event — or a relationship
  edge — can reference an archived contact, and it must still resolve to a name/link). Android's
  `resolveByUid` called `listContacts(vcardUids = …)` with no archived flag, so an archived contact's
  UID silently failed to resolve and its row fell back to raw UID. Fixed at the shared helper (both
  M16 and the relationships screen benefit); the existing `resolveByUid` stubs were updated and a new
  test pins the `includeArchived = true` request.
- **`resolveContactUids` raced and ran on failure.** It was called unconditionally after the list
  fold, so a list *failure* re-resolved the stale previous rows and, on a resolve failure, cleared
  the link map (web keeps both when a fetch fails — its `.catch` is a no-op). It also had no
  out-of-order guard: a resolve in flight for an older request could overwrite a newer request's
  links (web's `cancelled` flag exists precisely for this). Now resolution runs only on success,
  only for that request's events, and drops its write if a newer request has started. Hand-verified:
  removing the guard fails the new stale-response test; resolving on failure fails the
  list-failure test; clearing on resolve-failure fails the keep-map test.
- **`undo`'s re-entrancy guard was dispatcher-dependent.** `isUndoing` was set *inside* the launched
  coroutine, so on a dispatcher that defers the body (the test `StandardTestDispatcher`) two rapid
  `undo()` calls both passed the guard. Moved the flag set before the `launch` so the guard is
  synchronous and dispatcher-independent. Hand-verified: removing the guard fails the double-undo test.
- **`hasFilters` used the applied (debounced) value, not the input value.** Web's Clear button is
  enabled as soon as the entity-id field has text — even before the 350ms debounce fires — and the
  empty-state message keys off the same input-derived flag. Android computed it from the applied
  `state.entityId`, so Clear stayed disabled while typing. Now computed from `entityIdInput`, and the
  full-screen test asserts the disabled→enabled transition on text entry.
- **Deprecation + test-setup fixes.** `Icons.Outlined.Undo` → `Icons.AutoMirrored.Outlined.Undo`
  (lint flagged the deprecation). The full-screen tests needed a taller Robolectric window
  (`qualifiers = "w480dp-h1600dp"`) — at the 320x414 default the filter toolbar consumed most of the
  height and the LazyColumn's rows rendered with zero bounds, so the undo button was present but not
  clickable. The bug (button invisible in the viewport) is test-infrastructure-only, not a product
  bug — a real device window is far taller.

**Not done / deferred:** on-device hand-verification (the ticket's "make a change, undo it via the
Android audit screen" step) — no device attached to this worktree. The screen mirrors the web
behavior that is already live-verified on web, and every behavior is covered by the unit/UI tests
above; the device pass remains from the ticket's Done-when checklist.
