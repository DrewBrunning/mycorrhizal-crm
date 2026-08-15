# M6 — missing backend endpoints for the Android app: photo URL serving, user prefs, OIDC native return

| | |
|---|---|
| **Rating** | 2 |
| **Source** | M1 Phase-5 review pass, 2026-08-10 (on-device findings + the Android app's missing pieces) |
| **Depends on** | M1 Phases 1–5 (shipped). FCM endpoints are **not** here — they're [M2](81-M2-fcm-mobile-push.md). |
| **Status** | **DONE** (2026-08-14 — §1 photo URL response-shape change + §4 OIDC native return; see the landing note). §2 (dashboard) was taken over by [M3](82-M3-dashboard-overview-endpoint.md); §3 (user prefs) is **superseded** — the write endpoints already exist (see §3 below). |

> **Renumbered 2026-08-11**, from `82-M1-missing-endpoints.md` to `85`/`M6`. The 2026-08-10
> mobile-API session filed M2/M3/M4 at 81/82/83 without retiring the tickets they overlapped, so
> `main` briefly carried two `81-` and two `82-` files. This ticket keeps its three live gaps and
> moves clear of M3's number; the fully-superseded `81-N10-fcm-push.md` was retired into
> [M2](81-M2-fcm-mobile-push.md) (backend) and [M5](84-M5-android-polish-and-hardening.md) §5
> (its Android half). Old links to `82-M1-missing-endpoints.md` will not resolve.

> **Landing note, 2026-08-14** (the two live gaps, both backend-only):
>
> **§1 photo URL serving.** `ContactSummary.photo_thumbnail` and `ContactRecordResponse.
> photo_thumbnail` now carry a relative URL to the existing `GET /contacts/{id}/profile_picture`
> endpoint instead of the raw stored value — preferring `?thumbnail=true` when a base64 thumbnail
> is stored, falling back to the no-param full-photo variant for a disk photo whose thumbnail is a
> legacy filename (the thumbnail endpoint can't serve those), and **omitted entirely** for
> photo-less contacts (`omitempty`). The detail response's `Card.Media` photo entry's `uri` is
> rewritten to the same URL (full-photo variant) **on the read path only**: the slice is copied
> before rewriting, so the persisted Card is never mutated — exports (VCF/JSContact) and CardDAV
> keep the self-contained data URI, and the web edit round-trip (which PUTs the loaded Card back,
> media included) can't corrupt the stored photo (the T75 BeforeSave merge already re-derives the
> flat-owned photo entry from `Photo`/`PhotoThumbnail` regardless). The `?thumbnail=true` endpoint
> itself is untouched and still serves raw bytes.
>
> One deliberate deviation from a literal reading of the proposal: `ContactSummary.photo` (and
> the response's top-level `photo`) still carries the raw disk filename, not a URL — the proposal
> named only `photo_thumbnail` and Card `media[].uri`, and `photo`'s only web consumer is a
> truthiness gate. URL-ifying it is a one-line follow-up if a client ever needs the full photo URL
> at the top level.
>
> **§4 OIDC native return.** `GET /auth/oidc/login?client=android` sets an `oidc_client` cookie
> (scoped to the callback path, cleared like the other one-time cookies). The callback then, and
> only then, redirects to `mycorrhizal://oidc/callback?token=<jwt>&language=<lang>&date_format=<fmt>`
> on success — deliberately **without** setting the httpOnly `auth_token`/`id_token` cookies (a
> native client can't read a Custom-Tab cookie, and minting a browser session it can't manage is
> worse than none) — and to the same deep link with `error=<code>` on every failure path.
> state/nonce/PKCE are verified identically for both clients, and the token is only ever placed in
> a redirect when the `oidc_client=android` cookie is present (it can't be triggered by appending
> `client=android` to a callback URL the attacker can't otherwise authenticate). The web flow is
> byte-for-byte unchanged (`/login?error=…` failures, cookie + `/` success). The custom scheme
> `mycorrhizal://` is a backend constant matching the intent filter M5 §5 declares; no config knob
> was added (no consumer for one yet).
>
> **Tests.** Models unit tests for `ProfilePictureURL` derivation and both DTOs; a real-schema
> (`database.InitDB`) controller test pinning the list/detail URL shape, the photo-less omission
> (raw-JSON absence), the Card.Media read-path rewrite (and that it doesn't mutate the persisted
> Card), and that both exposed URL variants actually serve bytes over the unchanged endpoint. OIDC:
> login cookie set/no-set, android success deep-link (valid 3-part JWT + the user's real
> language/date_format + no auth cookies + one-time-cookie clearing), android provider-denied/
> state-mismatch/missing-PKCE deep-link errors, and non-android `client` values keeping the web
> flow. All new tests hand-verified to fail against the reintroduced bug before being restored.
> Full gates green (`go build/vet/fmt/test ./...`, frontend `tsc` + 714 vitest).
>
> **Review-pass finding, 2026-08-14** (fixed before landing). The read path exposes the relative URL
> in `Card.Media`, and the web client PUTs the loaded card back verbatim on every edit — so the URL
> would have **round-tripped into the persisted `contacts.card`** via `ApplyRecordToContact`'s
> `applyMedia` (which assigns `c.Card = r.Card` with the shared Media backing), breaking VCF/JSContact
> export and CardDAV (no external consumer can fetch a relative path). Fixed in `applyMedia`: a photo
> entry whose URI is neither embedded data nor a fetchable URL is recognized as this contact's own
> photo pointer and re-derived from the flat `Photo`/`PhotoThumbnail` instead of being persisted.
> Pinned by the model-level `TestApplyRecordToContact_ReDerivesRelativePhotoURL` (hand-verified to
> fail pre-fix) and the full real-schema controller round-trip
> `TestContactsDetail_PUTRoundTripDoesNotPersistPhotoURL`. A second review-pass finding: a photo
> entry in `Card.Media` that is *not* backed by a flat `Photo`/`PhotoThumbnail` (imported directly
> while `photoDir` was unavailable) has no profile-picture endpoint to point at — the rewrite now
> leaves its original URI untouched instead of blanking it
> (`TestNewContactRecordResponse_UnbackedMediaPhotoKept`).
>
> **Wire-shape consequences for consumers.** (1) Web avatars render the relative URL — resolves
> through the prod nginx `/api/` proxy (httpOnly cookie auth), but not through the bare Vite dev
> server (no `/api` proxy), a dev-only cosmetic regression; `ContactSummaryDTO.photo_thumbnail`
> became `?: string` in the TS type (the field is now genuinely absent for photo-less contacts,
> `/CLAUDE.md` frontend trap 8). (2) The Android client consumes this in M5 §3.1 (relative-URL
> resolution + Coil on the auth'd client); the OIDC deep link's consumer is M25's SSO login.

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
