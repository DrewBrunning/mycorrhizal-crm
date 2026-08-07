# P1b — Contact sharing: standing/live share + permission model (true cross-user sync)

| | |
|---|---|
| **Rating** | 1–2 |
| **Alpha** | after |
| **Source** | Tier 5, `92.7`, `90` D1; split out of the former `37-deferred.md` bundle, 2026-08-06 |

**XL. Depends on [P1](31-P1-contact-sharing.md)** (done — the one-time copy this extends).

**Not implementation-ready and not meant to be.** Needs its own design pass before it can be broken
into work packages. This records what it is and what a design pass would have to settle, so nobody
starts one by accident.

## What this is

Everything Tier 5's section describes beyond the one-time copy P1 already ships: persistence for a
*live* share that re-syncs, a shared-vs-private field model, a real permission model, and
**re-confirmation when a field is newly marked sensitive after the share was created**. This is the
closest existing formalization of "true synced contacts across users" — a standing, ongoing sync
relationship between two users' copies of a contact, rather than a point-in-time copy.

## A design pass must settle

- Does a standing share re-apply the field-selection default on every sync, or only at creation
  time? (Tier 5 flags this as an open question, not a decision.)
- What is the permission model — read-only, read-write, revocable, time-bounded?
- What happens to already-shared data when a share is revoked?
- How does the recipient's own editing interact with incoming updates? This is the same
  reconciliation problem as [T13](36-T13-two-way-calendar.md), with the same trap.

## Traps

**Do not start this as part of P1.** P1 is deliberately a one-time copy; conflating them is what
produced the original XL estimate.

### Post-alpha note
Real production data exists. Changes that modify schemas or data must be additive and
non-destructive. Migration files must be hand-written SQL up/down pairs. Test against
`database.InitDB`, not `AutoMigrate`.

## Done when

N/A — not scheduled. Pulled in only when a concrete need arises; the resulting work opens as a new
ticket from the design pass, not implemented from this file directly.
