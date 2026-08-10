# N10 — FCM push notifications for the Android app

| | |
|---|---|
| **Rating** | 3 — real-time reminder/cadence/birthday push to the native Android client |
| **Size** | L |
| **Depends on** | M1 Phase 1 (shipped); Android Phase 2–4 groundwork (a stable auth/network path the push lands on) |
| **Source** | [M1 §12 open question 5](67-M1-mobile-android-app.md) + §13 "ticketed separately" |
| **Status** | Scoped, not scheduled. Gated on M1's later phases being in progress. |

## Why this exists

The M1 design's default for Android notifications is **WorkManager polling**
(`ReminderNotificationWorker`, `CadenceCheckWorker`, `BirthdayCheckWorker` — M1 §6.4/§6.6): each
worker queries the API on an interval and posts a local notification. That works everywhere,
including de-Googled devices, but it is not real-time — a reminder fires only at the next poll tick,
and every poll is an authenticated request the battery pays for.

FCM removes the polling for Play-Services devices: the server pushes the reminder the moment it's
due, the Android client shows it instantly. This ticket is the backend half + the Android client
half of that. It deliberately **keeps WorkManager polling as the fallback** for devices without
Google Play Services (LineageOS without GApps, some Chinese-market devices, and the emulator in
`frontend-dev`-style dev setups) — FCM is additive, never the only path.

## What exists today

- **N9 shipped** (`notification_deliveries`, per-channel `NotificationSender` registry, per-user
  `notification_configs`, and **browser Web Push** via `webpush-go`: server-global VAPID keys in
  `server_settings`, `push_subscriptions` per device, `services/notification_service.go`'s
  `pushNotificationSender`).
- **`PushSubscription` model** — browser-shaped rows (endpoint/p256dh/auth) driven by the Push API.
  FCM is a different subscription shape: a single device **registration token** the client gets from
  Firebase, no endpoint/keys on our side.
- **M1 Android client** ships a polling `InteractionSyncWorker`-style architecture in the design but
  Phase 1 did **not** implement the notification workers yet — they land with Android Phase 4. This
  ticket's Android side therefore lands alongside that phase, not before it.
- The backend has **no Firebase SDK / service-account handling** anywhere today.

## What to build

### Backend

1. **FCM configuration.** Env-driven: `FCM_SERVICE_ACCOUNT_PATH` (the Firebase service-account JSON)
   + `FCM_PROJECT_ID`. When unset, the FCM channel is disabled (matches how ntfy/Gotify degrade when
   unconfigured). Reuse the existing "a missing credential disables the channel" precedent from
   `pushNotificationSender.Enabled`.
2. **A `PushSubscription`-parallel device-token store** (or a `token` column alongside it). Decide
   during implementation, but the wire shape is different enough that a separate
   `fcm_devices`/`device_tokens` table (user_id, fcm_token, platform, created_at) is the cleaner
   call. **Hard-delete** (transient device state, exactly like `PushSubscription`'s doc comment —
   a stale token is a stub, not user content). Add to the `DeleteUser` cascade.
3. **Registration API**: `POST /api/v1/notifications/fcm/devices` (register/replace a token),
   `DELETE /api/v1/notifications/fcm/devices/{id}` (unregister on logout). Owner-scoped like every
   other notification endpoint. **Never return the raw token** to a client that didn't just send it
   (the list shape is ids + platform + created_at).
4. **An `fcm` entry in the `NotificationSender` registry.** The web-push `push` channel stays
   browser-only; FCM is its own channel (`fcm`). A reminder is "due" for `fcm` when no
   `notification_deliveries` row exists for `channel='fcm'` + `status='sent'` — same dispatch
   bookkeeping as N9, no special-casing in `SendReminders`.
5. **Sending**: `firebase-admin` SDK (Go), `messaging.SendEachForMulticast` to the user's registered
   tokens. Message shape: title + body + `data` payload (`{type, reminder_id, contact_id,
   deep_link}`) so the Android client builds the right notification + deep link (M1 §4.6's
   `mycorrhizal://contacts/{contactId}` scheme). Failure handling: an invalid-token error
   (`UNREGISTERED` / `INVALID_ARGUMENT`) means that device row is stale — delete it and continue;
   a transport/auth failure is a channel failure and must not mark the reminder sent (same rule as
   every channel).
6. **Test-notification button** in Settings ("Send test notification" already exists per channel —
   extend it to FCM) so a misconfigured Firebase project fails loudly, not silently.

### Android

7. **Firebase messaging dependency** (`com.google.firebase:firebase-messaging`) + `google-services.json`
   wiring. The app already targets Play-Services-capable devices for the primary path; degrade
   gracefully when Play Services is absent (detect via `FirebaseMessaging.getInstance()` failure and
   keep the WorkManager polling workers as the fallback).
8. **`MyFirebaseMessagingService`** — `onMessageReceived` handles the `data` payload, maps
   `deep_link` → the `PendingIntent` (M1 §6.6 pattern), posts via the existing notification-channel
   constants (`reminders`/`cadence`/`birthdays` from M1 §6.8). `onNewToken` re-registers the token
   with the API.
9. **Token registration on login** (and deletion on logout) via the API above; `READ_PHONE_STATE` /
   notification permissions per M1 §8.
10. **The polling workers stay.** FCM short-circuits the common case; polling remains the safety net
    and the sole path on non-Play devices. Document that a device may briefly double-notify at the
    poll boundary and pick one idempotence key (reminder_id + due time) so the second display is
    suppressed, not duplicated.

## Constraints & traps

- **Firebase project is operator setup.** Self-hosters must create a Firebase project and add the
  service-account JSON. The docs and Settings UI must say so; a missing project degrades the channel
  off, exactly like an unset ntfy URL, with the test button as the diagnosis surface.
- **Google Play Services requirement.** This is the reason polling stays. Do **not** make FCM the
  only path — the M1 design's whole point is a self-hosted-friendly app.
- **Payload size.** FCM data messages are bounded (~4 KB). Keep `data` minimal (ids + deep link,
  no rendered body text beyond what's needed).
- **Rate/scale.** A user with several registered tokens (multi-device) sends one multicast call;
  `SendEachForMulticast` handles per-token results. Don't fan out one HTTP call per token.
- **Same notification-infrastructure rules as N9:** FCM sits inside the `SendReminders` job lock;
  a failed channel must not block or mark-sent other channels; templates (title/body) need short-form
  strings in all five locales; the SSRF guard is irrelevant here (FCM is Google's service, a fixed
  endpoint) but the service-account path is a config file — validate it exists/parses at boot.
- **Delete cascade:** the new device-token table joins `DeleteUser`'s explicit enumeration
  (`admin_user_controller.go`), per CLAUDE.md trap 6. Hard-delete, no `DeletedAt`.
- **Test against real Firebase**, not a mock — the "Done when" bar N9 set for real-instance
  verification applies here too (a fake FCM endpoint in CI covers the bookkeeping; a manual run with
  a real Firebase project proves the round-trip).

## Done when

- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green; Android
  `./gradlew testDebugUnitTest` green.
- Backend: controller tests for register/unregister (ownership scoping, cross-user 403, duplicate
  token replace), real-DB test for the device-token round-trip + `DeleteUser` cascade, sender tests
  (stale token removed, failure not marked sent, multicast fan-out).
- Android: `MyFirebaseMessagingService` handler test (data payload → deep-link PendingIntent),
  token-registration ViewModel test, fallback-to-polling test on missing Play Services.
- Verified end to end against a real Firebase project: server pushes a due reminder, the device
  shows the notification, tapping it deep-links to the contact (hand-verified, not just mocked).
- All five locale files carry the FCM short-form strings.

## Not in scope

- FCM **data messages only** — no notification-channel messaging from the server (the client builds
  the notification so it can honor per-app channel settings).
- iOS push (APNs) — the Android client is the only native client planned (M1 §13).
- Replacing the WorkManager polling workers — they are the fallback, intentionally retained.
