# Contract fixtures (issues #257 + #266)

Shared fixtures the web and Android contract suites parse against, generated
from `backend/openapi.yaml` — the OpenAPI spec is the single source of truth,
not hand-captured responses. Each pinned response's `example:` in the spec IS
the fixture, pretty-printed verbatim. See `/CLAUDE.md` frontend trap #8 for the
bug class (null-vs-absent, required-field crashes) this pins.

- `contacts-list.json` — `GET /api/v1/contacts` (the `200` response example)
- `contact-detail.json` — `GET /api/v1/contacts/:id/detail` (web-only fixture;
  Android's `ApiClient` has no method for this composite endpoint yet)
- `dashboard.json` — `GET /api/v1/dashboard`

Consumed directly by:
- `frontend/src/api/contractFixtures.test.ts` (via TS `import`, no copying)
- `android/core/network/src/test/kotlin/.../ContractFixtureTest.kt` (via a
  `resources.srcDirs` entry in `android/core/network/build.gradle.kts` pointing
  at this directory — one canonical copy, not a duplicate per client)

## Regenerating

When the backend response contract changes, edit the example in
`backend/openapi.yaml` (and the schema if the shape changed), then:

```bash
cd backend && go run ./cmd/gencontract
```

or `make gen-contract-fixtures` from the repo root. Review the fixture diff
before committing — these files are meant to change deliberately, not silently.

The drift test `backend/contract_fixtures_test.go` (`TestContractFixturesMatchSpec`)
enforces this workflow in CI: it regenerates from the spec in memory and fails
if the checked-in files are stale, so a spec example change without a
regeneration breaks the build with the exact command to re-run.

## Guarantees

- **Spec-derived, not captured:** the fixtures come from the spec's examples,
  so a backend change is one edit (the spec) plus one regeneration, and both
  client suites react to the same regenerated files.
- **Example↔schema consistency:** the spec's own validator
  (`backend/openapi_test.go`) and the generator both reject an example that
  does not fit its schema, so a fixture cannot silently drift from the
  documented contract.
