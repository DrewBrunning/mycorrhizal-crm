# CI, workflows, and quality gates

Read the root `AGENTS.md` first. This file covers the merge-blocking surface and the workflows that
maintain it.

## Adding a quality gate

The merge-blocking surface is **exactly** the `required_status_checks` contexts of the live
`main-protection` ruleset, mirrored by `.github/rulesets/main-protection.json` and kept equal to the
per-PR `mandatory` gates in `.github/release-gates.json` by `cd backend && go run ./cmd/governancecheck`
(every PR, in the docs-citations job). A check that is not a required context does not block a merge,
however it is wired.

**Default: add the check as a step inside an existing required job.** That is merge-blocking with no
governance change. The three jobs that carry the load:

- **`Backend (Go)`** — `unit-tests.yml`'s lint/analyzers and `go test ./...` (so any Go test is
  enforced: schema parity, the `AutoMigrate` ratchet, `omitemptyguard`, generated-artifact drift).
- **`Frontend (Vitest)`** — `tsc --noEmit` (picks up `src/api/contractConformance.ts`), type-aware
  `yarn lint`, `yarn knip` (unused files/exports/dependencies), and the per-file coverage ratchet.
- **`Docs & security-doc citations`** — runs **unconditionally**, so a repo-wide analyzer or
  docs/citation gate goes here (`pragmacheck`, `nightlyalertcheck`, `codecovcheck`, `docscheck`,
  `typos`, …).

Only create a new job/context when the check genuinely cannot live in one of those. Then, in the
same PR: (1) add a `tier: "per-pr"`, `mandatory: true` entry with a real `check_context` to
`.github/release-gates.json`; (2) add that context to `.github/rulesets/main-protection.json`'s
`required_status_checks`; (3) apply the committed ruleset to the live repo with
`gh api --method PUT /repos/DrewBrunning/mycorrhizal-crm/rulesets/<id> --input .github/rulesets/main-protection.json`;
(4) update `docs/development/release-gates.md` and `docs/development/repo-governance.md`.
`governancecheck` fails if 1–2 or 4 disagree; the weekly `governance-drift.yml` fails if the
committed and live rulesets disagree, since step 3 is the half no in-repo test can see.

Mirror the check in `.githooks/pre-commit` for fast local feedback (never a substitute for CI), and
hand-verify it: break the thing it guards, watch the gate fail, restore. **Gates are not silently
weakened** — lowering a coverage baseline, adding a `# pragma: no cover`, allowlisting an analyzer
finding, or removing a required context needs a recorded reason. The generated baselines/contracts
and gate configs are routed to the maintainer by `.github/CODEOWNERS` (routing only).

## Local pre-commit checks

`.githooks/` are vendored hooks (no external pre-commit framework) that mirror CI's fast,
deterministic gates. Install per clone **and per worktree** as described in the root `AGENTS.md`
(`bash scripts/install-git-hooks.sh` also sets a worktree-local `core.hooksPath` override when one
shadows the shared setting, and repairs the `CLAUDE.md` aliases).

- **Always:** client-side `gitleaks` secret scan (fail-closed on a missing binary).
- **Always:** `typos` spelling check over the whole tree (issue #1910), pinned to the same version
  `unit-tests.yml` runs; fail-closed on a missing or mismatched binary. Excludes/allowlist:
  `_typos.toml`.
- **Staged shell scripts:** `shellcheck` (issue #1904) over the same discovery CI uses — every tracked
  `*.sh` plus extension-less files whose shebang is `sh`/`bash` (`.githooks/*`, `android/gradlew`) —
  fail-closed on a missing binary. The single implementation is
  `.github/scripts/shellcheck-scripts.sh` (`--staged` locally).
- **Staged `backend/`:** `gofmt -l`, `go build ./...`, `go vet ./...`, `golangci-lint` (pinned to the
  version `unit-tests.yml` uses), `gormerrcheck`, `rawtimecheck`, the contract-fixtures/DATA-01/
  INT-01/API-baseline/generated-TS drift tests, and the mutation-scope config drift test.
- **Staged Go or module files:** `go mod tidy -diff` for `go.mod` and `tools.mod`, and `go mod verify`
  on both graphs (issue #1905) via `.github/scripts/check-go-modules.sh`.
- **Staged `frontend/`:** `tsc --noEmit`, `biome ci`, `eslint` (`yarn lint`, type-aware;
  `no-floating-promises`/`no-misused-promises`/`await-thenable` are errors), and `knip` (issue
  #1907).
- **Staged `android/`:** no Gradle step (too slow); run `cd android && ./gradlew --warning-mode=fail
  detekt lintDebug :app:lintObtainiumDebug :app:lintFossDebug` locally. `detekt` includes the ktlint
  formatting gate (no baseline);
  `./gradlew detekt --auto-correct` fixes formatting. Lint warnings are errors; `android/lint.xml`
  lists the reasoned exceptions.
- **Always:** `citecheck`, `depexceptions`, `deprecations`, `docscheck`, `contextcheck`,
  `releasegatecheck`, `governancecheck`, `pragmacheck`, `nightlyalertcheck`.
- **`pre-merge-commit`:** reruns the pre-commit checks on a clean merge commit (git skips
  `pre-commit` there), catching merge-skew in generated artifacts.
- **`commit-msg`:** rejects a missing/mismatched DCO `Signed-off-by:` trailer — use `git commit -s`.
- **Reminder only:** staging a migration, `backend/config/config.go`, or `.env.example` prints a
  reminder to add an `## Upgrade notes` block (or `no-changelog:`) to the PR body.

Deliberately CI-only: `govulncheck`, benchmarks/fuzz targets, and any full suite (`go test ./...`,
`vitest run`, Playwright, `yarn build`).

## Scheduled workflows and alerting

The scheduled tier is the only zero-retry run (PR/push retries a flake once), so a red nightly *is*
the flake signal. `.github/workflows/nightly-failure-alert.yml` opens/comments a `nightly-failure`
issue per workflow on a failed `schedule` run and closes it on the next green one. Its
`workflow_run.workflows` list must equal the set of workflows with a `schedule:` trigger —
`go run ./cmd/nightlyalertcheck` (docs-citations job) fails otherwise, so adding a scheduled
workflow means registering it there.

## Workflow-specific notes

- **Mutation testing** is nightly (`workflow_dispatch` too) and threshold-gated, never per-PR or
  release: `stryker.yml` for frontend (`src/api`, `src/hooks/use*.ts`, `src/utils`) and
  `go-mutation.yml` for the safety-critical Go paths. Scope/thresholds live in
  `internal/mutationscope.Scopes` (single source for the workflow matrix and the generated configs).
  `go-mutation.yml` runs `go clean -testcache` immediately before every `gremlins unleash` — a test
  cache hit makes gremlins derive a near-zero per-mutant timeout and misreport everything TIMED OUT.
  Don't remove it.
- **`filters.yaml`** is the path-filter map; a docs-only PR maps `docs/**` to nothing, which is why
  the docs/governance checks run unconditionally.
- **`release.yml`** is the one-tag workflow: it registers the release + schema fixture, commits the
  dump to `main`, and tags; `docker-publish.yml` reacts to the tag. It also refuses to tag unless the
  ASVS verification report has a new §10 row since the previous tag (issue #608) — do the pass at
  milestone-gate close, not at dispatch.
- **Breaking-change and dependency policy** live in `docs/breaking-change-policy.md` and
  `docs/dependency-upgrade-policy.md`; neither is restated here.
