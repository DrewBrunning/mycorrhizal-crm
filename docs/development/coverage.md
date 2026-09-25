---
title: Coverage Gate
parent: Development
nav_order: 5
---

# Coverage Gate

The diff-based coverage gate (issue #267) fails a PR whose changed lines fall
short of a per-area patch-coverage target. It is a gate on *changes*, not an
absolute coverage floor — the project-wide number is deliberately not gated
(an "at least N%" threshold on the whole codebase is the worst variant of a
coverage gate and is rejected).

## How it works

Codecov computes **patch coverage**: the fraction of the PR's changed lines that
the coverage tooling records as covered. The gate is one status **per area**,
each scoped to its own flag so a PR touching more than one area is judged
separately per area instead of on one blended number:

<!-- codecov-patch-status:begin -->

```yaml
coverage:
  status:
    patch:
      backend:
        target: 95%
        threshold: 5%
        flags: [backend]
        only_pulls: true
      frontend:
        target: 90%
        threshold: 10%
        flags: [frontend]
        only_pulls: true
      android:
        target: 80%
        threshold: 15%
        flags: [android]
        only_pulls: true
```

<!-- codecov-patch-status:end -->

This block is not illustrative — it is asserted byte-for-byte (after YAML
parsing) against `coverage.status.patch` in the real `codecov.yml` by
`cmd/codecovcheck` (issue #979). Changing a target/threshold/flag/only_pulls
value here without changing `codecov.yml` to match, or vice versa, fails CI.

`target` is the patch-coverage floor; `threshold` is how far a PR may miss it
and still pass (e.g. backend's 95%/5% passes anything at or above 90%).
`only_pulls: true` scopes the statuses to pull requests — a commit pushed
straight to main has no PR diff to act on and is not gated. Each status shows
up as its own GitHub check: `codecov/patch/backend`,
`codecov/patch/frontend`, `codecov/patch/android`.

**Why three different numbers, not one.** They aren't fitted to whatever PRs
have historically passed — they come from reading what's actually testable in
each area (2026-09-04 review, prompted by legitimate PRs failing the old flat
`target: 100%, threshold: 0%`):

- **Backend** stays closest to 100% because almost everything here really is
  testable and tests demonstrably catch real regressions — `dbtest.New(t)`
  runs against the real migrated schema (see CLAUDE.md's "Backend traps" for
  bugs a real-DB test caught), and INT-02 built `faults.Hook` seams
  specifically so failure branches that used to be untestable now are. The 5%
  threshold absorbs ordinary noise, not a declared "some backend code is fine
  untested."
- **Frontend** is looser because a large share of files are presentational
  MUI/JSX (component files substantially outnumber hook/logic files in
  `frontend/src/`) where a test that only asserts `render()` doesn't throw
  adds coverage without catching anything, and the tests that *do* protect
  real behavior (hooks, dialog state machines) sometimes hit genuinely fiddly
  async/race branches (e.g. `useContacts.ts`'s stale-response guard) for a
  disproportionate cost per marginal branch.
- **Android** is loosest because JaCoCo here only instruments debug unit
  tests — the unflavored `testDebugUnitTest` for libraries and the coverage
  flavor `:app:testObtainiumDebugUnitTest` for the app (see `AndroidConfig.kt`'s
  `configureJacoco` doc comment) — code exercised only by the instrumented E2E
  suite (issue #238) is structurally invisible to it. The clean-cut cases — hand-written Hilt DI
  wiring (`*Module` classes: one-line `@Provides`/`@Binds` delegations with
  no branch) and Activity/Application framework-lifecycle callback bodies
  (their real logic is already factored into separately-covered pure
  functions) — are excluded at the JaCoCo level in `JACOCO_EXCLUDES`, the
  same mechanism already used for Hilt/Room/Moshi-generated code, rather than
  papered over with a lower percentage. What's left and not cleanly
  excludable by file — Keystore-backed code (`RoomPassphraseStore`) and
  Firebase-SDK-availability gates — has too much real logic mixed in to
  blanket-exclude, so it's absorbed by a target still below backend/frontend
  instead of being pretended equally testable.

Three coverage reports feed the gate, one per area, all uploaded with the
flags in `codecov.yml`:

| Area | Report | Uploaded from |
|---|---|---|
| Backend (Go) | merged `coverage.out` (coverprofile, atomic mode, all three test legs) | `unit-tests.yml` → `backend` job |
| Frontend (vitest) | `coverage/lcov.info` (v8) | `unit-tests.yml` → `frontend` job |
| Android (JaCoCo) | `jacocoTestReportAggregated.xml` | `android-tests.yml` → `test` job |

## What counts as "uncovered"

A changed line is *uncovered* when the report for that area records it as
executed by no test:

- Go coverprofile: a statement block with count `0`.
- lcov: a `DA:<line>,<count>` record with count `0`.
- JaCoCo: a `<line nr=".." mi=".." ci=".."/>` with all instructions missed.

Non-executable lines (comments, blank lines, braces, declarations with no
statement coverage) are **not** recorded by the tooling and therefore never
count against the gate. A source file that never appears in its area's report
(an untested new Kotlin class, for example) counts its changed lines as
uncovered.

## When it does not fire

- **Project coverage** — the `codecov/project` status stays informational
  (always green) by design.
- **Pushes to `main`** — `only_pulls: true`.
- **PRs with no changed covered lines** — a test-only or workflow-only change
  still triggers a suite, so Codecov receives a report and posts all three
  statuses (with the unaffected areas carried forward); there is simply
  nothing to gate on that PR. A *coverage-measured* line is not required for
  the statuses to appear.
- **PRs that touch an area whose tests are path-gated off** — the flag carries
  forward (`carryforward: true` in `codecov.yml`) and unaffected areas are not
  re-measured; that area's status is simply not re-evaluated (a mixed PR can
  still fail on the one area it actually regressed).

### The no-upload case (issue #1188)

The one case the rules above do **not** cover is a PR that triggers *none* of
the coverage-uploading suites, so no report reaches Codecov at all: a
documentation-only change (`docs/**` maps to nothing in
`.github/filters.yaml`, so every `Detect Changes` output is false), an
infra-only one (the root `Dockerfile`, `docker/**`), or `codecov.yml` itself.
Codecov then creates no `codecov/patch/*` check run, and the three **required**
contexts sit at "Expected — waiting for status to be reported" indefinitely —
the PR can only merge through a bypass. (Verified on #1187, docs-only: zero
codecov check runs.) This is unrelated to whether the change has covered
lines: a *test-only* PR runs a suite, uploads, and gets all three statuses.

Two mechanisms close it, deliberately overlapping:

1. **`codecov.yml` sets `coverage.status.default_rules.flag_coverage_not_uploaded_behavior: pass`** —
   a status whose flag had no newly-uploaded coverage reports success instead
   of being withheld. This is the Codecov-side fix and covers every no-upload
   PR, including Dependabot's.
2. **The `codecov-patch-stub` job in `unit-tests.yml`** — when none of
   `backend`/`frontend`/`android`/`openapi`/`workflows` changed, it posts a
   success commit status for each area directly, the same "always run, always
   report" shape `reproducibility.yml` uses for the byte-reproducibility check
   (issues #264/#448). It is the deterministic backstop, and the only one that
   works for the maintainer's own PRs independent of Codecov behavior; it is
   skipped for fork/Dependabot PRs, whose `GITHUB_TOKEN` is read-only.

   `cmd/codecovcheck` (issue #1188) asserts the job's `PATCH_AREAS` equals
   `codecov.yml`'s `coverage.status.patch` keys, so adding a fourth area
   without arming the stub fails CI rather than stranding its context.

## Override path

Prefer the override path over leaning on an area's threshold buffer — the
buffer is for ordinary noise, not a substitute for marking a specific line as
deliberately untested. In order of preference:

1. **Write the test.** The gate is meant to be satisfiable by real tests, and
   most changed lines in every area are.
2. **Line-level ignore**, using the mechanism native to each area's coverage
   tooling (these exclude the line from the report the gate reads, so they work
   deterministically):
   - Go: `// # pragma: no cover` at the end of the line. The Go coverprofile
     has no native exclusion syntax, so this relies on Codecov honoring the
     `# pragma: no cover` marker in the source comment.
   - Frontend (vitest/v8): `/* v8 ignore next */` on the line above (v8's
     native coverage exclusion, honored by `@vitest/coverage-v8`).
   - Android (JaCoCo): annotate the declaration with `@Generated` (JaCoCo's
     standard exclusion).
   - Fallback for all three: Codecov's `# pragma: no cover` marker.
   Use these only for code that structurally cannot be hit (a defensive branch
   that only executes on a broken invariant), and keep the reason discoverable
   — the marker is invisible in the diff otherwise.

   `cmd/pragmacheck` makes "keep the reason discoverable" mechanical: it scans
   every non-test Go file under `backend/` for `# pragma: no cover` and every
   file under `frontend/src` for `/* v8 ignore ... */`, and fails if a marker
   has no reason — either non-trivial text after the marker on its own line,
   or on the line directly above it (the common pattern of stating the reason
   once on a guarding `if` and marking every line inside the block). It runs
   in `unit-tests.yml`'s `docs-citations` job and in `.githooks/pre-commit`,
   both unconditionally, for the same reason `citecheck`/`docscheck` are: a
   marker with no accompanying test change never trips a path filter.
3. **File-level ignore.** Add the path to the `ignore:` list in `codecov.yml`
   with a justifying comment. Coarse — only for an entire file that is
   genuinely outside the coverage model. `android/build-logic/.../AndroidConfig.kt`'s
   `JACOCO_EXCLUDES` is the Android-specific version of this same idea, applied
   at the JaCoCo report level rather than in `codecov.yml` — used for whole
   *categories* of hand-written framework glue (Hilt DI modules,
   Activity/Application lifecycle callback bodies) that JaCoCo's
   debug-unit-test-only instrumentation (libraries' `testDebugUnitTest` plus
   the app's coverage flavor `:app:testObtainiumDebugUnitTest`) structurally
   cannot see, alongside the codegen it already excluded.

   `cmd/codecovcheck` (issue #979) enforces the "with a justifying comment"
   part mechanically: every entry in `codecov.yml`'s `ignore:` list must have
   its own non-empty `#` comment — immediately above it, or trailing on the
   same line. It must be that entry's *own* comment, not one shared with a
   neighboring entry: a bare, uncommented `ignore:` entry appended right
   after an already-justified one — the shape a silent scope-narrowing edit
   would take — still fails CI.

## When measurement itself fails

The backend coverage number is a merge of seven per-leg profiles
(`unit-tests.yml`'s `backend-tests` matrix). A leg's dedicated no-rerun
coverage pass is `continue-on-error` — a crash or timeout there must not
fail the test gate — so its profile can go missing without the leg itself
going red. On a push to `main` (where `only_pulls: true` means nothing is
gated anyway) a missing leg is a `::warning::`: the Codecov upload is
skipped and `carryforward` holds the last known number. On a **pull
request**, the same gap is a hard failure of the `Backend (Go)` job instead
(issue #979) — silently leaving `codecov/patch/backend` to grade the PR
against stale carryforward data, with only a warning annotation as
evidence, is the exact failure mode that motivated this doc's own
`cmd/codecovcheck` gate above.

## Per-file no-regression ratchet

The patch-coverage gate above only judges a PR's *changed* lines. That leaves
two gaps it cannot close by design:

- A file that already sits at 0% (or 40%, or whatever) coverage stays there
  forever — no status ever re-measures a file a PR doesn't touch.
- A PR that deletes or guts a test for a file it doesn't otherwise edit trips
  nothing: the file's *source* lines are unchanged, so there's nothing for
  Codecov's patch diff to flag, even though its tests just got weaker.

Both are closed by a **ratchet**, not an absolute floor — deliberately the
same non-absolute philosophy as the patch gate and as
`frontend/bundle-budget.json` (issue #556): it fails on *regression* past a
tolerance, never on an existing low number by itself.

- **Backend**: `backend/cmd/coverageratchet` (logic in
  `backend/internal/coverageratchet`) reads the same merged
  `coverage.out` the `backend` job in `unit-tests.yml` already
  produces for Codecov, computes each file's statement-coverage percentage,
  and compares it against the committed
  `backend/internal/coverageratchet/testdata/baseline.json`. It honors the
  `// # pragma: no cover` marker (CLAUDE.md's Override path) at the
  coverprofile's own block granularity — a marked line anywhere inside a
  block excludes that whole block — so a deliberately-excluded line doesn't
  masquerade as a real drop.
- **Frontend**: `frontend/scripts/check-coverage-ratchet.mjs` reads
  `coverage/coverage-summary.json` (the `json-summary` reporter added to
  `vitest.config.ts` alongside the existing `text`/`html`/`lcov` reporters)
  and compares each file's line% and branch% against the committed
  `frontend/coverage-baseline.json`.

Both sides share the same rules:

- A file whose gated metric(s) drop by more than the baseline's tolerance
  fails. An improved file never fails, regardless of magnitude.
- A **new** file with no baseline entry is not gated here — that's
  `codecov/patch/*`'s job; gating it twice would just let the two disagree
  on some edge case.
- A **removed or renamed** file drops out silently (reported, not failed) —
  the baseline is simply stale for that entry until the next regeneration.

**Regenerating the baseline** (a deliberate, reviewed act — the diff *is* the
review, same convention as `bundle-budget.json`):

```bash
cd backend && make gen-coverage-baseline    # needs backend's coverage.out from a full-suite run
cd frontend && yarn coverage:ratchet:update # needs frontend/coverage/coverage-summary.json from `yarn test:coverage`
```

Both regenerate in place, keeping the existing `tolerancePercentPoints` /
`tolerancePct` unless you edit it by hand.

**Tolerance rationale.** The backend's seven coverage-producing legs include
property/generative tests (TEST-07, issue #435) whose iteration budget
(`RAPID_CHECKS`) is tiered by trigger — 200 on a PR, 1000 on a push, 8000 on
the nightly schedule — and whose generators use randomized inputs. A
run-to-run comparison of the `property` leg's coverprofile (two back-to-back
local runs at the PR-tier 200 iterations) found the *statement invocation
counts* varying by up to ~5% run to run (expected: different random inputs
exercise a block a different number of times), but **zero** blocks flipped
between covered and uncovered across the two runs — the set of `count == 0`
blocks was byte-identical. Coverage percentage is boolean per block
(covered/not), not proportional to invocation count, so this variance does
not by itself threaten the ratchet; the committed default (1.5 percentage
points, both sides) is a safety margin above that empirical zero, not a
number chosen to paper over observed flakiness. Because the tiered
iteration count still means a push/schedule run's coverage numbers are not
directly comparable to the PR-tier baseline, **the backend ratchet only runs
on `pull_request`** (`unit-tests.yml`'s "Per-file coverage ratchet
(pull_request only)" step) — matching the property-test depth the baseline
was generated at. The frontend ratchet has no such variance source (vitest
has no property/generative testing here) and runs on every trigger the
`frontend` job runs on.

If a real, intentional coverage change makes the ratchet fail (a test
legitimately removed because the code it tested was deleted, a refactor that
moves logic into a file whose baseline entry no longer applies), regenerate
the baseline and commit the diff — same override philosophy as the patch
gate: write the test first if you can, only lower the baseline when the drop
is deliberate.

## Making the gate block merges

The status checks are `codecov/patch/backend`, `codecov/patch/frontend`, and
`codecov/patch/android` (see "How it works" above — one per area, not a single
`codecov/patch`). For a red status to actually prevent a merge it must be in
the repository's **required checks** in GitHub branch protection, alongside
the other required checks — add each area's status separately, or only the
ones you want to block on. Configuring branch protection is a GitHub-side
setting, not something this repo's CI does.

## Reference

- Issue #267 — the gate; #251 — the coverage visibility it builds on; #294 —
  Codecov integration.
- [Codecov: patch status](https://docs.codecov.com/docs/commit-status#patch-status)
- [Codecov: "ensure all code is covered"](https://docs.codecov.com/docs/common-recipe-list#ensure-all-code-is-covered)
