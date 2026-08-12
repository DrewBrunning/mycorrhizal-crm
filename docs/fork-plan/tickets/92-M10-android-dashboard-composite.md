# M10 — Android Dashboard: actually consume the M3 composite endpoint

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 |
| **Size** | M — 1 new endpoint, plus 2 missing widgets and reminder complete/skip |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | [M3](82-M3-dashboard-overview-endpoint.md) (done — the composite endpoint Android should be calling) |
| **Status** | TO BE DONE |

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
