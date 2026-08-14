# M9 — Wire up already-built Android screens & dead code

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — cheap, high value; every item here is already built and tested, just unreachable |
| **Size** | M — 2 new client methods, but four unrelated wirings (routes, bulk actions, pagination, VCF confirm) |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 — "wire it up" bucket |
| **Depends on** | Nothing |
| **Status** | IMPLEMENTED, AWAITING ON-DEVICE VERIFICATION (2026-08-13) |

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

---

## Landing note (2026-08-13)

All four items shipped. `./gradlew testDebugUnitTest`, `lintDebug`, `assembleDebug` all green
project-wide. No device/emulator was available in the build environment, so the on-device
hand-verify step this ticket calls for is still outstanding — see the README status line.

**Scope correction, found before implementing item 4**: re-checking the M8 sign-off (per this
ticket's own "Correction while implementing" note) found VCF upload squarely inside the exclusion
— M8's Pass 3 explicitly says "this exclusion is specifically the upload-a-file path," which VCF
upload is. Flagged to the user; **they chose to build it anyway**, overriding the exclusion for the
VCF-only path specifically (CSV file import stays excluded, unchanged).

**Item 1 — global Notes/Activities drawer routes.** Built as real inboxes, not the ticket's
"at minimum" contact-picker fallback — the endpoint contract's `listNotes` requirement already
implied this. Two new screens (`NotesInboxScreen`/`ActivitiesInboxScreen`, `feature/timeline`),
each with its own ViewModel and repository method (`NoteRepository.listUnfiled`,
`ActivityRepository.listAll`). Both reuse the *existing* per-contact edit routes
(`contacts/{contactId}/notes|activities/{id}/edit`) with a contact id of `0` (never a real id) to
edit an already-unfiled note or an activity from the inbox — `NoteFormViewModel`/
`ActivityFormViewModel` only read that route argument for a brand-new record, never on edit, so
this needed zero new form code. **Deliberately not built**: creating a *new* unfiled note/activity
from either inbox — that needs a contact-less `POST /notes`/`POST /activities` path neither of
which the ticket's contract asked for (only `listNotes` was new); no FAB on either screen. Also
fixed in passing: `ActivitiesPage.activities` (and the new `NotesPage.notes`) were/are nullable
raw fields with a normalizing getter (mirrors `FieldDefinitionsResponse.definitions`'s existing
fix for the same `/CLAUDE.md` trap #8) — `GetActivities`/`GetUnassignedNotes` marshal a nil Go
slice as JSON `null` for a zero-row result, which would have crashed Moshi on first real use;
`listActivities()` had never had a UI caller before this ticket to hit it.

**Item 2 — bulk circle/tag actions.** `BulkOperationsViewModel.run()` already accepted
`circleId`/`tagId` correctly (including clearing selection only on success) — this was UI-only:
picker dialogs added to `BulkOperationsScreen`, `CircleRepository`/`TagRepository` injected to
load the picker lists. `BulkOperationsScreen` split into a stateless `BulkOperationsScreenContent`
(mirrors `ContactListScreenContent`) so the picker flow is directly testable.

**Item 3 — contact-list pagination.** Scroll-triggered, per the ticket's wording — a
`derivedStateOf` on the `LazyListState` fires `loadNextPage()` within 5 rows of the list's end.

**Item 4 — VCF import.** New `feature/import` screen (`VcfImportScreen`/`VcfImportViewModel`,
pick → preview → confirm), one new `ApiClient` method (`confirmVcfImport` — `uploadVcfImport` was
already wired to the right endpoint, only the confirm half was missing). A row with validation
errors is forced to "skip" and its action picker disabled, matching web's
`ImportContactsDialog.tsx` intent. Reachable from `ImportContactsScreen` via a new
"Import from a VCF file" button alongside the existing device-contacts import.

**Test coverage**: every new ViewModel/repository method has a unit test; every new/changed screen
has a Compose test proving it renders real content (not a placeholder) and its callbacks fire —
`BulkOperationsScreenTest`, `NotesInboxScreenTest`, `ActivitiesInboxScreenTest`,
`VcfImportScreenTest`. `ApiClientTest` covers `listNotes` (including the trap-#8 null-tolerance
case) and `confirmVcfImport`. Every new test was hand-verified per `/CLAUDE.md` (break the wiring,
confirm the test fails, restore) during implementation.

### Review pass (same day, before merge)

A review of the branch found two defects in the code it had just added; both fixed, both now
pinned by tests.

1. **Item 3's pagination never fired on a real device.** `ContactListScreenContent` used an
   unkeyed ``remember { derivedStateOf { … uiState.contacts.lastIndex … } }``. `uiState` is a
   plain parameter, not a `State`, so the lambda captured the *first* composition's `uiState`
   permanently — and the first composition is always the ViewModel's empty initial state, pinning
   `lastIndex` at `-1` and leaving the trigger dead forever. Only `listState.layoutInfo` is a real
   `State` and re-triggers the block on its own. Fixed with `remember(lastContactIndex)`.

   **The test-shape lesson is the reusable part**: the original test composed an
   already-populated list, which never happens in the real app (the ViewModel always starts
   empty). Any Compose test for state-dependent behaviour should compose the *initial* state and
   then deliver data, not start from the end state — otherwise it passes against wiring that is
   dead in production. This is a sibling of the trap the ticket already warned about for
   pagination ("a test that only asserts 'more items appeared' would pass against a re-fetch of
   page 1"): same failure mode, different axis. Worth a `/CLAUDE.md` trap entry if an Android
   traps section ever gets started.

2. **VCF import read the whole file before the size guard.** The picker is launched with
   `GetContent("*/*")` (vCard MIME types are unreliable across providers), so a user could pick a
   multi-gigabyte file; the 50MB check ran only *after* `readBytes()` had already allocated it,
   OOM-ing the app before the guard. Now probes `OpenableColumns.SIZE` and rejects without
   reading; the post-read check stays as the backstop for providers that declare no size.

Also checked and found clean: locale key/placeholder parity across all five files (325 keys,
identical sets); the `contacts/0/...` edit-route reuse for both notes and activities (`GET
/activities/{id}` does preload `Contacts`, so `ActivityFormViewModel` round-trips participants
rather than stripping them — verified against the controller, not the client's doc comment).

One thing deliberately left alone: `CachedNote` has no `contact_id`, so `listUnfiled` and
`listForContact` now write both unfiled and contact-scoped notes into one table with no way to
tell them apart. Harmless today — nothing reads `getAll()` in production, the mirror is
write-only — but a future offline-notes reader will need a column and therefore a migration
against real production data. Flagged rather than pre-emptively migrated.

**Not built as a nav-graph test**: `android/app/src` has no test source set at all — a
`MycorrhizalApp`-level Hilt+NavHost harness to directly assert "drawer tap → real screen" would be
new infrastructure, out of proportion to this ticket. The screen-level Compose tests plus this
note's outstanding on-device pass are the substitute; flagged for whoever picks up the on-device
step in case a real nav-graph test is wanted later.

## On-device verification (2026-08-14, Pixel 8a)

All four items confirmed reachable against the real account: the drawer's "Activities" and
"Notes" entries open the real `ActivitiesInboxScreen`/`NotesInboxScreen` (each showing its own
title and an "no … yet" empty state, not the old `PlaceholderScreen`'s "coming soon" copy).
"Bulk operations" shows Archive/Unarchive/Delete/Add-to-circle/Remove-from-circle (and more,
scrollable) instead of just the original three. The contacts list scrolled through several
dozen distinct real contacts continuously with no stall and no repeat of the first page's names
at the bottom of a subsequent scroll — the specific failure mode item 3's own test cases
target. "Import contacts" shows the "Import from a VCF file" entry point above the device
contacts list. Bulk actions and VCF import were not actually executed against real data (no
selection/import performed) to avoid mutating the account beyond this verification pass.
