# ASVS L2 + MASVS-L1 verification report

`asvs-l2.md` and `masvs-l1.md` are **mappings**: every control has a status and a citation. ASVS is a
**verification** standard, so a mapping alone does not support a level claim — the claim only holds
once each line item has been checked against evidence that exists and passes. This document is that
check: a dated record of *how* each class of control was verified, what the verification found, and
what is explicitly claimed as a result.

It deliberately does **not** restate the 301 control rows. The two checklists stay the single
per-control source of truth (every security-sensitive PR is already required to update the row it
touches), and this report is the point-in-time audit over them. A re-verification edits this file's
header, findings, and changelog — not 301 duplicated rows.

| | |
|---|---|
| **Pass** | #1 (initial self-assessment) |
| **Date** | 2026-08-26 |
| **Commit verified** | `6a7cb7a2` (branch `claude/issue-378-v0-6-1-6e589d`), release line v0.6.1 |
| **Standards** | OWASP ASVS 4.0.3 (V1–V14), OWASP API Security Top 10 (2023), OWASP MASVS 1.5.0 (V2–V7) |
| **Scope** | Backend (Go/Gin + SQLite), web frontend (React SPA), Android client, deployment artifacts (Docker/nginx/compose), CI |
| **Performed by** | Self-assessment (issue #378). Third-party verification is explicitly out of scope and is tracked separately as issue #511. |

## The claim

> **ASVS Level 2, with 31 documented exceptions.** **MASVS-L1, with 1 documented exception** (plus two
> L2 controls satisfied as a bonus, not as a level claim).

Stated plainly, without a silent downgrade: 186 of 257 ASVS control rows are `satisfied` with
verified evidence, 38 are `not-applicable` with a written reason, 2 are L3-only and out of scope, and
**31 are `partial`** — each naming its own gap. Every one of those 31 is enumerated in
[§7 Exception register](#7-exception-register). A reader who rejects any single exception should read
the claim as "L2 except that control", which is the point of enumerating them.

Nothing in either checklist is `fail`. That is a real property of this pass, not a definitional
dodge: `partial` here always means "the control is met in part and the shortfall is named", never
"unimplemented but softened".

### Status census

Generated from the checklists by `go run ./cmd/citecheck` (backend/), so these counts cannot drift
away from the tables they summarize.

| Chapter | satisfied | partial | not-applicable | out-of-scope |
|---|---|---|---|---|
| V1 — Architecture, Design and Threat Modeling | 32 | 2 | 4 | — |
| V2 — Authentication | 25 | 11 | 8 | — |
| V3 — Session Management | 13 | 4 | 1 | — |
| V4 — Access Control | 7 | 1 | 1 | — |
| V5 — Validation, Sanitization and Encoding | 21 | — | 5 | — |
| V6 — Stored Cryptography | 10 | 1 | 3 | 2 |
| V7 — Error Handling and Logging | 10 | 1 | 1 | — |
| V8 — Data Protection | 9 | 3 | 3 | — |
| V9 — Communication | 4 | — | 3 | — |
| V10 — Malicious Code | 4 | — | 1 | — |
| V11 — Business Logic | 4 | 3 | 1 | — |
| V12 — Files and Resources | 12 | 1 | 2 | — |
| V13 — API and Web Service | 7 | 2 | 2 | — |
| V14 — Configuration | 19 | 1 | 3 | — |
| API Security Top 10 (2023) | 9 | 1 | — | — |
| **ASVS total** | **186** | **31** | **38** | **2** |
| MASVS V2 — Data Storage and Privacy | 8 | 1 | — | — |
| MASVS V3 — Cryptography | 6 | — | — | — |
| MASVS V4 — Authentication and Session Management | 7 | — | 1 | — |
| MASVS V5 — Network Communication | 3 | — | — | — |
| MASVS V6 — Platform Interaction | 6 | — | 3 | — |
| MASVS V7 — Code Quality and Build Settings | 7 | — | 2 | — |
| **MASVS total** | **37** | **1** | **6** | — |

---

## 1. What "verified" means, per class of control

ASVS conformance is a claim about evidence, so the verification method has to differ by what the
evidence *is*. Five classes, five methods:

| Class | How it was verified this pass |
|---|---|
| **Code-cited** (`file:line`) | Two steps, because they fail differently. (a) *Resolution* — the path exists and the line range is inside the file: fully automated, `go run ./cmd/citecheck`, now a CI gate. (b) *Content* — the cited lines still say what the row claims: `go run ./cmd/citecheck -drift` produces the candidate list, then human review of every candidate. Step (b) is where this pass found almost everything (§4, F-1). |
| **Test-cited** (a `TestXxx` name, a `_test.go` file, a Kotlin `SomeTest.method`) | The identifier exists in the tree (automated, `citecheck`) **and** the suite it belongs to passes (§3). A named test that no longer exists is a gate failure, not a warning. |
| **Tool-cited** (SARIF, a CI workflow, a gate command) | The workflow file and step exist at the cited lines, the tool is a hard gate rather than report-only where the row claims it is, and its tier (PR / main-merge / nightly, per issue #578) is stated correctly. Inventoried in §3. |
| **Doc/position-cited** (a `docs/` section, one of the P1–P5 positions) | The referenced section exists and still says what the row says it says, and the position's stated revisit trigger has not fired. |
| **`not-applicable`** | Verified as a *structural* claim about the architecture, not an absence of effort: single process, no cloud IAM, no XML parser, RP-only OIDC, and so on. Each row carries its own one-line reason; this pass confirmed none of the 38 reasons has been invalidated by an architecture change (the trigger that would flip them). |

Two method notes worth writing down, because they bound how much this pass proves:

- **Resolution is exact; content is heuristic plus human.** `citecheck` can prove a citation points
  *somewhere real*; only a reader can confirm it points at the *right* thing. The `-drift` heuristic
  narrows 360 line-range citations to a few dozen candidates by asking whether the prose leading up
  to a citation shares any vocabulary with the lines it cites. It has false positives by design
  (negative claims and pure-structure citations legitimately share no words with their target), so
  its output is a review queue, never a gate.
- **Ambiguous basenames resolve permissively.** The checklists cite `auth.go:141-154` where the
  surrounding row makes clear whether that is `middleware/auth.go` or `carddav/auth.go`.
  `citecheck` accepts a line range that is valid for *any* candidate with that basename. The
  disambiguation is the reader's; the tool only rules out ranges that fit none of them.

### Reproducing the automated half

```bash
cd backend && go run ./cmd/citecheck && go run ./cmd/citecheck -drift
```

The first command is the gate (exit 1 on any unresolvable citation, out-of-range line, off-legend
status, empty evidence cell, `satisfied` row with no citation, or vanished test identifier). The
second prints the drift review queue. Both run on every PR as the `Security-doc citations` job in
`.github/workflows/unit-tests.yml`, deliberately without a path filter: a citation is orphaned by
*moving code*, not by editing the doc, and `.github/filters.yaml` maps `docs/**` to nothing.

---

## 2. Automated verification: citations and structure

`citecheck` over `asvs-l2.md`, `masvs-l1.md`, and `threat-model.md`:

| Measured | Result |
|---|---|
| Path citations | 744 (584 + 92 + 68), of which 360 carry a line or line range |
| Test-identifier citations | 39 (Go `TestXxx` and `TestFamily_*`, Kotlin `SomeTest.method`) |
| Control rows parsed | 301 (257 ASVS + 44 MASVS) |
| Citations that do not resolve | **0** (after the §4 corrections) |
| Line ranges outside their file | **0** (after the §4 corrections) |
| Rows with an off-legend status or empty evidence | **0** |
| `satisfied` rows citing nothing | **0** (13 before this pass — §4, F-2) |
| Deliberate non-file citations | 2, allowlisted with a reason in `cmd/citecheck/main.go` (`google-services.json`, `/.well-known/security.txt`) |

---

## 3. Automated evidence inventory

The suites and scanners that stand behind the `satisfied` statuses, with what each actually proves.
Tiering is issue #578's: **PR** = blocks a pull request, **main** = runs on merge to `main`,
**nightly** = scheduled.

### Suites run for this pass

| Suite | Result | Scale |
|---|---|---|
| `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` | pass (exit 0), 27 packages | 2,625 test functions across 364 test files |
| `cd backend && go test ./cmd/citecheck/` | pass | the gate above, plus 24 fixture cases covering each failure class |
| `cd frontend && npx tsc --noEmit && npx vitest run` | pass (exit 0) | 1,304 tests across 150 test files |
| `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/unit-tests.yml` | pass | the workflow carrying the new gate |

Not re-run in this pass, because they need a live stack rather than a checkout, and each is a CI gate
in its own right: Playwright E2E (`e2e-tests.yml`), Android instrumented E2E (`android-tests.yml`, 42
`@Test` methods), Schemathesis + `bolacheck` + `schemagate` (`schemathesis.yml`), ZAP DAST + `zapgate`
+ `dastcanary` (`zap-dast.yml`). Their evidence value for this pass is that they are hard gates whose
last runs on `main` are green, not that they were re-executed here.

### Security-relevant checks, by what they prove

| Evidence | Proves | Tier |
|---|---|---|
| `backend/routes/authorization_matrix_test.go` (#371) | Every route registered on the live router × six personas (unauth / owner / intruder / admin / disabled / expired). Enumerated from `router.Routes()`, so a new route with no declared authorization row **fails**, and a declared row with no route fails as stale. This is the strongest single piece of ASVS V4 evidence in the repo. | PR |
| `backend/cmd/bolacheck` (#369) | Cross-account BOLA sweep over the real HTTP surface: user B creates one resource of each of 13 entity types, user A attempts GET/PUT/DELETE on every one. 2xx or 5xx fails the run. | PR (schemathesis.yml) |
| Schemathesis + `backend/cmd/schemagate` (#369) | Spec-derived request fuzzing from `openapi.yaml`: 500s on malformed input, auth-protected operations returning data unauthenticated. `schemagate` is the gate, not the scanner's exit code. | PR |
| ZAP DAST + `zapgate` + `dastcanary` (#368) | Dynamic scan of the running app, gated on High/Medium with an ignore-list-with-justification. `dastcanary` is a deliberately-vulnerable sibling server whose planted findings prove the scanner and gate are actually working — a green scan cannot be a silently broken scan. | nightly |
| `backend/.semgrep/mycorrhizal-traps.yaml` (#370) | Five custom rules for this codebase's own recurring bug classes, each with a must-match fixture in `.semgrep/tests/`. `mycorrhizal-query-string-auth-material` is the standing enforcement behind ASVS 8.1.3 / 8.3.1 / 13.1.3. | PR (sast.yml) |
| `backend/errors/fail_secure_realdb_test.go`, `errors/middleware_test.go` (#366) | A forced DB failure yields a typed code and no raw driver error; a panic yields a generic 500 with stack and panic value logged server-side only. | PR |
| `backend/services/oidc_attack_matrix_test.go`, `controllers/oidc_attack_matrix_test.go` (#412) | `state`/nonce/PKCE binding, authorization-code replay, issuer/audience/`azp` validation, and the account-mix-up invariant (an OIDC identity may never authenticate as another local account because an attacker controls an email address). | PR |
| `backend/services/contact_sync_hostile_input_test.go` (#512) | A hostile remote CardDAV update cannot wipe or downgrade a secret-sensitivity custom field or relationship edge. | PR |
| `backend/controllers/contact_share_matrix_test.go` (#555) | Sensitivity filtering holds across an actual account boundary, asserted against the stored share payload rather than the API response; selecting a sensitive section cannot imply the opt-in. | PR |
| `backend/routes/session_lifecycle_test.go`, `middleware/auth_lifecycle_test.go` (#372) | `TokenVersion` revocation: password change / reset / admin reset each kill pre-existing sessions. | PR |
| `backend/httputil/fetch_test.go`, `services/webhook_ssrf_test.go`, `webhook_ssrf_integration_test.go`, `notification_service_test.go` (#373) | SSRF guard on the live webhook delivery path and the push path, not just the dialer in isolation. | PR |
| `backend/models/audit_chain.go` + `cmd/audit-verify` (#381) | Tamper-evidence: each `AuditEvent` commits `SHA-256(prev_hash ‖ content)`, a `BEFORE UPDATE` trigger rejects edits, and the operator can verify the chain out-of-band. | PR + operator |
| `database/concurrent_write_test.go` | `_txlock=immediate` — the DSN flag without which concurrent writes 500 with `database is locked` in under 5 ms. | PR |
| CodeQL, Trivy (misconfig + secret), zizmor, actionlint, shellcheck, golangci-lint (gosec + bodyclose), govulncheck, Dependency Review | SAST/SCA/workflow-security hard gates. | PR |
| Signed SBOM (`syft-sbom.yml`), Grype, TruffleHog git-history | Supply-chain second opinions. | main |
| Stryker mutation testing, full-length fuzz, CIS container hardening | Test-suite quality and container baseline. | nightly |
| **`backend/cmd/citecheck` (this pass)** | Every citation in the security checklists resolves, and no `satisfied` row cites nothing. | PR |

---

## 4. Manual verification records

The four manual checks issue #378 requires, plus what each found. "Manual" here means a human read
the code; where a script produced the candidate list, the script is named and its output was reviewed
item by item.

### A. Handler scoping — no IDOR

**Method.** Static sweep of all 50 non-test controllers: for every function containing a GORM query
verb (`First`/`Find`/`Take`/`Last`/`Where`/`Delete`/`Updates`/`Save`/`Create`/`Count`/`Scan`/`Pluck`/
`Model`/`Raw`/`Exec`), check whether the function body — comments stripped, so a comment mentioning
`user_id` cannot pass for scoping — carries an ownership token (`user_id`, `userID`, `VCardUID`,
`currentUserID`, or a scoped loader helper). Every function without one was then read.

**Result.** 222 controller functions contain a DB query. **4** carry no in-function ownership token,
and all four are correct:

| Site | Why it is correct |
|---|---|
| `admin_user_controller.go:186` `ListUsers` | Admin-only route, deliberately cross-user; gated by `middleware/admin.go` and covered by the authorization matrix's `admin` persona. |
| `user_controller.go:100` `LoginUser` | Looks up the account being authenticated. There is no caller identity to scope by yet — unscoped by definition. |
| `timeline_controller.go:304` `applyDateBounds` | Query-builder helper. It receives a `base` query that every caller has already scoped (`timeline_controller.go:346,368,372,391,411,434,482`). |
| `timeline_controller.go:313` `applyCursor` | Same. |

**Verdict: no IDOR found.** This is a static complement to the runtime evidence, which is the
stronger of the two: `authorization_matrix_test.go` probes every registered route with an `intruder`
persona and fails on a route with no declared authorization row, and `bolacheck` sweeps 13 entity
types cross-account over real HTTP.

### B. Session cookie flags

**Method.** Enumerated every `SetCookie` call site in the backend (script-extracted, including
multi-line calls, with the governing `SetSameSite` resolved per site), then read each one.

**Result.** 18 sites across `user_controller.go`, `two_factor_controller.go`, and
`oidc_controller.go`.

- **`HttpOnly`: true at all 18.** No site passes `false`; there is no JS-readable session cookie.
- **`Secure`: `cfg.CookieSecure` at every session-cookie site** (`auth_token`, `id_token`,
  `2fa_pending`, and their clears), with boot refusing the `FRONTEND_URL=https://` +
  `COOKIE_SECURE=false` combination (`config/config.go:527-532`).
- **`SameSite`: `Strict` on every session cookie** (`user_controller.go:203,234,262`,
  `two_factor_controller.go:415,466`, `oidc_controller.go:249`), tightened from `Lax` by issue #392.
- **`SameSite=Lax` on exactly four cookies, correctly**: the transient OIDC handshake cookies
  `oidc_state`/`oidc_nonce`/`oidc_pkce`/`oidc_client` (`oidc_controller.go:87-99`). These are read on
  a top-level cross-site navigation back from the identity provider, which a `Strict` cookie would
  never be attached to — `Strict` here would break every OIDC login. They are additionally
  path-scoped to `/api/v1/auth/oidc/callback` and expire in 600 s.

**One deviation found**, filed as issue #605: `oidc_client` and all four handshake-cookie clears
hardcode `Secure=true` instead of `cfg.CookieSecure`. That is *stricter* than configured, so it is
not a confidentiality gap — but on the supported plain-HTTP deployment a browser rejects the cookie
outright, so the Android client hint is never stored and the handshake cookies are never actively
cleared. Functional inconsistency, fails closed; recorded in row 3.4.1.

### C. Cryptography, and the absence of unauthenticated modes

**Method.** Read every cryptographic call site and every route registration.

| Checked | Found |
|---|---|
| Password hashing | bcrypt at `bcrypt.DefaultCost` (10) through one shared `HashPassword` (`services/user_service.go:16-27`). Passwords over 72 bytes are **rejected** (`ErrPasswordTooLong`), not silently truncated — the bcrypt cap is explicit. See P1 for why not Argon2id yet. |
| JWT algorithm | The verifier pins the HMAC family before the key callback returns the secret: `token.Method.(*jwt.SigningMethodHMAC)` (`middleware/auth.go:66-71`). That is what defeats `alg: none` and RSA→HMAC confusion. The pin is family-wide rather than HS256-exact; HS384/HS512 would also verify, and all three take the same symmetric secret, so there is no downgrade to reach. Row 3.2.4 previously overstated this as HS256-exact and has been made precise. |
| Signing secret strength | Enforced at boot in four ordered checks — non-empty, ≥ 32 bytes, not a known published placeholder, minimum Shannon entropy (`config/config.go:375-403`). A long-but-published secret is as forgeable as a short one, which is why the placeholder check exists alongside the length floor. |
| Symmetric encryption | AES-256-GCM only — authenticated as it encrypts — for TOTP/integration credentials (`services/credential_crypto.go:33-58`, key HKDF-SHA256-derived from `JWT_SECRET_KEY`) and the at-rest field envelope (`backend/atrest`, single wrapped DEK, see P4). No unauthenticated mode is reachable by a caller. |
| Weak primitives | No MD5/DES/RC4/ECB/Blowfish anywhere. The tree's single `crypto/sha1` import is HIBP's own k-anonymity wire format, annotated at the import (`services/hibp_service.go:6`). SHA-256 is used only for non-reversible one-time token digests. |
| Random | `crypto/rand` for TOTP secrets and recovery codes (`services/twofactor.go:4,107`); bcrypt supplies its own per-hash salt. No `math/rand` in a security path. |
| Unauthenticated route surface | **13 registrations**, enumerated from `routes/routes.go`: `/health`; the two `.well-known` CardDAV/CalDAV redirects; `/auth/oidc/config` plus `login`/`callback` (registered only when OIDC is enabled); `register`; `login`; `login/2fa`; `logout`; `check-password-strength`; `password-reset/request`; `password-reset/confirm`. Every one that touches credentials carries `AuthRateLimitMiddleware`. Against 241 `protected.` + 9 `admin.` + 9 CardDAV/CalDAV Basic-auth routes. |
| Any way to turn auth off | **None.** No `DISABLE_AUTH` / `SKIP_AUTH` / `INSECURE_*` switch exists in any non-test Go file; `AuthMiddleware` is applied at the group level, so a route is either inside `protected`/`admin` or is one of the 13 above. The authorization matrix's `unauth` persona asserts this for every route rather than trusting the grouping. |

### D. Error paths do not leak internals

**Method.** Read the error envelope and the last-resort handler, then searched every non-test Go file
for a response body carrying a raw `err.Error()` or an equivalent unbounded error string.

**Result.** The envelope is `{error:{code,message,details}, request_id, timestamp}`
(`errors/middleware.go:13-24,67-86`); internal errors are generic text; the panic-recovery middleware
logs the panic value and stack server-side and returns a generic 500
(`errors/middleware.go:27-44`). Both are pinned by tests (#366).

**One exception found**, and it is the only site in the backend that bypasses the envelope: the
self-service "test my notification channel" endpoint echoes the upstream error verbatim
(`notification_controller.go:167`). That is defensible — the endpoint exists so a user can find out
why *their own* ntfy/Gotify/push target rejected the message, and a generic message would make the
feature useless.

The outbound path was then traced to the end, because "unbounded error from an HTTP client" reads
like an SSRF oracle and it is worth being exact about whether it is one. **It is not, on the default
configuration.** `WEBHOOK_BLOCK_PRIVATE_URLS` defaults to `false` (`config/config.go:148`), so
`postNotificationJSON` skips its private-address pre-flight and `clientFor` returns the unguarded
client (`services/notification_service.go:836`, `services/webhook_service.go:64-70`); save-time
validation checks only the scheme (`middleware/validation.go:182-194`). That is the documented
opt-in-per-service position (row 5.2.6 / API7), and it is deliberate: pointing a self-hosted app at
an ntfy instance on your own LAN is the intended use, so defaulting the block on would break the
common case. The caller therefore already chose the target and already learns its reachability from
the returned status; the error text grants no capability they lack.

The genuine leak is the inverse, and only in the hardened configuration: with the flag **on**, the
guarded dialer returns two distinct sentinels — `ErrWebhookUnreachable` and
`ErrWebhookPrivateAddress` (`services/webhook_service.go:29-30`, `httputil/safedial.go:27-47`) — and
both are echoed verbatim, so an operator who turned the flag on to declare internal targets
off-limits gets an endpoint that reports which rule a target tripped. Thin (it confirms "the address
you supplied is private", about an address the caller supplied) but the wrong direction. Accepted and
now **documented** in row 7.4.1 rather than left as an unexplained outlier; bounding the string and
collapsing the two sentinels is issue #606.

---

## 5. Per-chapter verification notes

What this pass actually did per chapter, beyond the automated citation checks that cover all of them.

| Chapter | Verified this pass |
|---|---|
| **V1** Architecture | Confirmed the threat model (#377) is current except its §5, which asserted `SameSite=Lax` where the code has been `Strict` since #392 — corrected (F-3). CI inventory in 1.1.1 re-checked against the 23 workflows and their #578 tiers, and `citecheck` added. |
| **V2** Authentication | Manual audit C. The 11 `partial` rows are the largest cluster in the checklist and are all deliberate product positions (no view-password toggle, no notification on self-service change, no reuse detection on TOTP) — none is an unimplemented control. |
| **V3** Session Management | Manual audit B; every cookie flag read at its call site. 3.2.4 made precise about the algorithm pin. |
| **V4** Access Control | Manual audit A, plus confirming the authorization matrix still fails on an undeclared route (that property, not the current pass/fail, is what makes it evidence). |
| **V5** Validation | Confirmed the raw-SQL sites are parameterized: FTS search (`search_service.go:228-305`), the graph CTE (`graph_traversal.go:96-131`), and the export grouping query, whose only interpolated values are compile-time table/column constants (`export_controller.go:105-119`). |
| **V6** Stored Cryptography | Manual audit C. 6.2.5 and 6.2.7 were `satisfied` with no citation at all and now cite the actual call sites plus the gosec/CodeQL enforcement (F-2). |
| **V7** Error Handling | Manual audit D; found and documented the one envelope bypass. |
| **V8** Data Protection | 8.1.3 and 8.3.1 were uncited assertions about absence and now cite the Semgrep rule that continuously enforces them (F-2). |
| **V9** Communication | TLS boundary re-confirmed at `docs/deployment.md:17`; HSTS wiring re-located after `main.go` drift (`security_headers.go:43-45`, wired `main.go:266`). |
| **V10** Malicious Code | 10.3.2's SRI claim now cites why SRI is moot here (no external origin in the CSP) rather than asserting it (F-2). |
| **V11** Business Logic | The 3 `partial` rows are product decisions (per-user content quotas, unusual-activity monitoring, alerting) — unchanged, restated in §7. |
| **V12** Files and Resources | 12.3.6 now cites the lockfiles and the absence of `os/exec` in non-test packages rather than asserting it (F-2). |
| **V13** API | 13.1.3 now cites the Semgrep rule; 13.2.6 now states the deliberate absence of application-layer signing (F-2). API9 stays `partial` — no endpoint-inventory doc. |
| **V14** Configuration | The heaviest drift cluster: CORS, trusted proxies, and the boot validators had all moved within `main.go`/`config.go` (F-1). All re-located and re-read. |
| **API1–API10** | API1's evidence (the strongest claim in the file — "every handler AND-scopes") re-verified by manual audit A and the matrix/bolacheck pair. |
| **MASVS V2–V7** | Citations re-located after the Android tree moved (F-1); `deepLinkRoute`/`parseOidcReturn` re-read to confirm the strict scheme+host+path validation the PLATFORM-2/3 rows claim. STORAGE-5's login-screen gap is unchanged and remains the single MASVS exception. |

---

## 6. Findings

| # | Finding | Severity | Disposition |
|---|---|---|---|
| **F-1** | **48 distinct citations (74 occurrences) pointed at code that had moved.** Every one resolved to a real file with an in-bounds line range, so nothing flagged them: `main.go:191-209` cited for the CORS allowlist actually landed in the scheduler; `unit-tests.yml:175-177` cited for govulncheck landed in the fuzz step; `config.go:359-364` and `:375-380`, cited for the CORS and `COOKIE_SECURE` boot checks, landed in the at-rest-key comments the #380 work inserted above them. Also here: `webhook_service_test.go` (a file that no longer exists) and `errors/middleware.go:111-113` (a 112-line file). | High for the *checklist's* credibility; no code defect | **Fixed on this branch.** All 74 corrected and re-verified. `citecheck` now gates resolution on every PR, and `-drift` is the standing review queue for the content half. |
| **F-2** | **13 rows were `satisfied` with no citation whatsoever**, contradicting the checklist's own stated promise ("No row is left `satisfied` without a citation"). All 13 were negative controls — "no password expiry", "no KBA", "no CDN", "no plugin system" — where there is no `file:line` for a thing that does not exist, so they had been left as bare assertions. | Medium: unverifiable rows in a verification document | **Fixed on this branch.** Each now cites the artifact that proves the absence — the model/migration that has no such column, the Semgrep rule that fails a PR reintroducing it, the CSP that admits no external origin, the lockfiles. `citecheck` now fails a `satisfied` row that cites nothing. |
| **F-3** | **`threat-model.md` §5 stated the session cookie is `SameSite=Lax`.** It has been `Strict` since issue #392. A factual error, not a stale line number. | Medium: the threat model understated an implemented control | **Fixed on this branch**, including the reason `Lax` is retained for the OIDC handshake cookies only. |
| **F-4** | **OIDC handshake cookies hardcode `Secure=true`** on the `oidc_client` set and all four clears, while their siblings use `COOKIE_SECURE` (manual audit B). Stricter than configured, so not a confidentiality gap — but on the supported plain-HTTP deployment the browser rejects the cookie, so Android OIDC silently falls back to the web redirect and the handshake cookies are never actively cleared. | Low, fails closed | **Filed as issue #605** (v0.6.2). Recorded in row 3.4.1. |
| **F-5** | **The test-notification endpoint echoes an unbounded upstream error** to the client (manual audit D) — the only backend site bypassing the error envelope. Defensible as a self-service diagnostic (the user needs to know why *their own* ntfy/Gotify target refused the message), but unbounded. Traced to the end during review: **not** an SSRF oracle on the default configuration, because `WEBHOOK_BLOCK_PRIVATE_URLS` defaults off and pointing a self-hosted app at your own LAN is the intended use (row 5.2.6's opt-in-per-service position) — the caller already chose the target and already learns its reachability from the status code. The real, thin leak runs the other way: with the flag *on*, the guarded dialer's two distinct sentinels are echoed verbatim, so the hardening flag makes this endpoint marginally more informative rather than less. | Low | **Filed as issue #606** (v0.6.2, `p3`). Accepted as a documented exception in row 7.4.1. |
| **F-6** | **Row 3.2.4 overstated the JWT algorithm pin** as HS256-exact; the code pins the HMAC family. The security property (rejects `alg: none` and asymmetric confusion) is unchanged, but the row claimed more precision than the code has. | Low | **Fixed on this branch**: the row now states the family pin and why no downgrade is reachable. |

No finding in this pass required flipping a control from `satisfied` to `fail`. F-1 through F-3 and
F-6 were failures of the *documentation* to remain true; F-4 and F-5 are real code findings, both
low-severity and both failing closed.

### What this pass could not verify

Stated so the claim is not read as stronger than it is:

- **Content correctness beyond the drift queue.** 360 line-range citations were checked for
  resolution; the ones the drift heuristic did not surface were not each re-read line by line. A
  citation whose target moved *and* whose new neighbourhood happens to share vocabulary with the row
  would survive.
- **Runtime behaviour of the E2E/DAST tiers.** Verified as "green hard gates on `main`", not
  re-executed here (§3).
- **Anything a self-assessment structurally cannot establish.** No adversary tried to break this
  system as part of this pass. That is issue #511's job, and until it happens the honest framing is
  "we verified our own evidence", not "we were tested".

---

## 7. Exception register

"L2 with 31 documented exceptions" is only meaningful if the exceptions are enumerable. They are.

### The five written-down positions

Full reasoning lives in `asvs-l2.md` § Documented positions; the revisit trigger is what matters for
re-verification.

| | Position | Revisit when |
|---|---|---|
| **P1** | bcrypt cost 10, not Argon2id (satisfies NIST 800-63B as written) | A password-hash version field + rehash-on-login path exists, making migration a code change rather than a forced reset for every user |
| **P2** | No crypto-agility abstraction; bcrypt/JWT called directly | A second algorithm is actually adopted (Argon2id, EdDSA sessions, external KMS). An abstraction with one implementation is dead weight |
| **P3** | HIBP breach check opt-in, off by default | A hosted multi-tenant deployment mode exists, where the operator already accepts outbound-call tradeoffs on users' behalf |
| **P4** | FTS-indexed columns stay plaintext; single wrapped DEK; lost key = lost data | Search moves off SQL-trigger-driven FTS5, or key escrow becomes a product requirement |
| **P5** | Backup confidentiality and retention are the operator's boundary | The project ships a managed backup destination |

### The 31 ASVS `partial` rows

Grouped by why each is short, because the groups have very different meanings.

**Deliberate product positions (12)** — the control is understood and not wanted as ASVS states it:
2.1.1 (length floor 8 + 50 bits entropy, per 800-63B, not 12 chars), 2.1.2, 2.1.12 (no
view-password toggle), 2.1.7 (P3), 3.3.2 (no idle timeout — absolute 96 h expiry, chosen for a
personal CRM), 3.4.4 (no `__Host-` prefix while plain-HTTP LAN deployments are supported), 6.2.4
(P2), 8.3.5 (read-auditing is a privacy/performance choice), 11.1.3, 11.1.7, 12.1.3 (per-user
quotas are product decisions), 14.4.2.

**Missing UI or surface, mechanism present (5)**: 3.3.4 (no session-inventory UI — "log out
everything" works via password change or admin reset), 2.2.3 and 2.5.5 (notification exists on the
recovery path, not on self-service change), 4.3.1 (2FA available to all, not *enforced* for admins),
API9 (no endpoint-inventory doc; `openapi.yaml` is maintained and drift-checked).

**Partial mechanism (9)**: 1.6.1 and 1.6.3 (rotation works; no standalone runbook, and TOTP/
integration re-encryption after JWT rotation is not automated), 2.3.2, 2.6.2, 2.8.2, 2.8.4, 2.8.5
(no TOTP reuse detection), 3.5.2, 13.2.5.

**Known small gaps with an obvious fix (5)**: 7.2.2 (no distinct access-denied event), 8.1.1 and
8.2.1 (no `no-store` on API responses), 11.1.8 (no alerting on lockout spikes), 13.1.5 (content-type
rejection + correct status).

### The 1 MASVS `partial` row

**STORAGE-5** — the login screen's password field masks and autofills correctly but keeps default
keyboard options, so it lacks the `KeyboardType.Password` IME-learning signal that register /
forgot-password / settings fields have. The one field where a password is typed most often.

MASVS also carries six positions of its own (`masvs-l1.md` § Documented positions), of which two —
P1 certificate pinning and P3 root/SafetyNet detection — were re-evaluated and kept declined under
issue #507, and two — P4 Room cache encryption and P6 screenshot/tapjacking protection — were
resolved in the affirmative and are now `satisfied`.

---

## 8. Re-verification procedure

A re-pass is a diff against this file, not a rewrite. In order:

1. `cd backend && go run ./cmd/citecheck` — must exit 0. Any failure is a citation to fix before
   anything else; the rest of the report means little on top of broken citations.
2. `cd backend && go run ./cmd/citecheck -drift` — read every candidate. Correct the range, or
   satisfy yourself it is a false positive. This is the step that found F-1.
3. Run the suites in §3 and record the results, including scale, so a shrinking suite is visible.
4. Redo the four manual audits in §4. A/B/C/D are cheap: the scripts and one-liners behind them are
   described inline, and each produced a candidate list of single digits.
5. Re-read the `not-applicable` rows against the current architecture. They are written-down
   decisions about a single-process, self-hosted, no-cloud, RP-only-OIDC system; an architecture
   change is what flips them, and nothing else will.
6. Re-check every P1–P5 revisit trigger in §7.
7. Update the header (pass number, date, commit), the census (from `citecheck`), §6 findings, and add
   a changelog row.

## 9. Changelog

| Pass | Date | Commit | Claim | Findings |
|---|---|---|---|---|
| 1 | 2026-08-26 | `6a7cb7a2` | ASVS L2 with 31 documented exceptions; MASVS-L1 with 1 | 6 (F-1…F-6): 4 documentation defects fixed on the branch, 2 code findings filed as issues #605 and #606. No control flipped to `fail`. |
