# ADR 0038: Error reason codes — `reason_code` / `reason_params`

- **Status:** accepted
- **Date:** 2026-10-27
- **Implements:** issue #2095 (phase 1 of #2093).
- **Related:** `docs/breaking-change-policy.md` (MAINT-02), `backend/errors/reasons.go`.

## Context

Error `message` and `details.reason` are English strings, so web and Android
cannot localize server errors. Score facets already ship a
`reason_code` + `reason_params` pair that both clients parse
(`backend/internal/scoring/scoring.go`, the `ScoreFacet` schema in
`backend/openapi.yaml`). This ADR extends that mechanism to error responses.

## Decision

- **Additive, not breaking.** `error.reason_code` and `error.reason_params`
  are optional response fields (MAINT-02 "Adding a response field"). Both
  clients ignore unknown fields (`contractFixtures.test.ts`,
  `ContractFixtureTest.kt`).
- **The params field is `reason_params`**, matching the shipped score-facet
  precedent, not `params`.
- **Code shape** `^[a-z]+(_[a-z0-9]+)+$`; the first segment must be in
  `reasonAreas` in `backend/errors/reasons.go`. A new area is added to that list
  in the same PR as its first code. Codes are declared as
  `const ReasonXxx ReasonCode = "area_xxx"` and registered in `reasonRegistry`.
- **Score reason codes** (`closeness_direct`, …) are grandfathered: a separate
  namespace on a different payload; renaming them would be a MAINT-02 break.
  The prefix rule applies to error reason codes only.
- **Template syntax.** English templates use `{name}`. Android `strings.xml`
  uses positional `%N$s`/`%N$d`, numbered in the registry's declared param
  order. Web i18next uses `{{name}}`.
- **`reason_params` values** are strings or numbers, flat, with no secrets and
  no other user's data (ASVS V7). A param that names a field carries the JSON
  field name.
- **`message` stays English and the server never localizes it**, and
  `details.reason` stays. When a reason is attached both are set to the
  rendered English template. On `INVALID_INPUT` this replaces the generic
  "Invalid value for field 'x'" in `message`: a text change, not a contract
  change (no client matches on message text).
- **Fail-safe.** If the code is unregistered or the params do not match the
  registered names exactly, the error is sent *without* `reason_code` /
  `reason_params` and an Info-level `reason_params_mismatch` line is logged.
  A request path never panics over this.
- **Logging.** `LogError` adds `reason_code` (`logger.FieldReasonCode`) when set.

## Constructor defaults (issue #2099)

The generic constructors attach a code themselves, so their call sites need no
`.WithReason(...)`: `ErrDatabase`, `ErrDatabaseBusy`, `ErrInvalidCredentials`,
`ErrTokenExpired`/`ErrTokenInvalid`, `ErrClientNotSupported`,
`ErrRateLimitExceeded`, the three idempotency errors, `ErrMissingField`, the
empty-message `ErrPayloadTooLarge`, and `ErrNotFound`/`ErrAlreadyExists` for a
string-literal resource listed in `notFoundReasons`/`alreadyExistsReasons`
(`backend/errors/reasons.go`). A resource passed as a variable, or an
`ErrInternal` from an ordinary call site, gets no code. Only panic recovery and
`GetAppError`'s fallback carry `common_unexpected_error`. These defaults do not
set `details.reason` (the message already is the sentence), and
`ErrDatabase` no longer puts the operation name in `message`; it stays in
`details.operation` and the log line.

## Client fallback contract

An unknown `reason_code`, or a missing param, means the client shows `message`
(or `details.reason` per the web rules). Clients must never fail on a code they
do not know; codes are added over time.

## Milestone gate: the ratchet must reach zero

`internal/lint/reasoncode/allowlist.txt` is a counted ratchet: it stops the
number of unmigrated user-facing error sites from growing, but nothing in it
forces the number to reach zero. The migration is therefore bounded by a release
gate, not by the ratchet alone.

`reasoncodecheck -release-tag vX.Y.Z[-rc.N]` is additive to the normal check. For
a core version at or after `ZeroFromRelease` (`v1.6.0`, milestone v1.6.0,
issue #2093) it also fails while the allowlist lists any site. `release.yml`'s
`preflight` job runs it with the release version, so the tag is refused before any
expensive gate runs. A release candidate is ordered by its core version, so
`v1.6.0-rc.1` is gated too: the candidate that the final tag is cut from must
already be fully migrated. Releases before `v1.6.0` only need the ratchet to hold.
`TestReleaseWorkflowGatesReasonCodeMigration` fails if the workflow step is removed.

Raising `ZeroFromRelease` postpones the milestone and is a recorded decision, like
raising an allowlist count.

## Registry

`reasonRegistry` starts with one test-only code, `common_test_only`, removed in
#2100 once real codes exist. `ReasonCodes()` and `ReasonSpecFor()` feed the
client drift tests.

## Amendment: v2.0.0 target shape for the non-envelope error bodies (issue #2107)

Two error bodies bypass the standard envelope. Replacing them is a breaking
response-shape change (`error` goes from a string to an object), so it ships in
v2.0.0 (issue #2108) after the MAINT-02 deprecation window. This amendment records
the target only; **today's server behaviour is unchanged.**

| Body today | Target in v2.0.0 |
|---|---|
| Account-lockout 429 (`abortLocked`; password login, `/login/2fa`, passkey login, proof-gated routes): `{"error": "Account temporarily locked", "message", "retry_after", "retry_after_at", "reason_code", "reason_params"}` | Envelope: `error.code: "ACCOUNT_LOCKED"` (a new error code), `error.reason_code: "auth_account_locked"`, `error.reason_params: {retry_after}`, `error.details: {retry_after, retry_after_at}` |
| OIDC `failed to generate state` / `failed to generate nonce` (`oidc_controller.go`, flat `{"error": "<string>"}`) | `apperrors.ErrInternal(...)` through `AbortWithError` (the standard 500 envelope) |

Until v2.0.0 the flat lockout body is marked `deprecated: true` in `openapi.yaml`, its
responses carry the RFC 8594 `Deprecation`/`Sunset`/`Link` headers, and the register
row `flat-lockout-429-body` in `docs/deprecations.md` tracks the removal. Clients
must read `retry_after`/`retry_after_at` from either the top level or `error.details`
and resolve `reason_code` from either place (web `auth.ts`, Android `ApiClient`), so
a deployed client keeps working across the switch.
