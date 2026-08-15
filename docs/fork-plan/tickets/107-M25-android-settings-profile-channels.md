# M25 — Settings: profile & channels on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | L — 10 new endpoints across several distinct settings surfaces |
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

---

## Implementation contract (added 2026-08-12)

Added in the readiness pass: this ticket came out of the [M8](89-M8-web-android-parity-audit.md)
parity audit, whose job was to prove a gap was real — not to specify the build. Endpoints, test
cases and the CI gate below close that difference.

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Set language | `PATCH /users/language` | **No** |
| Set date format | `PATCH /users/date-format` | **No** |
| Change password | `POST /users/change-password` | **No** |
| Notification config | `GET,PUT /notifications/config` | **No** |
| Test a channel | `POST /notifications/config/test` | **No** |
| Webhooks CRUD | `GET,POST /webhooks`, `PUT,DELETE /webhooks/:id` | **No** |
| Test a webhook | `POST /webhooks/:id/test` | **No** |
| Read current user | `GET /users/me` | Yes (`currentUser`) |

Ten new client methods — the language and date-format PATCHes are why those settings are currently
read-only. Theme is a local preference with no endpoint.

> **Clarification 2026-08-14:** every route in this table **already exists on the backend** — the
> "In `ApiClient`? No" column means *the Android `ApiClient` lacks the method*, not that a backend
> endpoint is missing. The language/date-format/password routes are the same ones web already uses
> (`backend/routes/routes.go`, `frontend/src/api/users.ts`). There is no backend ticket for this
> work; it is Android-client + UI only. ([M6 §3](85-M6-photo-url-user-prefs-oidc.md) once proposed a
> `PATCH /users/me` for these; that was superseded in favour of the existing routes.)

### Test cases

1. **Language and date format** persist to the server and the UI reflects the change without a
   restart.
2. **Change password** — a wrong current password surfaces the server's error rather than a generic
   failure; success does not leave the new password in any log or state holder.
3. **Notification channels** — config round-trips, and "test" reports success and failure distinctly.
4. **Webhooks** — create/edit/delete round-trip; deletion is confirmed first.
5. **Secrets** — webhook secrets and passwords are never written to logs. Grep-level assertion is
   fine; the point is to make it deliberate.

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
