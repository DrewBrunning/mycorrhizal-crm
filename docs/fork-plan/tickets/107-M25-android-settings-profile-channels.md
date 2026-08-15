# M25 — Settings: profile & channels on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 |
| **Size** | L — 10 new endpoints across several distinct settings surfaces |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | **DONE** — 2026-08-14, see landing note |

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

---

## Landing note (2026-08-14)

Landed on `feature/m25-android-settings-profile-channels` (from `main`, in a worktree, per the
"implement on a branch based on main in a worktree" workflow — the M22 branch was in use). Gate
green: `./gradlew testDebugUnitTest lintDebug assembleDebug` (845 unit tests).

**What shipped**

- **12 new `ApiClient` methods** (the ticket counted "ten"; the table's rows expand to twelve once
  webhook test + deliveries are counted): `updateLanguage`, `updateDateFormat`, `changePassword`,
  `getNotificationConfig`, `saveNotificationConfig`, `testNotificationChannel`, `listWebhooks`,
  `createWebhook`, `updateWebhook`, `deleteWebhook`, `testWebhook`, `getWebhookDeliveries`. All in
  `ApiClient.kt` with MockWebServer tests (18 new, incl. the wrong-current-password 400 surfacing
  the server's message and the `{ok:false, error}` diagnostic test shape).
- **DTOs** in `core/model/.../network/Settings.kt` mirroring `backend/models/dtos.go` +
  `notification.go` by hand (no dynamic type-list endpoint exists anywhere — `/CLAUDE.md` trap #4;
  `WEBHOOK_EVENTS` + the `ntfy|gotify|push` test-channel enum are the hand-mirrored copies, each
  flagged). Moshi omits nulls, so `NotificationConfigInput`'s optional fields behave exactly like
  web's `undefined` — an omitted `notify_push`/`gotify_token` keeps its stored value server-side.
  The TS `WebhookDelivery` gap was deliberately not copied: Kotlin mirrors the backend DTO
  (`webhook_id`, `next_retry_at` included).
- **Repositories** (`core/domain` + `core/data`): `WebhookRepository`, `NotificationSettingsRepository`,
  and `AppSettingsRepository` (theme + UI-language override, DataStore-backed, with a synchronous
  `currentLanguageOverride()`/`currentThemePreference()` cache for `attachBaseContext`). `AuthRepository`
  gained `updateLanguage`/`updateDateFormat` (both merge into `SessionState` so `observeSession()`
  re-emits live) and `changePassword`.
- **Settings UI**: the main screen's session section gained editable **Language** (5) / **Date
  format** (9) / **Theme** (system/light/dark) dropdowns plus a **Change password** section
  (current/new/confirm; mismatch is a client-side validation never sent to the server). Two new
  sub-screens follow the `CustomLinkActionsScreen` callback-navigation pattern: **Webhooks**
  (list/test/edit/delete, delete-confirmed, expandable delivery history, one-shot secret reveal
  after create) and **Notification channels** (ntfy + Gotify enable/url/topic/token + test; test
  saves first, mirroring web, and a save failure *is* the test result — testing against stale
  stored config would report a misleading success).
- **Theme + locale wiring**: `MainActivity` now collects `AppSettingsRepository.themePreference()`
  reactively (theme changes recompose without restart) and overrides `attachBaseContext` via
  `LocaleContextWrapper` using the synchronous `AppLocale.languageTag` cache (hydrated once at
  app startup in `MycorrhizalApplication.onCreate`). A language change persists server-side,
  caches locally, and recreates the Activity so `values-XX` resources re-resolve — the "actually
  re-renders in it" done-when. Date-format changes flow through `SessionState.dateFormat` into the
  existing `DateFormat` util consumers.

**Decisions taken**

- **Password change forces re-login.** `POST /users/change-password` bumps `TokenVersion`,
  invalidating every JWT; web survives via its re-issued cookie but a bearer-token Android client
  cannot. The ViewModel clears the session and emits `PasswordChanged`; the screen navigates to
  login. The web's "stay logged in" is not achievable for this client.
- **`notify_push` deliberately absent from the Android surface.** Web push is browser-specific
  (VAPID); the backend's per-user `notify_push` toggle is left untouched by omitting it from the
  PUT body (Moshi omits null → server keeps stored value). Local OS notification channels
  (`MycorrhizalNotificationChannels.kt`) remain a separate, native mechanism — the ticket's
  don't-conflate instruction.
- **Language/date-format are server-persisted AND locally cached.** The server value drives email
  prefs; the local `AppSettingsRepository.languageOverride` drives on-device `values-XX`
  resolution so a cold start resolves the chosen language before any server round-trip.
- **`AppLocale` static cache** exists because `attachBaseContext` runs before Hilt injection —
  the same reason it's hydrated in `Application.onCreate` with a one-time blocking read of one tiny
  prefs file.

**Not done / deferred (matching the ticket)**

- No FCM registration (the ticket's flag: that's [M5 §5a](84-M5-android-polish-and-hardening.md),
  not this ticket). Web-push device list is n/a on Android (browser VAPID subscriptions).
- Immich config + API-token/link-field-type registry management stay out of scope per M8's
  deliberate-not-on-mobile list.
- Hand-verification on a real device (language re-render, password re-login, webhook delivery)
  was not possible from this environment — the on-device checks in "Done when" remain outstanding;
  the unit/screen/API surface is covered by tests.

**Hand-verified per `/CLAUDE.md`**: temporarily broke the password-error path and the webhook
2xx-detection and the notification save-before-test path; each broke its new test; restored.

**Review pass (same day, before landing)**: a full read-through of the branch found and fixed five
issues plus a testing gap:

- **Password fields were plaintext.** All three SettingsScreen password fields now use
  `PasswordVisualTransformation` + `KeyboardType.Password` (matching LoginScreen).
- **Gotify was unconfigurable from Android.** The notification screen had URL + toggle but no
  write-only token field, so a user could never provision a Gotify token on-device. Added the
  `gotifyToken` field (password-masked, with stored/hint supporting text), threaded it through
  save/test, and cleared it from state after save so it never lingers (mirrors web). Pinned by a
  ViewModel test asserting the token reaches the server exactly once and is then dropped.
- **Webhook editor's 11 event chips overflowed small screens.** The chip list is now scrollable
  (`heightIn(max=220.dp)` + `verticalScroll`).
- **Dead code removed**: `WebhooksEvent.Deleted` + its screen handling were emitted but unused
  (the state update already removed the row); the empty `WebhooksEvent` sealed interface went too.
- **Password-mismatch used a magic string** (`"settings_password_mismatch"` sentinel compared in
  the UI — a rename would silently break it and render the raw key). Replaced with the codebase's
  established `@StringRes errorRes: Int?` pattern from `LoginViewModel` (`passwordErrorRes` +
  `passwordError`).
- **Testing gaps closed**: `AuthRepositoryImpl` now covers the three new methods (server PATCH +
  session merge + failure propagation); `AppSettingsRepositoryImpl` covers theme/language
  persistence + `hydrate()` + the `AppLocale` sync cache; `NotificationChannelsScreenTest`
  (stateless content extracted) covers both sections + stored-token hint + test-failure rendering;
  `WebhooksScreenTest` covers the editor dialog's create/edit + validation gating. Full gate
  (`testDebugUnitTest` 861 tests / `lintDebug` / `assembleDebug`) green.
