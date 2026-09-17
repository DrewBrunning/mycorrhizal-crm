---
name: Milestone / release gate
about: The exit gate for a milestone — its acceptance criteria as citable checkboxes
title: 'vX.Y.Z — Milestone gate (verify acceptance criteria)'
labels: ''
assignees: ''

---

<!--
The gate issue is the LAST thing closed in its milestone. It exists so the
milestone's acceptance criteria are verified deliberately rather than assumed
once the final feature ticket merges.

Two rules make it worth having, and both are easy to erode:

  1. A box is checked only with a citation — a test name, a CI run, a document,
     or a `file:line`. "We did that work" is not evidence that the criterion
     holds; it is a memory of having intended it to.
  2. A criterion that cannot be met is either descoped by editing the milestone
     description (with a note here saying why) or filed as a follow-up issue
     with an explicit disposition. Never left silently unchecked.

Replace the milestone-specific criteria below; keep the standing ones. Fill in
the issue-list stub with every issue number this milestone contains — that
list is both an acceptance criterion (they're all closed) and the reference
the rest of this template's citations draw from.
-->

## Deliverable

The exit gate for milestone `vX.Y.Z`. This issue is the **last thing closed** in the milestone: it holds that milestone's acceptance criteria as checkboxes so they are verified deliberately rather than assumed once the last feature ticket merges.

> One-sentence statement of what this milestone establishes, copied from the milestone description.

Check a box only with a citation — a test, a CI run, a document, or a `file:line`. "We did that work" is not evidence that the criterion holds.

## Acceptance criteria

- [ ] Every issue in this milestone is closed with a citation: #___, #___, #___. <!-- list every issue number; add one bullet per issue below with the specific claim it closes, or group related issues under one bullet -->
- [ ] (milestone-specific criterion)
- [ ] (milestone-specific criterion)

### Standing criteria

Carried by every gate. Do not delete them when adapting this template — if one
genuinely does not apply to a milestone, say so here rather than dropping it.

> **Security-doc citations are no longer a per-milestone checkbox.** As of issue #608 the
> `citecheck` gate is enforced in three places with no human in the loop: the per-PR
> `Security-doc citations` job, the `release_gate: true` poll on the release commit, and a
> direct `go run ./cmd/citecheck` hard step in the REL-06 release workflow (`release.yml`).
> The obligation has one home — the release process — so this gate does not restate it. See
> `docs/security/asvs-l2-verification-report.md` §8–§9.

- [ ] No new *class* of security-relevant surface went unrecorded — if this milestone added
      one (a new client, a new outbound integration, a new persistence target, a new
      authentication path), §9 of `docs/security/asvs-l2-verification-report.md` has a row
      for it naming either the mechanism that fails when the next instance is added, or the
      issue that will build one. If the milestone added no new class, say so and check the
      box. (Standing criterion, issue #378. This is deliberately a question for whoever
      closes the gate — they know what the milestone contained — rather than a scheduled
      audit, which fires on a calendar and lands on someone without that context.)

- [ ] **The ASVS/MASVS re-verification changelog row is added as part of closing *this*
      gate, not deferred to release-dispatch time** — a new dated row in
      `docs/security/asvs-l2-verification-report.md`'s §10, following the §8 procedure. This
      project cuts a release at every point version (`v0.8.4`, `v0.8.5`, …), not only at
      `X.0.0` boundaries, so this applies to **every** gate, not a special "shipping
      milestone" subset. `release.yml`'s re-verification gate (issue #608) checks against
      the *previous release tag* (`git describe --tags`), not the milestone board, so
      closing this gate does not by itself satisfy it — and finding that out during the
      actual release dispatch forces a scramble (an emergency doc PR, or the recorded
      `ack_asvs_current=<reason>` skip). Cover this milestone's issues plus anything else
      that has landed on `main` since the last §10 row; if more commits land between this
      gate's closure and the actual release dispatch, redo the diff-review for just those
      before dispatching. (Standing criterion, issue #378. Added after closing the `v0.8.4`
      gate — #985 — skipped this and the next release dispatch failed on it; fixed
      retroactively by pass 1.26 / PR #1110, documented in CLAUDE.md by PR #1111.)

## Verify

Every box above is checked and carries a citation. Any criterion that cannot be met is either descoped by editing the milestone description (with a note here saying why) or filed as a follow-up issue with an explicit disposition — never left silently unchecked.

## Notes

Milestone `vX.Y.Z`. <!-- state its position in the current hardening sequence and link companion gates, e.g. "Nth in the 0.8.x review-hardening sequence. Companion gates: v0.8.0–v0.8.(N-1), v0.8.(N+1)–v0.8.8." --> Part of the `0.6.x` → `1.0.0` hardening program; see the program index issue.
