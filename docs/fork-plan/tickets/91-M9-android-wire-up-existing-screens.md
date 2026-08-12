# M9 — Wire up already-built Android screens & dead code

| | |
|---|---|
| **Rating** | 4 — cheap, high value; every item here is already built and tested, just unreachable |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 — "wire it up" bucket |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

M8's audit found four places where the implementation already exists — model, ViewModel,
backend call — and simply isn't wired to a UI entry point. No design work, no new backend calls,
just connecting what's there.

## Scope

1. **Global Notes/Activities drawer routes.** `MycorrhizalApp.kt:454-455` wraps the `"notes"` and
   `"activities"` composables in `PlaceholderScreen`. The real, full `NotesScreen`/`ActivitiesScreen`
   implementations exist and work — they're just wired only under
   `contacts/{contactId}/notes`/`activities`. Point the drawer routes at a
   contact-agnostic entry (an "unfiled + all contacts" view matching web's `NotesPage.tsx`/
   `ActivitiesPage.tsx` inboxes, or at minimum a contact picker into the existing screens) rather
   than a stub.
2. **Bulk circle/tag actions.** `BulkOperationsViewModel.run()` and the `BulkContactOperationInput`/
   `BulkActions` model already support `add_circle`/`remove_circle`/`add_tag`/`remove_tag` — the
   backend call is real. `BulkOperationsScreen`'s UI (`MergeAndBulkScreens.kt:199-212`) only renders
   three buttons: archive/unarchive/delete. Add circle/tag pickers and wire them to the existing
   `run()` calls.
3. **Contact-list pagination past page 1.** `ContactListViewModel.loadNextPage()`
   (`ContactListViewModel.kt:122-153`) is implemented and unit-tested
   (`ContactListViewModelTest.kt:88,97,106-107`) but has no call site anywhere in
   `ContactListScreen.kt`. Wire a scroll-triggered (or button) call so contact lists longer than one
   page are actually reachable — this is a real defect today, not just a missing nicety, on any
   instance with enough contacts to paginate.
4. **VCF upload import.** `ApiClient.uploadVcfImport()` (`ApiClient.kt:601-603`) hits the same
   backend `POST /contacts/import/vcf/upload` endpoint web's VCF import uses, but has zero callers.
   Wire a file-picker entry point (likely alongside the existing device-contacts `ImportContactsScreen`)
   that calls it. Full CSV import + column mapping is **out of scope** here — see
   [M26](109-T65-web-circle-tag-rename-delete.md)'s sibling note; CSV/VCF *file* import was marked
   deliberately not on mobile as part of M8's sign-off. VCF-only is in scope because the client call
   already exists and needs only a picker, unlike CSV which needs a whole mapping UI.

   **Correction while implementing:** re-check the M8 sign-off decision before doing item 4 — CSV/VCF
   file-based import as a *category* was marked deliberately-not-on-mobile. If VCF-only doesn't
   survive that read, drop item 4 and note it in the landing note; the dead `uploadVcfImport()` call
   is a much smaller cleanup either way (remove it, or leave it for a future call site).

## Done when

- All four items reachable from the UI, each pinned by a test that fails if the wiring is removed
  (mirroring the existing unit test for `loadNextPage()`).
- Hand-verified on-device per `/CLAUDE.md`'s workflow section.
- New user-facing strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

`ApiClient` already has 83 methods; diffed against the route table, this ticket needs **one** new one.

| Need | Route | In `ApiClient`? |
|---|---|---|
| Global notes list | `GET /notes` | **No** — only `listContactNotes`. Add `listNotes`. |
| Global activities list | `GET /activities` | Yes (`listActivities`) |
| Bulk circle/tag actions | `POST /contacts/bulk` | Yes (`bulkOperation`) |
| Contact-list pagination | `GET /contacts?cursor=` | Yes (`listContacts`) |
| VCF upload | `POST /contacts/import/vcf/upload` | Yes (`uploadVcfImport`) |

**Resolved 2026-08-12**: `confirmImport` posts to `/contacts/import/confirm` — the **CSV** route
(`ApiClient.kt:611`). The VCF confirm route `/contacts/import/vcf/confirm` has **no client method**,
so the VCF wiring needs `confirmVcfImport` added. That makes **two** new client methods for this
ticket, not one: `listNotes` and `confirmVcfImport`. `uploadVcfImport` already targets
`/contacts/import/vcf/upload` correctly — it is only the confirm half that is missing.

### Surface

Routes live in `app/.../MycorrhizalApp.kt` (the drawer destination list and the `composable(...)`
block). The screens themselves already exist; this is wiring, not construction.

### Test cases

1. **Pagination past page 1** — `ContactListViewModel`: after `loadMore()`, the list contains page 1
   *and* page 2 with no duplicates, and the cursor advances. This is the reported bug; a test that
   only asserts "more items appeared" would pass against a re-fetch of page 1.
2. **Bulk action** — selecting three contacts and adding a circle issues exactly one `bulkOperation`
   call carrying all three UIDs, and clears the selection on success but *not* on failure.
3. **`listNotes`** — MockWebServer: correct path, and the page envelope parses when the notes array
   is **absent** as well as empty (`/CLAUDE.md` frontend trap #8 is a Go `omitempty` problem and
   applies to every client, not just the web one).
4. **Reachability** — each newly wired destination resolves and renders its screen rather than the
   placeholder.

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
