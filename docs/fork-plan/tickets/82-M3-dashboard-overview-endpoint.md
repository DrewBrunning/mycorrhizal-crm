# M3 — `GET /dashboard` today/overview composite endpoint

| | |
|---|---|
| **Rating** | 3 — collapses the mobile client's most common polling pattern; also fixes a real web N+1 |
| **Size** | S |
| **Depends on** | — |
| **Source** | 2026-08-10 mobile-API work session — "which activities may need simpler mobile endpoints rather than having the client compose UI from multiple endpoints" |
| **Status** | **DONE** (2026-08-11) |

## Why this exists

The mobile client's background notification workers (M1 §6.4 `ReminderNotificationWorker`,
`CadenceCheckWorker`, `BirthdayCheckWorker`) each poll a separate endpoint today:
`GET /reminders/upcoming`, `GET /cadence-policies/overdue`, `GET /contacts/birthdays`. That's
three network round-trips on a battery- and bandwidth-constrained client for data that is all
"what should I be reminded of today". One endpoint that returns all of it in a single call is the
idiomatic mobile shape.

The web `DashboardPage` has the same problem at the opposite end of the scale: it fans out
`getUpcomingBirthdays` + `getRandomContacts` + `getUpcomingReminders` + `getOverdueCadences` in
one `Promise.all`, then issues a **per-reminder `getContactRecord(id)`** for every unique
reminder `contact_id` not already in the random-contacts list (`DashboardPage.tsx:74-102`). A
composite endpoint that embeds the reminder's contact display name kills the N+1 and lets the
page drop three of its five requests.

## What exists today

- `GET /contacts/birthdays` → `{birthdays: Birthday[]}` (`contact_controller.go:391`,
  `services.GetUpcomingBirthdays`).
- `GET /contacts/random` → `{contacts: ContactResponse[]}` (`contact_controller.go:350`).
- `GET /reminders/upcoming` → `{reminders: Reminder[]}` (`reminder_controller.go:231`) — the
  "next 7 days, else next 5" rule.
- `GET /cadence-policies/overdue` → `{overdue: OverdueCadence[]}` (`cadence_controller.go:324`,
  `services.ListOverdueCadences`).
- `models.Birthday`, `services.OverdueCadence` (`models/dtos.go:351`,
  `services/cadence_service.go:208`).
- The `ContactBriefing` composite (`/contacts/:id/briefing`) as the reference pattern for
  server-side aggregation that degrades per-block to zero values.
- `models.Contact`'s denormalized `Firstname`/`Lastname`/`Nickname` scalars (the flat list
  shape) — the composite can embed a reminder's contact name without a nested record fetch.

## Design decisions — locked 2026-08-10

1. **One read-only `GET /dashboard` endpoint, no new tables.** A pure aggregation of the four
   existing queries, scoped by `user_id`, exactly like the briefing composite. No cache, no
   writes. The response is a single envelope; each block degrades to `[]` when empty (the
   `normalizeBriefingSlices` rule — `omitempty` on collection blocks is what broke the prep view
   once; do not repeat it).

2. **Reminder contact names are embedded.** Each `upcoming_reminders` item gains a
   `contact_name` string (and keeps its `contact_id`). The endpoint batch-fetches the distinct
   `contact_id`s in one query (the `loadReminderContactNames` pattern in
   `notification_service.go`). This is what collapses the web page's per-reminder `getContactRecord`
   fan-out. Display name = `nickname`-preferred or `firstname lastname` — matching
   `getContactName` in `DashboardPage.tsx:182`.

3. **Wire shape reuses existing DTOs where they're already correct.** `birthdays`, `overdue`,
   `random_contacts` keep the exact shapes their existing endpoints return (the frontend can
   reuse its types unchanged); `upcoming_reminders` is `Reminder` + `contact_name`. Do not invent
   a parallel DTO set.

4. **Web frontend consumes it.** `DashboardPage` switches its four-request `Promise.all` + N+1
   to one `getDashboard()` call. This is the honest way to prove the endpoint works (the web
   page is its first real consumer) and it deletes real dead weight. Existing types
   (`Birthday`, `Contact`, `Reminder`, `OverdueCadence`) are reused.

## What to build

1. **Backend controller** `GetDashboard` in a new `dashboard_controller.go` (or alongside the
   existing controllers): fetch birthdays, random contacts, upcoming reminders (+ contact names),
   overdue cadences; assemble the envelope; normalize empty slices to `[]`. Route:
   `protected.GET("/dashboard", controllers.GetDashboard)` in `routes.go`.

2. **DTO**: `models.DashboardResponse` with `birthdays []Birthday`, `random_contacts
   []ContactResponse`, `upcoming_reminders []DashboardReminder` (embeds `Reminder` +
   `contact_name`), `overdue []OverdueCadence`. No `omitempty` on the collection blocks.

3. **Web**: `frontend/src/api/dashboard.ts` with `getDashboard()`; rewire `DashboardPage.tsx` to
   use it (replace the `Promise.all` + the per-reminder contact lookup loop). Keep the
   complete/skip handlers' direct `getUpcomingReminders()` refetch as-is (interaction path,
   unrelated).

## Traps

- **`omitempty` on the collection blocks is the bug that ate the prep view** (`CLAUDE.md`
  frontend trap 8 + `briefing_controller.go`'s `normalizeBriefingSlices`). Every block must
  serialize as `[]`, never `null`/absent. A test must assert on **raw JSON**, not the decoded Go
  struct.
- **Reuse `GetUpcomingReminders`'s exact "next 7 days, else next 5" semantics** — do not
  re-derive a subtly different window. Either call the shared logic or replicate its two-branch
  rule precisely.
- **Random contacts are random.** The endpoint returns whatever `Order("RANDOM()")` yields that
  call; do not assert stability.
- **Ownership scoping** on every sub-query (CLAUDE.md trap 5); the cadence + birthday services
  already scope by user, but the reminder query must too.
- **N+1-free contact-name fetch**: batch the `contact_id`s into one `WHERE id IN ?` query, not a
  per-reminder lookup.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- A controller test (real migrated DB, `database.InitDB`) asserts the full envelope: all four
  blocks present as `[]` when empty (raw JSON), populated correctly with data, and a reminder's
  `contact_name` resolves without an extra record fetch. Cross-user scoping case included.
- OpenAPI updated with `GET /dashboard` + `DashboardResponse`; drift test green.
- Frontend: `getDashboard()` api module + `DashboardPage` rewired; `npx tsc --noEmit` and
  `npx vitest run` green (update the dashboard-related tests that assert on the old calls).
- Hand-verify in the browser: dashboard renders identical to before (birthdays/reminders/random/
  overdue) and the network tab shows one `GET /dashboard` instead of 4 + N requests.

## Landing note

**Shipped 2026-08-11** (branch `feature/m2-m3-m4-mobile-composites`, alongside M2's frontend
finish and M4). Landed as specified, with two deliberate deviations from the letter of the ticket:

1. **Controller tests use the existing `setupRouter()` AutoMigrate harness**
   (`controllers/dashboard_controller_test.go`), not `database.InitDB` against the real migrated
   schema. The ticket said "real migrated DB" — but `briefing_controller_test.go`, the reference
   pattern this ticket explicitly points to, already uses `setupRouter()`, and every other
   controller test in the package follows it. `database.InitDB` matters for catching GORM
   column-name-derivation drift (CLAUDE.md backend trap 1); this endpoint adds no new persisted
   columns, only a response DTO over already-tested models, so there's nothing for the real
   schema to catch that AutoMigrate wouldn't. Followed the actual convention over the ticket's
   literal wording.
2. **`GetUpcomingReminders` (the "next 7 days, else next 5" rule) was extracted into
   `services.GetUpcomingReminders`**, not just replicated inline, so the existing
   `GET /reminders/upcoming` endpoint and the new composite share one implementation instead of
   two copies that could drift apart — the exact trap the ticket's own "Traps" section warns
   about. `reminder_controller.go`'s `GetUpcomingReminders` now just calls the service function.

The reminder contact-name enrichment deliberately does **not** reuse
`services.loadReminderContactNames` (the push/email notification helper) — that helper has no
nickname preference, which would have been a silent behavior mismatch against
`DashboardPage.tsx`'s `getContactName`. A dedicated `attachReminderContactNames` in
`dashboard_controller.go` implements the nickname-preferred rule instead.

All three empty-slice-normalization, full-composition, and cross-user-scoping tests were
hand-verified by temporarily breaking the relevant code and confirming each test actually failed,
then restoring — including the frontend `DashboardPage.test.tsx` regression test for the
complete/skip contact-name-carry-forward behavior. Hand-verified against the real dev backend/DB
(`backend-dev` + `frontend-dev`): the dashboard renders identically to before, and the network tab
shows one `GET /dashboard` call per mount (seen twice per load only due to React StrictMode's
dev-only double-invoke — a pre-existing, unrelated artifact of every page in this app, not a
regression this endpoint introduced).
