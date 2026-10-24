---
name: add-quality-gate
description: Use when adding a merge-blocking check, a required status context, a scheduled workflow, or any new CI gate.
license: MIT
---

# Add a quality gate

Outcome: the check blocks a merge through an existing required job, or it is
registered in all four places and applied live. Read `.github/AGENTS.md` and
`docs/development/repo-governance.md` first; the canonical gate registry is
`.github/release-gates.json`.

Open [gate-checklist](references/gate-checklist.md) for the four-step
registration and the break-it proof.

## Required per-PR contexts

These are the contexts the `main-protection` ruleset requires. The registry,
ruleset and docs must stay exactly equal; `governancecheck` fails otherwise.

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

## Steps

1. Prefer a step inside an existing required job: `Backend (Go)`,
   `Frontend (Vitest)`, or the unconditional `Docs & security-doc citations`.
   That is merge-blocking with no governance change.
2. Only create a new context when the check cannot live in those. Then register
   it in all four places: `.github/release-gates.json` (per-pr, mandatory, real
   `check_context`), `.github/rulesets/main-protection.json`, the live ruleset
   via `gh api`, and `docs/development/release-gates.md` +
   `docs/development/repo-governance.md`.
3. Mirror the check in `.githooks/pre-commit` for local feedback; the hook is
   never a substitute for CI.
4. If the gate is a scheduled workflow, register it in
   `nightly-failure-alert.yml`; `nightlyalertcheck` fails otherwise. A new
   per-PR mandatory workflow also needs `main-failure-alert.yml` registration.
5. Hand-verify: break the thing the gate guards, watch it fail, restore it. A
   gate that has never failed is not proven.

## DoD

```bash
cd backend && go run ./cmd/governancecheck
cd backend && go run ./cmd/releasegatecheck
cd backend && go run ./cmd/nightlyalertcheck
cd backend && go run ./cmd/mainalertcheck
```

Report any check you could not run; never imply it passed.

## Stop and ask

- The new gate duplicates an existing required job.
- The live ruleset would need a manual change you cannot apply/verify.
- A gate would be added to the release battery but not the per-PR ruleset.
- The break-it proof cannot be reproduced.

Canonical detail: `.github/AGENTS.md`, `docs/development/repo-governance.md`,
`docs/development/release-gates.md`.
