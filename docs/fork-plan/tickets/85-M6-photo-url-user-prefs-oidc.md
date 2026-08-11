# M6 — missing backend endpoints for the Android app: photo URL serving, user prefs, OIDC native return

| | |
|---|---|
| **Rating** | 2 |
| **Source** | M1 Phase-5 review pass, 2026-08-10 (on-device findings + the Android app's missing pieces) |
| **Depends on** | M1 Phases 1–5 (shipped). FCM endpoints are **not** here — they're [M2](81-M2-fcm-mobile-push.md). |
| **Status** | Scoped. Three of its original four gaps are still live; §2 (dashboard) was taken over by [M3](82-M3-dashboard-overview-endpoint.md). |

> **Renumbered 2026-08-11**, from `82-M1-missing-endpoints.md` to `85`/`M6`. The 2026-08-10
> mobile-API session filed M2/M3/M4 at 81/82/83 without retiring the tickets they overlapped, so
> `main` briefly carried two `81-` and two `82-` files. This ticket keeps its three live gaps and
> moves clear of M3's number; the fully-superseded `81-N10-fcm-push.md` was retired into
> [M2](81-M2-fcm-mobile-push.md) (backend) and [M5](84-M5-android-polish-and-hardening.md) §5
> (its Android half). Old links to `82-M1-missing-endpoints.md` will not resolve.

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

### 2. Dashboard aggregation — **SUPERSEDED by [M3](82-M3-dashboard-overview-endpoint.md)**

This gap ("the Android Dashboard costs three authenticated requests on every open — collapse
`GET /reminders/upcoming` + `/cadence-policies/overdue` + `/contacts/birthdays` into one call")
was re-scoped in full as [M3](82-M3-dashboard-overview-endpoint.md) on 2026-08-10, which also
picked up the equivalent web `DashboardPage` fan-out. **Build it from M3, not from here.** The
heading is kept so §3/§4's numbering still matches anything that cited it.

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

- **FCM** — [M2](81-M2-fcm-mobile-push.md), backend already done.
- **The dashboard composite** — [M3](82-M3-dashboard-overview-endpoint.md); see §2 above.
- Web-push subscription changes — N9 shapes stay untouched.
- Any Android UI for these — the Android client changes that consume them (photo Coil URL +
  authenticated image loader, editable settings rows, deep-link auth) are
  [M5](84-M5-android-polish-and-hardening.md) §3 and §5. That is now a real ticket; this section
  previously said they were "tracked in the M1 ticket / follow-up commits", which meant untracked.

## Done when

- Backend `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- List/detail responses expose the photo URL (not a raw disk path); `?thumbnail=true` still serves
  bytes; photo-less contacts omit the field.
- `PATCH /users/me` updates language/date-format with enum validation.
- `client=android` OIDC flow returns the token via the custom-scheme deep link; the web flow is
  byte-for-byte unchanged; state/nonce/PKCE enforced on both.
- All new behavior covered by controller/real-DB tests.
