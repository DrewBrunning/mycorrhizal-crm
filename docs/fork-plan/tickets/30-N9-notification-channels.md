# N9 — Notification channels beyond email

| | |
|---|---|
| **Rating** | 3 |
| **Size** | M |
| **Depends on** | — |
| **Alpha** | after |
| **Source** | New (gap found in the 2026-07-30 product review) |

## Why this exists

Reminders are **email-only**, which suits self-hosters poorly — running an SMTP path (or depending on
Resend) just to be told about a birthday is heavy, and many self-hosted setups have no outbound mail at
all. ntfy, Gotify, and web push are the idiomatic answers.

## What exists today

- `services/mailer.go` + `services/email_renderer.go` — templated email with embedded FS templates and
  i18n. `sendViaResend` has no test seam (documented, deliberately excluded from coverage).
- `services/reminder_service.go` → `SendReminders` — the daily job, gated by
  `completed=false AND email_sent=false`, wrapped in the `acquireJobLock`/`releaseJobLock` pattern with a
  `minInterval`. **`email_sent` is the delivery-state field, and it is email-shaped** — that is the main
  design problem this ticket has to solve.
- `services/webhook_service.go` — full delivery infrastructure with HMAC signing, retry with backoff,
  delivery records, SSRF guards enforced in the transport dialer, and a job-locked retry processor. A
  user could already wire ntfy through a webhook crudely.
- `models.Reminder.ByMail *bool` — a per-reminder channel flag already exists, in embryonic form.

## Design decisions — locked 2026-08-04

1. **Delivery state: per-channel delivery records (new `NotificationDelivery` table).** Mirrors
   `WebhookDelivery`'s existing pattern — one row per reminder per channel, independent failure
   tracking. Existing `reminders.email_sent` rows get backfilled into one delivery record each.
   `reminders.email_sent` is deprecated but not removed (existing consumers may read it); new code
   queries `NotificationDelivery` for channel status.

2. **Private-address policy: reuse `WEBHOOK_BLOCK_PRIVATE_URLS`.** Self-hosted ntfy/Gotify instances
   are typically on private addresses. Rather than a notification-specific flag, the existing webhook
   flag controls all outbound HTTP calls including notifications. A self-hosted ntfy user sets
   `WEBHOOK_BLOCK_PRIVATE_URLS=false`. Document this in the Settings UI alongside the ntfy config
   field — don't let a user type a private URL and get a silent failure with no explanation.

## What to build

1. **`NotificationDelivery` model + migration.** Columns: `reminder_id`, `channel` (`email|ntfy|gotify|push`),
   `sent_at` (nullable — NULL means not yet sent), `status` (`pending|sent|failed`), `error` (nullable
   string), timestamps. Backfill migration creates one `sent` delivery row per `reminders` row where
   `email_sent = true`. Hard-delete (join/edge row per T26 — accessory to a reminder, not
   independently meaningful). Add to the reminder's cascade path, not `DeleteContact`/`DeleteUser`
   directly.
2. **A channel abstraction** — an interface with email as the first implementation, so adding ntfy is a
   new implementation rather than a new branch in `SendReminders`. ntfy and Gotify are both trivial
   HTTP POSTs; the work is configuration and delivery bookkeeping, not protocol.
3. **`SendReminders` rewritten to dispatch per-channel.** Query reminders where `completed=false` joined
   against `NotificationDelivery` — a reminder is due for a channel when no delivery row exists with
   `channel = ? AND status = 'sent'`. Failure in one channel must not mark the reminder as sent for
   other channels, and must not block other channels from dispatching. The job lock still prevents
   double-send across instances.
4. **Per-user channel preference**, generalising `ByMail`. New column(s) on `User` as decided during
   implementation — keep `ByMail` as the email-specific toggle for backwards compat, add new columns
   for new channels.
5. **Settings UI** for configuring each channel (ntfy URL + topic, Gotify URL + token) and a "send
   test notification" button per channel — misconfigured notifications fail silently otherwise.

## Traps

- **Reuse the SSRF guard and its config flag.** A user-supplied ntfy/Gotify URL is exactly the attack
  shape `httputil/fetch.go`'s dialer-level protection exists for. Use the existing
  `WEBHOOK_BLOCK_PRIVATE_URLS` flag — no separate notification-specific flag. A self-hosted ntfy user
  sets `WEBHOOK_BLOCK_PRIVATE_URLS=false`. Document this next to the ntfy URL field in Settings so a
  private-address URL doesn't fail silently with no explanation.
- **Do not bypass the job lock.** `SendReminders` is job-locked so a multi-instance deploy does not
  double-send. A new channel must sit inside the same lock.
- Failure in one channel must not block another, and must not mark the reminder as sent.
- Templates are i18n'd per user language (`reminder_service.go` reads `user.Language`); a push
  notification needs its own short-form strings in all five locales, not the email body.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- Tests cover: a reminder dispatches to the configured channel; a channel failure does not mark it sent
  and does not block other channels; the job lock still prevents double-send; a private-address target is
  handled per the documented policy.
- Verified end to end against a real ntfy or Gotify instance, not a mock.

### Ticket-specific
- This extends the existing notification infrastructure — study how `SendReminders` uses `sendReminderEmail`
- The mailer (`services/mailer.go`) already supports Resend and SMTP — new channels (Pushover, ntfy, gotify, Matrix) are additive
- Each channel needs: config in `Config` struct + env vars, a sender function, registration in the notification dispatch
- Webhook as notification channel: reuse `services/webhook_service.go`'s existing delivery infrastructure
- User preference: which channels are enabled per user. New column(s) on `User` model with migration.
- For each channel: test the happy path + error handling (network failure, invalid config, rate limiting)

## Flash implementation notes

### Files to read first
- `/CLAUDE.md` at repo root (conventions, recurring traps, commands)
- Study an existing fully-implemented feature for the pattern: model → controller → routes → api → hooks → dialog → list → page wiring → i18n
- Common pattern references: `circle_controller.go` + test (newer idiom), `api/relationshipEdges.ts` + hook, `RelationshipEdgeDialog.tsx` + test, the `ContactInformation.tsx` tab + `ContactDetailPage.tsx` wiring

### Tests you must write before considering it done
- Backend: controller tests covering CRUD, ownership scoping, error states (not found, cross-user, 409 duplicate)
- Backend: real-DB test (`database.InitDB`, not `AutoMigrate`) for the core round-trip + any migration-dependent behavior
- Frontend: component test (`afterEach(cleanup)`, mock `fetch` with `vi.stubGlobal`) for dialog and list
- Hand-verify EVERY new test: break the code, confirm the test fails, restore. A test that has never failed has proven nothing.

### Self-verification checklist
1. `npx tsc --noEmit` — clean
2. `npx vitest run` — green (ALL tests, not just yours)
3. `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` — green
4. New migrations: run `make migrate-up` to verify they apply cleanly
5. All 5 locale files (`de/es/fr/it/en`) — real translations for any new strings

### Common traps (beyond CLAUDE.md)
- `gorm.Model` only works on uint-PK entities — UUID PK models need explicit `ID`/`CreatedAt`/`UpdatedAt` fields
- Backend tests use `setupRouter()` from `activity_controller_test.go` (sets `db`, `userID`, `cfg` in Gin context, uses AutoMigrate)
- Frontend component tests: `afterEach(cleanup)` mandatory; MUI appends `" *"` to required field `getByLabelText`
- Migration files: hand-written SQL up/down pairs — never add a column by editing the struct alone
- `gorm:"column:xxx"` tag is mandatory for acronyms/compound words — GORM silently derives wrong names
- New entities: decide soft vs hard delete per T26's rule (user-authored content → soft, edge/join rows → hard)
- Delete cascade: add new entities to `deleteContactAssociations` in `contact_controller.go` and `DeleteUser` in `admin_user_controller.go`

## Landing note — 2026-08-06

Shipped on `feature/n9-notification-channels`.

- **Delivery state** now lives in `notification_deliveries` (one row per reminder per
  channel), backfilled from `reminders.email_sent` by migration 000013. `email_sent`
  is kept as a backwards-compat mirror and written in step by the email sender.
- **Channel abstraction**: `services.NotificationSender` interface + the
  `notificationSenders` registry (email / ntfy / gotify / push). `SendReminders`
  dispatches per channel — a reminder is due for a channel when no `sent` delivery
  row exists; a failed channel never marks it sent and never blocks another.
- **Per-user config** is stored Immich-style in `notification_configs` (ntfy
  URL+topic, Gotify URL+token encrypted at rest via `credential_crypto.go`), with
  per-user toggles `users.notify_ntfy/notify_gotify/notify_push`. Email stays gated
  per-reminder on `ByMail`.
- **Web Push** implemented (VAPID keys generated once into `server_settings`,
  `push_subscriptions` per device, `service-worker.ts` push/notificationclick
  handlers). Browser subscription is via Settings' "Enable browser notifications".
- **SSRF**: the existing `WEBHOOK_BLOCK_PRIVATE_URLS` flag + `clientFor`/`isPrivateURL`
  govern all outbound channel HTTP; the Settings card warns client-side on
  private-looking URLs and the test button surfaces the server's reason.
- **API**: `GET/PUT /notifications/config`, `POST /notifications/config/test`,
  `GET/POST/DELETE /notifications/push-subscriptions`.
- Delivery rows are cleared whenever a reminder is deleted or a recurring reminder is
  rescheduled (all reminder-deletion sites + `CompleteReminder`), so the next
  occurrence notifies again.
- Real-instance happy path verified in Go integration tests against a live fake
  ntfy/Gotify/push-service endpoint on the real migrated schema; the Playwright spec
  covers the Settings UI end to end (config persistence, private-URL warning, and a
  misconfigured channel reporting its failure instead of failing silently).
