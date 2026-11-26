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

## Amendment: duplicate circle names are refused in v1.7.0, an accepted MAINT-02 break (issue #2441)

**Status:** accepted. The decision to reject duplicates was made on #2380 (2026-11-06) and
confirmed on 2026-11-10; the maintainer accepted it as the v1.7.0 break on 2026-11-25 (v1.7.0
gate, issue #2335). Implemented by issues #2362 and #2444.

`POST /circles` and `PUT /circles/{id}` now answer `409 Conflict` with
`reason_code: circle_already_exists` (and `details.name`) when another circle of the same
user already has the name, compared case-insensitively with surrounding whitespace ignored.
Before v1.7.0 any name was accepted, so this **narrows accepted input** on a covered
`/api/v1` surface, which the [breaking-change policy](../breaking-change-policy.md) classifies
as breaking. It ships in v1.7.0 instead of v2.0.0, and **without the MAINT-01 deprecation
window**. The policy has no exception mechanism; this amendment is the written record of the
explicit approval its process requires, not a deprecation row (nothing is being deprecated
and there is no replacement to migrate to).

- **Why:** circle names are meant to identify a circle; the maintainer chose to reject
  duplicates rather than tolerate them (#2380).
- **Data:** nothing is changed or merged, and no unique index or backfill was added, so
  circles that already share a name stay as they are and remain editable and deletable. A
  rename that keeps the circle's own name, or only changes its letter case, succeeds. Another
  user's circle with the same name never conflicts.
- **Not guarded:** the rule is an application check on those two routes only. Source,
  Meerkat and Monica import, the grouping import, account-bundle restore and CardDAV sync
  still create circles directly and can produce a duplicate name.
- **What API clients must do:** treat `409` with `circle_already_exists` as "choose another
  name", and look an existing circle up by name instead of creating it blindly (scripts that
  re-create circles on every run). Clients that never create or rename circles are unaffected.
  The web app shows the message under the name field; the Android error-reason mirror carries
  the same code.
- **Where it is recorded:** the 409 response in `backend/openapi.yaml`, the
  `circle_already_exists` reason in `backend/errors/reasons.go`, and the "No more duplicate
  circle names" entry and `> **Upgrade**` note in the v1.7.0 release notes.
