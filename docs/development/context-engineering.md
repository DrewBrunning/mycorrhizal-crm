---
title: Context Engineering
parent: Development
nav_order: 12
---

# Context Engineering

Agent context is code here: the `AGENTS.md` hierarchy, the skills tree, and the
`Suites`/fact registries are validated in CI and in the pre-commit hook so they
cannot silently drift from the repository. This page describes the layout, the
alias contracts, the skill contract, the registries and budgets, and the checks
that are deliberately manual.

## Layout

There is one `AGENTS.md` per area, and the root file is a router rather than a
copy of every rule:

| File | Covers |
|---|---|
| `AGENTS.md` | Root safety boundaries, workflow, commands, project map, trap index |
| `backend/AGENTS.md` | Go/Gin/GORM, backend traps 1–10, generated artifacts |
| `frontend/AGENTS.md` | React/TypeScript/MUI, frontend traps 1–8, web budgets |
| `android/AGENTS.md` | Kotlin/Compose modules, Gradle, detekt, E2E, macrobenchmark |
| `docs/AGENTS.md` | Docs-as-code gates, citations, ASVS/MASVS, retention/PII |
| `.github/AGENTS.md` | Gate registration, workflows, scheduled-job alerting |
| `backend/database/AGENTS.md` | Migrations, upgrade floor, schemas, backup/restore |

A platform file is the single home for its area's conventions; the root file
points at it. Put a rule in the file a contributor working in that area will
read, and do not duplicate it across files.

Recurring procedures live in `.agents/skills/<name>/SKILL.md`, with long-form
detail in that skill's `references/*.md`. `.claude/skills` is a symlink to
`../.agents/skills`, never a second copy. `.opencode/skills` is deliberately not
used. The Tier 1 tree and alias landed in #1957; both are required, and a
missing or wrong alias fails `contextcheck`.

## The alias contract

Every `AGENTS.md` has a colocated `CLAUDE.md` alias so Claude Code and other
tools that look for `CLAUDE.md` read the same text. The alias is one of:

- a symlink to the colocated `AGENTS.md` (the normal form), or
- a regular file containing exactly the one-line import `@AGENTS.md`.

An alias is never a second source of truth. Editing `CLAUDE.md` content — or
leaving anything other than the one-line import in it — fails the check. The
symlink form is preferred; the import exists for filesystems and checkouts that
do not preserve symlinks. The skills tree follows the same rule: `.claude/skills`
must resolve to `.agents/skills`, by symlink (or the documented materialized
fallback that `scripts/install-agent-links.sh` owns).

## Skill contract

Every `SKILL.md` is validated statically (issue #1956):

- Frontmatter `name` equals the directory name and matches
  `^[a-z0-9]+(-[a-z0-9]+)*$`; `description` is present and at most 1024
  characters. It is a trigger, not a summary.
- The body targets 100 lines and has a hard maximum of 200. A 101–200-line body
  passes; only exceeding 200 fails. The target is guidance: never cut a
  load-bearing step to hit it.
- Every `.md` file under the skill's `references/` is a declared reference. It
  must be named in its `SKILL.md` (the condition that should make an agent open
  it) and stays within the same 200-line hard maximum. A file on disk that is
  not linked is an orphan and fails; a link to a missing file fails through the
  shared resolver.
- Duplicate skill names across `.agents/skills` and a materialized
  `.claude/skills` fail.

Skills add procedure; they never override `AGENTS.md`. On conflict, `AGENTS.md`
wins.

## Registry and budgets

`backend/internal/agentcontext/registry.go` is the expected file set. It is a
plain Go table, like `integrations.Registry()`: adding an `AGENTS.md` or alias
means adding an entry, and removing one means removing the entry. The validator
fails in both directions — an unregistered `AGENTS.md` on disk and a registered
path with no file are both findings.

Each content file carries two budgets:

- **Target: 100 lines.** Guidance only. A file between 101 and 200 lines passes
  and emits no warning. The target exists so a file is not padded with filler;
  it is never a reason to cut load-bearing context.
- **Hard maximum: 200 lines.** Only exceeding this fails. The finding points at
  the first over-budget line.

Aliases are exempt from line budgets: they carry no content of their own. The
registry↔disk walk skips VCS, dependency, and build directories (`.git`,
`node_modules`, `build`, `dist`, `out`, `target`, `vendor`, `.gradle`) so a
dependency's own context file is never mistaken for ours.

## Suites registry

`internal/agentcontext.Suites()` is the single "what verifies this change"
registry shared by skills, CI and (in #1959) the eval harness. Each row carries:

- `Commands` — the exact DoD commands a skill may list.
- `CIJobs` — job ids in `.github/workflows/` that run the suite.
- `CICommand` — a literal token that must appear in at least one listed job.

The six Tier 1 suites landed in #1956 and the five Tier 2 suites in #1958. Every
command fence under a skill's `## DoD` heading must belong to a registered suite
(after normalizing `cd <dir> &&` prefixes and `&&` chains), and every suite's
jobs must exist and contain its `CICommand`. This check is static: it parses
YAML, it never runs a workflow or a test.

## Fact parity

Four stable, load-bearing values are read from their canonical source, never
from a copied literal (issue #1956):

| Fact | Canonical source | Context files |
|---|---|---|
| upgrade floor | `schemafixture.FloorTag` / `FloorVersion` | `backend/database/AGENTS.md`, database-migration skill |
| locale set | `frontend/src/i18n/locales/*.json` (not `json.d.ts`) | localization skill (and `frontend/AGENTS.md`) |
| e2e port | `frontend/e2e/global-setup.ts` / `playwright.config.ts` | `frontend/AGENTS.md` and the relevant skill |
| required gate jobs | `.github/release-gates.json` + `.github/rulesets/main-protection.json` | add-quality-gate skill |

Each `Fact` names its reader (`upgrade-floor`, `locale-set`, `e2e-port`,
`required-gate-jobs`), the regex that finds the value in the named files, and
the files themselves. A present file that disagrees with the canonical source
fails; an absent file (for example a skill a later stage has not created yet) is
skipped. Do not add churny counts to this table: every anchor is maintenance.

## contextcheck

`cmd/contextcheck` validates the `AGENTS.md` hierarchy, the skill contract and
references, the `Suites` registry, static path/command resolution, and fact
parity. It is read-only: it never executes a command parsed out of prose and
never rewrites a file.

```bash
cd backend && go run ./cmd/contextcheck
cd backend && go run ./cmd/contextcheck -list
cd backend && go run ./cmd/contextcheck -explain backend/controllers/contact_controller.go
```

It runs from the repository root or any subdirectory, resolving the root by
walking up to the directory containing both `.git` and the root `AGENTS.md`
(deliberately not `go.mod`, which lives in `backend/` and would stop the walk
early). Every finding is printed as `path:line: message`; any finding exits 1,
and a clean tree exits 0. `-list` prints the registry with each file's target
and hard maximum; `-explain` prints the `AGENTS.md` files that would load for a
path, root first, so override and discovery surprises are debuggable. `-root`
points the check at a fixture tree (used by the tests).

Static reference resolution covers relative links, `#anchor`s, repo paths,
`go run ./cmd/<tool>` targets, `make` targets, `yarn` scripts, the allowlisted
`npx tsc`/`npx vitest`/`npx playwright` binaries, and `file_test.go#TestFoo`
test references. Only fenced code, inline code and explicit link targets are
parsed; free prose is not. Tokens such as `user_id`, `suggested`,
`normal|private|secret` and `GET /api/v1/...` are never treated as paths.

The check runs in `.githooks/pre-commit` and in `unit-tests.yml`'s
`docs-citations` job, unconditionally, so a context-only change cannot merge
with a broken hierarchy. It does not run the dispatch-only agent eval harness
(#1959) and it does not execute any documented command; it proves the target
exists and the command is registered, not that the command passes.

## Trigger tests

The versioned prompt set is `testdata/agent-eval/triggers.yaml` (issue #1960).
It has 2–3 realistic prompts per skill and records the expected skill, the
neighbour that must not load, and the ambiguous pairs:

- `contact-data-fidelity` vs `relationship-semantics-change`;
- `api-contract-change` vs `release-readiness`;
- `security-sensitive-change` vs the platform files (for example
  `backend/AGENTS.md`).

This tests **selection**, not task completion. The prompts are deliberately
independent of `testdata/agent-eval/*/task.md`; do not copy an eval task prompt
into the trigger set.

Procedure (manual; no pass/fail automation yet):

1. Start a fresh agent session with only the repository context loaded (the
   `AGENTS.md` hierarchy and `.agents/skills/`). Do not paste the skill text or
   name the skill in the prompt.
2. Send each prompt verbatim, one at a time, and record which `SKILL.md` the
   agent opened (or that it answered from a platform file with no skill).
3. Compare with the `expect` field. For each `not` neighbour, record why it did
   not win; for each ambiguous pair, record both sides.
4. Put the result in the PR's Context-change checklist (or the milestone-gate
   issue). A mismatch means the skill description needs tightening, not that
   the prompt is wrong.

A completeness test (`TestTriggerPromptSetIsComplete` in
`backend/internal/agentcontext`) is deterministic: it proves every skill has
2–3 prompts, every named skill exists, and the ambiguous pairs are declared. It
does **not** prove selection; that is the manual run.

## Agent evaluation

`cmd/agenteval` runs one representative task per suite in a disposable worktree
and scores it against the same `Suites` commands a skill declares; it is never a
merge or release gate. Beyond command exit codes it records scope violations,
generated-artifact hand-edits, expected findings, and gate-integrity findings:
added test skips, removed or deleted tests, and renamed test files are hard
violations; lint/coverage suppressions and build constraints added to a test file
are warnings. A hard violation fails the run because a green suite earned by
weakening the gate is not evidence the task was completed; a warning needs a
human read and can be a legitimate choice for a specific task.

## Milestone-gate audit

At every milestone gate, do the context truth audit and record it in the gate
issue:

- Read every `AGENTS.md`, `SKILL.md`, and reference against the code since the
  last gate; update or delete what is stale.
- Run every `Suites` DoD command once from a clean checkout.
- Re-run the trigger prompts if any skill description changed.

The standing criterion is also in
`.github/ISSUE_TEMPLATE/milestone_gate.md`; this page is the long form.

## What stays manual

- **Whether the content is right.** The check proves a file exists, its alias
  resolves, and it fits the budget — not that it describes current behaviour.
- **Placement.** Whether a rule belongs at the root or in a platform file is a
  review decision.
- **The target budget.** Nothing enforces 100 lines; a reviewer should push back
  on padding without treating 101 lines as a defect.
- **Executable prose.** Commands are resolved statically (the path, make target
  or package script exists), never executed. A command that exists but no longer
  works is still a review problem.
- **Which suite verifies a change.** The registry proves the command and job are
  wired; choosing the right suite remains a review decision.
