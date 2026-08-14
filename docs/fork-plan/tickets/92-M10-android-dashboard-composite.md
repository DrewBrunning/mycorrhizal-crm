# M10 — Android Dashboard: actually consume the M3 composite endpoint

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 |
| **Size** | M — 1 new endpoint, plus 2 missing widgets and reminder complete/skip |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M3](82-M3-dashboard-overview-endpoint.md) (done — the composite endpoint Android should be calling) |
| **Status** | **DONE** (2026-08-14) |

M3 shipped `GET /dashboard` specifically so both web and Android could drop their N-request
fan-outs. Web was rewired onto it (M3's landing note). **Android never was.**
`DashboardViewModel.kt:35-44` still independently calls `apiClient.listUpcomingBirthdays()` and
`apiClient.listUpcomingReminders()` — two legacy endpoints, and only two of the composite's four
widgets.

## Scope

1. Replace `DashboardViewModel`'s two legacy calls with one call to the M3 composite.
2. **Random Contacts / "Stay in Touch" widget** — fully absent from `DashboardUiState`. Add it.
3. **Overdue Cadences widget** — fully absent from the UI, despite
   `ApiClient.listOverdueCadences()` (`ApiClient.kt:270-272`) already existing and simply never
   being called. Wire it into the composite response instead once item 1 lands (the composite
   already aggregates this server-side per M3).
4. **Birthdays widget gaps**: no age calculation, no "today" highlight styling, cards aren't
   clickable through to the contact (`DashboardScreen.kt:76-96`).
5. **Reminders widget gaps**: no overdue styling, no recurrence/by-mail chips, cards aren't
   clickable through to the contact, and there's no complete/skip action at all
   (`DashboardScreen.kt:97-116` vs. web's `DashboardPage.tsx:90-113,386-431`).
6. Info popovers explaining each widget (web has them; cosmetic, low priority — do last if at all).

## Done when

- One network call for the whole dashboard, matching web's request-count win from M3.
- All four M3 widgets present: birthdays, reminders, random contacts, overdue cadences.
- Reminder complete/skip work from the dashboard card, not just from the per-contact reminders
  screen.
- Hand-verified on-device against a real instance with birthdays, reminders, and overdue cadences
  populated.

## Landing note

**Shipped 2026-08-14** (branch `feature/m10-android-dashboard-composite`). The dashboard is
rewired onto the M3 composite, all four widgets render, and reminder complete/skip work from the
cards. CI gate (`testDebugUnitTest` + `lintDebug` + `assembleDebug`) green.

What changed:

- **`ApiClient.getDashboard()`** (`core/network`) parses the four-block composite into a new
  `DashboardResponse`/`DashboardReminder` model pair (`core/model`). `DashboardReminder` is a
  flattened mirror of `Reminder` + the embedded `contact_name` (the wire is flat; Moshi can't
  inherit). `random_contacts` reuses `ContactSummary`, `overdue` reuses `OverdueCadence` — their
  wire shapes are unchanged by the composite, so no second model set was invented.
- **`completeReminder` gained `skip: Boolean = false`** rather than a new method, per the ticket's
  resolution — `?skip=true` when set. The existing call sites (`ReminderRepositoryImpl.complete`,
  `ApiClientTest`) are unaffected (default param).
- **`DashboardViewModel`** now injects `AuthRepository` (for `date_format`, same pattern as
  `ContactDetailViewModel`) and calls `getDashboard()` once. `DashboardUiState` gained
  `randomContacts`, `overdueCadences`, `completingId` and `dateFormat`; the reminders list became
  `List<DashboardReminder>`. Complete/skip is **optimistic**: the row leaves the widget immediately
  and is restored at its original index if the call fails (`completingId` guards double-taps).
- **`DashboardScreen`** renders four sections. Deliberate layout choice: the overdue section is
  hidden when clear (web's comment that an all-clear dashboard stays clean), while birthdays,
  reminders and stay-in-touch always render with an empty row when empty — matching web's
  per-column empty cards rather than the old single global `EmptyState`. Cards are tappable through
  to the contact. Birthdays gained age calculation (`YYYY-MM-DD` only; yearless stays yearless)
  and a tertiary/moss border for today. Reminders gained overdue styling (chanterelle border +
  warning chip), recurrence/by-mail chips, and skip (confirmed via `AlertDialog`, matching web's
  `window.confirm`) + complete icon actions. All new strings are in all five locales.
- **Widget info popovers** (scope item 6) were deliberately **skipped** — the ticket marks them
  "cosmetic, low priority — do last if at all", and the "Done when" list doesn't include them.

Tests (all hand-verified to fail against the reintroduced bug, per `/CLAUDE.md`):

- `ApiClientTest`: `getDashboard` parses all four widgets with the embedded `contact_name`
  surviving; `[]` blocks and fully-absent blocks both normalize to empty lists; `completeReminder`
  with `skip=true` sends `?skip=true`.
- `DashboardViewModelTest` (new): one `getDashboard` populates all four widgets and the three
  legacy endpoints are `coVerify(exactly = 0)`'d; empty composite is not an error; fetch failure
  surfaces the error; the optimistic-removal ordering is pinned **inside the MockK `coAnswers`**
  (at the moment the API call executes the row must already be gone — a fetch-then-update
  implementation fails this); a failed complete restores the reminder at its original middle
  position; skip forwards `skip=true`; `dateFormat` reflects the session preference.

**Outstanding:** the ticket's "Hand-verified on-device against a real instance" step was not
possible in this session (no emulator/device). The composite endpoint itself was hand-verified on
web at M3's landing; the Android client needs a live pass with birthdays, reminders and overdue
cadences populated.

### Review pass (2026-08-14, same branch)

A full pass over the implementation found and fixed five real issues plus three test gaps:

1. **A failed complete/skip blanked the whole dashboard.** Complete/skip failures wrote the same
   `error` field a failed *load* writes, and the screen rendered that error instead of the widgets
   — so a single failed action hid the user's entire dashboard. Split into `error` (load failure →
   full-screen error + retry) vs `actionError` (action failure → transient snackbar over the intact
   widgets, cleared by `onActionErrorShown`). Hand-verified both ways: pointing the action at
   `error` fails the restore test AND the new `onActionErrorShown` test.
2. **The random-contact card showed a dead email line.** `fetchRandomContacts` selects only
   `ID/firstname/lastname/nickname/circles/photo_thumbnail`, so `ContactSummary.primaryEmail` is
   never populated on the dashboard — the line could never render. Removed.
3. **Web renders the age in parentheses** (`"(41 years old)"`); the strings lacked them. Fixed in
   all five locales, and the age now guards against a negative value for future-birth-year
   nonsense data (web computes the same raw subtraction but renders it).
4. **Card tap guards.** Birthday and reminder cards navigated to `contacts/0` when the id was the
   0 default; both now guard `> 0` like the overdue row already did. The overdue LazyColumn key is
   now always-String (`policy.id ?: "contact-$contactId"`) so a missing policy id can't collide
   with another row's `contactId == 0`.
5. **`load()` gained PrepViewModel's in-flight guard** so a double-tapped retry can't fire two
   overlapping fetches.

**Test gaps closed:** a new `DashboardScreenTest` (Robolectric, 9 cases) renders the four widgets
from a real `DashboardUiState`, asserts the empty-text states and that an all-clear dashboard hides
the overdue section, pins card-tap navigation for all four row types, and drives complete/skip —
including the confirm-then-skip and cancel paths (the stateless `DashboardContent` was extracted
from `DashboardScreen` for exactly this, following the `PrepViewContent` precedent). The ViewModel
suite gained the action-error split, `onActionErrorShown`, and the retry-in-flight guard tests; the
dashboard parse test now also asserts the reminder `contact_id`. `dashboard_empty` was removed from
all five locales (the old global empty state no longer exists). Hand-verified per `/CLAUDE.md`: the
skip-confirm wiring break fails the skip test, and the action-error split break fails both new
tests. CI gate (`testDebugUnitTest` + `lintDebug` + `assembleDebug`) green, no new lint warnings.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| The composite | `GET /dashboard` | **No.** Add `getDashboard`. |
| Complete a reminder | `POST /reminders/:id/complete` | Yes (`completeReminder`) |

The three calls the dashboard uses today — `listUpcomingBirthdays`, `listOverdueCadences`,
`listUpcomingReminders` — are the legacy fan-out M3 replaced. Removing them from the dashboard path
is part of the ticket.

**Resolved 2026-08-12**: skip is not a separate route — it is
`POST /reminders/:id/complete?skip=true` (`frontend/src/api/reminders.ts:141-149`). So
`completeReminder` gains a `skip: Boolean = false` parameter rather than a new method. Web also
confirms before skipping (`DashboardPage.tsx:102`); match that.

### Test cases

1. **One call, four widgets** — `DashboardViewModel` populates all four widgets from a single
   `getDashboard`, and the legacy three are **not** called. Assert the absence explicitly; otherwise
   a half-migrated dashboard passes.
2. **Empty arrays** — MockWebServer: the response parses when a widget's array is absent *and* when
   it is `[]`. M3 embeds the contact name per reminder; assert that survives parsing.
3. **Complete/skip** — completing a reminder removes it from the widget optimistically and restores
   it if the call fails.

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
