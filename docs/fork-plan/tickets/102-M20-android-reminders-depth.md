# M20 — Reminders depth on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 |
| **Size** | M — 3 new client methods plus overdue styling and recurrence display |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | DONE |

`RemindersScreen`/`ReminderFormScreen` cover message, recurrence select, mark-complete, edit, and
send-by-email natively. What's missing:

## Scope (mirrors `ReminderDialog.tsx`/`ReminderList.tsx`)

- Delete a reminder, with confirmation (`ReminderList.tsx:56-72,209-230`) — currently no delete
  action anywhere in the Android reminders list or form.
- Auto-computed date-from-recurrence on create (`ReminderDialog.tsx:50-60,81-86`) — Android's date
  field is plain free text with no recurrence-driven default.
- Date field min-date enforcement / real date picker (`ReminderDialog.tsx:167-175`) — Android is
  free-text, regex-validated only on save.
- "Reoccur from completion" checkbox (`ReminderDialog.tsx:177-187`) — the field doesn't exist in
  Android's form at all.
- Overdue visual styling on the list (red border/chip, `ReminderList.tsx:78-83,101-122`).
- By-mail and "flexible" (reoccur-from-completion) badges on the list card
  (`ReminderList.tsx:117-150`) — Android currently shows recurrence text only.

## Done when

- All six items above work and match web's behavior, particularly the two recurrence-related date
  behaviors (auto-compute on create, reoccur-from-completion) since those affect when the next
  reminder actually lands, not just cosmetics.
- Hand-verified on-device: create a recurring reminder both ways, delete one, confirm overdue
  styling appears once a reminder's date passes.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Delete a reminder | `DELETE /reminders/:id` | **No** |
| Per-contact completions | `GET /contacts/:id/reminder-completions` | **No** |
| Undo a completion | `DELETE /reminder-completions/:id` | **No** |
| Complete | `POST /reminders/:id/complete` | Yes (`completeReminder`) |
| Update | `PUT /reminders/:id` | Yes (`updateReminder`) |

Three new client methods.

### Reoccur-from-completion is server behavior

"Reoccur from completion" and "auto-date-from-recurrence" are recurrence semantics the backend owns.
Read what `POST /reminders/:id/complete` returns before implementing anything date-related on-device
— if it returns the next occurrence, the client displays it rather than computing it.

### Test cases

1. **Delete round-trip**, confirmed before firing.
2. **Overdue styling** is driven by the server's due date against now, and a reminder due today is
   **not** styled overdue (the off-by-one worth pinning).
3. **Completion → next occurrence** — completing a recurring reminder shows the next occurrence as
   the server returned it, not a locally computed date.
4. **Undo a completion** restores the reminder to its pre-completion state.

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

**IMPLEMENTED, AWAITING ON-DEVICE VERIFICATION** (2026-08-14). Same gap M19/M11/M17 landed with —
no physical device in this build environment.

Per the ticket's contract:

- **Three new client methods.** `deleteReminder` (`DELETE /reminders/:id`),
  `listContactReminderCompletions` (`GET /contacts/:id/reminder-completions`) and
  `deleteReminderCompletion` (`DELETE /reminder-completions/:id`) are all new `ApiClient` methods
  with MockWebServer tests (success + 404). The completions list response carries the
  `/CLAUDE.md` trap #8 nil-slice shape (`var completions []` + `Find` → JSON `null`, not `[]`), so
  `CompletionsResponse` follows the `ActivitiesPage` nullable-raw normalization pattern — pinned
  by an explicit-JSON-null MockWebServer test that failed against the naive non-null default
  (hand-verified). `ReminderRepositoryImpl.delete` drops the Room cache row only on success; a
  failed delete leaves the reminder in the list (two regression tests pin both directions).
- **Delete, confirmed first** (test case 1). `RemindersScreen` gained a delete icon per row routed
  through an `AlertDialog` with the reminder's message in the prompt; screen tests assert the
  delete callback fires and `RemindersViewModel.delete` round-trips (removes on success, stays +
  error on failure).
- **Overdue styling** (test case 2). `Reminder.isOverdue()` compares only the date part against
  today — a reminder due **today is not overdue**, the off-by-one the contract calls out. Both
  directions pinned by screen tests (yesterday → chip shown, today → no chip).
- **Completion → next occurrence** (test case 3). The recurring-reminder complete path already
  replaced the row with the server-returned rescheduled reminder; a new ViewModel test pins that
  the *date shown is the server's*, not a locally computed one (reverting to keeping the old date
  fails it — hand-verified).
- **Undo a completion** (test case 4). The contact-detail timeline now renders
  `ReminderCompletion` rows as "Reminder completed" with a delete (undo) action. The backend
  `DELETE /reminder-completions/:id` only removes the completion record — it does **not** restore a
  rescheduled reminder — so this mirrors web's timeline behavior (the completion row leaves the
  timeline; the reminder itself stays rescheduled). `ContactDetailViewModel` fetches completions
  independently from the contact record (the record payload carries reminders, not completions),
  never errors the screen on a fetch failure, and reloads the completion list after an undo —
  pinned by ViewModel tests incl. a failure-keeps-the-row case.
- **Form depth** (scope items 2–4). The remind-at field is now a real Material3 `DatePicker`
  dialog with today's min-date (create mode), replacing free text; changing recurrence in create
  mode auto-fills the due date from the recurrence (web's `getDateForRecurrence`: weekly +1w,
  monthly +1m, quarterly +3m, six-months +6m, yearly +1y), and edit mode never overwrites the
  existing date. A "Reschedule from completion date" switch appears for non-`once` recurrences and
  is sent on create/update; the form hydrates the saved value on edit and defaults to true on
  create (web's default). Recurrence dropdown and list labels now show localized strings, not raw
  enum tokens.
- **List badges** (scope item 6). By-mail and "Flexible" (reoccur-from-completion, recurring-only)
  chips mirror `ReminderList.tsx`'s Email/Repeat chips.

New strings ×5 locales, real translations, `LocalesConsistencyTest` green. Gate green: the full
`testDebugUnitTest` suite (all modules), `lintDebug`, `assembleDebug`. Hand-verified per
`/CLAUDE.md` on four axes (due-today-not-overdue, delete-cache-round-trip, server-returned-next-
occurrence, undo-reloads-the-list) — each failed the pinned test when reverted, then passed
restored. **On-device verification still outstanding** — no physical device in this build
environment, same gap M19/M11/M17 landed with.
