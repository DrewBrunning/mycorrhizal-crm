# Route change checklist

Update every matrix the route belongs to; do not add a parallel list.

| Concern | Where it is enforced | What to add/change |
|---|---|---|
| Spec ↔ router | `backend/openapi_request_test.go` (`TestOpenAPIRouteCoverage`) | register the route and document it in `backend/openapi.yaml` |
| Request fields | `backend/openapi_request_test.go` (`TestOpenAPIRequestFieldCoverage`) | bind every documented field; no mass assignment |
| Response shape | `backend/openapi_test.go`, `backend/openapi_spotcheck_test.go` | DTO keys and examples match the spec |
| Captured fixtures | `backend/testdata/contract-fixtures/` via `cmd/gencontract` | regenerate, never hand-edit |
| Generated TS | `frontend/src/generated/openapi.ts` via `cmd/gentsapi` | regenerate; `npx tsc --noEmit` must pass |
| Frozen baseline | `backend/internal/apibaseline/testdata/v1.json` via `cmd/genapibaseline` | deliberate diff; removals are declared breaking changes |
| Ownership | `backend/routes/ownership_matrix_test.go` | a new route/access path gets a `user_id` scoping regression |
| Authorization | `backend/middleware/`, `backend/routes/routes.go` | role/route policy matches the documented surface |
| Idempotency | `backend/controllers/idempotency_e2e_test.go`, ADR 0010 | a retry returns the original result, not a second effect |
| Conditional writes | `backend/controllers/conditional_write_*_test.go`, ADR 0008 | `If-Match`/revision comparison and 409 semantics |
| Pagination | handler + spec + frontend client | stable ordering and cursor/limit bounds |
| Errors | `apperrors`, spec responses | same code/status/envelope for the same failure |
| Android fixtures | `android/core/network/src/test/kotlin/com/mycorrhizal/crm/network/ContractFixtureTest.kt` | read the same committed JSON as web |
| API fuzzing | `schemathesis.yml`, `backend/cmd/schemagate` | no unhandled 500 or spec violation |

## Generated-artifact chain

`backend/openapi.yaml` → `go run ./cmd/gencontract` →
`go run ./cmd/gentsapi` → `go run ./cmd/genapibaseline` → fix `tsc`/Android.

The drift tests fail until every generated file is committed. Regenerate; never
edit `frontend/src/generated/openapi.ts` or `internal/apibaseline/testdata/v1.json`
by hand.

Canonical: `docs/breaking-change-policy.md`, `docs/client-compatibility-policy.md`,
`backend/AGENTS.md`.
