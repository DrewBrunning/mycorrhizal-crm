# Generated docs map

Regenerate with the command; commit the diff; run the drift test. Never
hand-edit these files. The full inventory is in `backend/AGENTS.md` and
`docs/AGENTS.md`; this is the docs-only map.

| Generated doc | Generator | Drift test |
|---|---|---|
| `docs/data-01-field-compatibility-matrix.md` | `cd backend && go run ./cmd/gencompatmatrix` | `backend/correspondence/matrix_test.go` |
| `docs/int-01-integration-classification-matrix.md` | `cd backend && go run ./cmd/genintegrationmatrix` | `backend/integrations/matrix_test.go` |
| `docs/development/perf-benchmarks.md` | `cd backend && go run ./cmd/perfbench` | `backend/internal/perfbench` baseline assertions (report itself is not drift-gated) |
| `docs/development/data-movement-benchmarks.md` | `cd backend && go run ./cmd/perfbench` | `backend/internal/perfbench` data-movement baseline assertions |
| `docs/configuration-reference.md` | `cd backend && go run ./cmd/genconfigreference` | `backend/config/register_test.go` (`TestConfigurationReferenceDocUpToDate`) |
| `backend/database/testdata/schemas/*.sql` | `cd backend && go run ./cmd/genschema` | schema-fixture completeness test |
| `THIRD_PARTY_NOTICES.md` | `cd backend && make gen-notices` | `cmd/gennotices -check` (unit-tests `docs-citations`) |
| `internal/apibaseline/testdata/v1.json` | `cd backend && go run ./cmd/genapibaseline` | `backend/internal/apibaseline` drift test |
| `frontend/src/generated/openapi.ts` | `cd backend && go run ./cmd/gentsapi` | `backend/internal/tsapi` + `npx tsc --noEmit` |
| `backend/testdata/contract-fixtures/` | `cd backend && go run ./cmd/gencontract` | `backend/contract_fixtures_test.go` |
| `.gremlins/*.yaml` | `cd backend && go run ./cmd/genmutationscope` | `backend/internal/mutationscope` drift test |

## Rules

- A generated file's diff is the review; do not summarize it away.
- If a generator command does not exist, the doc is hand-authored — do not
  invent a generator.
- A docs-only PR still runs `docscheck`/`citecheck` unconditionally in the
  `docs-citations` job; path filters do not exempt it.
- `docs/development/perf-benchmarks.md` and its data-movement sibling warn on a
  wall-clock trend; the deterministic baseline is the gate.

Canonical: `docs/AGENTS.md`, `backend/AGENTS.md`.
