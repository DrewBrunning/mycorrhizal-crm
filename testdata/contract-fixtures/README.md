# Contract fixtures (issue #257)

Real responses captured from a live backend, checked in so the web and Android contract-test
suites can assert their parsing code survives an actual server response — not a hand-written
mock. See `/CLAUDE.md` frontend trap #8 for the bug class this pins.

- `contacts-list.json` — `GET /api/v1/contacts`
- `contact-detail.json` — `GET /api/v1/contacts/:id/detail` (web-only fixture; Android's
  `ApiClient` has no method for this composite endpoint yet)
- `dashboard.json` — `GET /api/v1/dashboard`

Consumed directly by:
- `frontend/src/api/contractFixtures.test.ts` (via TS `import`, no copying)
- `android/core/network/src/test/kotlin/.../ContractFixtureTest.kt` (via a `resources.srcDirs`
  entry in `android/core/network/build.gradle.kts` pointing at this directory — one canonical
  copy, not a duplicate per client)

This is a small hand-captured set (per issue #257), not spec-generated — see issue #266 for the
deliberately-deferred spec-derived version.

## Regenerating

```bash
docker compose -f docker-compose.test.yml up -d --build --wait
CAPTURE_FIXTURES=1 npx playwright test e2e/scripts/captureContractFixtures.spec.ts
docker compose -f docker-compose.test.yml down -v
```

Review the diff before committing — these files are meant to change deliberately, not silently.
