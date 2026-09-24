# ADR 0026: Data decay — periodic reminders to verify contact info is still current

- **Status:** accepted
- **Date:** 2026-09-24
- **Implements:** issue #352 ("Design first — decide the granularity (contact vs field), how decay is
  tracked, and how reminders surface (dashboard block on Android + web) — before implementing"). This ADR
  is that design pass.
- **Depends on:** `docs/adrs/0001-neutral-hub-and-spoke-contact-model.md` (subject-scoped entity
  convention), the T19 `CadencePolicy` precedent (`models/cadence_policy.go`, `services/cadence_service.go`)
  — the closest existing analog and the template this ADR reuses almost verbatim.
- **Feeds:** the v1 implementation landed alongside this ADR (backend + web + Android, one branch).

## Context

Contact data (address, phone, employer, relationship facts) goes stale over time, but nothing prompts the
owner to re-check it. This is distinct from reach-out cadence (#177 / `Activity.Qualifying()`), which is
about staying in touch with a person — data decay is about the freshness of the *stored facts themselves*,
independent of whether the relationship is active.

## Decision

### 1. Granularity: per-contact, not per-field

The issue's own phrasing floats per-field "last verified" markers as one option. This ADR decides
**per-contact** for v1: one policy per contact, not one per field. Per-field tracking would require a new
tracking row per editable attribute (name, every address, every phone, employer, …) across the nested
`Card`/`CRMEnvelope` structure, a field-by-field review UI, and a much larger surface on both clients —
disproportionate to what the issue actually asks for ("periodic, gentle reminders to confirm a contact's
information is still up to date"). This mirrors the project's established pattern of shipping a lean v1
and deferring finer scope later (ADR 0024 did the same for Occasions: annual-only recurrence, Android
deferred). Per-field granularity is explicitly deferred, not ruled out — see "Deferred" below.

### 2. Data model: `DataDecayPolicy`, an opt-in per-contact rule, mirroring `CadencePolicy`

**`DataDecayPolicy`** (`backend/models/data_decay_policy.go`) is a new UUID-PK, soft-deleting entity
following `CadencePolicy`'s exact template:

- `EntityID` — the subject contact (`Contact.VCardUID`), same convention as every subject-scoped entity.
- `IntervalDays` — how often to re-verify (1–3650 days, same bounds as `CadencePolicy.TargetIntervalDays`).
- `LastVerifiedAt` (`*time.Time`, nullable) — set by the "confirm still current" action; null means "never
  verified since creation."
- `Active` (`bool`) — pause without losing history, mirroring `OccasionObligation.Active`.

No policy row for a contact means decay tracking is off for that contact — **opt-in**, same as
`CadencePolicy`. This is deliberately safe for existing production data (CLAUDE.md's post-`v0.2.0-alpha`
rule): upgrading never silently starts nagging about every existing contact.

A partial unique index on `(user_id, entity_id) WHERE deleted_at IS NULL` (migration `000064`) enforces
one policy per contact, exactly like `idx_cadence_policies_user_entity`.

### 3. Health: derived, with one deliberate deviation from the "never stored" rule

Like `CadenceHealth`, `DataDecayHealth` (`next_due`, `overdue_by`) is **computed, never stored** —
`services.ComputeDataDecayHealth` derives it from `LastVerifiedAt` (or `CreatedAt`, when never verified) +
`IntervalDays`, against a `now` boundary. There is no `next_due` column.

The one deviation: **`LastVerifiedAt` itself is a stored column**, unlike cadence's last-qualifying-
interaction, which is derived live from the `Activity` timeline. There is no existing "I verified this
contact's info" event type in that timeline, and conflating a self-referential record-keeping check with
the interaction timeline (calls, visits, meals — real relationship-maintenance activity) would be a
category error, not a convenience. Storing the one timestamp directly is a smaller, more honest surface
than inventing a new `Activity.Type` for a non-interaction. This also makes `ComputeDataDecayHealth`
strictly *simpler* than `ComputeCadenceHealth`: no DB query at all, since the input policy row already
carries everything the derivation needs.

Because there's always a baseline (`CreatedAt` when `LastVerifiedAt` is nil), `DataDecayHealth` has no
"undefined" state the way `CadenceHealth` does for a contact with zero qualifying interactions — `next_due`
is always defined.

### 4. Surfacing: a dedicated derived block, not folded into generic Reminders

Two designs were considered:

- **Materialize into the generic `Reminder` system**, the way `OccasionObligation` does (a scheduled job
  creates/refreshes a `Reminder` row, surfaced for free in the existing "Upcoming Reminders" widget).
- **A dedicated derived block**, computed live like `CadencePolicy`'s overdue list, with its own dashboard
  widget and a "confirm still current" action on the contact page.

**Decision: the dedicated derived block.** Reasons:

- Data decay has no fixed calendar anchor (unlike a birthday or an `OccasionObligation`'s annual
  month/day) — it's interval-since-last-verified, exactly the shape `CadencePolicy` already solves. Cadence
  is the closer precedent, not Occasions.
- No scheduled job is needed: health is computed on read, the same way overdue cadences are.
- Keeping "needs a data check" visually and conceptually distinct from "you asked me to remind you about
  X" (generic reminders) makes the dashboard read clearly — mixing them would make a decay nudge look like
  a self-authored reminder, when it's really a system-computed staleness signal.

### 5. The "confirm still current" action and the review surface

`POST /data-decay-policies/{id}/verify` stamps `LastVerifiedAt = now` — the one write path a user actually
clicks, surfaced as a button on the contact-detail page's data-decay section (mirroring the `cadence`
section's shape) and, implicitly, by no longer appearing in the overdue dashboard block. This is the
"review surface" the issue asks for: not a field-by-field diff UI (that's the deferred per-field
granularity), but a lightweight per-contact checkpoint.

### 6. Contact merge: same conflict-aware repoint as `CadencePolicy`

`DataDecayPolicy` carries the identical one-per-contact partial-unique-index constraint `CadencePolicy`
does, so a contact merge where both the keeper and loser have a policy can't be unioned or blind-repointed
— it needs the same conflict-resolution treatment T107 built for `CadencePolicy`
(`services.ComputeDataDecayPolicyConflict` / `repointDataDecayPolicy`, surfaced through the existing
generic `ContactMergeResolution.Conflicts` UI via the fixed key `data_decay_policy`). `LastVerifiedAt` is
deliberately excluded from the conflict summary — it's a history fact, not a rule the user is choosing
between, so two otherwise-identical policies never spuriously conflict just because one was verified more
recently.

## v1 scope vs deferred

**v1** (this branch):

1. `DataDecayPolicy` entity + migration + CRUD + verify endpoints (backend).
2. Overdue-list derivation + dashboard composite wiring (backend).
3. Contact-merge conflict handling (backend).
4. Web: dashboard widget, contact-detail review section.
5. Android: dashboard section, contact-detail review action.
6. Comprehensive tests on all three (per this project's usual bar).

**Deferred** (not part of v1, no ticket filed yet beyond this note):

- Per-field granularity (decision 1 above) — a genuinely bigger feature, not a missing detail.
- Materializing decay reminders into the generic `Reminder`/notification-delivery system (e.g. a push
  notification when a policy goes overdue) — today it's dashboard/contact-page-visible only, the same way
  overdue cadences are.
- Coupling to the relationship-health score (#383), the same explicit non-coupling ADR 0024 took for
  Occasions.
- A full-fidelity CSV export section for `DataDecayPolicy` (the way `CADENCE_POLICIES` has one) — real
  user data that arguably belongs in the backup, but out of scope for the design pass this ticket asked
  for; flagged, not forgotten.

## Consequences

- One new table (`data_decay_policies`) and one new dashboard composite field
  (`data_decay_overdue`); everything else is new endpoints/queries over existing data, following
  `CadencePolicy`'s shape closely enough that most of it is a mechanical mirror.
- The dashboard's per-user query count grows by one (`ListOverdueDataDecayPolicies`) — reflected in the
  regenerated PERF-02 baseline.
- No RRULE engine, no field-level tracking, no push-notification path — if any of those are ever wanted,
  they're new decisions on top of this one, not implied by it.
