# ADR 0034: Android passkeys — native Credential Manager, an operator opt-in gated on Digital Asset Links

- **Status:** proposed
- **Date:** 2026-09-29
- **Implements:** issue #1293 ("Android: passkey enrollment + login ceremony (WebAuthn parity)"), whose one
  genuinely open design question — RP ID / origin on Android — this ADR resolves.
- **Depends on:** issue #560 (passkey design pass: alternative second factor, hard-delete storage,
  `go-webauthn/webauthn`), #593 (backend ceremonies), #594 (web UI), #1306 / #1317 (proof-ceremony hardening),
  `docs/adrs/0022-distribution-variants.md` (one `applicationId`, three signing keys).
- **Related:** `docs/adrs/0029-deep-links.md` (its App Links analysis is the precedent; §"Why this is not that
  problem" below explains what carries over and what does not),
  `docs/adrs/0028-local-only-android-mode-and-server-profiles.md` (the Local profile has no domain, so it
  has no passkeys).

## Context

Passkeys are scoped to a **relying-party ID (RP ID) — a domain — not to an app.** Since #593 the server derives
the RP ID from the hostname of `FRONTEND_URL` and accepts exactly one origin, `FRONTEND_URL` itself
(`backend/services/webauthn.go`). That is enough for the web client. It is not enough for Android, where the
question is how a native app is allowed to act for a domain it does not own.

Verified against the Android documentation and the vendored library (`go-webauthn/webauthn` v0.18.2):

1. **Association is mandatory.** For Credential Manager to create or use a passkey for an RP ID, the domain must
   publish a Digital Asset Links file at `https://<rp-id>/.well-known/assetlinks.json` listing the app
   (`android_app` target: package name + signing-certificate SHA-256 fingerprint(s)) under the
   `delegate_permission/common.get_login_creds` relation. The file must be served `200` with
   `Content-Type: application/json`, **no redirects**, and the domain's `robots.txt` must let Google fetch
   `/.well-known/`. The docs frame the file as fetched by Google, so a domain Google's crawler cannot reach over
   HTTPS will not verify. There is no supported way for a non-privileged app to skip it.
2. **The ceremony origin is not an `https` origin.** For a native caller, `clientDataJSON.origin` is
   `android:apk-key-hash:<base64url-without-padding of the signing cert's SHA-256>`. The RP ID in the
   authenticator data stays the domain.
3. **The server library supports this directly.** `go-webauthn` v0.18.2 has `Config.RPOpaqueOrigins` for
   "opaque origins which do not carry an authority" such as `android:apk-key-hash:…`; they are compared by
   exact string match and are never matched against the top-origin. `RPOrigins` (the `https` set) is
   untouched, so enabling Android cannot widen what the web client accepts.
4. **Web is unaffected by the choice.** A passkey is a credential for the RP ID, so one created natively is the
   same credential the web app sees (subject only to the user's credential-provider sync). The existing web
   ceremony, `PasskeySettings`, and the credential list need no change.

### Why this is not the App Links problem (ADR 0029)

ADR 0029 rejected verified App Links for two reasons. Reading them against passkeys:

- **"There is no single host" — does not carry over.** App Links are verified for hosts *declared in the APK's
  manifest at build time*, which the shipped APK cannot know. A passkey request instead carries the RP ID **at
  runtime**, supplied by the server's own `begin` response. The app needs no build-time knowledge of any host,
  and the file lives on the operator's own domain, which the operator already controls.
- **"Three signing keys" — carries over, but is solvable.** Every operator's file must list all channel
  fingerprints (project keystore, F-Droid, Play App Signing — one `applicationId`, ADR 0022). Asking each
  operator to hand-write that, and to notice a rotation, is what ADR 0029 rejected. The difference here is
  that **the server can generate the file itself** from a fingerprint list the project ships, so the operator
  does not author or maintain it.

### The audience split

Some operators run publicly reachable HTTPS instances; many run `http://192.168.x.x` or a private VPN /
split-horizon name that Google cannot reach. For the latter group native Android passkeys **cannot work**
(criterion 1) — and web passkeys already cannot either: a raw IP is not a valid RP ID and plain HTTP is not a
secure context, so `NewWebAuthn` and the browser already refuse them. The design must therefore make native
passkeys an *explicit, configuration-gated capability* that is absent, cleanly, wherever it cannot function —
not a feature that appears in the UI and then fails.

## Decision

### 1. Native Credential Manager, opt-in per instance; no Custom-Tab fallback

Android uses the platform Credential Manager (`androidx.credentials`) for both enrollment and login, against
the **same** `/webauthn/register/*`, `/webauthn/login/*`, and `/webauthn/credentials` endpoints the web client
uses. No new ceremony endpoints and no client-side origin logic.

Where the operator has not enabled it (or cannot), Android does **not** get a browser-handoff ceremony. Such an
account keeps the factors that already work on Android — TOTP and recovery codes — and the client shows a clear
state instead of an unsatisfiable field (Decision 4). A Custom-Tab / deep-link session-handoff route was
considered and rejected: it adds a new session-issuance path (a one-time code exchange) whose security review
costs more than the audience it serves, and LAN / VPN deployments already have a network boundary plus biometric
app lock (ADR 0014) and TOTP.

### 2. Configuration: an explicit switch, validated against `FRONTEND_URL`

- **`WEBAUTHN_ANDROID_ENABLED`** (bool, default `false`, restart required). The registry description must say,
  plainly, that this exists **only for publicly reachable HTTPS instances**: Google must be able to fetch
  `https://<FRONTEND_URL host>/.well-known/assetlinks.json`, and that `FRONTEND_URL` must be the public URL,
  because its hostname becomes the RP ID.
- **`WEBAUTHN_ANDROID_CERT_SHA256`** (comma-separated SHA-256 fingerprints, optional). *Additive* to the built-in
  project channel fingerprints (Decision 3); for self-built or debug APKs and forks that sign with their own key.
- **No separate "external URL" variable.** `FRONTEND_URL` already is the operator-declared external origin and is
  already the WebAuthn RP source (its registry text says so, #1316). A second URL would be a second way to get
  the RP ID wrong. The app's *connection* URL and the RP ID may legitimately differ (an app on the LAN talking to
  an instance whose `FRONTEND_URL` is its public name): the RP ID comes from the server's `begin` response, and
  only the RP-ID host needs to be publicly reachable.
- **Validation, computed once at startup.** The feature is *effective* only when the switch is on **and**
  `FRONTEND_URL` is `https`, has a domain-name host (not an IP literal, not `localhost`, not a single label, not
  a `.local` / `.localhost` / `.lan` / `.internal` / `.home.arpa` name), **and** at least one fingerprint is
  known. If the switch is on but the check fails, the server logs one `ERROR`-level line naming the failed rule
  and the feature stays off — **it does not refuse to boot**, because a misconfigured optional capability must
  not take the instance down. This validation is a best-effort denylist of obviously-unreachable names, not a
  reachability proof; the config text says so.

### 3. Server surface

When effective:

- The server serves `GET /.well-known/assetlinks.json` — `200`, `Content-Type: application/json`, no auth, no
  redirect, no cookies — generated from the built-in project fingerprints plus
  `WEBAUTHN_ANDROID_CERT_SHA256`, for the app's `applicationId`, relation
  `delegate_permission/common.get_login_creds`. When not effective the route returns **404** (never an empty or
  placeholder statement).
- `NewWebAuthn` adds one `RPOpaqueOrigins` entry per fingerprint (`android:apk-key-hash:` + base64url of the raw
  SHA-256 bytes, unpadded). `RPOrigins` is unchanged.
- `/health` `capabilities` gains a token (e.g. `webauthn_android`) present **only** when the feature is
  effective. Clients gate on token presence, as they already do for other surfaces (ADR 0028).
- **No new outbound network client.** The DAL fetch is performed by Google / the credential provider, not by
  this server, so `docs/int-01-integration-classification-matrix.md` gains no row. An admin-side "does Google
  see my file?" diagnostic (which *would* be an outbound call disclosing the hostname) is deliberately out of
  scope; if ever wanted it needs its own INT-01 row and an opt-in.
- **The all-in-one image's nginx proxies only `/api/`, `/carddav/` and `/.well-known/carddav` to the backend**
  (`docs/deployment.md`); anything else falls through to the SPA's `index.html` with a `200`. `/.well-known/
  assetlinks.json` must be added to the proxied set, and the deploy-smoke check must assert the response is
  JSON, not HTML. External reverse proxies must forward the same path; `docs/deployment.md` documents it.

The built-in fingerprint list lives in one reviewed file (one entry per release channel, with a comment naming
the channel and where the value came from). **These values are supplied by the maintainer from the real signing
material (project keystore, Play Console's Play App Signing certificate, F-Droid's published key); they must
not be guessed or generated.** Until an entry exists for a channel, that channel's builds work only via
`WEBAUTHN_ANDROID_CERT_SHA256`.

### 4. Client behaviour (Android)

- **`methods` is parsed** from the `POST /login` two-factor response; absent or unknown → today's behaviour
  (TOTP prompt), so an older server regresses nothing.
- **Capability gate.** The passkey UI (enrollment and "Use a passkey") is offered only when the active **Remote**
  profile's `/health` reports the capability token **and** Credential Manager is usable on the device. Local
  (embedded) profiles never offer it — there is no domain.
- **Graceful degradation** when the account's `methods` includes `webauthn` but the gate is closed (server
  capability absent, or no Credential Manager provider): show "use a recovery code — passkeys aren't available
  on this app for this server", not a TOTP field the user cannot satisfy. This state ships **before** the
  ceremony does and stands on its own.
- **Runtime failure maps to the same state.** A Credential Manager error indicating the app is not associated
  with the RP ID (the DOM-exception class the troubleshooting guide attributes to a bad asset-link file) is
  surfaced as a distinct "this server isn't set up for Android passkeys" message and drops to the degraded
  state; it is not shown as a generic error.
- **Removal proof** reuses the web contract, including `exclude_id` (#1317).
- **OIDC-provisioned users** cannot enroll, matching the backend's `oidcUserErr`.
- The **`foss` (F-Droid) flavor** must not pick up a Google Play services dependency for this. It uses the
  platform Credential Manager where one exists (Android 14+ with a provider) and otherwise degrades under the
  same gate. Whether the `androidx.credentials` core artifact alone suffices on that flavor is a first task of
  the ceremony slice.

### 5. Security posture

- The passkey private key stays in the platform authenticator / credential provider — never app storage
  (recorded in `docs/security/masvs-l1.md`).
- Trusting an Android origin means trusting **any app signed with a listed key** to relay a ceremony for that
  RP ID. That is the same trust root the channels already have, listed explicitly and reviewably; a fork that
  signs with its own key opts in via `WEBAUTHN_ANDROID_CERT_SHA256` and only on its own instances.
- `RPOpaqueOrigins` never widens `RPOrigins`, and the ceremony store, single-use challenges, user verification
  and proof-ceremony rules from #593 / #1306 / #1317 are unchanged.
- `docs/security/asvs-l2.md` (2.7.x / 2.8.x) and `threat-model.md` are updated in the implementing PR; the
  citation gates apply.

## Consequences

- **Web:** no behavioural change. `RPOrigins` and the web UI are untouched; Android-created passkeys simply
  appear in the existing credential list.
- **Operators:** public-HTTPS operators get native Android passkeys by setting one switch and making sure
  `/.well-known/assetlinks.json` reaches the backend. Everyone else sees no change, no dead UI, and no new
  failure mode.
- **LAN / VPN users** keep TOTP, recovery codes, biometrics and their network boundary; nothing in this design
  pretends otherwise.
- **The maintainer** owns keeping the built-in fingerprint list current (a new release channel or a signing-key
  rotation is a one-line change plus a release note), instead of every operator owning it.
- **Cost accepted:** Google-side association is outside our control and not testable in CI; the design leans on
  a clear, distinct failure state and on an instrumented check that runs only where a real provider exists.

## Alternatives considered

- **Custom Tab ceremony against the web origin, session handed back over `mycorrhizal://`.** Works on any
  reachable host and reuses web enrollment, but introduces a new session-handoff code exchange and a browser hop
  for every login. Rejected (Decision 1); revisit only if LAN passkeys become a real requirement.
- **Operator-authored `assetlinks.json`.** Rejected: pushes fingerprint upkeep onto every operator, the exact
  failure ADR 0029 named.
- **Server-side "is this public?" detection.** Rejected: the server cannot know whether Google can reach it; a
  wrong guess is worse than an explicit switch plus a validated denylist plus a clear runtime failure state.
- **Doing nothing beyond `methods` parsing and degradation.** Removes the lockout dead-end but leaves passkeys
  unusable on Android. Kept as the *floor* (Decision 4), not the goal.

## Implementation

Three slices, in dependency order (S1 and S2 are independent and can run in parallel):

1. **S1 — Android, no backend:** parse `methods`; the degradation state (server capability absent → recovery-code
   guidance); `ApiClient` calls for the WebAuthn endpoints; five-locale strings.
2. **S2 — Backend:** `WEBAUTHN_ANDROID_ENABLED` / `WEBAUTHN_ANDROID_CERT_SHA256` config + startup validation,
   `RPOpaqueOrigins`, `GET /.well-known/assetlinks.json`, the `/health` capability token, nginx proxy + deploy
   smoke, `docs/deployment.md`, configuration reference regeneration, ASVS rows, and a virtual-authenticator
   ceremony test using an `android:apk-key-hash:` origin. **This ADR ships with S2.**
3. **S3 — Android ceremony:** Credential Manager enrollment UI (mirroring `TwoFactorScreen` /
   `PasskeySettings`), "Use a passkey" login, removal proof with `exclude_id`, the OIDC exclusion, the runtime
   association-failure state, MASVS / threat-model updates. Credential Manager is mocked at the credential layer
   in JVM tests; a real ceremony is never attempted in a unit test.
