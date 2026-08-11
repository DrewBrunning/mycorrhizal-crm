# M10 — Android Dashboard: actually consume the M3 composite endpoint

| | |
|---|---|
| **Rating** | 4 |
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
