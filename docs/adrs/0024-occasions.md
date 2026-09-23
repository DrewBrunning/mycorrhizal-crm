# ADR 0024: Occasions — recurring card/gift/invite obligations

- **Status:** accepted
- **Date:** 2026-09-23
- **Implements:** issue #387 ("needs a name and a design pass before any code"). This ADR is that design
  pass — it names the feature, decides the data model, and scopes v1 vs deferred so v1 can be filed as
  implementable tickets.
- **Depends on:** ADR 0001 (neutral hub-and-spoke contact model — `LifeEvent`/`Preference`/`Gift`'s shared
  UUID-PK template), ADR 0015 (temporal semantics — the annual month/day recurrence rule this reuses
  verbatim), ADR 0011 (scheduled-job catch-up — the reminder-generation job follows its pattern).
- **Feeds:** the v1 implementation issues filed alongside this ADR (see Consequences). Related but
  explicitly not coupled: #383 (relationship health score) — see "Prioritization hooks" below.

## Context

The data model already has most of the raw ingredients for "who needs a Christmas card / gift / invite
this month": `Contact.Birthday`/`Anniversary`, `LifeEvent` (with a `Remind` flag that already materializes
a yearly `Reminder` — `controllers/life_event_controller.go`'s `syncLifeEventReminder`), `Gift`
(idea/purchased/given/received tracking with a free-text `Occasion`), and `Preference` (category `gift`
for what to buy). Nothing ties these into a planning surface, and — separately — nothing captures the one
fact none of them can hold: a **standing, recurring obligation** ("this contact is on my holiday card
list every year", "get them a birthday gift, ordered by two weeks before"). That's a rule about the
user's own behavior, not a fact about the contact's life, so it doesn't belong in `LifeEvent`.

## Name

**Occasions**, per the issue's own recommendation — it slots next to the existing "reach-out" / "cadence"
vocabulary and reads naturally as "upcoming occasions" / "occasions list" in the UI.

## Decision

### 1. Data model: one new first-class entity, everything else reused

**`OccasionObligation`** is the only new table. It is a first-class, UUID-PK, soft-deleting entity
following the exact `LifeEvent`/`Gift`/`Preference` template (`ID`/`CreatedAt`/`UpdatedAt`/`DeletedAt`,
`UserID`, `EntityID` referencing `Contact.VCardUID`, `Revision`/`ETag` per ADR 0006). It is **not** a
derived view — recurrence rules alone can't express "I decided this person gets a holiday card" (that's a
user decision, not something inferable from existing data), which is why the issue's "first-class vs
derived" question resolves to first-class.

It does **not** duplicate `Gift`, `LifeEvent`, or `Preference` — it's the missing "standing rule" layer
that sits above them:

| Concept | Entity | What it answers |
|---|---|---|
| A recurring obligation exists | `OccasionObligation` (new) | "Does this contact get a holiday card / birthday gift / invite, every year?" |
| A specific instance of giving | `Gift` (existing) | "What did I actually give them, and when?" |
| A fact about the contact's life | `LifeEvent` (existing) | "When is their anniversary / graduation?" |
| What to buy | `Preference` category `gift` (existing) | "What do they like?" |

Fields:

- `EntityID` — the contact, same convention as every other subject-scoped entity.
- `Kind` — open classifier (unvalidated, same reasoning as `LifeEvent.Type`/`Preference.Category`):
  `card`, `gift`, `invite` are the three the issue names, but the column accepts any string so a future
  kind doesn't need a migration.
- `Label` — free text, e.g. "Christmas card", "Birthday gift", "Annual summer BBQ invite" — the
  human-readable name for *this* obligation, since a contact can have several of the same `Kind`.
- `AnchorMonth`/`AnchorDay` (`int`, nullable together) — the annual recurrence anchor, in the obligation's
  own right (see Rule 2 below for why this isn't a foreign key to `Contact.Birthday`).
- `LinkedLifeEventID` (nullable, soft reference, no FK — same convention as `Gift.LifeEventID`) —
  optional traceability to the `LifeEvent` this obligation is anchored to (e.g. the `anniversary` life
  event), purely informational.
- `LeadTimeDays` (`int`, default 0) — how many days before the anchor date this obligation should surface
  as due ("gift by 12/18" = a Dec 25 anchor with `LeadTimeDays: 7`).
- `Active` (`bool`, default true) — a paused/retired obligation without deleting its history (e.g. "we
  stopped exchanging holiday cards" without losing the record that they used to be on the list).
- `Sensitivity` — the standard `normal|private|secret` field (see "Sensitivity" below).
- `Notes` — free text, encrypted at rest per the `Preference.Notes`/`Gift.Notes` convention.

No unique constraint: like `Gift`/`Preference`, a contact can have any number of obligations, so there's
no natural key to protect and no reason to hard-delete (content the user authored → soft delete, per
CLAUDE.md backend trap 7).

### 2. Recurrence: annual month/day only, reusing ADR 0015 Rule 3 verbatim — no RRULE-lite

The issue asks "RRULE-lite vs a flat next-occurrence field". Neither: **`AnchorMonth`+`AnchorDay` is the
whole recurrence rule**, computed the same way `LifeEvent.Date`'s annual occurrence already is (ADR 0015
Rule 3 — "next occurrence is the first such date not before today; a month/day in the past wraps forward
a year"; Rule 6's 29-Feb-advances-to-1-March applies identically). No stored `next_due` column, matching
`CadencePolicy`'s "health is derived, never stored" precedent.

This is a deliberate ceiling, not an oversight: every occasion this feature scopes for v1 (birthdays,
anniversaries, recurring holidays) is annual. `FREQ=WEEKLY/MONTHLY`, "every N years" milestone occasions
(a 25th anniversary), and one-off (non-recurring) events are out of scope — seed data doesn't need them,
and a general RRULE engine is a lot of surface for a feature this ADR is trying to keep small. If a
genuine multi-year or non-annual use case shows up later, it's a new `RecurrenceUnit` field on this same
table, not a redesign.

An obligation with no `AnchorMonth`/`AnchorDay` (both null) is valid and simply never surfaces on the
upcoming-occasions widget — for a `Kind: invite` obligation whose date isn't fixed yet (e.g., "annual
summer BBQ, date TBD"), which is a real state, not an error.

### 3. Reminder integration: same materialize-a-yearly-`Reminder` pattern `LifeEvent.Remind` already uses

`OccasionObligation` does **not** get its own scheduler or notification path. It reuses exactly the
pattern `syncLifeEventReminder` established: a scheduled job (extending the existing birthday-check-style
job, not a new one) walks active obligations with a set anchor, and for each one materializes/refreshes a
single yearly `Reminder` row at `(anchor date − LeadTimeDays)`, linked back via a new nullable
`Reminder.OccasionObligationID` column (same shape as the existing `Reminder.LifeEventID`). Completing
that reminder is not "the obligation is satisfied forever" — it regenerates next year, same as a life-event
reminder does.

### 4. Sensitivity: the general query-filtered rule, explicitly **not** the CSV-full-fidelity exception

`OccasionObligation.Sensitivity` follows the general rule (CLAUDE.md, "Sensitivity" section): anything
above `normal` is excluded from any list/export that **leaves the instance** — filtered in the query, not
the caller, re-includable only via `?include_sensitive=true`. This explicitly governs the "Christmas
cards" address-list export the issue asks for: that export is a **new, purpose-built, sensitivity-filtered
CSV** (or reuse of the existing filtered vCard/address projection), **not** a route through
`GET /api/v1/export`, which is the deliberate full-fidelity backup exception (CLAUDE.md, `export_csv_full_fidelity_test.go`)
that exists precisely because it's the user's own full backup — a mailing-label export is a copy meant to
leave the instance (to a printer, a card-fulfillment service, etc.), which is exactly the case the
full-fidelity exception does not cover. Any implementation that wires the card list through the full CSV
export path is a sensitivity-filter bug, not a shortcut.

### 5. Prioritization hooks: no coupling now

The issue asks that Occasions "feed the relationship-health score rather than duplicating it" (#383).
#383 doesn't exist yet as shipped code, so there's nothing to feed today. This ADR takes no dependency on
it: `OccasionObligation` is scoped and implemented standalone, and a future #383 integration point
(e.g., an overdue holiday-card obligation nudging a health score down) is that ticket's job to add, not
this one's to pre-build.

### 6. Event planning / RSVP: explicitly deferred

The issue itself says "lightweight — this is not a full calendar; defer to CalDAV/calendar where it
overlaps." No RSVP/invitee-tracking entity is part of v1. `Kind: invite` obligations exist in v1 only as
a recurring reminder ("time to plan the summer BBQ"), not an event/attendee tracker.

## v1 scope vs deferred

**v1** (implementable now, filed as follow-up issues against this ADR):

1. `OccasionObligation` entity + migration + CRUD endpoints (backend).
2. Reminder-generation job wiring `OccasionObligation` into the existing yearly-reminder pattern
   (backend; depends on 1).
3. An "upcoming occasions" aggregate endpoint (next 30/90 days) composing `Contact.Birthday`/`Anniversary`,
   `LifeEvent` rows with `Remind: true`, and active `OccasionObligation` rows into one sorted list
   (backend; depends on 1).
4. A sensitivity-filtered "card list" address export for `Kind: card` obligations (backend; depends on 1).
5. A "gift shopping list" endpoint — active `Kind: gift` obligations due within a window, joined against
   `Gift` to show which already have a linked idea/purchase this cycle (backend; depends on 1).
6. Frontend: occasion registry management UI, the upcoming-occasions dashboard widget, and the card/gift
   list views (depends on 1–5, backend-first per this project's usual split).

**Deferred** (not part of v1, no ticket filed yet beyond a tracking stub):

- Event planning, invitee suggestion from circles/relationship edges, RSVP tracking.
- Non-annual recurrence (RRULE-lite, every-N-years milestones).
- Any coupling to the relationship-health score (#383) or reach-out cadence beyond what already exists.
- Android parity (per the established Android-parity backlog pattern, this follows once the web/backend
  surface has shipped and stabilized — not scoped as part of the v1 issues below).

## Consequences

- One new table (`occasion_obligations`) and one new nullable column (`Reminder.OccasionObligationID`);
  everything else in v1 is new endpoints/queries over existing data.
- The card-list export is a new sensitivity-filtered code path, explicitly not a branch of the
  full-fidelity `GET /api/v1/export` — implementers should treat that as a hard rule, not a style
  preference (see "Sensitivity" above).
- No RRULE engine, no per-user event/RSVP model — if those are ever wanted, they're new decisions on top
  of this one, not implied by it.
