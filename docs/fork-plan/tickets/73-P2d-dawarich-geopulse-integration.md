# P2d — Dawarich / GeoPulse integration (location-history correlation)

| | |
|---|---|
| **Rating** | 1–2 |
| **Depends on** | [T14](32-T14-external-link-substrate.md) (done) |
| **Alpha** | after |
| **Source** | `92.7` P2 bucket; split into its own ticket, 2026-08-06 |

**Not implementation-ready — a feature idea, not a scoped plan.** One
`93-integration-spec-template.md` (`93.3`) instance on top of the T14 substrate, *if* pursued.
Pulled in only when a concrete need arises, not before.

## Why this exists (the idea, not a commitment)

Self-hosted location-history tools (Dawarich, GeoPulse) could in principle correlate "you were near
this contact's address" or "you were both at the same place" into life-event or activity
suggestions — but this is genuinely speculative: it's an L4 ("Intelligence" — cross-referencing
across systems) idea per `93.2`'s maturity model, not a simple L1 link, and L4+ output is bound by
the propose-then-approve rule (`93.2`'s binding rule for L4–L5) — nothing here would ever silently
create data; it would surface a suggestion a user confirms, the same pattern
[T1](09-T1-households.md)'s household inference already uses.

## What a real design pass would need to settle

- Which of Dawarich or GeoPulse (or both) is the actual target — they're different projects with
  different APIs; this ticket bundles them only because they solve the same category of idea, not
  because they're interchangeable.
- What "correlation" concretely produces — a suggested `Activity`? A suggested `LifeEvent`? Neither
  currently has a natural home for "location proximity" as a triggering signal.
- Whether this is worth building before there's a concrete user need for it — per `90` D1, this
  project is explicitly not AI/inference-first; this idea should stay exactly here, in Feature
  ideas, until a real need makes it worth a design pass.

## Done when

N/A — not scheduled, and not scoped enough to schedule. Re-evaluate if a concrete need arises.
