# ADR 0029: Deep links — a custom `mycorrhizal://` scheme with a closed, navigation-only route set

- **Status:** accepted
- **Date:** 2026-09-26
- **Implements:** issue #384 ("A short design doc picks App Links vs custom scheme, enumerates the allowed
  intent set, and specifies the validation/allowlist rules. Implementation is a follow-up."). This ADR is
  that design doc; the follow-up work is filed on the v1.3.0 milestone (see "Implementation").
- **Depends on:** issue #965 (OIDC native return: PKCE-bound exchange code instead of a JWT in the
  custom-scheme URI), issue #152 / #679 (notification deep links, `deepLinkRoute`),
  `docs/adrs/0014-local-app-lock-and-biometric-resume.md` (the app-lock gate every deep link must pass),
  `docs/adrs/0022-distribution-variants.md` (three signing keys for one `applicationId`).
- **Related:** `docs/adrs/0028-local-only-android-mode-and-server-profiles.md` (links resolve against the
  active server profile; a profile switch drops a pending link),
  `docs/adrs/0034-android-passkeys.md` (Digital Asset Links **are** used there, for a different reason: a
  passkey's RP ID arrives at runtime rather than from the manifest, and the server can generate the
  fingerprint list, so the two objections below do not apply to it).

## Context

The Android app already has a `mycorrhizal://` URI surface, in two halves that grew separately:

| Surface | How it arrives | Parser | Exposure today |
|---|---|---|---|
| `mycorrhizal://oidc/callback?code=…&state=…` | `VIEW` intent-filter on `MainActivity` (`exported`, `singleTask`) | `parseOidcReturn` — scheme **and** host **and** path | Public: any app or browser can fire it. Safe because the code is PKCE-bound and state-checked (#965). |
| `mycorrhizal://contacts/{id}`, `…/activities`, `home`, `circles\|tags\|households/{id}` | a string extra (`NotificationBuilder.EXTRA_DEEP_LINK`) on the launch intent of a notification's `PendingIntent` | `deepLinkRoute` — closed allowlist, unknown → `null` | De facto public: `MainActivity` is exported, so any app can send an explicit intent carrying the extra. No intent-filter advertises it. |

The backend emits the second shape itself (`services/notification_service.go` puts
`deep_link = mycorrhizal://contacts/<id>` into FCM data). The web client has no URI surface beyond its own
routes (`/contacts/:id`, `/search?q=`), and its Web Push `notificationclick` handler always opens `/`.

Issue #384 asks whether to grow this into a general deep-link surface other apps can call — and, first,
whether it should be a custom scheme or verified Android App Links. The issue's stated default is App
Links "unless there's a concrete reason not to."

### Why App Links do not fit a self-hosted app

Android App Links work by the OS fetching `https://<host>/.well-known/assetlinks.json` for **every host
declared in the APK's manifest** and checking it lists the installing APK's signing-certificate
fingerprint. Both halves are fixed at build time. For this project:

1. **There is no single host.** Every operator runs their own server on their own domain
   (`docs/supported-versions.md`). The one APK every user installs (Obtainium, F-Droid, Play) cannot declare
   hosts it has never heard of, and `autoVerify` cannot be added at runtime. Android 15's "dynamic App
   Links" only let an already-declared host adjust paths server-side; they do not add hosts.
2. **There are three signing keys** (ADR 0022: the project keystore, F-Droid's key, Google's Play App
   Signing key). Every operator's `assetlinks.json` would have to list all three fingerprints, and would
   silently break when F-Droid rotates or a fourth channel is added.
3. **A project-owned relay domain** (`https://links.<project-domain>/contacts/42`, verified once, with the
   app mapping it to the user's own server) is the only way to get one verified host. It was rejected:
   the project owns no domain or hosting (a hobby project with no infrastructure budget — the same stance
   as ASVS P8), a link that misses the app lands on a third-party page that would learn the contact id and
   the clicking device, and the link still carries no information about *which* instance it points at.

So the concrete reason exists: verified App Links are not achievable for a self-hosted server whose domain
the shipped APK does not know. The residual risks App Links would have removed — hijacking and spoofing —
are instead neutralised by **what the links are allowed to do**, which is the real subject of this ADR.

## Decision

### 1. Scheme: keep `mycorrhizal://`, and design every route as if it were hijackable and forgeable

`mycorrhizal://` stays the one and only app-level scheme. There is no second scheme (no `mcrm://`, no
`intent://` grammar of our own). Two threat assumptions are **normative** for every current and future
route, because a custom scheme cannot be protected from them:

- **Hijack (outbound):** another installed app may register `mycorrhizal://` and receive a link we emit or
  the user taps. Therefore **no route may carry a secret or personal data.** Opaque ids only — never a name,
  phone number, email, note text, or token. (The OIDC callback is the existing exception, and it is only
  safe because what it carries is useless without the on-device PKCE verifier. No new route may rely on a
  similar binding; a route that would need one is out of scope for deep links.)
- **Spoof (inbound):** any app may fire any `mycorrhizal://` URI, with any values, at the exported
  `MainActivity`, by intent-filter or by explicit component. Therefore **no route may change state.** A
  deep link can only *navigate* (or prefill a form the user must still confirm and save by hand). The worst
  a forged link can do is show the user a screen of their own data that they would have been able to open
  anyway.

### 2. The allowed intent set (v1) — a closed, shared grammar

Routes are expressed as one grammar with two renderings: the Android custom-scheme form, and the web path
on the user's own server. The `mycorrhizal://` form's authority-plus-path is **the same string** as the web
path (with `home` standing for `/`), so a route is written once and never re-invented per client.

| Route | Android (`mycorrhizal://…`) | Web (`https://<server>…`) | Params |
|---|---|---|---|
| Home / dashboard | `home` | `/` | — |
| Contact detail | `contacts/{id}` | `/contacts/{id}` | `id`: positive integer, ≤ 2³¹−1 |
| Contact activity timeline | `contacts/{id}/activities` | degrades to `/contacts/{id}` | as above |
| Search | `search?q={q}` | `/search?q={q}` | `q`: see §3 |
| Circle / tag / household | `circles/{id}`, `tags/{id}`, `households/{id}` | degrades to `/circles`, `/circles?tab=tags`, `/households` | `id`: opaque id, see §3 |
| OIDC native return | `oidc/callback?code&state[&language&date_format]` | *(not a web route)* | auth-only; not navigable; unchanged from #965 |

The user-facing rendering of this table — and the "a link opens only after you unlock the app"
guarantee — is the [Deep links section of the Android app page](../android-app.md#deep-links).

Rules for the set:

- **Closed.** An unknown host, unknown path, extra path segment, or malformed parameter resolves to
  **nothing** — not to a default screen, not to home, not to an error page that echoes the input. The app
  simply opens (or stays on) whatever it would have shown without the link.
- **Degrade to the nearest parent, per client.** A client that lacks a route's exact screen opens its
  nearest ancestor listed in the table (never an unrelated screen). The web has no per-circle page, so
  `circles/{id}` opens `/circles` there.
- **Instance-local meaning.** An id is meaningful only on the server that minted it and only within the
  signed-in user's own scope. The app resolves it through the normal, `user_id`-scoped API: another user's
  id produces the ordinary not-found state, never a leak. Deep links are therefore **not** a cross-user
  sharing mechanism (that is contact shares), and none of them names the server. Under ADR 0028's server
  profiles, a link resolves against the **active** profile (a `Local` profile included) and never selects
  or switches a profile itself.
- **Adding a route** is an edit to this table, the shared test vectors (§5), and each client's parser in the
  same PR — and must satisfy §1 (no personal data, navigation-only). "Do anything" routes (`action=…`,
  `route=<arbitrary nav string>`, a pass-through of an arbitrary NavHost route) are forbidden by design.

Explicitly **not** in the set: create/edit/delete screens reached directly (`contacts/new`,
`…/edit`, `…/notes/new`), settings, admin, account, import, merge — anything whose next tap is a write
without the user having navigated there themselves.

### 3. Input validation — every value is untrusted

- **Scheme, host, path are matched exactly** against the table, the way `parseOidcReturn` already matches
  scheme **and** host **and** path. Case-sensitive; no percent-decoding tricks (match on
  `Uri.getPath()`'s decoded segments, reject any segment that is empty, `.` or `..`, or contains `/`
  after decoding).
- **Integer ids:** decimal digits only, no sign, no leading `+`, no leading zeros, `1..2147483647`
  (fits the NavHost `IntType` arg). Today's `toIntOrNull()` accepts `+42` and `042`; the parser is tightened
  to the strict form.
- **Opaque string ids** (circle/tag/household): `[A-Za-z0-9._:-]{1,128}`. Today's check is only "non-blank,
  no `/`".
- **Search `q`:** trimmed; Unicode control and format characters (`Cc`, `Cf`) stripped; at most 200
  characters after that (longer is truncated, not rejected); an empty result opens the plain list. `q` is
  placed into the search field and run through the normal, scoped search endpoint — never interpolated into
  a route string, an SQL fragment, or a log line. The *caller* chose `q`, so carrying it inbound does not
  violate §1; the app **never emits** a `search` link itself.
- **Unknown query parameters are ignored**, not forwarded. Only the parameters in the table are read.
- **Deep-link URIs are never logged** (not to logcat, not to the backend's operational events), since a
  third-party caller may have put personal data into `q` despite §1.

### 4. Exported-component hygiene and stale intents

`MainActivity` remains the single exported entry point (`singleTask`); no new exported activity, alias, or
receiver is added for deep links. Handling is hardened so that an intent can only take effect once, while
the right user is signed in and unlocked:

- **One entry, one parser.** The notification extra and the public `VIEW` intent go through the same
  `deepLinkRoute` allowlist. The extra grants nothing the public intent does not; it exists only because a
  notification's launch intent is the natural carrier.
- **Consume once.** In `onCreate`, handle the intent only when `savedInstanceState == null` (a language
  change recreates the activity — M25 — and must not replay the launch intent's link), and ignore any
  intent carrying `FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY` (reopening from Recents re-delivers the original
  intent). After handling, the link is cleared from the activity's intent (`setIntent` with the data and the
  extra removed). The same rule applies to the OIDC return, so a replayed callback can never surface a
  spurious "login failed" snackbar.
- **Gate, then navigate.** A pending link is held until the main tree composes — i.e. after login, the
  server-compatibility gate, and the ADR 0014 app lock — exactly as today. Added: the pending link is
  **dropped** on logout, on a session for a different user or server (including an ADR 0028 active-profile
  switch), and if it is older than 10 minutes
  when the gate finally opens. A link fired while signed out must not wait indefinitely and then open in a
  different account.
- **Never ahead of the lock.** No deep-link handling code path renders data, prefetches the target, or
  shows the target's title (e.g. in a snackbar or the task description) before the app-lock gate is open.
  `FLAG_SECURE` already keeps the Recents thumbnail blank.
- The public `VIEW` intent-filter lists each allowed host explicitly with `BROWSABLE` + `DEFAULT`; it does
  **not** use a scheme-only filter. (The filter is advisory — explicit-component intents bypass it — which
  is why the parser, not the manifest, is the security boundary.)

### 5. Cross-client consistency is enforced, not remembered

The grammar in §2 is committed once as **test vectors** (`testdata/deep-links/vectors.json`, hand-authored,
following the `testdata/phonekey-vectors/vectors.json` precedent that `PhoneKeyVectorTest.kt` and
`backend/models/phonekey_vectors_test.go` already share): each vector is an input URI plus the expected
Android route, the expected web path, or "rejected". Three suites consume the one file:

- Android `NotificationDeepLinkRouteTest` (`app/src/test/…`) — `deepLinkRoute` over every vector.
- Web vitest — the service-worker `notificationclick` target resolver over every vector's web form.
- Backend — every URI the server **emits** (notification payloads) must appear as an accepted vector, so the
  server cannot start emitting a link the clients reject.

A route added to one client without a vector, or a vector one client fails, breaks that client's build.

### 6. Share-to-CRM is an intent, not a deep link

"Share to CRM" arrives as `ACTION_SEND` (`text/plain`), not as a `mycorrhizal://` URI, and follows the same
two rules: the shared text is untrusted input, and it only **prefills** a note draft; the user picks the
contact and presses save. It is handled by the same exported `MainActivity` (a second intent-filter), under
the same consume-once and gate-then-navigate rules. Shared `text/x-vcard` / `text/vcard` is **out of scope**
here: importing a card is a write with its own review flow (the import wizard) and is a separate decision.

## Consequences

- Third-party apps (launchers, automation tools, notes apps, a browser bookmark) can open a contact, run a
  search, or reach the dashboard — the "call into the CRM" capability #384 asked for — without gaining any
  ability to read data out of the app or change it.
- Link hijacking is accepted as a residual risk rather than prevented. Its impact is bounded by §1: a
  hijacker learns an opaque integer id from an instance it does not know.
- The OIDC return keeps relying on PKCE rather than on channel security. If it ever regresses to carrying a
  bearer credential, that is a violation of this ADR, not just of #965.
- The web client gains a small but real behaviour change: a Web Push notification about a contact opens that
  contact rather than `/`.
- `docs/security/masvs-l1.md` PLATFORM-2, PLATFORM-3 and STORAGE-6 are updated by the implementing PRs to
  cite the new parser, vector suite and intent-filters; `docs/security/threat-model.md` gains the
  hijack/spoof entries with their §1 mitigations.

## Alternatives considered

- **Verified Android App Links on each operator's domain.** Rejected: impossible for the shipped APKs (hosts
  and signing fingerprints are fixed at build time; see Context). A self-builder *could* bake their domain
  into their own APK via a manifest placeholder and serve `assetlinks.json` from their instance; that is a
  possible later, opt-in addition for operators who build from source, and it would reuse this ADR's grammar
  unchanged (`https://<their-host>/contacts/42` is already the web rendering). Not scheduled.
- **A project-owned relay domain with App Links.** Rejected: requires infrastructure the project does not
  have, and leaks ids and click metadata to that host when the app is not installed.
- **No public deep links at all** (keep the notification extra private by moving notification taps to a
  non-exported trampoline activity). Rejected as the end state: it closes the spoofing surface but also
  closes the capability #384 asks for, and once routes are navigation-only the spoofing surface is
  harmless. The hardening in §4 gives the same "no replay, no state change" guarantee without a second
  component.
- **Generic "open any route" links** (`mycorrhizal://navigate?route=…`). Rejected: it turns every screen,
  including write screens, into an externally reachable target, and couples third parties to internal
  NavHost route strings.
- **Opaque UUIDs (`VCardUID`) instead of integer contact ids in links.** Deferred: integers are what both
  clients' routes and the backend's notification payloads already use, and scoping makes them safe. If
  links are ever meant to survive export/import between instances, switching the `contacts/{id}` parameter
  to the `VCardUID` is the upgrade path; it would be a new route (`contacts/uid/{uid}`), not a redefinition.

## Implementation

Filed on the v1.3.0 milestone (dependency chain and PR grouping are in each issue's comments):

- #1267 — shared grammar test vectors + backend emitter conformance (§5). Blocks the rest.
- #1268 — Android: harden existing deep-link handling: consume-once, strict parsing, pending-link expiry (§3–§4).
- #1269 — Android: public `VIEW` intent-filter for the allowed route set, including `search` (§2).
- #1270 — Web: a Web Push notification click opens the linked route (§2, §5).
- #1271 — Android: "Share to Mycorrhizal" (`ACTION_SEND text/plain` → prefilled note draft) (§6).
