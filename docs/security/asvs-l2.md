# Security checklist: OWASP ASVS L2 + API Security Top 10 (2023)

The living answer to "is this secure?" — a per-control map of two recognized standards to the
code and tests that satisfy them. If a control's status changes, the PR that changes it updates
this file. A security-sensitive PR should touch the relevant row here; if it cannot point at a
row, it does not know what it is changing.

| | |
|---|---|
| **Standards pinned** | OWASP ASVS 4.0.3 (V1–V14), OWASP API Security Top 10 (2023) |
| **Level** | ASVS **Level 2** (rows marked `✓` in the L2 column of the ASVS). L1 rows are included because L2 subsumes them; L3-only rows are listed per chapter as out of scope. |
| **Last full pass** | 2026-08-23 (audit of backend + frontend + CI + deployment) |
| **Scope** | Single-process Go/Gin + SQLite app, React SPA behind an nginx container, TLS terminated by an operator-supplied reverse proxy. Many cloud/microservice controls are deliberately `not-applicable` — each such row says why. |

## Status legend

| Status | Meaning |
|---|---|
| **satisfied** | Control is met; the row cites `file:line` or a test name. |
| **partial** | Control is met in part, or met with a documented deviation (each `partial` row names the gap). |
| **not-applicable** | Control does not apply (single-process, self-hosted, no cloud, no XML, …) — one-line reason given. |
| **out-of-scope** | L3-only control (not in the L2 target); listed for grep-ability. |

"Answered in the doc" means: take any `Vx.y.z` from ASVS 4.0.3 or `API#` from the 2023 Top 10,
grep for it below, and get a status + citation. No row is left `satisfied` without a citation.

## Documented positions

Two deliberate, written-down decisions the issue asks to fold in. They are positions, not code
changes; revisit each pre-1.0 or when the cited trigger happens.

### P1 — Password hashing: bcrypt, not Argon2id (NIST 800-63B §5.1.1.2)

Passwords are hashed with **bcrypt at cost 10** (`bcrypt.DefaultCost`) in the single shared
`services.HashPassword` (`backend/services/user_service.go:25`). Per NIST 800-63B the approved
memory-hard functions are Argon2id, scrypt, or bcrypt with a work factor of 10+ — bcrypt at 10
**satisfies** 800-63B as written (ASVS 2.4.4, min. work factor 10). Details:

- Salt: bcrypt's per-hash 128-bit random salt (ASVS 2.4.2).
- No silent truncation: passwords over 72 bytes are **rejected** with `ErrPasswordTooLong`
  (`backend/services/user_service.go:14,21-23`), so the bcrypt 72-byte cap is explicit, not a
  quiet truncation (ASVS 2.1.3).
- Deviation: ASVS 2.1.1 wants ≥ 12 characters; we enforce `min=8` + ≥ 50 bits of entropy
  (`backend/middleware/password_strength.go:21-45`), which is 800-63B's actual rule (length +
  guessability, not composition). Stronger-than-8 passwords are encouraged by the strength meter
  (`backend/controllers/user_controller.go:278-290`).

**Why not Argon2id yet:** bcrypt at 10 is adequate for this threat model (self-hosted, per-account
lockout, no federated login at rest), and migrating hashes is not a code change — it requires a
forced re-hash or password reset for every existing user, which is a data-adjacent decision for a
product with real production data (see the v0.2.0 note in `CLAUDE.md`). Decision: adopt Argon2id
in the same release that introduces a password-hash version field and a rehash-on-login path;
until then bcrypt cost is the knob (currently the library default 10; raising it is a one-line
change in `services/user_service.go`).

### P2 — Cryptographic agility (bcrypt/JWT are direct calls, not behind an abstraction)

`bcrypt` (password hashing) and HS256 `JWT` (sessions) are called directly at their call sites
(`backend/services/user_service.go:25,54`, `backend/middleware/auth.go:66-98`) rather than behind
a swap-able algorithm interface. This is a **conscious pre-1.0 decision**, not an oversight:

- Swapping either algorithm today is a code change plus a credential-rotation event, and the
  rotation machinery already exists: JWT keys rotate via `TokenVersion` + the JWT secret
  validation in `config/config.go:351-379` (all sessions die on bump), and bcrypt hashes can
  migrate via the P1 rehash-on-login path.
- ASVS 6.2.4 (algorithms swappable) is therefore marked **partial** below, with this position as
  the cited reason.
- Decision: introduce an abstraction **only** when a second algorithm is actually adopted
  (Argon2id, EdDSA sessions, or external KMS). An abstraction with one implementation is dead
  weight and a review surface.

Related at-rest note: the TOTP secret and integration credentials are AES-256-GCM encrypted with a
key HKDF-derived from `JWT_SECRET_KEY` (`backend/services/credential_crypto.go:22-29`). Rotating
`JWT_SECRET_KEY` therefore invalidates those stored secrets (documented at `credential_crypto.go:20-21`);
the ≥ 32-byte length plus placeholder/entropy rejection at boot (`config/config.go:351-379`) is what
keeps that key strong. TOTP/recovery secrets are generated at runtime from `crypto/rand` (no
env-configurable default to guard), and the credential key is derived from `JWT_SECRET_KEY`, so the
JWT checks cover every security-critical secret that can be configured (issue #393).

---

## V1 — Architecture, Design and Threat Modeling

L3-only, out of scope: 1.11.3.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 1.1.1 | Secure SDLC | satisfied | Issues-first backlog, one branch per concern, CI gates — `CLAUDE.md` (Workflow), `.github/workflows/` |
| 1.1.2 | Threat modeling per design change | partial | No formal per-sprint threat model; threat analysis lives in ADRs (`docs/adrs/`) and the 14-finding security review that all findings were patched (see Security posture, `CLAUDE.md`). Gap: no standing threat-model step. |
| 1.1.3 | User stories carry security constraints | satisfied | Issues carry explicit acceptance/security criteria, e.g. the "verified by" gates in this repo's security tickets (see e.g. T75/T26 records in `CLAUDE.md`). |
| 1.1.4 | Trust boundaries documented | satisfied | `docs/deployment.md:17` (TLS at external reverse proxy, nginx→app loopback); Cookie/`COOKIE_SECURE` boundary in `.env.example:110-122`; CORS boundary `backend/main.go:191-209` |
| 1.1.5 | High-level architecture + remote services analyzed | satisfied | `docs/deployment.md`, `docs/adrs/`, `docs/data-model.md`; remote-service clients (CardDAV/WebDAV, Immich, Seafile, Resend/SMTP) each have their own guarded client (`backend/services/immich_client.go`, `seafile_client.go`, `contact_sync_service.go`, `mailer.go`) |
| 1.1.6 | Centralized, reusable security controls | satisfied | Single auth middleware `backend/middleware/auth.go`; single validation middleware `backend/middleware/validation.go`; single rate-limiter `backend/middleware/rate_limiter.go`; single SSRF dialer `backend/httputil/safedial.go`; single error envelope `backend/errors/` |
| 1.1.7 | Secure coding checklist available | satisfied | `CLAUDE.md` (Backend/Frontend traps + Security posture) is the standing checklist all work is expected to read |
| 1.2.1 | Low-privilege OS accounts | satisfied | Non-root `appuser` in `backend/Dockerfile` + `docker/entrypoint.sh:4-5` (PUID/PGID configurable, default 1001); CIS-enforced in CI — `docker/cis-hardening.sh` (4.1 non-root, 4.8 setuid/setgid stripped) via `.github/workflows/container-hardening.yml` |
| 1.2.2 | Component-to-component comms authenticated | not-applicable | Single process; nginx→app is loopback (`docker/nginx.conf:7,25`); no microservices, so no mTLS surface |
| 1.2.3 | Single vetted auth mechanism | satisfied | One JWT HS256 verifier for all API routes (`backend/middleware/auth.go:66-98`, wired `routes/routes.go:52-55`); CardDAV Basic auth reuses the same password/API-token validation (`backend/carddav/auth.go:24-114`) |
| 1.2.4 | Consistent auth strength across pathways | satisfied | Every protected route shares `middleware.AuthMiddleware`; CardDAV/CalDAV Basic path enforces the same password policy + account lockout (`backend/carddav/auth.go:37-48`); OIDC is the only other pathway and lands on the same JWT cookie (`backend/controllers/oidc_controller.go:229-233`) |
| 1.4.1 | Access control at trusted enforcement point | satisfied | All authorization is server-side (`routes/routes.go:52-55`, per-controller `Where("user_id = ?")`); frontend route guards are UX only |
| 1.4.4 | Single access control mechanism | satisfied | Every handler resolves the user from context (`backend/controllers/helpers.go:30-44`) and AND-scopes every query by `user_id` — canonical example `backend/controllers/circle_controller.go:49` |
| 1.4.5 | Feature/attribute-based (not just role-based) | satisfied | Ownership is per-entity (`Contact.VCardUID`, `user_id`) not role-based; admin is a separate gate (`backend/middleware/admin.go`) |
| 1.5.1 | Input/output handling requirements defined | satisfied | Sensitivity model `normal\|private\|secret` (`backend/models/dtos.go:218,352`) defines handling: >normal is excluded from exports and sync in the query (`backend/models/contact_record.go:116-136`) |
| 1.5.2 | No serialization with untrusted clients | satisfied | Wire format is JSON DTOs only (`encoding/json`); no gob/pickle/object serialization anywhere |
| 1.5.3 | Input validation on trusted layer | satisfied | `ValidateJSONMiddleware` runs server-side on every input route (`backend/middleware/validation.go:332-368`); client-side validation is UX only |
| 1.5.4 | Output encoding near the interpreter | satisfied | React auto-escapes all rendering (no `dangerouslySetInnerHTML` in `frontend/src`); CSV formula injection neutralized at the export boundary (`backend/controllers/export_controller.go:41-57`) |
| 1.6.1 | Cryptographic key management policy | partial | No formal key-lifecycle document; JWT secret validated at boot — ≥ 32 bytes, no known placeholder, minimum entropy (`backend/config/config.go:351-379`); TOTP/integration secrets AES-256-GCM at rest (`backend/services/credential_crypto.go`). Gap: rotation runbook is P2-referenced but not a doc. |
| 1.6.2 | Key vault / API-based key access | not-applicable | Self-hosted single process; secrets come from environment (`backend/.env.example`); see P2 (crypto agility position) |
| 1.6.3 | Keys replaceable, re-encrypt path defined | partial | JWT rotation is clean (`TokenVersion`, `config/config.go:351-379`); re-encrypting TOTP/integration secrets after key rotation is not automated (`credential_crypto.go:20-21` documents the coupling) |
| 1.6.4 | Client-side secrets treated as insecure | satisfied | Session token is an httpOnly cookie, never JS-readable (`frontend/src/auth.ts:127-134`); no secrets in `localStorage` |
| 1.7.1 | Common logging format | satisfied | zerolog JSON everywhere (`backend/logger/logger.go:26-63`) |
| 1.7.2 | Logs shipped to remote system | not-applicable | Self-hosted; logs go to stdout → operator's docker log driver |
| 1.8.1 | Sensitive data classified | satisfied | `sensitivity` field on contacts/edges + explicit class (PII in contacts; credentials in encrypted columns) |
| 1.8.2 | Protection levels have requirements | satisfied | >normal: filtered in projection query (`backend/models/contact_record.go:116-136`), graph traversal (`backend/services/graph_traversal.go:113-115`), suggestions (`backend/services/graph_suggestion_service.go:85`), briefings (`backend/controllers/briefing_controller.go:236-244`) |
| 1.9.1 | Communications encrypted | satisfied | TLS at external reverse proxy (`docs/deployment.md:17`); HSTS emitted when HTTPS is configured (`backend/middleware/security_headers.go:36-38`, `backend/main.go:212`); SMTP STARTTLS/implicit TLS (`backend/services/mailer.go`, `SMTP_USE_TLS`) |
| 1.9.2 | Peer authenticity verified | not-applicable | Single process (loopback between nginx and app); outbound calls use Go's standard TLS certificate verification (`httputil/fetch.go:94-119`, `mailer.go:124`) |
| 1.10.1 | Source control + traceability | satisfied | Git + issues-driven commits; one branch per concern (`CLAUDE.md` Workflow) |
| 1.11.1 | Components documented by function | satisfied | `docs/adrs/`, `docs/data-model.md`, `docs/development.md` |
| 1.11.2 | High-value flows don't share unsynchronized state | satisfied | Stateless JWT + per-request `token_version` DB check (`backend/middleware/auth.go:141-154`); no shared mutable session store; the only shared state (in-memory rate limiter) is per-key and restart-visible by design (`rate_limiter.go:32-36`) |
| 1.12.2 | Uploaded files served safely + CSP | satisfied | Attachments download-only with `Content-Disposition: attachment` + `nosniff` (`backend/controllers/attachment_controller.go:212-238`); photos re-encoded to JPEG, SVG rejected (`backend/controllers/photo_controller.go:315-324,348-353`); CSP `frame-ancestors 'none'` (`backend/middleware/security_headers.go:23`, `docker/nginx.conf:21`) |
| 1.14.1 | Segregation of trust levels | satisfied | nginx reverse proxy in front of the app (`docker/nginx.conf`); non-root container |
| 1.14.2 | Binary signatures / verified endpoints | satisfied | cosign-signed images + SLSA provenance (`docker-publish.yml:339-367`) |
| 1.14.3 | Build pipeline warns on outdated components | satisfied | govulncheck (`unit-tests.yml:175-177`), Trivy (`docker-publish.yml:428-445`), CodeQL (`codeql.yml`), zizmor (`zizmor.yml`), Dependabot + Dependency Review (`dependency-review.yml`) |
| 1.14.4 | Automated build + verify | satisfied | CI builds, unit + Playwright e2e + Android instrumented e2e (`.github/workflows/unit-tests.yml`, `e2e-tests.yml`, `android-tests.yml`) |
| 1.14.5 | Sandboxing/containerization | satisfied | Single non-root container (`backend/Dockerfile`), `docker/nginx.conf` limits exposure; CIS hardening baseline gated in CI (`docker/cis-hardening.sh`, `.github/workflows/container-hardening.yml`) |
| 1.14.6 | No deprecated client tech | satisfied | React/TS only; no Flash/ActiveX/Silverlight/Java applets |

## V2 — Authentication

L3-only, out of scope: 2.2.4, 2.2.5, 2.2.6, 2.2.7.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 2.1.1 | Passwords ≥ 12 chars | partial | We enforce `min=8` + ≥ 50 bits entropy (`backend/middleware/password_strength.go:21-45`); deviation documented in **P1** |
| 2.1.2 | ≥ 64 chars permitted, > 128 denied | partial | 64–72 bytes permitted; > 72 bytes rejected by the bcrypt cap (`backend/services/user_service.go:14,21-23`) — denied earlier than 128, deliberately |
| 2.1.3 | No password truncation | satisfied | Explicit 72-byte rejection instead of silent truncation (`backend/services/user_service.go:21-23`); test `services/user_service_test.go:13-32` |
| 2.1.4 | Any printable Unicode permitted | satisfied | No charset restrictions; policy is entropy-based (`password_strength.go:33-45`) |
| 2.1.5 | Users can change password | satisfied | `backend/controllers/user_controller.go:626-737` |
| 2.1.6 | Change requires current + new | satisfied | Current verified at `user_controller.go:674`, new≠old at `:679` |
| 2.1.7 | Breached-password check | partial | Local common-password blocklist incl. symbol-stripped variants (`backend/middleware/password_strength.go:168-198`); no external breach feed. Gap: blocklist is ~30 entries, not top-10k. |
| 2.1.8 | Password strength meter | satisfied | `/check-password-strength` endpoint (`backend/controllers/user_controller.go:278-290`, route `routes/routes.go:45`); meter on `frontend/src/RegisterPage.tsx` |
| 2.1.9 | No composition rules | satisfied | Entropy-based; no upper/lower/digit/symbol requirements (`password_strength.go:33-45`) |
| 2.1.10 | No rotation/history requirements | satisfied | No password expiry or history anywhere |
| 2.1.11 | Paste + password managers allowed | satisfied | Standard `<input type="password">`; no paste blocking |
| 2.1.12 | View-password toggle | partial | Not implemented — password fields are plain masked inputs (`frontend/src/LoginPage.tsx:175`, `RegisterPage.tsx:124`); browsers' built-in reveal handles it. Gap: no in-app toggle. |
| 2.2.1 | Anti-automation (≤ 100 failed/h/account) | satisfied | Per-account lockout: 5 failures → 1 min, exponential to 30 min cap (`backend/middleware/rate_limiter.go:12-22,68-110`) — caps well under 100/h; per-IP auth limiter 2 req/s burst 50 (`:236`); tests `rate_limiter_test.go:292-531`, `two_factor_controller_test.go:270-305` |
| 2.2.2 | Weak authenticators only as secondary | satisfied | No SMS/email OTP at all; the only second factor is TOTP (`backend/services/twofactor.go`) |
| 2.2.3 | Notify on credential changes | partial | Audit trail records all changes (`backend/models/audit.go` hooks); no proactive email/push notification on password change or 2FA toggle. Gap: notification delivery. |
| 2.3.1 | System-generated initial secrets random + expiring | satisfied | Password-reset tokens: 32 bytes `crypto/rand`, 1 h TTL, hash-only storage (`backend/services/password_reset_service.go:15-41`) |
| 2.3.2 | User-provided authenticator devices | partial | TOTP authenticator apps fully supported (setup/confirm/disable/recovery, `backend/services/twofactor.go`, `two_factor_controller.go`); FIDO/U2F hardware keys not supported |
| 2.3.3 | Renewal instructions for time-bound authenticators | not-applicable | TOTP secrets do not expire; no time-bound authenticators |
| 2.4.1 | Passwords hashed, salted, KDF | satisfied | bcrypt (KDF) with per-hash 128-bit salt, cost 10 (`backend/services/user_service.go:25`); tests `services/user_service_test.go` |
| 2.4.2 | Unique salt ≥ 32 bits | satisfied | bcrypt 16-byte random salt per hash |
| 2.4.3 | PBKDF2 iterations | not-applicable | bcrypt used, not PBKDF2 |
| 2.4.4 | bcrypt work factor ≥ 10 | satisfied | `bcrypt.DefaultCost` = 10 (`services/user_service.go:25`); see P1 |
| 2.4.5 | Secret pepper / additional KDF round | not-applicable | Requires a separately-stored secret device (HSM); see P1/P2 for the single-process position |
| 2.5.1 | Recovery secret never sent in clear | satisfied | Reset sends a one-time random link token, never a password; password is only ever user-chosen (`password_reset_service.go`, `user_controller.go:362-437`) |
| 2.5.2 | No hints / KBA | satisfied | None exist |
| 2.5.3 | Recovery never reveals current password | satisfied | Reset re-hashes a new password; the old hash is never recoverable/returned (`user_controller.go:411`) |
| 2.5.4 | No shared/default accounts | satisfied | No seeded accounts; first registered user becomes admin (`user_controller.go:55-63`) |
| 2.5.5 | Notify on factor change | partial | Audited (`backend/models/audit.go`) but not proactively notified; same gap as 2.2.3 |
| 2.5.6 | Secure recovery mechanism | satisfied | crypto/rand token, hash-only storage, 1 h TTL, endpoints rate-limited (`routes/routes.go:46-49`); tests `services/password_reset_service_test.go` |
| 2.5.7 | Identity proofing on factor loss | not-applicable | Enrollment has no identity proofing (self-hosted); recovery codes fill the loss path |
| 2.6.1 | Lookup secrets single use | satisfied | Recovery codes deleted in the same `WHERE` that consumes them (`backend/services/twofactor.go:171-175`); test `two_factor_controller_test.go:250` |
| 2.6.2 | Lookup-secret entropy ≥ 112 bits or salted+hashed | partial | Recovery codes are 15 chars from a 32-symbol alphabet = 75 bits, stored SHA-256 hashed **without** a per-code salt (`twofactor.go:38,88-148`). Gap vs the letter; see 2.6.3 for why it's still safe. |
| 2.6.3 | Resistant to offline attack | satisfied | 75-bit random codes (≈3.8×10²²) + SHA-256; API tokens are 256-bit (`controllers/api_token_controller.go:65-70`) |
| 2.7.1–2.7.6 | Out-of-band verifier | not-applicable | No SMS/email/push OOB authenticator exists; the 2FA step is in-band TOTP (see V2.8) |
| 2.8.1 | TOTP has defined lifetime | satisfied | RFC 6238, 30 s period, skew 1 (`backend/services/twofactor.go:72-83`) |
| 2.8.2 | TOTP keys highly protected | partial | Secret encrypted at rest AES-256-GCM, HKDF-derived key (`backend/services/credential_crypto.go:22-54`) — no HSM/OS keyring; see P2 |
| 2.8.3 | Approved algorithms for OTP | satisfied | RFC 6238 TOTP (HMAC-SHA1), `pquerna/otp` (`twofactor.go:55-83`) |
| 2.8.4 | TOTP single use within validity | partial | No one-time-use rejection inside the 30 s window (standard TOTP behavior); brute force is rate-limited (`two_factor_controller.go:346-356,377-387`) |
| 2.8.5 | Reused TOTP logged + notified | partial | Reuse is not tracked/notified; failed-code attempts are rate-limited and logged as 429s. Gap: reuse detection. |
| 2.8.6 | TOTP revocable, immediate effect | satisfied | Disable 2FA requires a live code, then clears secret + bumps `token_version` (kills all sessions) (`two_factor_controller.go:196-256,231`) |
| 2.8.7 | Biometrics only as secondary factor | not-applicable | No biometric authenticators |
| 2.9.1–2.9.3 | Cryptographic verifiers (smart cards/FIDO) | not-applicable | No FIDO/smart-card authenticators; see 2.3.2 |
| 2.10.1–2.10.3 | Service authentication | not-applicable | No intra-service accounts; single process |
| 2.10.4 | Integration secrets not in source, protected | satisfied | WebDAV/Immich/Seafile credentials are user-supplied and stored AES-256-GCM encrypted (`backend/services/credential_crypto.go`); nothing in source or git |

## V3 — Session Management

L3-only, out of scope: 3.6.1, 3.6.2.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 3.1.1 | No session tokens in URLs | satisfied | Tokens live in httpOnly cookie / `Authorization` header only (`backend/middleware/auth.go:23-43`); OIDC state/nonce in cookies (`oidc_controller.go:80-83`) |
| 3.2.1 | New token on authentication | satisfied | Fresh HS256 JWT minted at login and at 2FA completion (`backend/services/user_service.go:32-61`, `two_factor_controller.go:393-410`) |
| 3.2.2 | ≥ 64 bits entropy | satisfied | Token is a signed assertion; the signing secret is ≥ 32 bytes (256 bits) enforced at boot, and placeholder/low-entropy secrets are rejected (`config/config.go:351-379`) |
| 3.2.3 | Tokens in secure browser storage | satisfied | httpOnly cookie (`user_controller.go:205-214`); never `localStorage`/`sessionStorage` (`frontend/src/auth.ts:127-134`) |
| 3.2.4 | Approved crypto for tokens | satisfied | HS256 (HMAC-SHA-256), explicit `SigningMethodHMAC` check (`middleware/auth.go:66-98`) |
| 3.3.1 | Logout/expiry invalidate session | satisfied | Logout clears cookies (`user_controller.go:232-236`); JWT expiry 96 h default (`config/config.go:96-101`); `token_version` revocation (`middleware/auth.go:141-154`); test `middleware/auth_lifecycle_test.go:59-84` |
| 3.3.2 | Re-authentication after idle | partial | Absolute 96 h expiry but **no idle timeout** (`config/config.go:96-101`). Gap vs L2's 12 h/30 min idle guidance; documented as the chosen trade-off for a personal CRM |
| 3.3.3 | Terminate other sessions after password change | satisfied | `TokenVersion++` on change/reset kills all other sessions; caller's own cookie is re-issued (`user_controller.go:701,422-428,709-734`) |
| 3.3.4 | View and log out all sessions | partial | Stateless JWTs — no per-device session list; "log out everything" is achievable via password change (3.3.3) or admin reset (`admin_user_controller.go:394-409`). Gap: no session inventory UI. |
| 3.4.1 | Cookie `Secure` attribute | satisfied | `Secure = cfg.CookieSecure`, and boot refuses `FRONTEND_URL=https` + `COOKIE_SECURE=false` (`user_controller.go:205-214`, `config/config.go:375-380`) |
| 3.4.2 | Cookie `HttpOnly` | satisfied | `user_controller.go:205-214` |
| 3.4.3 | Cookie `SameSite` | satisfied | `SameSite=Lax` (`user_controller.go:205-214`) |
| 3.4.4 | `__Host-` prefix | partial | No `__Host-` prefix; the cookie is host-only with `Path=/` and no Domain, and `COOKIE_SECURE` may be off in plain-HTTP LAN setups (`.env.example:110-122`). Gap: prefix impossible while dev/HTTP setups exist. |
| 3.4.5 | Precise cookie path | satisfied | Single app at `/`; no sibling apps on the domain (`Path=/`) |
| 3.5.1 | Revocable OAuth tokens | not-applicable | No OAuth token issuance (OIDC is RP-only); scoped API tokens are revocable (`api_token_controller.go:111-138`) |
| 3.5.2 | Session tokens over static API keys | partial | Sessions are JWTs; long-lived API tokens exist **by design** for CardDAV/scripting — scoped (`full`/`carddav`), hashed at rest, default 90-day expiry, revocable (`api_token_controller.go:51-109`, `models/dtos.go:493-509`) |
| 3.5.3 | Stateless tokens signed + tamper-proof | satisfied | HS256 with explicit algorithm pinning (`middleware/auth.go:66-98`); `purpose:"2fa"` challenge tokens structurally can't be sessions (`:109-118`); test `auth_lifecycle_test.go:88` |
| 3.7.1 | Full login required for sensitive ops | satisfied | 2FA challenges can't authenticate (`auth.go:109-118`); password change requires the current password (`user_controller.go:674`); 2FA disable requires a live code (`two_factor_controller.go:196-256`) |

## V4 — Access Control

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 4.1.1 | Enforce access control on trusted layer | satisfied | Server-side only; canonical `backend/controllers/circle_controller.go:49` (`Where("id = ? AND user_id = ?")`) |
| 4.1.2 | Policy data not user-manipulable | satisfied | `IsAdmin` excluded from registration DTO and JWT (`models/dtos.go:294-300`, `services/user_service.go:43`); relationship status/provenance server-derived (`relationship_edge_controller.go:155-160`) |
| 4.1.3 | Least privilege | satisfied | `user_id`/`VCardUID` scoping on every handler; audit of unscoped queries (2026-08-23) found only deliberate cross-user surfaces: contact sharing (`contact_share_controller.go:50`), thin user directory (`admin_user_controller.go:162-181`), auth lookups, internal jobs |
| 4.1.5 | Fail securely | satisfied | Ownership misses are AND-scoped 404s (never 403 revealing existence) (`contact_share_controller.go:212-233`); unknown errors → generic 500 (`errors/errors.go:273-283`) |
| 4.2.1 | IDOR protection (create/read/update/delete) | satisfied | Every controller query AND-scoped; pinned by per-entity cross-user tests (e.g. `circle_controller_test.go:182`, `household_controller_test.go:254,277`, `relationship_edge_controller_test.go:84,107`, `two_factor_controller_test.go:413`, `sync_conflict_controller_test.go:109` — the contact-sync-conflict list/restore/dismiss endpoints, issue #395) |
| 4.2.2 | Anti-CSRF | satisfied | No CSRF tokens — documented mitigation: `SameSite=Lax` cookies + strict CORS origin allowlist (`backend/main.go:191-209`) with wildcard refused in release (`config/config.go:359-364`); API clients use `Authorization` headers, not cookies |
| 4.3.1 | Admin interfaces use MFA | partial | 2FA is available to all users but not **enforced** for admins (`models/user.go:43-52`). Gap: no `require 2FA for admins` policy. |
| 4.3.2 | Directory browsing disabled | satisfied | No static serving of user dirs; uploads served only through controllers; nginx `autoindex` off (default), only built SPA assets served (`docker/nginx.conf`) |
| 4.3.3 | Step-up/adaptive auth | not-applicable | Single risk tier (personal CRM); TOTP is the step-up where it matters (sensitive ops, V3.7.1) |

## V5 — Validation, Sanitization and Encoding

L3-only, out of scope: none in this chapter (5.4 is L2).

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 5.1.1 | HTTP parameter pollution | satisfied | Gin binds JSON from the request body only; query/cookie/header sources are never merged into input structs (`backend/middleware/validation.go:332-368`) |
| 5.1.2 | Mass assignment protection | satisfied | Explicit DTO allowlists; `IsAdmin` excluded (`models/dtos.go:294-300`); relationship provenance/status/confidence absent from input by design (`models/dtos.go:339-354`); field-by-field update copies (`circle_controller.go:164`, `life_event_controller.go:346-353`) |
| 5.1.3 | Positive (allow-list) validation | satisfied | Struct tags (`required`, `uuid4`, `oneof`, `httpurl`, …) + custom validators (`backend/middleware/validation.go:19-33`) |
| 5.1.4 | Strong typing + schema | satisfied | Typed DTOs with per-field validation incl. cross-field rule (gift value ⇒ currency, `gift_controller.go:19-24`); `phone`/`birthday`/`safeurl` custom validators (`validation.go:120-211`) |
| 5.1.5 | URL redirects allow-listed | satisfied | No server-side open redirects; frontend navigates only hardcoded routes (`frontend/src/App.tsx:586-588`); OIDC redirect uses validated state |
| 5.2.1 | HTML sanitizer for WYSIWYG | not-applicable | No HTML input anywhere; notes/fields are plain text; frontend renders text only (no `dangerouslySetInnerHTML` in `frontend/src`) |
| 5.2.2 | Unstructured data sanitized | satisfied | `SanitizeString` strips null/control chars (`validation.go:95-116`); length caps on every free-text field (`models/dtos.go`) |
| 5.2.3 | SMTP injection protection | satisfied | `To` is `required,email`-validated (`models/dtos.go:303-311`); `Subject` MIME-Q-encoded (`services/mailer.go:167`); body is fixed-template |
| 5.2.4 | No eval/dynamic execution | satisfied | No `eval` in Go or frontend; `eslint-plugin-security` in CI |
| 5.2.5 | Template injection | satisfied | Email templates are fixed strings from an embedded FS; no user input in template logic (`services/email_renderer.go`) |
| 5.2.6 | SSRF protection | satisfied | Public-IP-only dialer with DNS-rebinding pinning (`backend/httputil/safedial.go:27-47`, `ipguard.go:30-59`); pre-flight URL checks (`httputil/fetch.go:17-58`); per-service opt-in flags (webhooks/Immich/CardDAV/Seafile, `config/config.go:65-83,136-147`); tests `httputil/fetch_test.go`, `services/webhook_ssrf_test.go` |
| 5.2.7 | SVG scriptable content | satisfied | SVG rejected at photo upload/proxy (`photo_controller.go:348-353`) and by attachment markup-signature check (`attachment_controller.go:42-63`) |
| 5.2.8 | Markdown/CSS/template content | not-applicable | No markdown/CSS/XSL/BBCode rendering of user content |
| 5.3.1–5.3.3 | Output encoding / XSS | satisfied | React auto-escaping (no raw-HTML sinks); CSV cells neutralized (`export_controller.go:41-57`); JSON via `encoding/json` |
| 5.3.4 | Parameterized queries | satisfied | GORM everywhere; raw SQL is parameterized (search FTS `search_service.go:228-305`, graph CTE `graph_traversal.go:120-135`, export `export_controller.go:189-197`) |
| 5.3.5 | Contextual encoding where no parameterization | satisfied | No non-parameterized SQL paths; identifier interpolation is compile-time constants only (`export_controller.go:105-128`) |
| 5.3.6 | JSON injection / JSON eval | satisfied | Responses via `encoding/json`; frontend uses `fetch().json()` (JSON.parse), never eval |
| 5.3.7 | LDAP injection | not-applicable | No LDAP |
| 5.3.8 | OS command injection | satisfied | No `os/exec` anywhere (security review ban); parameterized everything |
| 5.3.9 | LFI/RFI | satisfied | Server-generated UUID filenames (`photo_controller.go:248-250`, `attachments/attachments.go:39-52`); traversal guards reject `..`/absolute (`photo_controller.go:94-98`, `attachments.go:27-35`) |
| 5.3.10 | XPath/XML injection | not-applicable | No XML parsing of untrusted input (CardDAV is text/vCard) |
| 5.4.1–5.4.3 | Memory safety / format strings / integer overflow | not-applicable | Go is memory-safe (runtime bounds checks); `unsafe` unused in app code; `go vet` in CI |
| 5.5.1 | Serialized objects integrity | satisfied | Only JSON DTOs cross the wire; the only signed serialized object is the JWT (HS256, V3.5.3) |
| 5.5.2 | XXE / restrictive XML parsing | not-applicable | No XML parsers exist in request paths |
| 5.5.3 | No untrusted deserialization | satisfied | `encoding/json` into typed structs only; no gob/pickle/yaml of untrusted input |
| 5.5.4 | JSON.parse, not eval | satisfied | `fetch().json()` everywhere in `frontend/src/api/*` |

## V6 — Stored Cryptography

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 6.1.1 | Regulated private data encrypted at rest | partial | Secrets (TOTP, integration credentials) are AES-256-GCM encrypted (`backend/services/credential_crypto.go:33-54`); contact PII in SQLite is **not** encrypted at rest — filesystem/disk encryption is the operator's (self-hosted; file perms 0600/0700/0750, see V12.4.1). |
| 6.1.2 | Regulated health data at rest | not-applicable | Neutral contact model has no medical fields; no regulated health data by design |
| 6.1.3 | Regulated financial data at rest | not-applicable | Gift records are non-sensitive user-authored notes (no account/credit/tax data) |
| 6.2.1 | Crypto fails securely, no padding oracle | satisfied | AEAD (GCM) — no padding to oracle; failures are generic 500s (`errors/errors.go:273-283`) |
| 6.2.2 | Approved algorithms/libraries | satisfied | bcrypt, AES-256-GCM, HMAC-SHA-256, SHA-256, HKDF-SHA256, RFC 6238 TOTP — all stdlib/`golang.org/x/crypto` |
| 6.2.3 | IV/cipher/mode config | satisfied | GCM 12-byte nonces from `crypto/rand` (`credential_crypto.go:47-50`); no ECB; no custom modes |
| 6.2.4 | Algorithms swappable | partial | Direct calls, not behind an abstraction — deliberate pre-1.0 decision, documented in **P2** |
| 6.2.5 | No weak modes/hashes | satisfied | No MD5/SHA1/ECB/DES/Blowfish; SHA-256 only for non-reversible one-time tokens |
| 6.2.6 | Nonce never reused per key | satisfied | Fresh `crypto/rand` nonce per encryption (`credential_crypto.go:47-50`); test `services/credential_crypto_test.go` |
| 6.2.7 | Ciphertext authenticated | satisfied | AES-256-GCM (authenticated) — exceeds L2 (L3 row, met anyway) |
| 6.2.8 | Constant-time comparisons | out-of-scope | L3; note bcrypt/HMAC comparisons are constant-time by library design |
| 6.3.1 | CSPRNG for secrets | satisfied | `crypto/rand` for API tokens (`api_token_controller.go:65-70`), reset tokens (`password_reset_service.go:22-25`), recovery codes (`twofactor.go:103-122`), GCM nonces |
| 6.3.2 | UUID v4 via CSPRNG | satisfied | `google/uuid` v4 in `BeforeCreate` (e.g. `models/circle.go:31`, `models/contact.go:415`) |
| 6.3.3 | Entropy under load | out-of-scope | L3 |
| 6.4.1 | Secrets-management solution | partial | Env-var secrets with boot-time validation — length, placeholder, and entropy checks on `JWT_SECRET_KEY` (`config/config.go:351-379`); no vault (self-hosted; see P2). |
| 6.4.2 | Key material isolated from app | not-applicable | Single process; the secret must live in the process env by design (self-hosted; see P2) |

## V7 — Error Handling and Logging

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 7.1.1 | No credentials/payment in logs | satisfied | Logger context has `request_id`, `user_id`, `method`, `path`, `ip` — no body, no Authorization header, no cookies (`backend/logger/logger.go:86-111`); passwords never logged |
| 7.1.2 | No other sensitive data in logs | satisfied | Sensitive query values are redacted before the query string is logged — `RedactQueryValues` scrubs `code`/`token`/`access_token`/`key`/`secret`/`password`/`signature` (case-insensitive, after percent-decoding the key) to `[REDACTED]` (`backend/logger/redact.go`), applied in the request log's `query` field (`backend/middleware/logging.go:18`). Tests: `logger/redact_test.go`, `middleware/logging_test.go`. |
| 7.1.3 | Security events logged | partial | Request log records 401/403/429s; audit hooks record all data mutations (`backend/models/audit.go`). Gap: no distinct auth-success/failure event type, no deserialization events. |
| 7.1.4 | Log event timeline detail | satisfied | `request_id`, `user_id`, IP, UA, method, path, status, duration (`backend/logger/logger.go:86-111`) |
| 7.2.1 | Authentication decisions logged | satisfied | Login/2FA/register attempts (success and failure) are request-logged with IP, method, path, status (`backend/middleware/logging.go`); lockout events surface as 429s (`rate_limiter.go`); 2FA challenge issuance/consumption is audit-visible via the 2FA controller flow |
| 7.2.2 | Access-control failures logged | partial | 401/403/404s are logged, but 404-masked IDOR misses are indistinguishable from genuine 404s *by design* (V4.1.5). Gap: no separate access-denied event. |
| 7.3.1 | Log injection prevented | satisfied | User-controlled values are control-character-escaped and length-capped before logging (`backend/logger/sanitize.go`), applied to request path/query/UA/error (`backend/middleware/logging.go`), request-path context (`backend/logger/logger.go:104-108`), and user-content diagnostics in the message position (`carddav/backend.go:352,445`, `export_controller.go:657,726`, `contact_share_controller.go:87`, `photo_controller.go:338,350`) — the console writer prints the message verbatim, so raw newlines there were a real line-injection vector; tests: `logger/sanitize_test.go`, `middleware/logging_test.go` |
| 7.3.3 | Logs protected | not-applicable | stdout → operator's docker log driver |
| 7.3.4 | Time synchronization, UTC | not-applicable | Operator/OS concern; timestamps are UTC (`zerolog Timestamp()`) |
| 7.4.1 | Generic error + referenceable ID | satisfied | Envelope `{error:{code,message,details}, request_id, timestamp}` (`backend/errors/middleware.go:13-24,67-86`); internal errors are generic text |
| 7.4.2 | Exception handling across codebase | satisfied | Typed `AppError` + `AbortWithError` (`errors/errors.go`, `errors/middleware.go:111-113`); `.Error` checked on every write (trap 4, `CLAUDE.md`) |
| 7.4.3 | Last-resort handler | satisfied | Panic-recovery middleware → generic 500, stack logged server-side only (`errors/middleware.go:27-46`); `safeGo` for goroutines (`main.go:30-43`) |

## V8 — Data Protection

L3-only, out of scope: 8.1.5, 8.1.6.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 8.1.1 | No sensitive data in server caches | partial | No in-process/HTTP cache of API responses exists, but API responses carry no `Cache-Control: no-store`, so an operator proxy could cache them. Gap: add no-store to API responses. |
| 8.1.2 | Cached copies purged | not-applicable | No server-side caching layer |
| 8.1.3 | Minimal request parameters | satisfied | Small DTOs; sensitive data in body/headers, never query strings |
| 8.1.4 | Abnormal request detection/alert | satisfied | Per-IP API limiter (1/600 ms, burst 1000) + per-account lockout + configurable intervals (`backend/middleware/rate_limiter.go:249,255-258,275-280`) |
| 8.2.1 | Anti-caching headers for browsers | partial | SPA HTML gets `Cache-Control: no-cache` (`docker/nginx.conf:133`); API responses get no anti-cache headers. Gap: same as 8.1.1. |
| 8.2.2 | No sensitive data in browser storage | satisfied | Token in httpOnly cookie; `localStorage` holds only prefs + `user_info` (id/username/admin) (`frontend/src/auth.ts:15,120`); service worker caches app-shell and `.png` only — never API responses (`src/service-worker.ts`) |
| 8.2.3 | Client data cleared on logout | satisfied | Logout clears the cookie and `USER_INFO_KEY` (`frontend/src/auth.ts:172`); SPA state dies with the session |
| 8.3.1 | Sensitive data not in query strings | satisfied | POST/PATCH bodies; GET params are filters only |
| 8.3.2 | Export/remove data on demand | satisfied | Exports: CSV/vCard3/vCard4/jSContact (`routes/routes.go:341-345`); deletion: per-entity delete + soft-delete undo (`contact_controller.go` `DeleteContact`, audit undo `audit_controller.go`). Note: account deletion is admin-only — no user self-service account wipe (accepted for single-user self-host). |
| 8.3.3 | Clear consent language | not-applicable | Self-hosted; no third-party data collection (privacy is the operator's, by design) |
| 8.3.4 | Sensitive-data policy in place | satisfied | `sensitivity` classification enforced in exports/sync/graph (V1.8.2); ADRs document the model (`docs/adrs/`) |
| 8.3.5 | Sensitive-data access audited | partial | Mutations of all major entities audited with redacted before-snapshots (`backend/models/audit.go:105-175`); **reads** are not audited. Gap: read-auditing would be a deliberate privacy/performance choice. |
| 8.3.6 | Memory zeroization | not-applicable | Go runtime; no explicit zeroization (accepted; see P2's scope note) |
| 8.3.7 | Encryption with confidentiality+integrity | satisfied | AES-256-GCM for at-rest secrets (`credential_crypto.go:33-54`); TLS for transport |
| 8.3.8 | Retention classification, auto-delete | satisfied | T26 purge jobs (feed/audit retention, `backend/services/purge_service.go`, `audit_purge_service.go`); 410 Gone past purge window (`controllers/helpers.go:348-360`); soft-delete is the retention buffer for user content |

## V9 — Communication

L3-only, out of scope: 9.2.5.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 9.1.1 | TLS for all client connectivity, no fallback | satisfied | TLS at the external reverse proxy (`docs/deployment.md:17`); HSTS emitted when HTTPS is configured (`backend/middleware/security_headers.go:36-38`); no plaintext listener exposed |
| 9.1.2 | Only strong ciphers | not-applicable | TLS config belongs to the operator's external proxy (Mozilla-level guidance in `docs/deployment.md`) |
| 9.1.3 | TLS 1.2/1.3 only | not-applicable | Same — external proxy's responsibility (documented) |
| 9.2.1 | Trusted TLS certs for outbound | satisfied | Go standard library verifies certificates on all outbound calls; SMTP pins `ServerName` via `tls.Dial` (`backend/services/mailer.go:124`) |
| 9.2.2 | TLS for all outbound connections | satisfied | Outbound fetches are http(s)-only (`httputil/fetch.go:17-58`); SMTP STARTTLS/TLS (`mailer.go`, `SMTP_USE_TLS`); CardDAV/WebDAV clients use https |
| 9.2.3 | Outbound connections authenticated | satisfied | SMTP auth, CardDAV/WebDAV credentials, Immich/Seafile API keys (`services/` client packages) |
| 9.2.4 | OCSP stapling | not-applicable | External proxy's TLS configuration |

## V10 — Malicious Code

L3-only, out of scope: 10.1.1, 10.2.3, 10.2.4, 10.2.5, 10.2.6.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 10.2.1 | No phone-home / data collection | satisfied | No telemetry/analytics anywhere in app code; dependency scanners + CodeQL in CI (`.github/workflows/codeql.yml`); review culture per `CLAUDE.md` |
| 10.2.2 | No excessive permissions | satisfied | Android manifest minimal; mobsfscan manifest/secret scan in CI (`.github/workflows/sast.yml`) |
| 10.3.1 | Signed updates | satisfied | No auto-update feature; images are cosign-signed with SLSA provenance (`docker-publish.yml:339-367`) |
| 10.3.2 | Integrity of loaded code (SRI) | satisfied | All assets self-hosted (no CDN); SRI moot; npm/go deps pinned (lockfiles); no runtime loading from external sources |
| 10.3.3 | Subdomain-takeover protection | not-applicable | Self-hosted; DNS is operator-managed (documented in `docs/deployment.md`) |

## V11 — Business Logic

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 11.1.1 | Sequential, unbypassable flows | satisfied | No multi-step business flows to bypass; the one two-step flow (2FA enrollment setup→confirm) is server-state-enforced (`two_factor_controller.go:60-191`) |
| 11.1.2 | Realistic human-time steps | not-applicable | No multi-step flows; anti-automation covers the rest (11.1.4) |
| 11.1.3 | Per-user limits on business actions | partial | Rate limiting is per-IP (API) + per-account (auth only); no per-user quotas on content creation. Gap: quotas would be a product decision. |
| 11.1.4 | Anti-automation / anti-DoS | satisfied | Body-size limits 10 MB/1 MB (`middleware/body_limit.go:11-40`), `MaxMultipartMemory` (`main.go:188`), API rate limiter, account lockout |
| 11.1.5 | Business-logic limits per threat model | satisfied | Duplicate-add 409s (`circle_controller.go:242`, `household_controller.go:245`, `tag_controller.go:242`), one cadence policy per contact (`cadence_controller.go:66-74`), gift value⇒currency rule (`gift_controller.go:19-24`), self-edge rejection (`relationship_edge_controller.go:104-106`), accept-only-suggested (`:365-368`) |
| 11.1.6 | No TOCTOU/race conditions | satisfied | `_txlock=immediate` + WAL so write transactions take the write lock up front (`database/migrate.go:56-57`); pinned by `database/concurrent_write_test.go`; single-use recovery code consumed in one `WHERE` (`twofactor.go:171-175`) |
| 11.1.7 | Monitor unusual business activity | partial | Anomaly signals exist (429s, lockouts) and are logged; no standing business-anomaly monitoring. Gap: product decision. |
| 11.1.8 | Configurable alerting | partial | Webhooks exist (user-configured, `services/webhook_broadcast.go`) but are not wired to security-anomaly events. Gap: no alert on lockout spikes. |

## V12 — Files and Resources

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 12.1.1 | No oversized uploads / DoS | satisfied | Photo 10 MB cap (`photo_controller.go:146-150`), attachment 25 MB (`attachment_controller.go:96-115`), global body caps + `MaxBytesReader` (`body_limit.go:21-32`) |
| 12.1.2 | Zip-bomb protection | not-applicable | The app never decompresses archives (no zip/gz processing of uploads) |
| 12.1.3 | Per-user storage quota | partial | No quota (self-hosted; disk is the operator's). Gap: product decision. |
| 12.2.1 | File type validated by content | satisfied | Magic-byte sniffing + decode-verify (JPEG/PNG/HEIC) (`photo_controller.go:204-243`, `photostore/photostore.go:47-70`) |
| 12.3.1 | Filename metadata not used by FS | satisfied | Server-generated UUID names (`photo_controller.go:248-250`, `attachments.go:39-52`) |
| 12.3.2 | LFI via filenames | satisfied | Traversal guards (`photo_controller.go:94-98`, `attachments.go:27-35`) |
| 12.3.3 | RFI/SSRF via filenames/URLs | satisfied | Public-IP-only dialer (`httputil/safedial.go:27-47`); URL import path shares it (`photostore.go:166-168`) |
| 12.3.4 | Reflective File Download | satisfied | `Content-Disposition` with fixed/server names + `nosniff` on downloads (`attachment_controller.go:212-238`, `photo_controller.go:367`) |
| 12.3.5 | No OS-command injection via files | satisfied | No `os/exec`; no filename in system calls |
| 12.3.6 | No execution of untrusted code | satisfied | No runtime loading/plugin system; deps pinned (lockfiles) |
| 12.4.1 | Uploads outside web root, limited perms | satisfied | Photo/attachment dirs 0700/0750 (`photostore.go:96`, `attachments.go:40,48`); served via controllers, not static |
| 12.4.2 | Antivirus scanning | not-applicable | Self-hosted; replaced by magic-byte + decode + markup-signature validation (V12.2.1, `attachment_controller.go:42-63`) |
| 12.5.1 | Extension allow-list on web tier | satisfied | No static serving of uploads; only built SPA assets are static (`docker/nginx.conf`) |
| 12.5.2 | Uploads never executed as HTML/JS | satisfied | `nosniff`, download-only default, SVG/HTML rejected, photos re-encoded to JPEG (`photo_controller.go:315-324`, `attachment_controller.go:212-238`) |
| 12.6.1 | SSRF resource allow-list | satisfied | Public-IP-only dialer rejects loopback/link-local/private/CGNAT and pins resolved IPs (`httputil/ipguard.go:30-59`, `safedial.go:27-47`); per-service opt-in (V5.2.6) |

## V13 — API and Web Service

L3-only, out of scope: none in this chapter.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 13.1.1 | Single parser/encoding stack | satisfied | One Go `net/http`+`encoding/json` stack for all endpoints; CardDAV/CalDAV are separate RFC text protocols with their own strict parsers |
| 13.1.3 | No sensitive info in API URLs | satisfied | Tokens in cookies/headers only; no keys/ids-in-URL beyond resource ids |
| 13.1.4 | Authz at URI and resource level | satisfied | Route-group middleware (`routes/routes.go:52-55`) + per-resource `user_id` scoping in every handler |
| 13.1.5 | Reject unexpected content types | partial | Non-JSON bodies fail `ShouldBindJSON` → 400 `VALIDATION_ERROR` (`validation.go:332-368`); not the 406/415 the standard asks for. Gap: content-type check + correct status. |
| 13.2.1 | HTTP methods valid for action | satisfied | Route table defines only implemented methods; anything else → 404 (`routes/routes.go`) |
| 13.2.2 | JSON schema validation before accept | satisfied | Struct-tag schema via `ValidateJSONMiddleware` on every input route (`validation.go:332-368`) |
| 13.2.3 | CSRF protection for cookie-based REST | satisfied | `SameSite=Lax` + strict CORS allowlist (V4.2.2, `main.go:191-209`) |
| 13.2.5 | Explicit Content-Type check | partial | Same as 13.1.5 (bound by `ShouldBindJSON`, status is 400 not 415) |
| 13.2.6 | Header/payload integrity in transit | satisfied | TLS transport (external proxy) provides confidentiality + integrity |
| 13.3.1–13.3.2 | SOAP/WS-Security | not-applicable | No SOAP |
| 13.4.1–13.4.2 | GraphQL | not-applicable | No GraphQL; REST only |

## V14 — Configuration

L3-only, out of scope: 14.1.5.

| ID | Requirement (abbrev.) | Status | Evidence |
|---|---|---|---|
| 14.1.1 | Secure, repeatable build/deploy | satisfied | CI/CD via GitHub Actions + `docker-compose` (`docker-compose.yml`, `.github/workflows/`) |
| 14.1.2 | Compiler hardening flags | not-applicable | Go is memory-safe; `go vet` + gosec in CI (`unit-tests.yml`) |
| 14.1.3 | Server hardened per framework guidance | satisfied | Non-root container (`Dockerfile`, `entrypoint.sh`), security headers (`security_headers.go`), `GIN_MODE=release` documented for production (`.env.example:21,102`) |
| 14.1.4 | Redeployable from documented runbook | satisfied | `docker-compose` + `docs/deployment.md`; migrations run on boot (`database.InitDB`) |
| 14.2.1 | Dependencies current (checker in build) | satisfied | govulncheck (`unit-tests.yml:175-177`), Trivy (`docker-publish.yml:428-445`), Trivy license scan (`license-compliance.yml`, issue #361), Dependabot, Dependency Review |
| 14.2.2 | Unneeded features removed | satisfied | No debug endpoints; release-mode config guards (`config.go:359-364`); no sample data |
| 14.2.3 | SRI for CDN assets | not-applicable | All assets self-hosted; no CDN |
| 14.2.4 | Third-party from trusted repos | satisfied | Pinned actions by SHA (zizmor), lockfiles (`go.sum`, `yarn.lock`), pinned Go toolchain |
| 14.2.5 | SBOM maintained | satisfied | SBOM generated + attached as image referrer (`docker-publish.yml:343-344`) |
| 14.2.6 | Third-party sandboxed | not-applicable | Single-process app; dependency vetting via scanners (14.2.1) |
| 14.3.2 | Debug modes disabled in production | satisfied | `GIN_MODE=release` required for prod-safe config (`.env.example:21,102`); enforced by config tests (`config_test.go:45-91`) |
| 14.3.3 | No version disclosure in headers | satisfied | No version headers/`X-Powered-By`; server errors are generic (V7.4.1) |
| 14.4.1 | Content-Type with safe charset | satisfied | Gin sets `application/json; charset=utf-8` |
| 14.4.2 | `Content-Disposition: attachment` on API responses | partial | Set on file downloads (`attachment_controller.go:212-238`); JSON API responses don't carry it. Gap: mostly cosmetic for a JSON SPA but requested by ASVS. |
| 14.4.3 | CSP header | satisfied | API: `default-src 'none'; frame-ancestors 'none'` (`security_headers.go:23`); SPA: strict CSP in nginx (`docker/nginx.conf:21`) |
| 14.4.4 | `X-Content-Type-Options: nosniff` | satisfied | `security_headers.go:33`, `docker/nginx.conf:19` |
| 14.4.5 | HSTS on all responses | satisfied | `max-age=31536000; includeSubDomains` when HTTPS configured — API (`security_headers.go:36-38`) and nginx edge (`docker/nginx.conf` + `docker/entrypoint.sh`, gated on `COOKIE_SECURE`); boot refuses the insecure combo (`config.go:375-380`) |
| 14.4.6 | Referrer-Policy | satisfied | `strict-origin-when-cross-origin` (`security_headers.go:34`) |
| 14.4.7 | Not frameable | satisfied | `X-Frame-Options: DENY` + `frame-ancestors 'none'` (`security_headers.go:32`, `security_headers_test.go:12-48`) |
| 14.5.1 | Only methods in use accepted | satisfied | Route table defines the set; others 404 (`routes/routes.go`) |
| 14.5.2 | Origin not used for authz | satisfied | Auth is cookie/header-based; CORS is origin-based *only for cross-origin policy*, never for decisions |
| 14.5.3 | CORS strict allow-list, no "null" | satisfied | `AllowOrigins = [FrontendURL]`; `"*"` refused in release (`main.go:191-209`, `config.go:359-364`) |
| 14.5.4 | Proxy-supplied headers authenticated | satisfied | No proxy headers are trusted for auth; `X-Forwarded-*` used only for logging/ClientIP via validated trusted proxies (`config.go:410-427`, `main.go:231-233`) |

`Permissions-Policy` is not an ASVS 4.0.3 L2 control, but it is set on every
response (`camera=(), microphone=(), geolocation=(), interest-cohort=()` — API
`security_headers.go`, SPA `docker/nginx.conf` + `frontend/nginx.conf`). It
closes the last remaining gap in the "Network, Headers" hardening checklist
(issue #364).

---

## OWASP API Security Top 10 (2023)

| # | Risk | Status | Evidence |
|---|---|---|---|
| **API1** | Broken Object Level Authorization (BOLA/IDOR) | satisfied | Every handler AND-scopes by `user_id`/`VCardUID` (`circle_controller.go:49`); 404-masking hides existence (`contact_share_controller.go:212-233`); cross-user tests per entity (V4.2.1, incl. `sync_conflict_controller_test.go:109`) |
| **API2** | Broken Authentication | satisfied | bcrypt cost 10 + explicit 72-byte cap (P1); dummy bcrypt compare on unknown users (`carddav/auth.go:16-19,56-57`); per-account exponential lockout (`rate_limiter.go:12-22,68-110`); TOTP 2FA (V2.8); `TokenVersion` revocation (`auth.go:141-154`) |
| **API3** | Broken Object Property Level Authorization (BOPLA) | satisfied | DTO allowlists; `IsAdmin`/status/provenance/confidence client-unsets (`models/dtos.go:294-300,339-354`); field-by-field update copies (`life_event_controller.go:346-353`) |
| **API4** | Unrestricted Resource Consumption | satisfied | Body limits 10 MB/1 MB (`body_limit.go:11-40`), `MaxMultipartMemory` (`main.go:188`), per-IP API limiter (`rate_limiter.go:249`), timeouts (`HTTP_READ/WRITE_TIMEOUT`, `.env.example`) |
| **API5** | Broken Function Level Authorization | satisfied | Admin routes gated by `AdminMiddleware` (`middleware/admin.go`); `is_admin` never in JWT (`services/user_service.go:43`); CardDAV-scope tokens blocked from API (`auth.go:54-58`) |
| **API6** | Unrestricted Access to Sensitive Business Flows | satisfied | Rate limits on auth/business endpoints (`routes/routes.go:27-49,53`); sensitivity filtering in exports/sync/graph (V1.8.2); account lockout on login and 2FA (`two_factor_controller.go:346-356`) |
| **API7** | Server Side Request Forgery | satisfied | Public-IP-only dialer with DNS-rebinding pinning (`httputil/safedial.go:27-47`); pre-flight checks (`fetch.go:17-58`); enforced always on photo proxy/import, opt-in per service (V5.2.6); tests `httputil/fetch_test.go`, `webhook_ssrf_test.go` |
| **API8** | Security Misconfiguration | satisfied | Boot-time config validation (`config.go:269-279,359-364,375-380`); security headers (V14.4); release-mode guards; `/.well-known/security.txt` (RFC 9116); config tests `config_test.go` |
| **API9** | Improper Inventory Management | partial | `openapi.yaml` maintained + drift-checked (`backend/openapi_spotcheck_test.go`, `openapi_request_test.go`); no formal versioning/deprecation policy beyond API v1 path. Gap: endpoint inventory doc. |
| **API10** | Unsafe Consumption of APIs | satisfied | SSRF-guarded dialers on every outbound fetch (V5.2.6); response size caps (`fetch.go:61,152-160`); content-type enforcement on fetched media (`fetch.go:126-163`); SVG/HTML rejection (`photo_controller.go:348-353`, `attachment_controller.go:42-63`) |

---

## How to keep this honest (the "living" part)

- **PR/review anchor:** a security-sensitive PR must update the row(s) it affects. If the PR
  changes a control's implementation, flip the row's status and citation in the same commit.
- **New endpoints/entities:** the V4.2.1 and API1 rows are where a new `user_id` scope or its
  absence gets recorded; the V7.2.2 row is where a new access-denied path gets noted.
- **New dependencies:** update V14.2.1/14.2.4 if the CI scanner set changes; the SBOM row if
  image attestation changes.
- **N/A rows are decisions:** if a control starts to apply (say, a second process or a managed
  database appears), its row must flip from `not-applicable` to a real status.
- **grep-ability check:** no row is `satisfied` without a `file:line` or test-name citation; a
  review can `grep -n "4.2.1" docs/security/asvs-l2.md` and land on the answer.
