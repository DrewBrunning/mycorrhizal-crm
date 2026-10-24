---
name: api-contract-change
description: Use when changing an API request or response shape, validation, error code, pagination, route, or other /api/v1 semantics.
license: MIT
---

# API contract change

Outcome: the server, `backend/openapi.yaml`, the generated fixtures/types, the
baseline, and both clients agree. Read `docs/breaking-change-policy.md` and
`docs/adrs/0008-conditional-write-enforcement.md` before editing.

## Steps

1. Change the handler/model and the spec together. The spec example is the
   source for contract fixtures; a hand-edited response shape is a failure.
2. Open [route-change-checklist](references/route-change-checklist.md) and update
   every matrix the route belongs to: authorization, ownership, idempotency,
   conditional writes, pagination and error shape.
3. Regenerate the artifact chain in order: `gencontract`, then `gentsapi`, then
   `genapibaseline`. Commit the diffs; the baseline diff is the review.
4. Make the consumers compile: `npx tsc --noEmit` for web and the Android
   contract-fixture test for the shared JSON.
5. A removed or narrowed field is a breaking change under MAINT-02: add the
   deprecation window and migration path, not just the baseline diff.
6. Every new route or access path needs an ownership regression test scoped by
   `user_id` (or `Contact.VCardUID` for graph entities).

## DoD

```bash
cd backend && go run ./cmd/gencontract
cd backend && go run ./cmd/gentsapi
cd backend && go run ./cmd/genapibaseline
cd backend && go test ./internal/apibaseline/... ./internal/contractfixtures/... ./internal/tsapi/...
cd frontend && npx tsc --noEmit
cd android && ./gradlew :core:network:testDebugUnitTest --tests '*ContractFixtureTest*'
```

Report any check you could not run; never imply it passed.

## Stop and ask

- The change removes or narrows a field, status code or route without a
  deprecation window.
- The spec and the implementation disagree and you are tempted to hand-edit a
  generated fixture or baseline.
- A new route has no ownership regression test.
- A client would need a coordinated release before the server changes.

Canonical detail: `docs/breaking-change-policy.md`, `docs/deprecation-policy.md`,
`backend/AGENTS.md` (generated artifacts).
