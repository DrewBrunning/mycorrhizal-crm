# M5 — Android app: deferred polish, native-endpoint consumers, and the missing test tier

| | |
|---|---|
| **Rating** | 3 |
| **Source** | Post-M1 review pass, 2026-08-11 (full read of `android/` after Phases 1–5 landed) |
| **Depends on** | M1 Phases 1–5 (shipped). Items §3 and §5 are gated on the backend tickets noted inline. |
| **Status** | Scoped, not started. Promoted to the Android list 2026-08-14 (was under Feature ideas). |

This is the Android-client counterpart to the M-series backend tickets: [M2](81-M2-fcm-mobile-push.md)
(FCM push), [M3](82-M3-dashboard-overview-endpoint.md) (`GET /dashboard`), and
[M4](83-M4-contact-detail-composite.md) (`GET /contacts/:id/detail`) are all backend-side; this is
the app-side work, and where those tickets' Android consumers live.

Two Android gaps are **not** here, filed 2026-08-11 after this ticket was written:
[M7](88-M7-android-contact-record-coverage.md) — the contact editor covers 8 of ~30 `Card` field
groups — and [M8](89-M8-web-android-parity-audit.md) — the web↔Android parity audit. This ticket is
deferred *polish*; those two are missing *functionality*, which is a different bar.

## Why this exists

M1's five phases shipped a genuinely working Android client, and its landing note honestly records
what it deferred. That deferred list now lives only inside a 3000-line design document, mixed in
with the design itself, where it is not schedulable. This ticket pulls the still-outstanding
Android-side work into one place.

A separate review pass (2026-08-11) fixed everything that was an outright *defect* — two lint
errors that had kept Android CI red on every run since the app merged, a `BootReceiver` that could
never fire, a stale `:app` resource copy shadowing `:core:ui`, a hardcoded date format, a dead
no-op in the sync worker, a static View leak, and ~80 unlocalized strings. See M1's landing note
for that pass. **None of the below are defects of that kind** — they are deferred scope, gaps, and
hardening.

---

## 1. Tablet layout (M1 item 31 — explicitly deferred)

M1 §4.1/item 31 calls for `NavigationRail` and a two-pane contact list/detail on large screens. The
app is phone-only today: a single `NavHost` with a modal drawer at every width. On a tablet the
contact detail fills the whole screen and the list is a separate destination.

- `NavigationRail` instead of the drawer at `WindowWidthSizeClass.Expanded`.
- Two-pane list+detail on expanded widths, single-pane below.
- The screens are already split into stateless `*ScreenContent` composables (that split was done
  for the Robolectric UI tests), so this is a host-level change, not a rewrite of each screen.

Test bar: the existing Compose tests run against `*ScreenContent`, so add width-class tests at the
host level rather than duplicating per-screen tests.

## 2. Accessibility audit (M1 item 32 — explicitly deferred)

Never performed. The 2026-08-11 pass incidentally localized the four hardcoded `contentDescription`
values, which is not the same thing as an audit.

- TalkBack pass over every screen: does each interactive element announce something useful, and is
  the reading order sane?
- `contentDescription` coverage on icon-only buttons (there are many — FABs, overflow menus, the
  field-action chips on contact detail).
- Touch target sizes (48dp minimum), especially the copy/action icons on contact detail rows.
- Contrast check on the brand palette in both light and dark themes — the bone/green pairing has
  never been measured against WCAG AA.
- `Modifier.semantics` grouping so a contact row reads as one node rather than five.

## 3. The recorded UI deviations from the web app

M1's landing note lists four cosmetic differences spotted on-device against the web app, parked for
"a consolidated polish pass". Two of them are more than cosmetic:

1. **Contact photos still do not display** — and this is now a three-part problem, not one:
   - The backend does not yet return a usable photo URL. That is the photo-serving item in
     [M6](85-M6-photo-url-user-prefs-oidc.md) — **gated on it**.
   - `ContactAvatar` only accepts an *absolute* `http(s)` URL (`uri.startsWith("http")`). The
     proposed wire value is a **relative** URL (`/api/v1/contacts/{id}/profile_picture?thumbnail=true`),
     which will fall straight through to the person-icon fallback. It needs to resolve relative
     URLs against the configured server origin.
   - **Coil is not wired to the authenticated OkHttp client.** There is no `ImageLoader` /
     `SingletonImageLoader.Factory` anywhere in the app, so `AsyncImage` uses Coil's default
     network stack, which carries no `Authorization` header — an auth'd photo URL would 401.
     Coil must be given the app's `OkHttpClient` (the one with `AuthInterceptor`, which already
     host-gates the JWT so this is safe).
2. **Last name is not shown in the list or detail.** Both render `card.displayName` (vCard `fn`)
   directly. Where the backend's `fn` is a given name only, the app shows just "Elizabeth" while
   the web shows "Elizabeth Brunning" — the web derives a display name from the name components
   when `fn` is short. Android should do the same derivation from `components` given+surname.
3. **Font role placement.** Android's M3 scale puts EB Garamond on `display*` and IBM Plex Sans on
   `body*`; the web uses Plex Sans as the app-wide default with Garamond only in brand spots. The
   detail/list read Garamond-heavy against the web's Plex. Align which roles use which font.
4. **Section styling** — dividers, spacing, avatar size, empty-state placement. Low priority.

## 4. The in-overlay quick-capture sheet (M1 item 23 — partial)

`QuickCaptureOverlay` currently shows a pill after a call ends that deep-links into the app. M1 §6.5
describes a pre-filled activity form *in the overlay*, so an interaction can be logged without
leaving the call screen. The pill is the graceful-degradation path; the sheet is the feature.

Note the overlay was converted from a singleton `object` to a service-owned instance in the
2026-08-11 pass (it was leaking a Context via a static field) — build the sheet on that shape.

## 5. Consume the backend endpoints once they land

[M6](85-M6-photo-url-user-prefs-oidc.md) originally said its Android consumers were "tracked in the
M1 ticket / follow-up commits", which in practice meant untracked. This section is where they now
live:

- **Photo URL** ([M6](85-M6-photo-url-user-prefs-oidc.md) §1) → §3.1 above.
- **`GET /dashboard`** ([M3](82-M3-dashboard-overview-endpoint.md)) → **DONE 2026-08-14 by
  [M10](92-M10-android-dashboard-composite.md)** — `DashboardViewModel` now makes a single
  `GET /dashboard` call. Nothing left here.
- **`GET /contacts/:id/detail`** ([M4](83-M4-contact-detail-composite.md)) → not in the original
  M1-endpoints list; adopt the composite on the contact detail once it exists.
- **User-prefs write** (was `PATCH /users/me`, [M6](85-M6-photo-url-user-prefs-oidc.md) §3) →
  **superseded** — the backend routes already exist (`PATCH /users/language`,
  `PATCH /users/date-format`), so there is no M6 backend half; the Android client + Settings UI is
  now owned by [M25](107-M25-android-settings-profile-channels.md), not this ticket.
- **OIDC native return** ([M6](85-M6-photo-url-user-prefs-oidc.md) §4) → the app has no SSO path
  at all today; the deep-link handling (`mycorrhizal://oidc/callback`) and its intent filter land
  here once the backend's `client=android` branch exists.

Each of the above is gated on its backend half. The FCM client below is not.

### 5a. FCM client — unblocked now ([M2](81-M2-fcm-mobile-push.md)'s backend is merged)

Absorbed from the retired `81-N10-fcm-push.md`, whose backend half became M2. This is the Android
half, and nothing is blocking it:

- **Firebase messaging dependency** (`com.google.firebase:firebase-messaging`) +
  `google-services.json` wiring.
- **`MyFirebaseMessagingService`** — `onMessageReceived` handles the `data` payload, maps
  `deep_link` → a `PendingIntent` (M1 §6.6 pattern), and posts via the existing channel constants
  (`reminders`/`cadence`/`birthdays`, M1 §6.8). `onNewToken` re-registers with the API.
- **Token registration on login, deletion on logout** — `POST /notifications/devices` with
  `{token, client:"fcm", device_label}` and `DELETE /notifications/devices/:id`, per M2's landing
  note. The web app never enrolls devices; it only views and deletes them.

Two constraints from N10 that must survive the move, because they are the whole reason this app
is self-hosted-friendly:

- **The WorkManager polling workers stay.** FCM short-circuits the common case; polling remains
  the safety net and the *only* path on de-Googled devices. Degrade gracefully when Play Services
  is absent — do **not** make FCM the sole path.
- **A device may briefly double-notify at the poll boundary.** Pick one idempotence key
  (`reminder_id` + due time) so the second display is suppressed rather than duplicated.

Test bar (also from N10): a handler test for data payload → deep-link `PendingIntent`, a
token-registration ViewModel test, and a fallback-to-polling test on missing Play Services. Plus
an end-to-end hand-verification against a real Firebase project — server pushes a due reminder,
the device shows it, tapping deep-links to the contact. Mocks alone do not close this.

## 6. There is no instrumented-test tier at all

`find android -type d -name androidTest` returns **zero** directories. M1 §10.3/§10.5 assume
integration tests for Room + networking and Android-specific tests for permissions, receivers and
workers; what exists is 358 JVM/Robolectric unit tests and nothing that runs on a device or
emulator. `android-tests.yml` runs `testDebugUnitTest`, `lintDebug`, `assembleDebug` — no
`connectedAndroidTest`.

This is defensible (Robolectric covers a lot, and emulator CI is slow and flaky) but it should be a
decision, not an accident. Either:
- add a thin `androidTest` tier for the things Robolectric genuinely cannot verify — the real
  `ContentResolver` against `ContactsContract`, WorkManager's actual scheduling, the foreground
  service and overlay permission paths — and wire an emulator job into CI; **or**
- record explicitly in M1 that the Robolectric tier is the whole pyramid and why.

Do not leave it implicit.

## 7. Smaller hardening items

- **No release signing config.** `app/build.gradle.kts`'s `release` block sets minify/shrink but no
  `signingConfig`, so `assembleRelease` produces an unsigned APK that cannot be installed. R8 was
  verified (4MB APK) but distribution was never set up. Decide how the app is delivered (signed
  APK from CI? F-Droid? Play?) and wire the keystore in via env/properties, never committed.
- **Unverified MIMETYPEs.** M1's landing note records that the Signal and Google Meet
  `ContactsContract.Data` MIMETYPEs were corrected against a real device, but Telegram, Zoom and
  Discord "remain unverified guesses pending a contact row per app". Verify on-device or drop them
  — a wrong MIMETYPE silently yields no chips, which is indistinguishable from "app not installed".
- **Household address suggestions.** The `suggest-addresses`/`accept`/`dismiss` endpoints (T40)
  exist on the backend; the Android households feature deliberately skipped them.
- **Import/export file picker.** The full import/export API client and DTOs shipped in Phase 3
  item 19, but the file-picker UI was deferred to T57 / Phase 5 and was not built.
- **`ObsoleteSdkInt` dead branches.** Three `Build.VERSION.SDK_INT` guards against API 26 that can
  never be false at `minSdk = 26` (`SettingsViewModel.startCallDetectionService`,
  `MycorrhizalNotificationChannels.createAll`), plus a `mipmap-anydpi-v26` folder that lint says
  should merge into `mipmap-anydpi`. Warnings only; clean up when touching those files.

## Out of scope

- **The FCM backend channel** — [M2](81-M2-fcm-mobile-push.md), already done. Its Android half is
  in §5 above, not out of scope.
- **Backend endpoint work** — [M6](85-M6-photo-url-user-prefs-oidc.md),
  [M3](82-M3-dashboard-overview-endpoint.md), [M4](83-M4-contact-detail-composite.md). This ticket
  is only the Android side of consuming them.
- **iOS / Wear OS** — out of scope per M1 §13.

## Done when

Each numbered section is independently shippable; this is a container, not an all-or-nothing gate.
Per section:

- `./gradlew testDebugUnitTest lintDebug assembleDebug` stays green (currently 358 tests, 0 lint
  errors) — run with `ANDROID_HOME=/home/drew/Android/Sdk JAVA_HOME=/home/drew/android-studio/jbr`,
  since the system JDKs are JREs with no `javac` and there is no `local.properties`.
- Any new user-facing string lands in `core/ui/src/main/res/values{,-de,-es,-fr,-it}/strings.xml`
  with real translations — `LocalesConsistencyTest` now also fails a key-prefix namespace left
  byte-identical to English.
- New tests hand-verified per `/CLAUDE.md` (broken, seen to fail, restored).
- On-device verification on the Pixel 8a for anything with a device-only surface, matching how
  every M1 phase was signed off.
