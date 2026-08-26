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

Replace the milestone-specific criteria below; keep the standing ones.
-->

## Deliverable

The exit gate for milestone `vX.Y.Z`. This issue is the **last thing closed** in the milestone: it holds that milestone's acceptance criteria as checkboxes so they are verified deliberately rather than assumed once the last feature ticket merges.

> One-sentence statement of what this milestone establishes, copied from the milestone description.

Check a box only with a citation — a test, a CI run, a document, or a `file:line`. "We did that work" is not evidence that the criterion holds.

## Acceptance criteria

- [ ] (milestone-specific criterion)
- [ ] (milestone-specific criterion)

### Standing criteria

Carried by every gate. Do not delete them when adapting this template — if one
genuinely does not apply to a milestone, say so here rather than dropping it.

- [ ] Security-doc citations still hold — the `Security-doc citations` job is green on the
      merge commit, and any line added to `docs/security/citation-drift.ignore` carries a
      justification. Cite the run. (Standing criterion, issue #378. The job is unfiltered by
      path because moving code, not editing the doc, is what orphans a citation.)

<!--
RELEASE gates only (0.8.0 / 0.9.0 / 1.0.0, and any future shipping milestone)
also carry the full re-verification criterion. A per-milestone gate does not —
re-running the whole ASVS pass every milestone is disproportionate, while
letting a released claim go unverified for a year is not. Uncomment for a gate
that actually ships something:

- [ ] The ASVS L2 / MASVS-L1 claim has been **re-verified against the shipped code**, not
      inherited from an earlier milestone — cite a dated row in
      `docs/security/asvs-l2-verification-report.md`'s changelog. §8 of that report is the
      procedure: steps 1–2 are automated, and the four manual audits each produced
      single-digit candidate lists on the first pass, so a re-pass is about an hour rather
      than a rebuild. (Standing criterion, issue #378.)
-->

## Verify

Every box above is checked and carries a citation. Any criterion that cannot be met is either descoped by editing the milestone description (with a note here saying why) or filed as a follow-up issue with an explicit disposition — never left silently unchecked.

## Notes

Milestone `vX.Y.Z`. Part of the `0.6.x` → `1.0.0` hardening program; see the program index issue. Companion gates: #500 (`0.8.0`), #503 (`0.9.0`), #525 (`1.0.0`).
