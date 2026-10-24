---
title: Context Engineering
parent: Development
nav_order: 12
---

# Context Engineering

Agent context is code here: the `AGENTS.md` hierarchy is validated in CI and in
the pre-commit hook so it cannot silently drift from the repository. This page
describes the layout, the alias contract, the registry and budgets, and the
checks that are deliberately manual.

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

## The alias contract

Every `AGENTS.md` has a colocated `CLAUDE.md` alias so Claude Code and other
tools that look for `CLAUDE.md` read the same text. The alias is one of:

- a symlink to the colocated `AGENTS.md` (the normal form), or
- a regular file containing exactly the one-line import `@AGENTS.md`.

An alias is never a second source of truth. Editing `CLAUDE.md` content — or
leaving anything other than the one-line import in it — fails the check. The
symlink form is preferred; the import exists for filesystems and checkouts that
do not preserve symlinks.

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

## contextcheck

`cmd/contextcheck` validates only the `AGENTS.md` files. It is read-only: it
never executes a command parsed out of prose and never rewrites a file.

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

The check runs in `.githooks/pre-commit` and in `unit-tests.yml`'s
`docs-citations` job, unconditionally, so a context-only change cannot merge
with a broken hierarchy. It does not validate skill frontmatter, reference
linkage, `Suites`, fact parity, command resolution, `.claude/skills`, or the
eval harness; those are later stages.

## What stays manual

- **Whether the content is right.** The check proves a file exists, its alias
  resolves, and it fits the budget — not that it describes current behaviour.
- **Placement.** Whether a rule belongs at the root or in a platform file is a
  review decision.
- **The target budget.** Nothing enforces 100 lines; a reviewer should push back
  on padding without treating 101 lines as a defect.
- **Executable prose.** Commands in context files are not executed or resolved
  by this stage, and neither are links outside `docs/` (the docs structural
  check covers `docs/` only). Verify a cited command/path by hand when you edit
  it.
