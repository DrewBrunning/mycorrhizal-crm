---
title: Implementation Workflow
parent: Development
nav_order: 10
---

# Implementation Workflow

This is the long form of the non-negotiable workflow in the root `AGENTS.md`. It exists so a
contributor or agent can answer "what does done look like here?" without asking, and so a reviewer
can point at a named step instead of re-litigating the process in a PR.

The ordering is the point: **tests come first, the implementation second, the self-review third,
and the pre-PR gate before the commit.** None of those steps is replaced by CI. CI catches
what you missed; it does not prove you wrote the right test.

## Why test-first

A test written after the implementation tends to assert what the code *does* rather than what the
behaviour *must be*. It also usually passes the first time it runs — which means it has never been
shown to fail, so the failure it claims to prevent has not been demonstrated. This repo has caught
real bugs (the `ETag`/`etag` schema drift, `RecordForContact` data loss, the `omitempty` response
crash) only because a test was made to fail before the fix. "Hand-verify your tests" is the same
rule at review time: break the production code, watch the new test fail, restore.

Test-first is not a demand for red-green on every line. It is a demand that **the failure is
observed**, not assumed, before you rely on the test.

## What "test correctness" means

Before you implement, review the tests you just wrote against these questions:

- **Does it fail for the intended reason?** Run it on the unfixed code and read the failure. A
  compile error, a nil pointer in the harness, or a timeout is not a red test for the behaviour.
- **Would it pass if the implementation were subtly wrong?** If a plausible wrong implementation —
  off-by-one, wrong branch, missing scoping, skipped filter — still passes, the assertion is too
  weak. Assert the behaviour and the boundary, not the implementation detail.
- **Is it at the lowest layer that can catch the failure class?** See
  [`testing.md`](testing.md). A unit test that cannot see persistence drift is the wrong layer for a
  schema bug; a `dbtest`-backed service test is the right one.
- **Is it deterministic?** No sleeps, no real wall-clock reads (read the injected clock), no
  network, no dependence on test execution order, no shared mutable fixture. Data goes through
  `dbtest.New(t)` for anything touching persistence.
- **Are the assertions specific?** "No error" is not an assertion for a handler that should return a
  particular body. Assert status, shape, and the negative case — and read raw JSON when the bug is
  an absent-vs-empty distinction.
- **Is the regression test a reproduction?** For a bug, the test must fail on the buggy code for the
  reported reason. If you cannot reproduce it, say so rather than writing a test that only pins the
  new code.

If a test is genuinely wrong, change it deliberately, re-verify that the corrected test fails
first, and explain why in the PR. Never weaken or delete a failing test to make an implementation
pass.

## Self-review checklist

Run this before you commit. It is the same list a reviewer will use.

### Correctness

- Compiles, lints, and the touched tests pass.
- Errors are checked — including every GORM `.Error` (`.Error` on `db.Save`/`db.Updates` is not a
  returned `error`, so errcheck cannot see it).
- Ownership scoping: every read/write is scoped by `user_id` (or `Contact.VCardUID`).
- Transactions and concurrency: the `_txlock=immediate` assumption holds; no read-then-write race
  was introduced; partial failure does not leave half-written state.
- Clock: no raw `time.Now`/`Since`/`Until` in controllers, services, or middleware — use the
  injected clock.
- Domain invariants: `RecordForContact` not `RecordFromContact`; `ApplyRecordToContact` before
  `Create`; soft-vs-hard delete follows the model; `suggested` edges are not projected; sensitivity
  is filtered in the query; the CSV/account-bundle/contact-map exceptions are preserved.
- Generated artifacts were regenerated, not hand-edited, and the diff is understood.

### Completeness

- Every acceptance criterion in the issue is met (or explicitly descoped with a reason).
- A contract change updates both clients (web and Android) and regenerates the fixtures/types.
- A migration has up/down, a backfill where required, and a data-preservation check.
- Operator-visible changes have the changelog/upgrade note (`docs/changelog-policy.md`).
- Security-sensitive changes update the ASVS/MASVS row, threat model, retention lifecycle, or PII
  inventory that they touch.
- Context files (`AGENTS.md` hierarchy) are updated when a cited command, path, or test moved.

### Coverage

- Changed lines are covered; edge and negative paths are tested, not just the happy path.
- A regression test fails before the fix and passes after.
- The test is at the lowest layer that catches the class.
- No weakened, skipped, or flaky test; no unexplained `# pragma: no cover`; the patch and ratchet
  targets still hold; the baseline is raised in the same PR when tests are added. See
  [`coverage.md`](coverage.md).

### Safety

- No secrets in code, logs, fixtures, or commit messages.
- Input is validated; SSRF/path/CSV/import parsing guards are intact.
- Sensitivity filtering still applies to every copy that leaves the instance.
- No `os/exec` on any server or request path (build/test tooling is the scoped exception in
  `docs/security/asvs-l2.md` P12); no mass assignment (explicit update allowlists only).

### Hygiene

- One concern per branch and PR; the diff contains only in-scope changes.
- No debug artifacts, commented-out code, or stray files.
- Commits are signed off (DCO), and the PR body explains *why* and lists the checks run and not run.
- No `--no-verify`; no force-push except the documented `--force-with-lease` DCO rebase.

## Pre-PR gate and flake stress

The pre-commit hook is a fast subset, not the full suite. Before opening a PR, run the pre-PR gate
for the areas you touched. Select those areas from `.github/filters.yaml` using the branch diff:

```bash
git diff --name-only origin/main...HEAD
```

**Backend**

```bash
cd backend
go test ./...
go test ./... -coverpkg=./... -covermode=atomic -p 4 -coverprofile=coverage-cross.out
go run ./cmd/coverageratchet
```

When tests were added, regenerate and commit the baseline diff in the same PR:

```bash
cd backend && make gen-coverage-baseline
```

**Frontend**

```bash
cd frontend
npx tsc --noEmit && npx vitest run --coverage && yarn coverage:ratchet
```

When tests were added, update the baseline in the same PR:

```bash
cd frontend && yarn coverage:ratchet:update
```

Run the relevant Playwright specs for any e2e-affecting change.

**Android**

```bash
cd android
./gradlew detekt
./gradlew --warning-mode=fail jacocoTestReportAggregated
cd ../backend && go run ./cmd/androidcoverageratchet
```

Run the relevant unit tests alongside `detekt`. When tests were added, update the baseline in the
same PR:

```bash
cd backend && go run ./cmd/androidcoverageratchet -update
```

**Docs:** the docs-citations checks already run in the pre-commit hook (`docs/AGENTS.md`).

### Flake stress (new/changed tests only)

Stress only the tests you added or changed, not the whole suite:

- **Go:** `go test -race -count=10 -shuffle=on ./pkg`, or `-run '^TestX$' -count=20` for one test.
- **Playwright:** `npx playwright test <spec> --repeat-each=5`.
- **Vitest:** run the new file in a loop (and/or `--sequence.shuffle`).
- **Android:** repeat the new instrumented class on the emulator.

Determinism rules: injected clock, no `time.Sleep`, `dbtest.New` for persistence, unique temp
directories, no shared globals, stable ordering, `t.Cleanup`, and no network. Do not mask a flake
with retries — PR/push retries once and records passed-on-retry, and the flake ledger opens a
`flaky-test` p1 after three events.

Codecov patch coverage is computed server-side; the local proxy is the area coverage run plus the
per-file ratchet. If failures persist, a future `cmd/patchcov` can intersect the profile with
`git diff -U0` and warn on changed-line coverage below the area target (95/90/80), but it is not a
second gate today.

**No polling:** open the PR and stop. GitHub notifies on a check failure. If CI fails, start a new
session with the failing job link and the exact local command that should have caught it. A future
`pr-failure-alert.yml` (modelled on `main-failure-alert.yml`, which currently only alerts on pushes
to `main`) can post that comment; do not have an agent watch `gh pr checks`.

If a gate is too slow or needs a tool you do not have, say exactly that in the PR — **report checks
you could not run; never imply they passed.**

## Policy

An "implement this issue" request authorizes a signed feature-branch commit and a pull request once
the gates pass. It does **not** authorize committing to `main` or merging; those still require
explicit approval. One branch per concern, one concern per PR, and the issue is closed when the
ticket lands.
