# ADR 0044: Passwordless passkeys — a discoverable credential is a first factor

- **Status:** accepted
- **Date:** 2026-11-16
- **Implements:** discussion #2355 ("Why do you still have to fill in username and password when
  logging in with a passkey?").
- **Depends on:** issue #560 (passkey design), #593 (backend ceremonies), #594 (web ceremony UI),
  ADR 0034 (Android passkeys and the RP ID/origin model).
- **Supersedes:** the single-factor prohibition in `asvs-l2.md` V2.8.7 for passkeys that complete a
  user-verified ceremony.

## Context

Since #593 a passkey has been an **alternative second factor**: `POST /login` verifies a password,
mints a short-lived `2fa_pending` challenge, and `/webauthn/login/*` exchanges that challenge for a
session. That design deliberately kept a password in front of every passkey so a passkey could never
be the only factor.

The result is a passwordless credential that still demands a password. A user asking for passkey
sign-in expects the platform's "Sign in with a passkey" prompt on the login page, not after typing a
username and password. It also means a user who forgets their password cannot recover with the
passkey they still hold.

The existing registration already asks the authenticator for a resident (discoverable) credential
(`ResidentKeyRequirementPreferred`), and the library (`go-webauthn` v0.18.2) supports
`BeginDiscoverableLogin` / `ValidatePasskeyLogin`, so the server can resolve the account from the
credential the authenticator returns without asking for an identifier.

## Decision

### 1. A user-verified discoverable passkey is a complete credential

Add a passwordless sign-in path:

- `POST /api/v1/webauthn/login/passwordless/begin` calls `BeginDiscoverableLogin` with
  `userVerification: required` and returns the WebAuthn request options plus a one-use
  `ceremony_token`.
- `POST /api/v1/webauthn/login/passwordless/finish` takes `{ceremony_token, assertion}`, consumes the
  ceremony, resolves the **credential id to its stored owner**, and validates the assertion. On success
  it mints the same session cookie as `/login`, `/login/2fa` and the existing passkey path.

A passkey that completes this path mints a session **even when the account also has TOTP enrolled**.
A passkey is a possession factor unlocked by a local biometric/PIN (user verification), which is at
least as strong as the password+TOTP path it replaces; requiring TOTP as well would make the user
carry two authenticators for one login and would still leave the password as the weaker credential.
A user who wants two independent factors can keep using password + TOTP, or can use the existing
password + passkey second-factor path; the new path is additive.

User verification is **required**, not merely preferred. Without it the assertion would be a bare
possession factor and would not justify replacing a password.

### 2. Resolution is by credential id, never by the client-supplied user handle

The begin step does not know the user. On finish the handler looks the credential id up in
`webauthn_credentials`, loads that row's owner, and returns it to `go-webauthn`; the library then
checks the authenticator's user handle against `WebAuthnID()` for that owner. A forged user handle
therefore cannot select a different account, and an unknown credential resolves no account at all.
The credential row is the ownership boundary, matching the rest of the WebAuthn surface.

### 3. The ceremony token is server-side, one-use and never a cookie

The existing ceremony store is keyed by `(purpose, user id)`, which is unavailable at begin. The
passwordless ceremony uses a random 256-bit token as the key and returns it in the begin response;
finish must echo it and the token is consumed before validation, so a captured response cannot be
replayed. It is not a session: it authorizes only the one challenge, and the real session is still
issued only after a verified assertion.

### 4. The existing second-factor path stays

`/webauthn/login/begin|finish` are unchanged. Accounts whose passkey is a non-discoverable security
key (allowed because registration asks for resident keys but does not require them) can still use
password + passkey. The web login page offers "Use a passkey instead" on the 2FA step and
"Sign in with a passkey" on the credentials step; the latter fails with a clear message if the
authenticator has no discoverable credential for this relying party.

### 5. Protections preserved

- Same per-account, per-IP lockout as `/login` and `/login/2fa`; a failed assertion is audited.
- Clone detection (`CloneWarning`) is enforced exactly as on the second-factor path.
- One-use challenge, short TTL, RP origin checks, and the same `FRONTEND_URL`-derived RP ID.
- `POST /webauthn/login/passwordless/finish` is a session-minting public route: it carries the auth
  rate limiter and the client-version floor, and is pinned in the session-minting route gate.

## Alternatives considered

- **Username first, then a passkey (no password).** Removes the password but keeps an enumeration
  surface and a form field; discoverable credentials make the identifier unnecessary.
- **Passkey replaces the password, but TOTP still follows.** No TOTP bypass, but mixed-factor users
  must complete two authenticators; rejected as the primary UX while keeping the old path available.
- **Only passkey-only accounts may sign in passwordlessly.** Preserves the old "never single-factor"
  rule for mixed accounts, but leaves the common TOTP+passkey user with the exact complaint that
  started this discussion.
- **Require resident keys at registration immediately.** Would force hardware security keys to
  support discoverable credentials or be refused; an additive login path with a clear failure is less
  disruptive, and new platform passkeys are resident by default.

## Consequences

- Web users get standard passwordless passkey sign-in. Android keeps its existing second-factor flow
  until it adopts the same two endpoints; the backend path is platform-neutral.
- A passkey can now be a single factor. `asvs-l2.md` V2.3.2/V2.8.7 and the threat model record that,
  with user verification required.
- Passkeys created by a non-discoverable authenticator still work, but only as a second factor.
- No schema change, no new persistent data, and no new cookie.
