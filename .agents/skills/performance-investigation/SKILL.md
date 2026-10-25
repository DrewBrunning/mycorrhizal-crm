---
name: performance-investigation
description: Use when profiling, optimizing, changing a query shape, or editing a performance budget or benchmark baseline.
license: MIT
---

# Performance investigation

Outcome: the change is measured before and after, the per-PR deterministic gate
passes, and a budget/baseline edit carries a written reason. Read
`docs/development/perf-budgets.md` and `docs/development/perf-benchmarks.md`
before touching `backend/internal/perfbench`.

## Steps

1. Reproduce the cost before optimizing. Run the owning benchmark or the
   per-PR query/row gate against the smoke profile. Do not optimize from a
   profile you cannot reproduce.
2. Confirm which class moved: query count, rows touched, memory-growth class, or
   wall-clock. Only the first three are deterministic per-PR gates; wall-clock
   is a trend and a release-validation ceiling.
3. Optimize the operation, then rerun the same gate. A change that improves one
   operation but regresses another is not ready.
4. Regenerate the baseline **only** when the operation's query/data-movement
   shape changed deliberately:
   `cd backend && make gen-perf-baseline`. Otherwise leave the baseline alone;
   a re-recorded noise baseline hides the regression it exists to catch.
5. If you edit `backend/internal/perfbench/testdata/budgets.json`, update the
   sibling `reason` in the same edit. A new number without a new reason is a
   review failure, and a changed budget is a governance decision.
6. For the gated cross-scale run, use
   `cd backend && MYCORRHIZAL_LARGE_TESTS=1 go test ./internal/perfbench/ -run '^TestBenchmarksAtScale$' -count=1`
   (large data, not the per-PR path).
7. If the change affects a documented budget, update `docs/development/perf-budgets.md`
   and any generated benchmark report in the same PR.

## DoD

```bash
cd backend && go test ./internal/perfbench/ -run '^TestCoreOperationBenchmarks$' -count=1
```

Report any check you could not run; never imply it passed. Wall-clock on a
shared runner is not evidence of a regression without the deterministic gate.

## Stop and ask

- The change needs a budget increase without a measured before/after.
- A baseline regeneration would change more than the operation under test.
- The optimization trades correctness or data retention for speed.
- The at-scale gate would be skipped for a query-shape change.

Canonical detail: `docs/development/perf-budgets.md`,
`docs/development/perf-benchmarks.md`, `docs/development/testing.md`
(Performance/load).
