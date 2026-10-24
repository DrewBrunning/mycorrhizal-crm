---
name: release-readiness
description: Use when preparing a release or release candidate, cutting a tag, or checking whether the release gates and evidence are ready.
license: MIT
---

# Release readiness

Outcome: every gate is green on the release commit, the schema fixture is
registered, the ASVS/MASVS obligation is discharged, and the PR/release carries
the operator notes. Read `docs/development/release-gates.md` and
`docs/changelog-policy.md` before dispatching.

## Steps

1. Open the [release runbook](references/release-runbook.md) and follow it in
   order; it links the canonical gate registry instead of copying it.
2. Confirm the commit, not just the branch: `release.yml` builds the release
   commit and `releasegatecheck` re-checks the composed gates.
3. Final releases register the schema fixture: add the tag/version to
   `backend/internal/schemafixture/releases.go`, run `go run ./cmd/genschema`,
   and commit the new `backend/database/testdata/schemas/<tag>.sql`. An RC ships
   the final's schema and is exempt until promotion.
4. Discharge the ASVS/MASVS §10 obligation: the verification report needs a row
   since the previous release tag, or a documented `ack_asvs_current` reason.
   Do this at milestone-gate close, not at dispatch.
5. Attest or explicitly skip every manual gate that applies
   (`.github/manual-gates.json`, `attest_manual_gates`). A skip needs a reason;
   it is not a pass.
6. Ensure the PR body has `## Upgrade notes` (or `no-changelog:`) and, when
   applicable, breaking-change/security sections.
7. Run the release gates from the runbook and record failures as failures.

## DoD

```bash
cd backend && go run ./cmd/releasegatecheck
cd backend && go run ./cmd/manualgatecheck validate -ledger ../.github/manual-gates.json -root ..
cd backend && go run ./cmd/asvsstale
cd backend && go run ./cmd/genschema
```

Report any check you could not run; never imply it passed. A release gate that
could not be run is not green.

## Stop and ask

- A required gate is red, flaky, or silently skipped on the release commit.
- The schema dump is missing, dirty, or not frozen (`genschema` rewrites only the
  expected new dump).
- The ASVS/MASVS report has no new §10 row and no acknowledged reason.
- A manual gate applies but has no fresh attestation and no recorded skip.
- The operator notes are missing a migration, config, or breaking change.

Canonical detail: `docs/development/release-gates.md`,
`docs/security/release-verification.md`, `docs/changelog-policy.md`.
