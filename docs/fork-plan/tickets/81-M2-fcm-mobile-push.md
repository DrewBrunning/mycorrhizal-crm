# M2 — Mobile push device registration + FCM delivery (Android)

| | |
|---|---|
| **Rating** | 4 — the mobile client's primary notification path; a named sub-piece of M1 |
| **Size** | M |
| **Depends on** | [N9](30-N9-notification-channels.md) (done — owns the channel abstraction and push sender) |
| **Source** | M1's open question 5 / §13 ("Push notifications from server: requires FCM integration on the backend, ticketed separately") + the 2026-08-10 mobile-API work session |
| **Status** | **Backend done; frontend Settings UI split off, not started** |

## Why this exists

The web app registers browser push via `POST /notifications/push-subscriptions`, which stores a
Web Push (VAPID) subscription: `endpoint` + `p256dh` + `auth`, handed to the browser by the
browser Push API. A native Android app cannot do that — it registers a Firebase Cloud
Messaging (FCM) **device token** with Google Play Services instead. The server-side channel
abstraction (N9) is already in place; what's missing is (a) a device-token registration surface
the Android client can call and (b) an FCM sender implementation wired into the existing `push`
channel dispatch.

**Generalized shape, decided 2026-08-10 with the user:** the registration endpoint is deliberately
NOT FCM-specific. It takes a `token` (string) and a `client` (string, `fcm`|`apns`), so an iOS
client can later register APNS tokens against the same endpoint without a contract change. The
backend dispatches delivery by `client`; only `fcm` delivery is implemented in this ticket, and
`apns` registrations are accepted but their delivery is a documented no-op until an APNS sender
exists.

## What exists today

- `pushNotificationSender` (`services/notification_service.go`) implements the N9
  `NotificationSender` interface for `ChannelPush`, looping over `push_subscriptions` and calling
  `sendPushMessage` (webpush-go VAPID). `Enabled` counts `push_subscriptions`; `Send` and
  `TestNotificationChannel` iterate them.
- `PushSubscription` model + `GET/POST/DELETE /notifications/push-subscriptions` — the browser
  path, unchanged by this ticket.
- `config.Config` + `.env.example` pattern for server-side channel config (mail, OIDC, Immich).
- `credential_crypto.go` (AES-256-GCM, key derived from `JWT_SECRET_KEY`) for tokens at rest.
- `clientFor(cfg)` / `isPrivateURL` — the webhook SSRF dialer policy reused by the notification
  senders.
- `golang-jwt/jwt/v4` and `golang.org/x/oauth2` already in `go.mod` — everything needed to mint a
  service-account JWT and exchange it for a Google OAuth2 token (no new dependency required).
- The `sendReminderEmailFn` package-level function-var test seam pattern (`notification_service_test.go`).

## Design decisions — locked 2026-08-10

1. **New `device_registrations` table, not a column on `push_subscriptions`.** Web Push
   subscriptions and mobile device tokens are different shapes (endpoint+keys vs a bare token)
   and different lifetimes (browser re-subscribe vs app reinstall). One table each is the honest
   model. **Hard-delete**, exactly like `PushSubscription`: transient device state, re-registered
   on every app launch, auto-removed when FCM reports a dead token — tombstones would accumulate
   for nothing.

   **Natural key is `(client, token)`, NOT `(user_id, client, token)`** — revised 2026-08-10
   during implementation, prompted by the Gemini review notes on the M1 work session (their
   "handle token ownership re-assignment" point). An FCM/APNS token identifies a physical app
   install, not a person: if the key included `user_id`, a device that logs out of user A and
   into user B would accumulate a live row for *both* — they'd only ever differ by `user_id` — and
   A's reminders would keep pushing to a device now signed in as B until FCM eventually 404s the
   stale row. Re-registering an existing `(client, token)` now reassigns the row's `UserID`
   (and label) to the calling user instead of duplicating it, mirroring how the platform push
   services themselves treat the token — one install, one current owner. Pinned by
   `TestCreateDeviceRegistration_ReassignsAcrossUsers`.

2. **Generalized `client` enum: `fcm`, `apns`** (`oneof=fcm apns`). Delivery dispatch keyed on
   it. Only `fcm` is wired to a sender here; `apns` is stored/listed/deleted but its delivery
   path is a logged skip ("not implemented"), never a fake success and never a marked-sent row —
   so when an APNS sender lands it plugs into the same dispatch without a contract change.

3. **FCM delivery via the HTTP v1 API** (`https://fcm.googleapis.com/v1/projects/{project}/messages:send`),
   not the legacy `fcm/send` endpoint. Legacy server keys are being phased out by Google and are
   not available on new Firebase projects; HTTP v1 with a service account is the forward-compatible
   path. Auth: mint an RS256 JWT signed with the service account's private key (claims
   `iss=client_email`, `aud=https://oauth2.googleapis.com/token`,
   `scope=https://www.googleapis.com/auth/firebase.messaging`, `iat`/`exp`), exchange it for an
   access token at the Google OAuth2 token endpoint, then POST the message.

4. **Server config via `FCM_SERVICE_ACCOUNT_FILE`** (path to the Firebase service-account JSON:
   `project_id`, `client_email`, `private_key`). When unset, FCM delivery is disabled and the
   channel enablement check behaves as today (browser push only). `fcm` device registrations are
   still accepted when unset — they're inert until the operator configures delivery, same as a
   user enabling a channel before saving its config. This is deliberately one env var (a file
   path), consistent with how this repo treats other secrets: the JSON never goes in the DB or a
   query param.

5. **One `push` channel dispatches both browser Web Push and mobile FCM.** No new channel enum
   value, no new per-user toggle: `notify_push` already means "deliver reminders via push", and a
   reminder is due-for-push until every registered push endpoint has a `sent` row (the existing
   per-channel delivery-record semantics). `pushNotificationSender.Enabled` becomes "toggle on AND
   (web subscriptions OR fcm registrations)"; `Send` iterates both; the Settings test button
   probes both. This keeps `notification_deliveries.channel` CHECK constraint and the frontend
   channel list untouched.

## What to build

1. **Migration `000019_device_registrations`** (up/down). Columns: `id`, `created_at`,
   `updated_at`, `user_id` (FK `users(id) ON DELETE CASCADE`), `token` (NOT NULL),
   `client` (NOT NULL, CHECK `client IN ('fcm','apns')`), `device_label` (NOT NULL DEFAULT '').
   Unique index `(client, token)` (not `(user_id, client, token)` — see design decision 1) so
   re-registration upserts, including across a change of owning user. No soft delete, no
   `DeletedAt` field (hard-delete per design decision 1). Model `DeviceRegistration` +
   `DeviceRegistrationInput` in `backend/models/notification.go`, with an explicit
   `gorm:"column:..."` only where GORM would disagree (no acronyms here, but follow the
   `PushSubscription` explicit-ID pattern so the JSON response is clean).

2. **`services` functions** in `notification_service.go`:
   - `ListDeviceRegistrations`, `CreateDeviceRegistration` (dedupe on the natural key),
     `DeleteDeviceRegistration` — mirroring the `PushSubscription` trio.
   - An `fcmSender`-style delivery helper `sendFCMMessage(cfg, serviceAccount, token, title, body)`
     that mints the JWT, exchanges it, and POSTs the message, returning a stale-token signal on
     FCM's 404-style error (`UNREGISTERED`/`INVALID_ARGUMENT` for a malformed token) so the
     caller can drop dead registrations — same shape as `sendPushMessage`'s `stale bool`.
   - `pushNotificationSender.Enabled` / `Send` / `TestNotificationChannel` extended to include
     `client='fcm'` registrations. `apns` rows are skipped with a logged warning, never marked
     sent.

3. **Config**: `FCMServiceAccountFile` field on `Config`, loaded from
   `FCM_SERVICE_ACCOUNT_FILE`, documented in `.env.example` next to the VAPID note. Add a small
   loader (parse the JSON, validate `project_id`/`client_email`/`private_key` present) with a
   clear error if the file is set but invalid — reject at boot rather than failing the first send.

4. **Controller + routes**: `GET/POST /notifications/devices`,
   `DELETE /notifications/devices/:id`, following `notification_controller.go`'s existing
   `...PushSubscription` handlers verbatim (same scoping, same error mapping, same response
   shapes).

5. **Web Settings UI**: `NotificationSettings.tsx` gains a "Mobile devices" list under the push
   section showing `device_registrations` (label + client + registered-at) with per-device
   delete, reusing the existing `subscriptions` list pattern. No web-side *enroll* button (that's
   the Android app's job); the section is informational + delete + included in the push test
   button. New i18n keys in all five locales.

## Traps

- **Do not mark an `apns` row sent.** The delivery loop must skip it entirely; marking it sent
  would suppress the reminder for an un-delivered device and break the "skip is a logged,
  documented stub" contract in design decision 2.
- **Dead FCM tokens must be pruned, not just recorded.** A `UNREGISTERED` response means the app
  was uninstalled / token rotated. Drop the row (like `sendPushMessage` drops 404/410 web subs)
  and keep the reminder due for push so the user's *other* devices still get it.
- **Reuse the SSRF-guarded dialer** (`clientFor(cfg)`) for the token-exchange and FCM POST.
  Google endpoints are public, but the guard is the house style for every outbound call and costs
  nothing.
- **JWT expiry on the service-account token:** exchange-scoped access tokens expire (~1h); the
  token endpoint response carries `expires_in`. Do not cache it for the daily job's whole
  lifetime — either mint fresh per send run (fine: reminder dispatch is a low-frequency job) or
  cache with an expiry. Prefer minting per run for simplicity, unless a test shows otherwise.
- **Keep the channel registry test green.** `TestNotificationSendersCoverAllChannels` pins the
  sender list to `AllNotificationChannels` — no new channel is being added, so it should stay
  green, but the push sender's `Enabled`/`Send` changes need their own tests.
- **GORM explicit-ID pattern** for `DeviceRegistration` (like `PushSubscription`): declare
  `ID`/`CreatedAt`/`UpdatedAt` explicitly so the JSON response is the clean
  `{id, created_at, ...}` shape; do not embed `gorm.Model` (it serializes capitalized field names).
- **Ownership scoping:** every device handler scopes by `user_id` (CLAUDE.md trap 5). No IDOR.
- **Cascades:** add `device_registrations` to `DeleteUser`'s manual cascade list
  (`admin_user_controller.go`). It is NOT a contact-scoped entity, so `deleteContactAssociations`
  does not need it. Per T26 hard-delete rule, no purge job interaction.

## Done when

- [x] `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- [x] Tests cover: register/list/delete device tokens with ownership scoping (cross-user
      404/empty); re-registering the same `(client, token)` reassigns to the calling user instead
      of duplicating, including across a change of owner (`TestCreateDeviceRegistration_
      ReassignsAcrossUsers`); `client` validation rejects unknown values; FCM delivery happy path
      against a fake token+FCM server (real migrated DB via `database.InitDB`); a dead FCM token
      is dropped and the reminder stays due for push; an `apns` registration is never marked sent;
      the push channel dispatches to both a browser web sub and an FCM device in one
      `SendReminders` run; `TestNotificationChannel` push probes an FCM device (added — this was
      missing from the first implementation pass, see landing note); config boot fails clearly on
      an invalid service-account file (wired into `main.go`, not just `config.Validate`'s
      file-exists check — see landing note).
- [ ] Web Settings shows the mobile device list with delete; all five locales have real
      translations. **Deferred** — split into its own follow-on, not part of this branch.
- [ ] `npx tsc --noEmit` and `npx vitest run` green in `frontend/`. **Deferred** with the above.
- [x] OpenAPI updated with the three device endpoints + schemas; the existing drift test green.
- [ ] Hand-verify the FCM happy path against a real Firebase project — **not done**, needs a real
      Firebase project + service-account credentials, which this environment doesn't have. The
      fake-server test suite (`TestSendFCMMessage_HappyPath` et al.) covers the request/response
      shape and JWT signing but is not a substitute for this bar (N9/T51's "real push service, not
      a mock"). Do this by hand before relying on FCM delivery in production.

### Ticket-specific

- The Android client calls `POST /notifications/devices` with `{token, client:"fcm", device_label}`
  after Firebase `getToken()`, and `DELETE /notifications/devices/:id` on
  `onDeletedToken`/logout. The web app never enrolls devices; it only views/deletes them.
- FCM HTTP v1 body shape: `{"message":{"token":"<token>","notification":{"title":t,"body":m}}}`.
- The JWT mint + token exchange can reuse `golang-jwt/jwt/v4` (already a direct dependency) —
  do not add `golang.org/x/oauth2` unless the exchange genuinely needs it (it may be simpler to
  hand-roll the one POST).

## Landing note

**Backend landed 2026-08-10; frontend split off, not started.** Picked up an in-progress branch
that already had the migration, model, config, controller, routes, and FCM sender/dispatch
written to spec, plus solid test coverage at both layers. Found and fixed three real gaps before
calling the backend done, each hand-verified by reverting the fix and confirming the relevant
test actually fails:

1. **Natural key changed from `(user_id, client, token)` to `(client, token)`**, with
   re-registration now reassigning `UserID` instead of erroring/duplicating — see design decision
   1. Without this, the same physical device logging out of one user and into another would leak
   the first user's reminder pushes to it indefinitely (raised independently by the Gemini review
   notes on the source work session).
2. **`TestNotificationChannel`'s push case only probed browser Web Push subscriptions**, never
   FCM devices — silently contradicting the ticket's own design decision 5 ("the Settings test
   button probes both"). Fixed and pinned by `TestTestNotificationChannel_ProbesFCMDevice` /
   `TestTestNotificationChannel_FCMDeviceWithoutServerConfig`.
3. **`openapi.yaml` was never updated** for the three `/notifications/devices` routes —
   `TestOpenAPIRouteCoverage` was failing in the branch. Added paths + `DeviceRegistrationInput`/
   `DeviceRegistrationResponse` schemas.
4. **Boot-time FCM config validation was file-existence only**; a malformed service-account file
   (bad JSON, missing fields) would only surface as a warning on the first reminder run, not a
   boot failure — contradicting design decision 4's "reject at boot rather than failing the first
   send". `main.go` now calls `services.LoadFCMServiceAccount` at startup (after `cfg.
   ValidateOrPanic()`) and fails fast; `config.Validate` still does the cheap file-exists check on
   its own since `config` can't import `services`.

`go build/vet/gofmt/test` all green. Frontend Settings UI (mobile device list + 5-locale i18n)
and the real-Firebase-project hand-verification remain open — see the unchecked Done-when items.
