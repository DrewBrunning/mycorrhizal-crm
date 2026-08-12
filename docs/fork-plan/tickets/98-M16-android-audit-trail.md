# M16 — Audit trail + undo (T60) on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | S–M — 2 endpoints, one list screen, one confirm dialog |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — T60/T18's backend already exists and serves web today |
| **Status** | TO BE DONE |

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
