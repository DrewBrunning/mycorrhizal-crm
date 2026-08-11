# M25 — Settings: profile & channels on Android

| | |
|---|---|
| **Rating** | 3 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

`SettingsScreen.kt` currently shows language/date-format/admin-flag read-only. Everything below was
explicitly left as build-it at M8's sign-off (only Immich config and API-token/link-field-type
registry management were marked deliberately-not-on-mobile from the settings area — those are out
of scope here).

## Scope

- **Language** — editable, not read-only (`SettingsScreen.kt:110` vs.
  `SettingsPage.tsx:160-173,240-251`).
- **Date format** — editable, not read-only (`SettingsScreen.kt:111` vs.
  `SettingsPage.tsx:179-192,274-289`, 9 options).
- **Theme** (system/light/dark) — no concept exists on Android at all today
  (`SettingsPage.tsx:175-177,308-322`).
- **Password change** — no call anywhere in Android source
  (`SettingsPage.tsx:194-218,340-378`).
- **Webhooks** — full CRUD + test + delivery history (`WebhooksSettings.tsx`). Zero Android
  footprint today.
- **Notification channels (ntfy/Gotify)** — configure + enable/disable + test
  (`NotificationSettings.tsx:280-390`). Distinct from Android's own local OS notification channels
  (`MycorrhizalNotificationChannels.kt`), which are a different, already-native mechanism for the
  app's own local alerts — don't conflate the two when implementing.
- **Web-push device list is n/a on Android** (browser-specific VAPID subscriptions) — nothing to do
  there. But note Android currently has **no FCM registration at all**
  (`grep -rli 'FCM|FirebaseMessaging|registerDevice' android/` → zero hits), which is a prerequisite
  already tracked separately under [M5](84-M5-android-polish-and-hardening.md) §5a — this ticket
  doesn't duplicate that work, just flags the dependency for sequencing.

## Done when

- Language, date format, theme, and password are all editable from Android Settings.
- Webhook CRUD + test + history works and matches web's data model.
- ntfy/Gotify channel configuration works, distinct from and not confused with local OS notification
  channels.
- Hand-verified on-device: change language and confirm the app actually re-renders in it; change
  password and confirm re-login with the new one works; create+test a webhook and confirm delivery
  on the receiving end.
- New strings translated in all five locales.
