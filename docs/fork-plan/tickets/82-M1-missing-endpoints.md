# M1 mobile — missing backend endpoints: photo URL serving, dashboard, user prefs, OIDC native return

| | |
|---|---|
| **Rating** | 2 |
| **Source** | M1 Phase-5 review pass, 2026-08-10 (on-device findings + the Android app's missing pieces) |
| **Depends on** | M1 Phases 1–5 (shipped). FCM endpoints are **not** here — they're scoped in [N10](81-N10-fcm-push.md). |
| **Status** | Scoped. Backend + web + cross-platform tickets follow this one; Android picks up again after. |

## Why this exists

The Phase-5 review pass on the Android app surfaced four gaps that are **backend-side**, not
Android-side. Each blocks or degrades a shipped Android feature, and none has a route today. FCM
is deliberately excluded (N10 covers it); this ticket is everything else the Android client needs
from the server.

## The four gaps

### 1. Native photo serving — the list returns a raw value Android can't render

**Problem.** `GET /contacts` (the T17 list) returns `ContactSummary.photo_thumbnail` as whatever is
stored on `Contact.PhotoThumbnail` — a `data:` base64 URI *or* a legacy disk-file name. The Android
client only renders `data:` URIs (it checks `startsWith("data:")`), so a file-backed photo is
silently invisible, and even the base64 case depends on the client re-encoding. There is no stable
auth'd URL the native client can hand to an image loader.

**What exists.** `GET /contacts/{id}/profile_picture?thumbnail=true` already serves auth'd raw
bytes (base64 thumbnail decoded to bytes, or the full photo from disk) — this is the server-side
primitive, but the list/detail payloads don't expose a URL to it.

**Proposal (response-shape change, not a new route):**
- `ContactSummary.photo_thumbnail` and the Card `media[].uri` for kind=photo should carry a **relative URL** to the existing profile-picture endpoint (e.g. `/api/v1/contacts/{id}/profile_picture?thumbnail=true`) when a photo exists, and be absent when none does.
- Keep the `data:` URI as a fallback for offline/cache paths if the client needs it, but the primary
  wire value becomes the URL. The Android Coil loader then just fetches it over the auth'd client
  (the app's AuthInterceptor already gates by host).

Backend test bar: the list/detail responses expose the URL (not a raw disk path), a photo-less
contact omits it, and `?thumbnail=true` still serves bytes.

### 2. Dashboard aggregation — one round-trip instead of three

**Problem.** The Android Dashboard (added in the Phase-5 review to mirror the web's) fetches
upcoming birthdays, upcoming reminders, and overdue cadences with three separate authenticated
requests on every open. The web does the same, but a mobile first-tab that costs 3 requests is
wasteful and slow on flaky networks.

**Proposal — new endpoint:**
- `GET /api/v1/dashboard` → `{ birthdays: [...], reminders: [...], overdue_cadences: [...], as_of: <timestamp> }`.
  Each array is the same shape those endpoints already return, so the Android client reuses its
  existing DTOs. Cacheable for a short TTL (`Cache-Control: private, max-age=60`) since the
  dashboard is a glance, not a live read.

Backend test bar: aggregates the three sources, omits empty arrays, owner-scoped like every other
endpoint.

### 3. User preferences update — Android can't change language/date-format

**Problem.** `GET /users/me` returns `language`/`date_format`, and the Android Settings displays
them, but there is **no write path** — a user can only change these on the web. The Android
Settings "Language"/"Date format" rows are read-only stubs.

**Proposal — new endpoint:**
- `PATCH /api/v1/users/me` → `{ language?, date_format? }`, returns the updated `UserProfile`
  (same shape as `GET /users/me`). Validates against the same enums the web profile form uses.

Backend test bar: partial update (only provided fields change), enum validation, owner-scoped.

### 4. OIDC native return — the SSO button exists but the token can't reach the app

**Problem.** The Android login now has a "Sign in with SSO" button that opens the backend's
`/api/v1/auth/oidc/login` in a browser/Custom Tab. But the OIDC callback (`oidc_controller.go`)
sets the `auth_token` httpOnly cookie and `Redirect("/")` — the web SPA. A native client can't
read an httpOnly cookie set in a Custom Tab's browser context, so **the flow never returns the
token to the app**. The M1 ticket §13 explicitly flags this as needing a bridge; this is that work.

**Proposal (a config/redirect change + an app deep link, not a new route):**
- `GET /api/v1/auth/oidc/login?client=android` makes the callback redirect to a **custom scheme**
  deep link carrying the token, e.g. `mycorrhizal://oidc/callback?token=<jwt>&language=<lang>&date_format=<fmt>`.
  Without `client=android` the flow is unchanged (web keeps the cookie + `/` redirect).
- The Android app declares the `mycorrhizal://oidc/callback` intent filter (and ideally
  `android:autoVerify` isn't applicable to custom schemes, so just the intent filter) and its
  `MainActivity` handles the token from the deep link exactly like the normal login path
  (same `SessionManager.setSession`, same persisted state).
- Logout stays as-is (the app calls the logout endpoint directly; RP-Initiated Logout for the IdP
  session is a separate, already-partial concern).

Security notes: the token in the deep link is only delivered to the app's own intent filter; the
state/nonce/PKCE flow is unchanged. Do **not** put the token in a redirect that could leak to the
SPA or logs — verify the callback path scopes the `?token=` branch to `client=android` only.

Backend test bar: `client=android` redirects to the custom scheme with a valid token; default flow
unchanged; state/nonce/PKCE still enforced on the callback regardless of client.

## Out of scope

- **FCM** — fully scoped in [N10](81-N10-fcm-push.md). Reference it for the push channel.
- Web-push subscription changes — N9 shapes stay untouched.
- Any Android UI for these — the Android client changes that consume them land after this ticket
  (photo Coil URL, dashboard one-call, editable settings rows, deep-link auth) and are tracked in
  the M1 ticket / follow-up commits.

## Done when

- Backend `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- List/detail responses expose the photo URL (not a raw disk path); `?thumbnail=true` still serves
  bytes; photo-less contacts omit the field.
- `GET /dashboard` aggregates the three sources, owner-scoped, empty arrays omitted.
- `PATCH /users/me` updates language/date-format with enum validation.
- `client=android` OIDC flow returns the token via the custom-scheme deep link; the web flow is
  byte-for-byte unchanged; state/nonce/PKCE enforced on both.
- All new behavior covered by controller/real-DB tests.
