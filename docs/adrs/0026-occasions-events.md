# ADR 0026: Occasions — event planning & RSVP tracking

- **Status:** accepted
- **Date:** 2026-09-24
- **Implements:** issue #1228 (the deferred "event planning & RSVP tracking" half of #387). ADR 0024 part 6
  deferred it out of the #1222–#1227 v1 cut; this is the design pass that issue asked for, and it is
  implementable now as the same vertical slice shape those tickets shipped in.
- **Depends on:** ADR 0024 (Occasions — the recurring-obligation layer this sits beside, not inside), ADR
  0001 (neutral hub-and-spoke contact model — `EntityID` is always `Contact.VCardUID`), ADR 0004 (soft vs
  hard delete — user-authored content soft-deletes, join rows hard-delete), ADR 0006 (the revision-token
  scope this deliberately stays outside of), ADR 0011 (scheduled-job catch-up — not used here; events are
  one-off, there is no recurring job).
- **Related:** issue #383 (relationship health score — no coupling, same stance ADR 0024 took), the
  CardDAV/CalDAV surface (`models/CalendarEventLink`, `backend/caldav/`) — see the overlap boundary below.

## Context

ADR 0024 scoped Occasions v1 to the *standing rule* layer: "this contact is on my holiday card list every
year." Its part 6 explicitly deferred the other half of the original #387 ask — "create an event, suggest
invitees from circles/relationship edges, track RSVPs" — on the issue's own "lightweight, this is not a
full calendar; defer to CalDAV/calendar where it overlaps" wording, and filed issue #1228 as a tracking
stub that needed its own design pass first.

This ADR is that pass. It settles the three questions #1228 left open, then scopes a v1 small enough to
land in one slice.

## Decision

### 1. A new entity, `OccasionEvent` — not a specialized `CalendarEventLink`

`CalendarEventLink` (`models/calendar_subscription.go`) is a **machine-owned import mapping**: it maps an
iCalendar object's `UID` to the `Activity` an inbound sync created, is keyed by
`(subscription_id, uid)`, and exists so a re-sync updates instead of duplicating. It hard-deletes, it has
no user-authored fields, and it is written by the CalDAV/ICS pipeline, never by a person. Bending it into
"the event I am hosting" would put user-authored content, an attendee list, and a soft-delete lifecycle
into a row whose entire reason to exist is the opposite of all three.

So an event is a **new first-class `OccasionEvent`**: UUID-string PK, `UserID`, `CreatedAt`/`UpdatedAt`/
`DeletedAt` (soft delete — user-authored content, ADR 0004), following the `OccasionObligation` template
exactly. It is one-off by design (a concrete `starts_at`, not a month/day recurrence) — ADR 0024 part 2's
"annual month/day only" ceiling continues to hold for obligations, and events are not obligations.

**No coupling to `OccasionObligation` and no coupling to `CalendarEventLink`.** `Kind: invite`
obligations remain recurring reminders ("time to plan the summer BBQ"), not events; an event is not
projected to CalDAV in v1, and an imported CalDAV event is not an `OccasionEvent`. If a later ticket wants
to push a locally-authored event out to a calendar, that is a new sync decision layered on top of this
one, not implied by it. This mirrors ADR 0024 part 6's "no RSVP/invitee-tracking entity is part of v1"
without contradicting it — that line was about the v1 cut, and v1's boundary is exactly what this ADR
moves.

### 2. RSVP is a manually-recorded status on an owned join row, not an invitation

The invitee is a `Contact` row the user already owns. It has no inbox, no external identity, and no way to
answer. Building actual invitation delivery would require a mail/SMS transport, an external-identity
model, a public response link, and a delivery-failure story — a different product ("a calendar plus a
mailer") from the personal relationship ledger this app is.

An **RSVP here is what the user records after they asked in whatever channel they actually use**: they
called, texted, or emailed, and they now know Carol is coming and Dave cannot. That makes the attendee a
**join row** (`OccasionEventAttendee`, keyed by `(event_id, entity_id)` with an `entity_id` that is a
`Contact.VCardUID`), carrying a status ∈ `pending|accepted|declined|maybe`. It is created the moment the
user adds an invitee (default `pending`) and updated by hand as answers come in. This is a planning
ledger, not a delivery system, and the UI copy says so ("RSVP — record what they told you", not "send
invite"). No email/SMS/push is sent by this feature; there is no `NotificationDelivery` row and no
outbound client, so it needs no entry in the integration classification matrix (INT-01) or its
failure-behavior suite.

### 3. Invitee suggestion: circles in v1, relationship edges deferred

**Suggest invitees from one or more `Circle`s**, because a circle is already "the group I'd invite" —
`CircleMember` is a flat membership list, and "everyone in my *Family* circle" is exactly the affordance
the issue names. The endpoint expands the named circles to their member `VCardUID`s, batch-loads the
contacts, de-duplicates across circles, and returns `{contact_id, contact_name, vcard_uid}` candidates
(newest data, not a stored suggestion). It is a read-only computation over data the user already has, so
it mints no row and is idempotent by construction.

**Relationship edges are deliberately not a suggestion source in v1.** An edge is a *directed role fact*
("A is B's parent"), not a grouping — "invite everything related to B" is not a well-defined set without a
second decision (which edge types? how many hops? do suggested edges count?), and the common case it
would serve ("invite the family") is already a circle. Leaving it out keeps the suggestion surface one
predictable query instead of a graph walk with a policy attached. If it is wanted later, it is a new
`source`-parameterized branch on the same endpoint, not a redesign.

### 4. Attendees are nested sub-resources; sensitivity is carried but there is no export

Per CLAUDE.md's backend convention ("join rows get real nested sub-resource endpoints, not a bulk-replace
field"), attendee lifecycle is:
`POST /occasion-events/:id/attendees`, `PUT /occasion-events/:id/attendees/:vcard_uid`,
`DELETE /occasion-events/:id/attendees/:vcard_uid`. The event's own `PUT` never carries an attendee list.
A duplicate add is a checked `409 ErrAlreadyExists`, mirroring `AddCircleMember`.

`OccasionEvent` carries the standard `normal|private|secret` `Sensitivity` field for consistency with every
other user-authored entity, but **nothing in v1 exports or syncs an event**, so the field is a stored
marker only — there is no card-list-style copy that leaves the instance for it to gate, and no CSV/vCard/
JSContact projection. (A card-list export is `Kind: card` obligations, ADR 0024 part 4, and stays exactly
that.) If an event ever gets an export, this is the field that gates it, not a new one.

### 5. Reuse the established delete, pagination, and validation machinery

- `OccasionEvent` soft-deletes; `OccasionEventAttendee` hard-deletes (join-shaped, `(event_id, entity_id)`
  natural key — ADR 0004). Deleting an event hard-deletes its attendees in the same transaction, the same
  way `DeleteOccasionObligation` hard-deletes its materialized reminder.
- Both tables join the manual cascade checklist: `occasion_events` is swept by `DeleteUser`;
  `occasion_event_attendees` is swept by **both** `deleteContactAssociations` (the contact being removed
  from every event it attends) and `DeleteUser`. The delete-cascade coverage test
  (`controllers/delete_cascade_coverage_test.go`) is the pin, so adding either table without a bucket fails
  the build — as does forgetting to seed it in the sweeps.
- List/change-feed shape mirrors `ListOccasionObligations` verbatim: cursor pagination, `?since=`
  soft-delete tombstones (T17), `?entity_id=` browse filter. An additional `?from=`/`?to=` window filters
  `starts_at` for the calendar-ish list view.
- `StartsAt`/`EndsAt` are RFC 3339 instants (ADR 0015 category 1 — an event starts at a moment in time, it
  is not a date-only occurrence). `EndsAt` is optional and, when present, must not precede `StartsAt`
  (controller cross-field check, the same pattern `validateOccasionAnchorPair` uses).
- No `Revision`/`ETag`: like `OccasionObligation`, this entity has no sync surface and no conditional-write
  requirement, so it stays outside ADR 0006/0008 and is conditional-write-exempt.

## v1 scope vs deferred

**v1** (this ADR's implementation slice):

1. `occasion_events` + `occasion_event_attendees` migrations, models, and DTOs.
2. `OccasionEvent` CRUD + attendee sub-resources (add / set-RSVP / remove) + the circle-based
   `invitee-suggestions` read endpoint (backend).
3. Delete-cascade wiring for both tables plus the coverage-test buckets.
4. OpenAPI paths/schemas and the route matrices (authorization / ownership / idempotency /
   conditional-write) — every new route needs its declared row.
5. Frontend: events list/section on the Occasions surface, create/edit dialog, per-event attendee
   management with an RSVP status control, and the "suggest from circles" picker.
6. Android parity, in the same PR (this project ships web and Android together; parity is not deferred
   behind a separate backlog ticket): an "Occasions" destination with the event list + create/edit/delete,
   a per-event attendee/RSVP screen, contact-search add, and the circle-based suggestions. The event list
   is mirrored into a `cached_occasion_events` Room table (Room version 19) following the same
   full-resync cache pattern as the cadence/timeline entities; attendees are online-only.

**Deferred** (explicitly not in this slice):

- Invitation **delivery** of any kind (email/SMS/push) and external RSVP collection — §2.
- Relationship-edge-based invitee suggestion — §3.
- Any projection to / import from CalDAV, and any `OccasionObligation` ↔ `OccasionEvent` link — §1.
- Recurring events (ADR 0024 part 2's annual-only ceiling stands).

## Consequences

- Two new tables (`occasion_events`, `occasion_event_attendees`) and one new read endpoint; everything
  else is endpoints over data that already exists.
- The feature sends nothing. A reviewer who sees an "RSVP" label should read `OccasionEventAttendee.Status`
  as a user-recorded fact, never as evidence of a delivered invitation.
- The event/obligation/imported-calendar boundaries are now written down: three surfaces that all sound
  like "calendar" and are deliberately not coupled.
- Android adds one mirror table (`cached_occasion_events`, Room version 19) and a new `:feature:occasions`
  module; both are cache/UI only, with the server as the source of truth.
