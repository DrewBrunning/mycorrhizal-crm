---
title: Coverage Gate
parent: Development
nav_order: 5
---

# Coverage Gate

The diff-based coverage gate (issue #267) fails a PR that adds **new uncovered
lines** in changed files. It is a gate on *changes*, not an absolute coverage
floor — the project-wide number is deliberately not gated (an "at least N%"
threshold is the worst variant of a coverage gate and is rejected).

## How it works

Codecov computes **patch coverage**: the fraction of the PR's changed lines that
the coverage tooling records as covered. The gate is

```yaml
coverage:
  status:
    patch:
      default:
        target: 100%
        only_pulls: true
```

`target: 100%` means every changed executable line must be covered; any new
uncovered line turns the `codecov/patch` status red and fails the PR.
`only_pulls: true` scopes the status to pull requests — a commit pushed straight
to main has no PR diff to act on and is not gated.

Three coverage reports feed it, one per area, all uploaded with the flags in
`codecov.yml`:

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
  re-measured.

## Override path

Overrides are deliberate and rare — a line that is "fine untested" should have a
reason, not be the default. In order of preference:

1. **Write the test.** The gate is meant to be satisfiable by real tests.
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
   genuinely outside the coverage model.

There is no percentage-based leniency: the gate is all-or-nothing per line, so
"a couple of uncovered lines is fine" never becomes the implicit norm.

## Making the gate block merges

The status check is `codecov/patch`. For the red status to actually prevent a
merge it must be in the repository's **required checks** in GitHub branch
protection, alongside the other required checks. Configuring branch protection
is a GitHub-side setting, not something this repo's CI does.

## Reference

- Issue #267 — the gate; #251 — the coverage visibility it builds on; #294 —
  Codecov integration.
- [Codecov: patch status](https://docs.codecov.com/docs/commit-status#patch-status)
- [Codecov: "ensure all code is covered"](https://docs.codecov.com/docs/common-recipe-list#ensure-all-code-is-covered)
