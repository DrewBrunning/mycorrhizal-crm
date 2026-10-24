# Working in this repo

Mycorrhizal CRM — a personal relationship OS: a Go/Gin + SQLite backend, a React 19 +
TypeScript/MUI frontend, CardDAV/CalDAV sync, and a neutral RFC 9553/9554/9555 contact model.

This root file is a router. Read the platform file for the area you touch before writing code;
each carries the conventions and recurring bug classes that have shipped broken here.

**Real production data exists.** Migrations must preserve it, and a soft-deleted row is a user's
undo button, not a test fixture. Breaking *data* needs a reason and a heads-up; breaking a request
shape is ordinary pre-1.0 practice. Upgrade floor and release mechanics: `backend/database/AGENTS.md`.

## Project map

| Where | What |
|---|---|
| **GitHub Issues** | The backlog — the live plan and source of truth for outstanding work. |
| [AGENTS.md](AGENTS.md) | This router; root safety, workflow, and trap index. |
| [backend/AGENTS.md](backend/AGENTS.md) | Go/Gin/GORM conventions, backend traps 1–10, generated artifacts. |
| [frontend/AGENTS.md](frontend/AGENTS.md) | React/TS/MUI conventions, frontend traps 1–8, web budgets. |
| [android/AGENTS.md](android/AGENTS.md) | Kotlin/Compose modules, Gradle, detekt, E2E, macrobenchmark. |
| [docs/AGENTS.md](docs/AGENTS.md) | Docs-as-code gates, citations, ASVS/MASVS, retention/PII. |
| [.github/AGENTS.md](.github/AGENTS.md) | Gate registration, workflows, scheduled-job alerting. |
| [backend/database/AGENTS.md](backend/database/AGENTS.md) | Migrations, upgrade floor, schemas, backup/restore. |
| `docs/adrs/` | Architecture decision records — the load-bearing design decisions. |
| `docs/specs/` | Curated RFC excerpts — the external ground truth. |
| `docs/golden-fixtures/` | Verbatim RFC example cards — the external test oracle. |

## Best examples to copy

| Concern | Canonical example |
|---|---|
| Backend controller/service | `backend/controllers/circle_controller.go`, `life_event_controller.go` |
| Frontend full slice | `frontend/src/api/relationshipEdges.ts` + `src/hooks/useRelationshipEdges.ts` + `RelationshipEdgeDialog/List` |
| Persistence test | any `dbtest.New(t)` consumer (real migrated schema) |
| Docs page | `docs/development/testing.md`; decisions in `docs/adrs/` |

Secrets: `.githooks/pre-commit` runs `gitleaks` with `.gitleaks.toml` and fails closed on a missing
binary; setup and the manual check are in `README-developer.md`. Never commit a secret.

## Commands

```bash
cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...
```

```bash
cd frontend && npx tsc --noEmit && npx vitest run
```

Run once per clone **and once per worktree**, before your first commit there (idempotent):

```bash
bash scripts/install-git-hooks.sh
```

Dev server: use `.claude/launch.json`'s `frontend-dev`, never a raw `npm start`. The backend needs
`JWT_SECRET_KEY`, `PROFILE_PHOTO_DIR`, `SQLITE_DB_PATH`, and `FRONTEND_URL` matching the frontend's
actual port or CORS fails. Service-worker testing needs `frontend-prod` — see `frontend/AGENTS.md`.

## Safety boundaries (non-negotiable)

- **Migrations preserve data.** A rename/drop/retype needs a backfill or a deliberate, recorded call.
- **Ownership scoping is not optional.** Every handler scopes by `user_id` (or `Contact.VCardUID`);
  there are zero IDOR holes today — keep it that way.
- **Sensitivity** (`normal|private|secret`) is filtered **in the query** for anything that leaves the
  instance (sync, shares, neutral-`Card` exports), re-includable only via `?include_sensitive=true`.
  The CSV export, account bundle, and contact map are deliberate full-fidelity exceptions — policy,
  not leaks. Details: `backend/AGENTS.md`.
- **Soft vs hard delete is a model property**, decided once per entity. Cascades are manual and
  registry-enforced — add a new entity there in the same change (`backend/AGENTS.md` traps 6–7).
- **Generated artifacts are regenerated, never hand-edited** — see `backend/AGENTS.md`.
- **Security:** no secrets, no `os/exec` in app code, explicit update allowlists (no mass assignment),
  parameterized SQL, and the SSRF/CSV guards stay.
- **Never commit to `main` or merge without explicit approval.** Every commit needs a DCO
  `Signed-off-by:` (`git commit -s`). Never `--no-verify` without the user's explicit go-ahead.
- **Report checks you could not run; never imply they passed.**
- **Context files track code:** moving a cited command/path/test means updating the context file in
  the same PR.
- **Test-first:** fail-first evidence, test-correctness review, TDD, self-review, local per-PR suite.

## Implementation workflow (non-negotiable)

1. Read the issue and the platform `AGENTS.md`; choose the lowest test layer (`docs/development/testing.md`).
2. Write failing tests first (a bug's regression test must reproduce it). Run them; save the output.
3. Review the tests before implementing: right layer, fails for the intended reason, behaviour not
   implementation, deterministic (injected clock, `dbtest`), specific assertions.
4. Implement in small TDD loops. Never weaken or delete a failing test to make it pass. If a test is
   genuinely wrong, change it deliberately, re-verify fail-first, and say why in the PR.
5. Self-review against `docs/development/implementation-workflow.md` (correctness, completeness,
   coverage, safety, hygiene). Break the production code and confirm the test fails.
6. Run the pre-PR gate locally for the areas touched (`.github/filters.yaml`):
   build/lint/tests, then the area coverage ratchet and a flake-stress pass over
   new/changed tests. The pre-commit hook is a subset, not the full suite.
   Raise the coverage baseline in the same PR when tests were added.
   Report checks you could not run; never imply they passed. Open the PR and stop.
7. Commit with `-s`, push the feature branch, open a PR with the evidence. Never commit to `main`,
   never merge without explicit approval, never `--no-verify`, no force-push except the documented
   `--force-with-lease` DCO rebase.

An "implement this issue" request authorizes a signed feature-branch commit and a PR once the gates
pass. It does **not** authorize committing to `main` or merging; those still need explicit approval.

## When working in X, read Y

| Area | Read |
|---|---|
| Backend controllers/services/models | `backend/AGENTS.md` |
| Frontend components/api/hooks | `frontend/AGENTS.md` |
| Android | `android/AGENTS.md` |
| Documentation, security docs | `docs/AGENTS.md` |
| CI, workflows, quality gates | `.github/AGENTS.md` |
| Migrations, schema, backups | `backend/database/AGENTS.md` |

**Trap index** (stable IDs; existing code comments cite these):
Backend [#1](backend/AGENTS.md#trap-1) [#2](backend/AGENTS.md#trap-2) [#3](backend/AGENTS.md#trap-3)
[#4](backend/AGENTS.md#trap-4) [#5](backend/AGENTS.md#trap-5) [#6](backend/AGENTS.md#trap-6)
[#7](backend/AGENTS.md#trap-7) [#8](backend/AGENTS.md#trap-8) [#9](backend/AGENTS.md#trap-9)
[#10](backend/AGENTS.md#trap-10).
Frontend [#1](frontend/AGENTS.md#trap-1) [#2](frontend/AGENTS.md#trap-2) [#3](frontend/AGENTS.md#trap-3)
[#4](frontend/AGENTS.md#trap-4) [#5](frontend/AGENTS.md#trap-5) [#6](frontend/AGENTS.md#trap-6)
[#7](frontend/AGENTS.md#trap-7) [#8](frontend/AGENTS.md#trap-8). Android shares the frontend IDs.

Generated artifacts: one table in `backend/AGENTS.md` — regenerate, never hand-edit.

## PR and commit

- One branch per concern (`feature/<thing>`); implement → verify → commit → push → PR.
- PR body says what changed and why, lists checks run and not run, and adds `## Upgrade notes`
  (or `no-changelog: <reason>`) when the change is operator-visible (`docs/changelog-policy.md`).
- Hand-verify tests: break the code, watch the new test fail, restore.
- Gates are not silently weakened — a baseline/allowlist/required-context change needs a recorded reason.
- Close the corresponding issue when a ticket lands.
