# M19 — Notes/Activities depth on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 |
| **Size** | M — 3 new client methods plus filters, pagination, and the multi-participant fix |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M9](91-M9-android-wire-up-existing-screens.md) items 1 (global routes) is independent but related — do that first so there's a natural home for search/filter across all contacts, not just per-contact |
| **Status** | DONE |

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

---

## Landing note

**IMPLEMENTED, AWAITING ON-DEVICE VERIFICATION** (2026-08-14). This ticket needed a backend half
that its "Platform: Android" tag didn't advertise: the per-contact `GET /contacts/:id/notes` and
`GET /contacts/:id/activities` endpoints were plain "everything at once" loads with no
search/date/cursor support at all (the global `/notes` + `/activities` endpoints had it; the
per-contact ones didn't). Both now accept `search`/`fromDate`/`toDate`/`cursor`/`limit`/`order` and
return the T17 `next_cursor` envelope, mirroring their global siblings. `?since=` change-feed
handling was deliberately not ported (a contact-scoped feed has no consumer); per-contact search
stays on title/description/location for activities — matching the viewed contact's own name on
their own list would be meaningless. OpenAPI updated for both endpoints.

Client side, per the ticket's contract:

- **Delete round-trips** — `DELETE /notes/:id` + `DELETE /activities/:id` are brand-new `ApiClient`
  methods (the ticket's core finding) with MockWebServer tests (success + 404-failure), and the
  repositories drop the Room cache row only on success (a failed delete leaves the item in the list
  and on-screen — two regression tests pin both directions).
- **Delete is confirmed first** — both list screens route the row's delete icon through an
  `AlertDialog` (M17's rule), with the item's content/title in the prompt. Screen tests assert
  *no* repository call until confirm and that cancel is inert.
- **Search/date-filter/pagination** — both `NotesScreen`/`ActivitiesScreen` gained a shared
  `TimelineFilterRow` (search field + from/to `YYYY-MM-DD` bounds) feeding 300ms-debounced server
  filters, plus a "Load more" button driven by `next_cursor` that appends without duplicating.
  ViewModel tests pin the debounce (superseded queries never reach the repository), the filter
  params on the wire, and the append-on-load-more.
- **Multi-participant fix** — the Kotlin `ActivityInput` already carried `contact_ids` as a list,
  and the ViewModel already preserved a loaded activity's participants; the "silently one
  participant" layer was the *create-mode form UI*: it never exposed a participant picker, so a
  new activity defaulted to the route contact and extras were unreachable, not discarded.
  `ActivityFormScreen` now has a real multi-contact picker (debounced search + removable `InputChip`s),
  seeded with the route contact in create mode; `toInput()` sends the whole set. The ticket's
  "assert the count explicitly" test is `adding participants sends both on save and reloads with
  both` — hand-verified: reverting to `.take(1)` failed it (and the edit-preserves-participants
  test) as expected.
- **Participant chips on the list card** — `ActivityListItem` renders each participant as an
  `AssistChip` navigating to that contact (not opening the activity edit), matching the M9 inbox row.
- **Contact reassignment on note edit** — `NoteFormScreen` gained the "assigned contact" chip
  (removable → back to unfiled) + the same debounced contact-search picker; create mode seeds it
  with the route contact and resolves its name best-effort. `PUT /notes/:id` carries `contact_id`
  so a reassignment round-trips to web; clearing creates/updates an unassigned note via the newly
  wired `POST /notes` (`createUnassignedNote` — the gap M9's inbox had already flagged).

New strings ×5 locales, real translations, `LocalesConsistencyTest` green. Gate green: the full
`testDebugUnitTest` suite (all modules), `lintDebug`, `assembleDebug`. Hand-verified per
`/CLAUDE.md` on three axes (multi-participant discard, delete-confirm-before-call, backend date
filter) — each failed the pinned test when reverted, then passed restored. **On-device
verification still outstanding** — no physical device in this build environment, same gap M11/M17
landed with.

### Review pass (same day)

A follow-up review pass fixed four things the first implementation got wrong, each caught by a new
test rather than by inspection alone:

- **Edit mode silently re-added a participant it couldn't drop.** `toInput()`'s route-contact
  fallback fired in edit mode too: removing every participant then saving re-added the route contact
  (or, from the inbox route with `contactId=0`, sent `null` and changed nothing). The fallback is
  now create-mode-only; edit mode sends `contact_ids: []` when the set is empty, which the backend
  `UpdateActivity` honors as `Association.Replace(nil)`. Pinned by `edit removing all participants
  clears the set`.
- **A note's assigned-contact name always rendered `#0`.** The note API never populates a note's
  nested `contact` (GetNote/UpdateNote/CreateNote have no `Preload("Contact")` — it serializes a
  zero-valued struct), and `toFormState` read the name off that struct. The name is now resolved via
  `ContactRepository.getContact` (best-effort, `#id` fallback) the same way create mode resolves the
  route contact. Pinned by `edit contact name is resolved via the repository, never the empty nested
  contact` — reverting the fix reproduced the `#0`.
- **`createUnassignedNote` double-prepended the placeholder origin** (`$PLACEHOLDER_ORIGIN`
  prefix on a path that `executePost` already prefixes), producing a malformed URL. Its new
  MockWebServer test caught it (the test asserted the wrong path until the code was fixed).
- **Filter-switch races.** A slower in-flight request could land last and overwrite a newer
  filter's results; both list ViewModels now cancel the in-flight load before starting a new one
  (`loadJob`), and `loadMore()` is ignored while a reload is in flight. Pinned by `a superseding
  load cancels the in-flight request so stale results never win` and `load more is ignored while the
  list is reloading`.

Also from the pass: the participant chips gained a trailing Close icon (the removal affordance was
unclear), and the form screen tests were made robust to the Robolectric test viewport (taller
`@Config` qualifiers; the two date-filter text fields are asserted one field per test because a
single test typing into two sibling text fields under Robolectric cross-fires both callbacks).

