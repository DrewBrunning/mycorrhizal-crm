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
- **Android** is loosest because JaCoCo here only instruments
  `testDebugUnitTest` (see `AndroidConfig.kt`'s `configureJacoco` doc
  comment) — code exercised only by the instrumented E2E suite (issue #238)
  is structurally invisible to it. The clean-cut cases — hand-written Hilt DI
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
- **PRs with no changed covered lines** — a docs-only or workflow-only change
  has nothing to gate.
- **PRs that touch an area whose tests are path-gated off** — the flag carries
  forward (`carryforward: true` in `codecov.yml`) and unaffected areas are not
  re-measured; that area's status is simply not re-evaluated (a mixed PR can
  still fail on the one area it actually regressed).

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
3. **File-level ignore.** Add the path to the `ignore:` list in `codecov.yml`
   with a justifying comment. Coarse — only for an entire file that is
   genuinely outside the coverage model. `android/build-logic/.../AndroidConfig.kt`'s
   `JACOCO_EXCLUDES` is the Android-specific version of this same idea, applied
   at the JaCoCo report level rather than in `codecov.yml` — used for whole
   *categories* of hand-written framework glue (Hilt DI modules,
   Activity/Application lifecycle callback bodies) that JaCoCo's
   `testDebugUnitTest`-only instrumentation structurally cannot see, alongside
   the codegen it already excluded.

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
