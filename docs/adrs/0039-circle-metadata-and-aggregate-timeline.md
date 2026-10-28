# ADR 0039: Circle description/colour and the aggregate circle timeline

- **Status:** accepted
- **Date:** 2026-10-27
- **Decides:** issue #1995

## Context

`Circle` was name-only. Users could not describe or colour a circle, nor see
what had happened with the people in it. `Note` and `Activity` link to contacts
only. Part 2 of the issue required choosing between an *aggregate* circle
timeline (merged from members, no schema change) and an *explicit* one
(notes/activities owned by or linked to a circle through join tables).

## Decision

### Metadata

`circles.description` (trimmed, at most 500 characters) and `circles.color`
(a palette token, `''` = none) are plain `NOT NULL DEFAULT ''` columns
(migration 000080). Existing circles read as undescribed and uncoloured; no
backfill is needed.

The colour is one of `models.CircleColors` (`red orange yellow green teal blue
purple pink grey`), never arbitrary CSS, so web and Android each map a token to
a theme-appropriate shade. The handler rejects any other value with a 400.

On `PUT`, the two fields are pointers: an absent key keeps the stored value so a
rename-only client cannot wipe them, and `""` clears. Both are internal-only:
circles have no standards projection, so they never reach the neutral `Card`,
CardDAV/CalDAV, the CSV export, or sync. The one place they are carried is the
account bundle, which is the owner's full-fidelity backup between their own
instances; dropping them there would be silent data loss on restore.

### Timeline: aggregate

A circle's timeline is the merged timeline of its live (non-archived,
non-deleted) members, served by `GET /circles/{id}/timeline`. It reuses
`services.ComposeTimeline` with a `CircleID` restriction, so filters, ordering,
cursors and the six event types behave exactly as on a contact timeline, and an
activity shared by several members appears once. The response adds
`member_count`.

Rejected: **explicit** circle-owned notes and activities. They would need new
join tables, a second delete-cascade registration, a new ownership surface, and
a decision about what happens to those rows when membership changes. The
aggregate view answers "what is happening with this group" without any of that.
It can be revisited if users need notes about a circle itself; that would be an
additive change.

"Add every contact in this circle to an activity" remains a picker convenience
and is not part of this decision.

## Consequences

- No new tables; one migration with a symmetric `.down.sql`.
- Ownership: the circle is resolved by `id AND user_id`, and the composer scopes
  membership rows by `user_id` as well.
- Android parity (edit UI and timeline screen) is not part of this change.
