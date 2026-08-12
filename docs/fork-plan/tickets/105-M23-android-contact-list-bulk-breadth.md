# M23 — Contact list & bulk breadth on Android

| | |
|---|---|
| **Rating** | 3 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M9](91-M9-android-wire-up-existing-screens.md) item 2 (bulk circle/tag wiring) should land first — this ticket's merge/breadth work is unrelated but touches the same screens |
| **Status** | TO BE DONE |

`ContactListScreen`/`BulkOperationsScreen`/`MergeContactsScreen`/`MergeAndBulkScreens.kt` cover
search, add, archive/unarchive, and delete-with-confirm natively. Remaining gaps, per
`ContactsPage.tsx`/`BulkActionsBar.tsx`/`MergeContactsDialog.tsx`:

## Scope

- **Circle filter dropdown** on the main contact list (`ContactsPage.tsx:172-186`) — absent on
  Android entirely.
- **Archived-contacts toggle** (`ContactsPage.tsx:187-197`) — Android never requests
  `includeArchived`; archived contacts have no visibility control.
- **Per-row select on the main list** — web's bulk-select is inline on the contact list itself
  (`ContactsPage.tsx:273-279,234-250`); Android's only exists as a separate `BulkOperationsScreen`
  with its own unpaginated, unfiltered contact fetch. Consider whether to bring select-mode onto
  the main list (matching web's UX) rather than keeping it a fully separate screen — a design call
  worth making explicit before implementing, not a strict requirement to match web's structure
  exactly.
- **"Select all" in bulk mode** (`ContactsPage.tsx:104-125`) — Android requires tapping every
  contact individually.
- **Merge: search-based target picker** (`MergeContactsDialog.tsx:60-94,129-157`) — Android
  requires typing the target contact's raw numeric ID (`MergeAndBulkScreens.kt:82-89`).
- **Merge: full association-count breakdown** — web shows ~11 categories
  (`MergeContactsDialog.tsx:113-119,238-247`); Android shows only notes/activities/edges
  (`MergeAndBulkScreens.kt:96-107`).

## Done when

- Circle filter and archived toggle both work on the main contact list.
- Merge target is picked by search, not typed ID.
- Merge preview shows the same association categories web does (household, circle, tag, life-event,
  field-value, sync-link counts, not just notes/activities/edges).
- Hand-verified on-device: filter by circle, show archived, merge two contacts using search-based
  target selection.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Archived toggle | `GET /contacts?include_archived=` | Yes — `listContacts` already has `includeArchived` |
| Circle filter | `GET /contacts?circle=` | **No** — `listContacts` has no `circle` parameter (`ApiClient.kt:125-136`) |
| Merge preview / commit | `POST /contacts/merge/preview`, `POST /contacts/merge` | Yes |
| Contact search for the merge picker | `GET /contacts?search=` | Yes |

**Resolved 2026-08-12**: no new *methods*, but one new **parameter**. `listContacts` accepts
`cursor`, `limit`, `search` and `includeArchived` — so the archived toggle is already wired at the
client layer and needs only UI, while the circle filter needs `circle` added to the signature and
query string. Merge preview/commit and contact search are all present.

### Test cases

1. **Circle filter** goes on the query string and changing it resets pagination rather than appending
   a filtered page onto an unfiltered list.
2. **Archived toggle** — archived contacts are excluded by default and included when toggled.
3. **Selection is cleared when the filter changes** — web clears it deliberately, because a stale
   selection lets a bulk action (including delete) run against contacts the user can no longer see.
   The same hazard exists here.
4. **Merge by search, not by ID** — the picker resolves a typed name to a contact; assert a merge can
   be initiated without ever entering a numeric ID, which is the reported gap.

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
