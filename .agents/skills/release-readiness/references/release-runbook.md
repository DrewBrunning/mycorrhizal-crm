# Release runbook

Ordered pre-dispatch runbook. The registry is `.github/release-gates.json`;
`docs/development/release-gates.md` is its human view. Do not add a second gate
list here.

## Before dispatch

1. Pick the release commit and confirm it is on `main` (or the RC branch) and
   that every per-PR required check is green on that commit.
2. Run `cd backend && go run ./cmd/releasegatecheck` — every gate workflow exists
   and the composer covers them.
3. Final only: add `{Tag: "<tag>", Version: <migration>}` to
   `backend/internal/schemafixture/releases.go` if the release workflow has not
   done it, run `go run ./cmd/genschema`, and review that only the new dump was
   created. Commit it with the registration.
4. Final only: add the ASVS/MASVS §10 verification row (or use the documented
   `ack_asvs_current` path), and re-check the threat-model/retention changes
   merged since the last gate close.
5. Attest or skip each applicable manual gate in `.github/manual-gates.json`.
   `manualgatecheck validate` checks the ledger shape; `status`/`check` answer
   whether the evidence is fresh for this commit.
6. Verify the PR/release body: `## Upgrade notes` or `no-changelog:`, plus
   `## Breaking changes` where MAINT-02 requires it.

## Dispatch

- Final release: `.github/workflows/release.yml` with the version, the manual
  gate attestations, and (when used) the ASVS acknowledgement.
- RC: the same workflow; it skips schema registration and the final-only ASVS
  row, and `promote-rc.yml` copies it later.
- The tag-triggered `docker-publish.yml` re-runs the release gate battery and
  publishes from the tested candidate digest. Never rebuild the all-in-one image
  outside that path.

## Common failures

| Symptom | Cause | Fix |
|---|---|---|
| `schema-fixture-gate` fails | dump not committed for the tag | register + `genschema` + commit on `main` |
| `genschema` rewrites an old dump | frozen chain changed | revert the old migration; add a new one |
| ASVS obligation fails | no §10 row since the last tag | do the verification pass or use the acknowledged-current path with a reason |
| manual gate fails | missing/stale attestation or unreported applicable gate | re-run `scripts/android-local-mode-device-test.sh --record` or record a reasoned skip |
| release-gate fails | a required per-PR check is red on the release commit | fix it and cut again; do not override silently |
| release notes missing notes | PR body lacked Upgrade notes | add them before merging future PRs; for this release, record the note in the dispatch release body |

## Evidence

The release body and `release-readiness.json` record the candidate digest,
manual-gate mode/reason/attester, ASVS acknowledgement, and gate outcomes. Keep
the generated schema dump and the verification-report row in the same release
commit.

Canonical: `docs/development/release-gates.md`,
`docs/security/release-verification.md`, `docs/changelog-policy.md`.
