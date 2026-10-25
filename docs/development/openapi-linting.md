---
title: OpenAPI linting
parent: Development
nav_order: 13
---

# OpenAPI linting

Issue [#1909](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1909) added a
design-consistency gate for [`backend/openapi.yaml`](../../backend/openapi.yaml), the source
for the contract fixtures (`gencontract`), the generated TypeScript types (`gentsapi`), the
`/api/v1` breaking-change baseline (`genapibaseline`), and the Schemathesis fuzz gate
(`schemagate`). The spec's own Go validator (`backend/openapi_test.go`) proves it is
well-formed; it does not check that operations are *documented consistently*.

## Spike: Spectral versus vacuum

Both tools were run against the pre-change spec (359 operations, 262 paths) on 2026-10-25.

| Tool | Version | Ruleset | Findings |
|---|---|---|---|
| Spectral | `@stoplight/spectral-cli` 6.15.0 | `spectral:oas` | 474: `operation-tags` 359, `operation-description` 112, `oas3-valid-media-example` 2 (errors), `info-contact` 1 |
| vacuum | 0.35.4 | `recommended` (55 rules) | 3,443: `oas3-missing-example` 1,299, `camel-case-properties` 846, `description-duplication` 478, `operation-tags` 359, `oas3-parameter-description` 170, `component-description` 150, `operation-description` 130, `oas-missing-type` 6, `no-request-body` 2, `paths-kebab-case` 1, `nullable-enum-contains-null` 1, `circular-references` 1 |

Spectral's `spectral:oas` set is a closer fit out of the box (fewer style opinions), and its
`oas3-valid-media-example` rule found two real errors. vacuum's `recommended` set is dominated
by documentation-style rules (`oas3-missing-example`, `description-duplication`) and by a
camelCase naming rule that directly contradicts the project's RFC 9553/9554/9555 neutral model.

**Decision: vacuum, not Spectral.** The deciding constraint is toolchain fit, not the initial
finding count. The API-contract checks are Go-based by design (see `openapi_test.go`'s tooling
note), the required `Backend (Go)` job already resolves its tools from `backend/tools.mod`, and
there is no Node toolchain assumed on that runner. vacuum is a Go tool, so its version is pinned
by `tools.mod` + `tools.sum` and it adds no runtime registry fetch. Spectral's one advantage
(media-example validation) is covered below by a small Go regression test for the class it found.

## Adopted ruleset

`backend/vacuum.yaml` extends vacuum's `recommended` set, turns off the style rules this spec
answers differently (with the reason in the file), promotes the contract rules to `error`, and
adds two project rules:

- `operation-summary-or-description` — every operation has a summary or description. The spec
  uses summaries; this replaces vacuum's `operation-description` rule.
- `error-envelope` — every documented JSON 4xx/5xx body is the shared
  `apperrors.AbortWithError` envelope (`#/components/schemas/Error`). It walks the raw
  (unresolved) document so the `$ref` is visible. A response with no JSON body is out of scope;
  the deliberate exceptions carry a local `x-lint-ignore: [error-envelope]` with a reason:
  `/health` and `/health/ready` (dedicated readiness/health bodies), `/.well-known/assetlinks.json`
  (bare 404 by design), `GET /feeds/atom` (the indistinguishable empty 404, ADR 0030), and the
  OIDC login 500 (a legacy bare `{error}` body on a non-injectable crypto/rand failure).

Initial findings under the adopted ruleset: **446** — `operation-tags` 359, `error-envelope` 85,
`nullable-enum-contains-null` 1, and the non-fatal `circular-references` warning 1.

The spec was fixed rather than suppressing those rules:

- all 359 operations now carry a tag, and 40 tag definitions were added;
- 80 documented error responses now carry the `Error` envelope (the other five are the
  deliberate exceptions above);
- `SystemEvent.result` no longer claims to be nullable (the Go model uses `omitempty`);
- five composed schemas (`cadence_policy_offer`, `duplicate_match`, `merge_diff`, `target`,
  and `ContactDetailResponse.immich.summary`) gained the `type: object` that OAS 3.0 requires
  for `nullable` to take effect.

After the fixes the adopted ruleset reports **0 rule findings**. One resolver warning remains:
`ContactFlat` → `Reminder` → `ContactFlat` is a real cycle in the legacy flat DTO. OpenAPI
allows recursive schemas, and vacuum reports the cycle at warning severity regardless of the
ruleset; it does not fail the gate (the default failure severity is `error`, and every project
rule is promoted to `error`).

## Overlap with existing checks

| Existing check | What it owns | What the linter adds |
|---|---|---|
| `backend/openapi_test.go` (`TestOpenAPISpecValidates`, `TestOpenAPIRouteCoverage`) | Well-formedness, schema presence, route↔spec drift | Operation tags, summaries, error-envelope shape, unused components |
| `backend/openapi_request_test.go`, `openapi_spotcheck_test.go` | Request binding and real response bodies against the spec | Structural consistency of operations the spot-check does not exercise |
| `backend/routes/authorization_matrix_test.go`, `authorization_matrix_credentials_test.go`, `ownership_matrix_test.go` | The live route surface's authentication/ownership classification | **No duplication, by design.** The spec declares a global `security` block and each public operation overrides with `security: []`; the linter only verifies that referenced schemes exist (`oas3-operation-security-defined`). Which routes are public/admin is a runtime property, owned by the matrices |
| `gencontract`, `gentsapi`, `genapibaseline`, `schemagate` | Generated artifacts and spec-derived fuzzing | They consume the spec; they do not lint its design |

The linter does not replace any of these; the route and authorization matrices remain the
security authority.

## CI and local use

The `Backend checks` job in `.github/workflows/unit-tests.yml` runs:

```bash
cd backend && go tool -modfile=tools.mod vacuum lint -r vacuum.yaml -b -q --no-update-check openapi.yaml
```

Run the same command locally after editing `backend/openapi.yaml`. `--no-update-check` stops
vacuum's advisory network check. `vacuum` is pinned in `backend/tools.mod`; do not replace it
with `npx`/`go run @latest`.

Hand-verification (issue #1909 acceptance): deleting an `operationId` on a scratch copy fails
with `operation-operationId`; deleting a single `Error` response body fails with
`error-envelope`. Both were checked, then the scratch copy was discarded.

## Residual gap

vacuum does not validate media-type `example:` values (Spectral's
`oas3-valid-media-example` does). The two examples that rule found were fixed, and
`backend/openapi_nullable_test.go` pins the OAS 3.0 `nullable`-requires-`type` class it
surfaced (the test fails on the pre-change spec for five schemas). Other example coverage
remains the spec validator plus the exercised-response spot-check.
