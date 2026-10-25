# Gate registration checklist

Synthesis only; `.github/AGENTS.md` and `docs/development/repo-governance.md`
are canonical. The gate registry is `.github/release-gates.json`.

## Required per-PR contexts

The `main-protection` ruleset requires exactly these contexts. Keep the
registry, ruleset and docs equal; `governancecheck` fails on drift.

- `Detect Changes`
- `Backend (Go)`
- `Frontend (Vitest)`
- `Frontend bundle-size budget`
- `Run E2E Tests`
- `Android (Gradle)`
- `Android E2E (emulator)`
- `Android scan (mobsfscan)`
- `Scan workflows (zizmor)`
- `CIS container hardening scan`
- `Docs & security-doc citations`
- `Go server binary is byte-reproducible`
- `codecov/patch/backend`
- `codecov/patch/frontend`
- `codecov/patch/android`

## The four-step registration (new context only)

1. `.github/release-gates.json`: add a `tier: per-pr`, `mandatory: true` gate
   with a real, stable `check_context`.
2. `.github/rulesets/main-protection.json`: add the same context to
   `required_status_checks`.
3. Apply the committed ruleset live:
   `gh api --method PUT /repos/DrewBrunning/mycorrhizal-crm/rulesets/<id> --input .github/rulesets/main-protection.json`
4. Update `docs/development/release-gates.md` and
   `docs/development/repo-governance.md` (the marked table).

`governancecheck` catches 1–2 and 4; `governance-drift.yml` catches the live
ruleset diverging weekly. Step 3 is the half no in-repo test can see.

## The three load-bearing jobs

- `Backend (Go)` — lint/analyzers plus `go test ./...` (every Go test).
- `Frontend (Vitest)` — `tsc`, type-aware lint, `knip`, coverage ratchet.
- `Docs & security-doc citations` — unconditional docs/citation/governance
  analyzers.

Prefer adding a step there. A new job/context is the exception.

## Scheduled and per-PR alert registration

- Scheduled workflow → add its name to
  `.github/workflows/nightly-failure-alert.yml`'s `workflow_run.workflows`;
  `nightlyalertcheck` enforces equality with the `schedule:` set.
- New mandatory per-PR workflow → add it to
  `.github/workflows/main-failure-alert.yml`; `mainalertcheck` enforces it.

## Break-it proof

1. Remove one required context from `.github/rulesets/main-protection.json`.
2. Run `cd backend && go run ./cmd/governancecheck`; it must fail naming the
   missing context.
3. Restore the file and rerun; it must pass.

For a scheduled workflow, remove its registration entry and watch
`nightlyalertcheck` fail, then restore.

## Common mistakes

- Adding a check step without a required context: it is green but not blocking.
- Adding the context to the ruleset but not the registry/docs.
- Applying the ruleset in the UI but not committing the JSON.
- Forgetting `main-failure-alert.yml` for a new per-PR workflow.

Canonical: `.github/AGENTS.md`, `docs/development/repo-governance.md`,
`docs/development/release-gates.md`.
