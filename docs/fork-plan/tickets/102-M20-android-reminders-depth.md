# M20 — Reminders depth on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 |
| **Size** | M — 3 new client methods plus overdue styling and recurrence display |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

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
