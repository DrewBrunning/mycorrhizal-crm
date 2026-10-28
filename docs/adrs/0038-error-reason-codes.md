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

## Client fallback contract

An unknown `reason_code`, or a missing param, means the client shows `message`
(or `details.reason` per the web rules). Clients must never fail on a code they
do not know; codes are added over time.

## Registry

`reasonRegistry` starts with one test-only code, `common_test_only`, removed in
#2100 once real codes exist. `ReasonCodes()` and `ReasonSpecFor()` feed the
client drift tests.
