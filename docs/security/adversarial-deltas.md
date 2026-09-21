---
title: Per-release adversarial deltas
nav_order: 9
---

# Per-release adversarial deltas

The [threat model](threat-model.md) is a living document: a design change that
adds a new trust boundary (a new integration, a new sync direction, a new
client) updates it in the same pull request. The milestone adversarial passes —
the #500 self-pass and the #502 independent review — are **one-time** events,
though, so a release between two milestones could add a security-relevant
surface (a client, an outbound integration, an authentication path, a
persistence target) with no required delta review at all. This page and the
gate over it (issue #953) close that gap.

## The obligation

A **final** release that the gate detects as touching a security-relevant
surface class records a **delta** here: one dated row answering "what new
surface did this release add, and what covers it?". A release that touched no
recognised class needs no row (a row saying `none` is still welcome, and is
what a surface-bearing release that turned out to add no new *class* should
say).

The row names the class and points at the mechanism that catches the next
instance of it (or the issue that will build that mechanism). This is the same
question [`asvs-l2-verification-report.md`](asvs-l2-verification-report.md) §9
asks at the milestone, asked at every release instead.

## How it is enforced

`.github/workflows/release.yml` runs `backend/cmd/adversarialdelta` as a
mandatory gate on a final release, beside the ASVS re-verification row check.
It feeds the command the paths changed since the previous release tag
(`git diff --name-only`), classifies the **surface classes** those paths touch,
and requires this page to hold a row for the release naming each class it
touched:

| Surface class | Paths that trigger it |
|---|---|
| `route` | anything under `backend/routes/` |
| `persistence` | anything under `backend/database/migrations/` |
| `outbound-client` | anything under `backend/integrations/`, or a `backend/services/*_client.go` file |
| `auth-path` | the session-minting / authentication-precondition files enumerated in `backend/internal/adversarialdelta` |

The classification is deliberately coarse and over-inclusive: a false positive
costs one reviewed row, while a false negative reproduces the gap this exists to
close. These classes are **not** the fine-grained surface inventory — each of
those already has its own mechanical owner (the authorization matrix, the
outbound-client classification, the cascade-coverage test, the session-minting
route gate; see §9 of the verification report). This gate is the per-release
backstop for a *new class* none of them anticipated, and a release that added
one should extend the table above in the same change.

Both this gate and the ASVS row check are skipped on an **RC cut**: an RC ships
the same tree as the final, and neither obligation is about the candidate. The
deferral target is promotion, so `.github/workflows/promote-rc.yml` runs both
gates against the RC commit, before it pushes the final tag — otherwise a
release cut as an RC and promoted (the normal path since `v0.9.0`) would never
have either enforced. The gate logic lives once, in
`.github/scripts/release-obligations.sh`, which both `release.yml`'s final path
and `promote-rc.yml` call (issue #1195); the invariant that both invoke it is
checked by `cmd/releasegatecheck`.

A release can be dispatched past the gate with a recorded acknowledgement
reason (the same escape shape as the ASVS row gate); the reason is recorded in
`release-metadata.json` for a direct final cut, or `promotion-metadata.json` for
a promoted RC, not swallowed.

## Ledger

Rows are newest-first. The `Release` cell is the release tag passed to the
gate. Every surface-class token the gate detected in the release's diff must
appear in the row's text, so this table is what the gate actually reads (and
edits to it are reviewable in the release PR).

The ledger begins with the release that introduced it (issue #953). Releases
before that are **not** reconstructed: the milestone adversarial passes (#500,
#502) already reviewed the surface that existed then, and backfilling "none"
rows nobody reviewed would be worse than an explicit empty start.

| Release | Date | Surface | Coverage |
|---|---|---|---|
| v1.0.0 | 2026-09-21 | route | The `v0.9.0 → v1.0.0` diff changed only test files under `backend/routes/` (the authorization, credential, ownership, conditional-write, idempotency and session matrices) — **no new route**. The `route` class is deliberately path-based and over-inclusive. Covered by `backend/routes/authorization_matrix_test.go` and the session-minting route gate; backfilled here because the gate was skipped for this RC-derived final (issue #1195). |

When the gate reports a touched surface class, add a row like:

```text
| vX.Y.Z | YYYY-MM-DD | route, auth-path | What the release added and the mechanism or issue that covers the next instance. |
```
