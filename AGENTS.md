# Working in this repo

Mycorrhizal CRM — a personal relationship OS: a Go/Gin + SQLite backend, a React 19 +
TypeScript/MUI frontend, CardDAV/CalDAV sync, and a neutral RFC 9553/9554/9555 contact model.

This root file is a router. Read the platform file for the area you touch before writing code;
each carries the conventions and recurring bug classes that have shipped broken here.

**Real production data exists.** Migrations must preserve it, and a soft-deleted row is a user's
undo button, not a test fixture. Breaking *data* needs a reason and a heads-up; breaking an API
request or response shape is governed by `docs/breaking-change-policy.md` (MAINT-02: deprecation
window, migration path, release note, explicit approval). Upgrade floor and release mechanics:
`backend/database/AGENTS.md`.

## Project map

| Where | What |
|---|---|
| **GitHub Issues** | The backlog — the live plan and source of truth for outstanding work. |
| [.agents/skills/](.agents/skills/) | Tier 1 procedure skills (migration, contact fidelity, relationships, API changes, security, releases); `.claude/skills` is an alias. |
| [docs/development/context-engineering.md](docs/development/context-engineering.md) | The AGENTS/skills layout, alias contracts, line budgets, Suites, and `contextcheck`. |
| [backend/AGENTS.md](backend/AGENTS.md) | Go/Gin/GORM conventions, backend traps 1–10, generated artifacts. |
| [frontend/AGENTS.md](frontend/AGENTS.md) | React/TS/MUI conventions, frontend traps 1–8, web budgets. |
| [android/AGENTS.md](android/AGENTS.md) | Kotlin/Compose modules, Gradle, detekt, E2E, macrobenchmark. |
| [docs/AGENTS.md](docs/AGENTS.md) | Docs-as-code gates, citations, ASVS/MASVS, retention/PII. |
| [.github/AGENTS.md](.github/AGENTS.md) | Gate registration, workflows, scheduled-job alerting. |
| [backend/database/AGENTS.md](backend/database/AGENTS.md) | Migrations, upgrade floor, schemas, backup/restore. |
| `docs/adrs/` | Architecture decision records — the load-bearing design decisions. |
| `docs/specs/` | Curated RFC excerpts — the external ground truth. |
| `docs/golden-fixtures/` | Verbatim RFC example cards — the external test oracle. |

When a task matches a recurring procedure, load the matching `SKILL.md` from
`.agents/skills/` before writing code; skills add procedure but never override
this hierarchy. On conflict, the `AGENTS.md` file wins.

Secrets: `.githooks/pre-commit` runs `gitleaks` with `.gitleaks.toml` and fails closed on a missing
binary; setup and the manual check are in `README-developer.md`.

## Commands

```bash
cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...
```

```bash
cd backend && go run ./cmd/contextcheck
```

```bash
cd frontend && npx tsc --noEmit && npx vitest run
```

Run once per clone **and once per worktree**, before your first commit there (idempotent; also
repairs the `CLAUDE.md` and `.claude/skills` symlink aliases):

```bash
bash scripts/install-git-hooks.sh
```

Dev server: use `.claude/launch.json`'s `frontend-dev`, never a raw `npm start`. The backend needs
`JWT_SECRET_KEY`, `PROFILE_PHOTO_DIR`, `SQLITE_DB_PATH`, and `FRONTEND_URL` matching the frontend's
actual port or CORS fails. Service-worker testing needs `frontend-prod` — see `frontend/AGENTS.md`.

## Safety boundaries (non-negotiable)

- **Migrations preserve data.** A rename/drop/retype needs a backfill or a deliberate, recorded call.
- **Ownership is enforced at the data-access boundary.** Every user-scoped handler must scope by
  `user_id` (or `Contact.VCardUID` for graph entities). Treat missing or inconsistent ownership
  checks as a security defect; every new route or access path needs regression coverage.
- **Sensitivity** (`normal|private|secret`) is filtered **in the query** for anything that leaves the
  instance (sync, shares, neutral-`Card` exports), re-includable only via `?include_sensitive=true`.
  The CSV export, account bundle, and contact map are deliberate full-fidelity exceptions — policy,
  not leaks. Details: `backend/AGENTS.md`.
- **Soft vs hard delete is a model property**, decided once per entity. Cascades are manual and
  registry-enforced — add a new entity there in the same change (`backend/AGENTS.md` traps 6–7).
- **Generated artifacts are regenerated, never hand-edited.** Each area's file names its artifacts
  and drift test; the main backend inventory is `backend/AGENTS.md`.
- **Security:** no secrets, no `os/exec` on any server or request path (build/test tooling is the
  scoped exception in `docs/security/asvs-l2.md` P12), templates from an embedded FS, explicit update
  allowlists (no mass assignment), parameterized SQL, CSV formula-injection neutralization, SSRF
  guards in the transport dialer, and the Go toolchain stays pinned.
- **Never commit to `main` or merge without explicit approval.** Every commit needs a DCO
  `Signed-off-by:` (`git commit -s`). Never `--no-verify` without the user's explicit go-ahead.
- **Report checks you could not run; never imply they passed.**
- **Context files track code:** moving a cited command/path/test means updating the context file in
  the same PR.

## Implementation workflow (non-negotiable)

1. Read the issue and the platform `AGENTS.md`; choose the lowest test layer (`docs/development/testing.md`).
2. If the change has testable behaviour, write failing tests first (a bug's regression test must
   reproduce it). Run them; save the output. Otherwise use the matching verification in "When
   test-first does not apply" below.
3. Review the tests before implementing: right layer, fails for the intended reason, behaviour not
   implementation, deterministic (injected clock, `dbtest`), specific assertions. A test that needs a
   CI-only prerequisite uses `internal/citest.SkipOrRequire`, never skip-as-pass.
4. Implement in small TDD loops. Never weaken or delete a failing test to make it pass. If a test is
   genuinely wrong, change it deliberately, re-verify fail-first, and say why in the PR.
5. Self-review against `docs/development/implementation-workflow.md` (correctness, completeness,
   coverage, safety, hygiene). For a testable change, break the production code and confirm the test
   fails.
6. Run the pre-PR gate locally for the areas touched (`.github/filters.yaml`):
   build/lint/tests, then the area coverage ratchet and a flake-stress pass over
   new/changed tests. The pre-commit hook is a subset, not the full suite.
   Raise the coverage baseline in the same PR when tests were added. Open the PR and stop.
7. Commit with `-s`, push the feature branch, open a PR with the evidence. No force-push except the
   documented `--force-with-lease` DCO rebase.

### When test-first does not apply

Test-first is mandatory for any functional or behavioural change, bug fix, new route, or new access
path. Three categories replace it with their own verification — not with no verification:

- **Documentation-only** (`docs/`, `*.md`, context files): no test is expected; run the unconditional
  gates in `docs/AGENTS.md` (`docscheck`, `citecheck`, `depexceptions`, `deprecations`,
  `releasegatecheck`, `governancecheck`, `pragmacheck`, `nightlyalertcheck`).
- **Generated artifacts:** regenerate with the owning area's command, commit the diff, and run the
  named drift test. Never hand-edit.
- **Purely mechanical changes** (renames, moves, formatting, or tooling/dependency updates with no
  behaviour change): run build/vet/lint and the relevant existing tests. Add a characterization test
  if the change could alter behaviour.

Any change that can be tested uses the test-first process.

An "implement this issue" request authorizes a signed feature-branch commit and a PR once the gates
pass. It does **not** authorize committing to `main` or merging; those still need explicit approval.

## Trap index

**Stable IDs; existing code comments cite these** (`CLAUDE.md` aliases point at the colocated
`AGENTS.md`):
Backend [#1](backend/AGENTS.md#trap-1) [#2](backend/AGENTS.md#trap-2) [#3](backend/AGENTS.md#trap-3)
[#4](backend/AGENTS.md#trap-4) [#5](backend/AGENTS.md#trap-5) [#6](backend/AGENTS.md#trap-6)
[#7](backend/AGENTS.md#trap-7) [#8](backend/AGENTS.md#trap-8) [#9](backend/AGENTS.md#trap-9)
[#10](backend/AGENTS.md#trap-10).
Frontend [#1](frontend/AGENTS.md#trap-1) [#2](frontend/AGENTS.md#trap-2) [#3](frontend/AGENTS.md#trap-3)
[#4](frontend/AGENTS.md#trap-4) [#5](frontend/AGENTS.md#trap-5) [#6](frontend/AGENTS.md#trap-6)
[#7](frontend/AGENTS.md#trap-7) [#8](frontend/AGENTS.md#trap-8). Android shares the frontend IDs.

## PR and commit

- One branch per concern, using the matching prefix (`feature/`, `fix/`, `docs/`, `ci/`, `chore/`);
  implement → verify → commit → push → PR.
- PR body says what changed and why, lists checks run and not run, and adds `## Upgrade notes`
  (or `no-changelog: <reason>`) when the change is operator-visible (`docs/changelog-policy.md`).
- Close the corresponding issue when a ticket lands.
