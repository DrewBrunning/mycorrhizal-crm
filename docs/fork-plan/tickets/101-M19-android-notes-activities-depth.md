# M19 — Notes/Activities depth on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 |
| **Size** | M — 3 new client methods plus filters, pagination, and the multi-participant fix |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M9](91-M9-android-wire-up-existing-screens.md) items 1 (global routes) is independent but related — do that first so there's a natural home for search/filter across all contacts, not just per-contact |
| **Status** | TO BE DONE |

Per-contact `NotesScreen`/`ActivitiesScreen` are real and functional but materially thinner than
web's `NotesPage.tsx`/`ActivitiesPage.tsx`.

## Scope

**Notes** (`NotesScreen.kt:42-108` vs. `NotesPage.tsx`):
- Search notes by text.
- Filter by from/to date.
- Cursor "load more" pagination.
- Delete a note (currently no delete action anywhere in the Android notes list or form).
- Contact-reassignment field on edit (`EditTimelineItemDialog.tsx:155-225` lets you move a note to a
  different contact; `NoteFormScreen.kt:104-145` has no equivalent).

**Activities** (`ActivitiesScreen.kt:41-107` vs. `ActivitiesPage.tsx`):
- Search activities by text.
- Filter by from/to date.
- Cursor "load more" pagination.
- Delete an activity.
- Multi-contact picker on create/edit (`AddActivityDialog.tsx:181-217` is a proper autocomplete
  multi-select; `ActivityFormViewModel.kt:48-56` silently reuses the single route contact instead —
  this means an activity created/edited on Android can never represent more than one participant,
  which is a real behavior gap, not just missing UI polish).
- Contact chips on the activity list card, navigating to each participant.

## Done when

- All items above work per-contact on Android (global-inbox versions are M9's job if not already
  landed).
- An activity edited on Android to add/remove a participant reflects correctly on web.
- Hand-verified on-device: search, date-filter, paginate, delete, and — for activities — edit
  participants and confirm on web.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Delete a note | `DELETE /notes/:id` | **No** |
| Delete an activity | `DELETE /activities/:id` | **No** |
| Global notes list | `GET /notes` | **No** (shared with [M9](91-M9-android-wire-up-existing-screens.md)) |
| Per-contact notes | `GET /contacts/:id/notes` | Yes (`listContactNotes`) |
| Per-contact activities | `GET /contacts/:id/activities` | Yes (`listContactActivities`) |

**The two deletes being absent from the client is the finding** — this is not just missing UI. Check
the search/date-filter/pagination query parameters the existing list methods accept before adding
new ones; T17's cursor pagination is already server-side.

### Multi-participant activities

The ticket notes activities "silently can't have more than one participant" on Android. `Activity`
supports multiple contacts server-side.

**Localize the gap before fixing it** — it is one of three layers and the fix differs by layer:
the Kotlin `Activity`/`ActivityInput` model (does it hold a list?), the repository mapping, or the
form UI. Start by checking whether `ActivityInput` carries a contact **list** or a single id; that
one field answers it. "Silently" means today's path discards extras rather than erroring, so
whichever layer narrows to one is the culprit.

### Test cases

1. **Delete round-trip** — MockWebServer for both new methods; the item leaves the list on success
   and stays on failure.
2. **Delete is confirmed first** (same rule as [M17](99-M17-android-entity-scaffold-edit-delete-confirm.md)).
3. **Multi-participant** — creating an activity with two participants sends both and reloads with
   both. Assert the count explicitly; a test with one participant cannot detect the discard.
4. **Search/date filter/pagination** — filters go on the query string, and "load more" appends
   without duplicating.

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
