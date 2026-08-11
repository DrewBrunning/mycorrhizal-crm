# M20 — Reminders depth on Android

| | |
|---|---|
| **Rating** | 4 |
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
