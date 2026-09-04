# Performance budgets (PERF-04, issue #471)

[PERF-02](perf-benchmarks.md) (issue #469) and [PERF-03](data-movement-benchmarks.md)
(issue #470) produce measurements. This is what turns a measurement into a
**budget**: a line that, when a number crosses it, fails a build (or, for
wall-clock, asks a human to look).

The milestone's own criterion is deliberately hedged — "Performance regressions
can be detected in CI or release validation **where practical**." That hedge is
honoured here rather than engineered around: the deterministic signals are hard
per-PR gates, wall-clock is a trend for review, and the few absolute ceilings
run in release validation, not on every noisy PR runner.

## What a budget is

> A budget is *"this operation must not get materially slower than it is
> today."*

It is a **regression gate anchored to the measured baseline**, never a number
picked from aspiration — an aspirational gate is either always green or
permanently red. So:

| Kind | Metric | Where it lives | When it runs | On breach |
|---|---|---|---|---|
| **Deterministic** | query count (PERF-02), rows touched (PERF-03) | the committed `baseline.json` / `datamovement-baseline.json` themselves | **every PR** (smoke) + release validation (typical/large) | **hard fail** |
| **Absolute wall-clock** | median at the anchor profile | `testdata/budgets.json` → `max_wall_ms` | **release validation only** (gated at-scale tests) | **hard fail** |
| **Absolute memory** | peak heap at the anchor profile | `testdata/budgets.json` → `max_peak_heap_mib` | **release validation only** | **hard fail** |
| **Wall-clock trend** | median / duration vs the last committed run | `testdata/wallclock-trend.json` | release validation | **`::warning::` for human review — never fails** |

The deterministic budget *is* the baseline. `budgets.json` does not copy those
numbers (that would be a second source of truth that drifts); it records the
**decision** for every operation — a real ceiling or an explicit waiver — plus
the short list of absolute ceilings.

### Anchor profile

Absolute (wall-clock, memory) budgets are evaluated **only at the `typical`
profile** — the "intended MVP scale". `typical` at 900 contacts under `-race`
is too slow for every PR, so those checks ride the gated
`MYCORRHIZAL_LARGE_TESTS=1` at-scale job (`migration-tests.yml` large-dataset).
The deterministic budget is cheap and deterministic, so it runs at `smoke` on
every PR — that is the N+1 tripwire.

## The budgets

Hand-authored in [`backend/internal/perfbench/testdata/budgets.json`](../../backend/internal/perfbench/testdata/budgets.json)
— the one file in `internal/perfbench` that is **not** generated. Each entry
carries a `reason`. Ceilings are set an order of magnitude above the measured
baseline so only a real regression trips them; the generating-run medians they
were sized against are in [perf-benchmarks.md](perf-benchmarks.md) /
[data-movement-benchmarks.md](data-movement-benchmarks.md).

### Core operations (PERF-02)

| Operation | `max_wall_ms` | Note |
|---|--:|---|
| `cadence.list_overdue` | 200 | home-screen widget, one indexed query |
| `contact_create` | 500 | form submit |
| `contact_detail` | 300 | contact drawer — the classic N+1 target |
| `contact_detail.pathological` | 300 | same handler, awkward record |
| `contact_list.filtered_sorted` | 300 | main list + filter + sort |
| `contact_list.plain` | 300 | first screen after login |
| `contact_merge` | 1500 | explicit admin action, run once |
| `contact_update` | 500 | form save |
| `dashboard` | 300 | home screen, 8 fixed aggregates |
| `delete_cascade` | 1000 | explicit delete, 35-query manual cascade |
| `duplicates.find_pairs` | — *(waived)* | background / review surface, not interactive; already a super-linear finding |
| `fts.contact_list_search` | 300 | list search box |
| `fts.search_all` | 300 | global search |
| `graph.traverse_deep` | — *(waived)* | opt-in; density-scaled by the PERF-01 profiles by design |
| `graph.traverse_hub` | — *(waived)* | density-scaled like `graph.traverse_deep` |
| `graph.traverse_shallow` | 500 | 1-hop relationship view |
| `reachout.detect` | 300 | reach-out suggestions strip |

A waived (`max_wall_ms: 0`) operation still carries its deterministic
query-count budget and its wall-clock trend line — only the absolute ceiling
is dropped, with the reason recorded.

### Data-movement operations (PERF-03)

Every bulk operation has a peak-heap ceiling — memory is the axis that
OOM-kills a bulk job on a small self-hosted box.

| Operation | `max_peak_heap_mib` | Note |
|---|--:|---|
| `backup.vacuum_into` | 32 | runs inside SQLite; Go holds nothing |
| `delete_cascade.hub` | 32 | SQL `DELETE`s, rows not loaded |
| `duplicates.find_pairs` | 64 | recorded super-linear finding; `O(cluster²)` pair expansion |
| `export.bundle` | 128 | buffers the whole dump (linear, expected) |
| `export.jscontact` | 128 | as `export.bundle` |
| `export.vcard4` | 128 | as `export.bundle` |
| `fts.rebuild` | 32 | `INSERT … SELECT` inside SQLite |
| `import.vcf` | 96 | recorded super-linear finding; `O(n²)` preview pass |
| `restore.snapshot` | 32 | streams the snapshot back into place |

## On a breach

**Deterministic breach (query count / rows touched)** — a PR turned a
benchmarked operation's query shape worse. Either:

1. **It is a bug** (an accidental N+1, a query that stopped using its index) —
   fix it.
2. **It is deliberate** (a new preload that genuinely costs a query and pays
   for itself) — regenerate the baseline (`make gen-perf-baseline`) and commit
   the diff. The diff is the record; a reviewer sees the query count go up.

**Absolute wall-clock / memory breach** — surfaces only in release validation.
The person closing the milestone gate (or whoever the at-scale job pings)
looks. Same fork: fix the regression, or — if the new cost is justified —
raise the ceiling in `budgets.json` **and rewrite its `reason`** in the same,
reviewed commit.

> A budget that can be silently raised in the same PR that breaks it is
> decoration.

The guard against that is not mechanical subtlety, it is review: `budgets.json`
is hand-authored, every number is paired with a prose `reason`, and
`TestEmbeddedBudgetsValid` fails if a `reason` is empty or an entry is stale.
Raising a number without touching its reason is a visible, reviewable defect in
the diff.

**Wall-clock trend `::warning::`** — informational. An operation moved more
than 3× since the last committed `wallclock-trend.json` on the same
architecture. Decide whether it is real (a genuine regression → treat as a
breach) or noise (a slower runner, a fixture change → regenerate the trend
file). Never blocks a merge.

## Tests

| Test | Gate | Covers |
|---|---|---|
| `TestEmbeddedBudgetsValid`, `TestBudgetsCoverEveryOperation` | per PR (no DB) | every #469/#470 operation has a well-formed entry with a reason |
| `TestCoreOperationBenchmarks/WithinBudgets` | per PR (smoke) | deterministic core budget — **the hand-verify target** |
| `TestDataMovementBenchmarks/WithinBudgets` | per PR (smoke) | deterministic data-movement budget |
| `TestBenchmarksAtScale/WithinBudgetsAtScale` | `MYCORRHIZAL_LARGE_TESTS=1` | absolute wall-clock budgets at `typical` |
| `TestDataMovementAtScale/WithinBudgetsAtScale` | `MYCORRHIZAL_LARGE_TESTS=1` | absolute peak-heap ceilings at `typical` |
| `TestBenchmarksAtScale/WallClockTrend`, `TestDataMovementAtScale/WallClockTrend` | `MYCORRHIZAL_LARGE_TESTS=1` | wall-clock trend advisory (`::warning::`, never fails) |

### Hand-verify (per CLAUDE.md)

Add an N+1 to a benchmarked endpoint and confirm the query-count budget fails,
then restore:

```bash
cd backend
# e.g. add a stray `db.Model(&x).Association("Y").Find(&ys)` inside GetContactDetail
go test ./internal/perfbench/ -run '^TestCoreOperationBenchmarks$/WithinBudgets' -count=1 -v
# expect: FAIL — "contact_detail @ smoke: query_count N over budget 25"
git checkout -- controllers/   # restore
```

## Regenerating

```bash
cd backend && make gen-perf-baseline
```

regenerates `baseline.json`, `datamovement-baseline.json`, both markdown
reports, **and** `wallclock-trend.json`. `budgets.json` is never regenerated —
it is edited by hand, deliberately.

## Related

- [scale-profiles.md](scale-profiles.md) — the PERF-01 representative datasets
- [capacity-under-constraint.md](capacity-under-constraint.md) — CAP-01 (#498), behaviour when a resource is *exhausted*
- [coverage.md](coverage.md) — the sibling "a gate that must not become noise" design (#267)
- issue #447 REL-03 / #467 — release-validation gates
- issue #263 — the Android macrobenchmark trend signal (same "trend not gate" stance)
