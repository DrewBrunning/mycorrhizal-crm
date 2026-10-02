# ADR 0033: GeoPulse location-history correlation — on-demand lookup, no schema change to Activity

- **Status:** proposed
- **Date:** 2026-09-28
- **Implements:** issue #160 ("Dawarich / GeoPulse integration (location-history correlation)").
  #160's own "Done when" states its v1.4.0 deliverable as the design pass being resolved and written
  down as an ADR or design-pass comment — this ADR is that.
- **Depends on:** none.
- **Related:** `docs/int-01-integration-classification-matrix.md` (new `Registry()` entry this ADR
  requires), `docs/security/data-retention-lifecycle.md` (no new persistent copy — see Consequences).

## Context

#160's concrete trigger (2026-09-16): GeoPulse's timeline already correlates location history with
photos in its own UI — a natural prompt for logging a real in-person `Activity` in Mycorrhizal
("you were here on this date, here's photo evidence — who were you with?"). The user confirms the
contact(s); Mycorrhizal never infers a contact from location alone.

Two candidate tools existed (Dawarich, GeoPulse). The user confirmed GeoPulse
(`github.com/tess1o/geopulse`) is the one they actually run — this ADR is written against GeoPulse
only; Dawarich is not pursued.

Docs-only research could not confirm whether GeoPulse's timeline API returns photos in the same
response as location data. This ADR settles that by reading GeoPulse's actual source
(`StreamingTimelineResource.java`, `ImmichResource.java` — see below) rather than relying on
documentation, which turned out to describe a UI composition, not a single API response shape.

## Decision

### 1. Verified API contract

- **Timeline**: `GET /api/streaming-timeline?startTime=<ISO-8601>&endTime=<ISO-8601>` →
  `MovementTimelineDTO { stays: [TimelineStayLocationDTO { id, timestamp, locationName, city,
  country, latitude, longitude, stayDuration, ... }], trips: [...], dataGaps: [...] }`. Confirmed by
  reading `StreamingTimelineResource.java` end to end: it contains no reference to photos anywhere.
  No documented or observed range cap — treat it as unbounded and self-limit to a single day per
  request rather than relying on a server-side ceiling that doesn't exist.
- **Photos**: a real, separate, parameterized endpoint on GeoPulse's own side —
  `GET /api/users/{userId}/immich/photos/search?startDate&endDate&latitude&longitude&radiusMeters
  &city&country&limit` — GeoPulse proxies this to the Immich instance *it* has configured
  (`immich/rest/ImmichResource.java`). Mycorrhizal does not need its own Immich integration for this
  flow: call this endpoint per stay, using that stay's `latitude`/`longitude` and the day's date
  range, with the one GeoPulse API token.
- **Auth**: GeoPulse API tokens (issued in GeoPulse's own UI, Profile → Security), sent as
  `X-API-Key` or `Authorization: Bearer`.

### 2. Ingestion shape: on-demand, not a standing poll

Both GeoPulse and Dawarich are poll-only (no webhooks) and neither carries a stability guarantee —
Dawarich has a documented breaking-change precedent, and GeoPulse's docs state no API versioning
commitment. #160's own stated preference is "whichever needs the smaller footprint." On-demand wins:
a standing poll would mean scheduled-job catch-up semantics (CON-04) and INT-02 failure-behavior
coverage for a background process, for a feature that is inherently retrospective (the user is
reviewing *past* location history, not needing to be notified of *new* GPS points in real time).

Flow: the user opens a "Log activity from location history" entry point, picks a date. Mycorrhizal
makes one synchronous call to `/api/streaming-timeline` for that day, then — for each returned stay
— one synchronous call to the Immich photo-search endpoint using that stay's coordinates and the
day's date range. Results are returned as an **ephemeral, unpersisted** suggestion list (nothing is
written to the database at this point). The user picks a stay that corresponds to a real activity,
confirms which contact(s) they were with, and submits — which is simply a normal, already-reviewed
`POST /activities` call, pre-filled with:
- `Location`: from the stay's `locationName`/`city`
- `Date`: from the stay's `timestamp`
- `ExternalRef`: `geopulse:stay:<id>` — using `Activity.ExternalRef`
  (`backend/models/activity.go:52-57`), an existing opaque string field explicitly documented as for
  "not-yet-built or separately-owned subsystems." **No schema change to `Activity` is needed.**

### 3. No `Activity.Status`/suggested field

Deliberately not added. `RelationshipEdge.Status` (`confirmed`/`suggested`) is the established
propose-then-approve mechanism in this codebase, but adopting it here would mean a schema change to
`Activity` — a content-authored, soft-delete entity (CLAUDE.md backend trap #7 family) — for
something that only needs to be an ephemeral API response. Propose-then-approve is satisfied without
any schema change: nothing persists until the user's explicit confirmation calls the existing
`POST /activities` endpoint.

### 4. Configuration

A new per-user `GeoPulseConfig` model (`UserID`, `BaseURL`, `APIKeyEncrypted` via the existing
`backend/services/credential_crypto.go`) + controller + Settings page, mirroring `ImmichConfig`/
`PaperlessConfig` exactly. Unlike the map tile source (ADR 0031), this genuinely is a personal
external account — each user (or household member) may run/connect their own GeoPulse instance — so
the per-user pattern is the right one here, not an instance-level flag.

**Amendment, 2026-10-01 (maintainer decisions from the pre-implementation review of #160):**

- **GeoPulse `{userId}`.** If the API token alone does not let Mycorrhizal discover the GeoPulse user
  id (check for a self/profile endpoint at implementation time; none is confirmed), `GeoPulseConfig`
  gains a `GeoPulseUserID` field the user fills in on the Settings page. The config carries
  everything the client needs; nothing is guessed.
- **Duplicate prevention.** Confirming a suggestion does a lookup-before-create on
  `ExternalRef = geopulse:stay:<id>` scoped to `user_id`; if an Activity already carries it, the
  confirm returns the existing one rather than creating a second. No schema change to `Activity`.
- **Photos are display-only.** They appear in the ephemeral suggestion and are never written to the
  `Activity`.
- **Default `radiusMeters` = 200**, a named constant passed to the photo-search call (a stay is a
  clustered place; photos land within a building or yard of its centroid, GPS error is tens of
  metres, and past ~500 m neighbouring stays bleed in).

### 5. Integration classification

Needs a `docs/int-01-integration-classification-matrix.md` `Registry()` entry and `Dispositions()`
classification, plus an INT-02 failure-behavior test, like any outbound client
(`TestEveryOutboundClientIsClassified` requires this regardless of ingestion shape). This is **not**
the heavier standing-poll/scheduled-job class of concern the milestone gate's own standing criteria
warn about — no catch-up job, no background retry policy — because every call is synchronous and
user-initiated. State this precisely in implementation so the failure-behavior test stays scoped to
"a single request-scoped outbound call failed," not scheduled-job semantics.

## Consequences

- The feature only ever runs when the user explicitly asks for it, for a specific past date — no
  standing outbound traffic to GeoPulse, no new background job, no new retained copy of location
  data (the suggestion list is never persisted; only a confirmed `Activity` is, and that's the
  existing `Activity` retention story).
- If the user's GeoPulse instance is unreachable, the failure mode is "the lookup returns an error,"
  not "a scheduled job silently stops updating" — a materially simpler failure surface than a poll
  would have had.
- Dawarich is not integrated. If a future need arises, it would be a new design pass, not an
  extension of this one — the two tools' APIs are shaped differently enough (Dawarich's own
  `/api/v1/timeline` *does* return photos inline) that a shared abstraction isn't assumed.

## Alternatives considered

- **Target Dawarich instead** (or both). Dawarich's own API is arguably the technically cleaner
  single-call design (photos inline in one response). Rejected because the user confirmed GeoPulse is
  what they actually run — designing against a tool nobody here uses would produce an untested,
  unverifiable ticket.
- **Standing poll ingestion.** Rejected per Decision §2 above — bigger commitment (INT-01/INT-02
  scheduled-job class + CON-04 catch-up semantics) for a retrospective, not real-time, use case.
- **`Activity.Status` suggested/confirmed field**, mirroring `RelationshipEdge`. Rejected per
  Decision §3 — an ephemeral, unpersisted suggestion satisfies propose-then-approve without a schema
  change to a soft-deleted, content-authored entity.
- **Mycorrhizal's own Immich integration for photos**, instead of GeoPulse's photo-search endpoint.
  Rejected once the source-verified GeoPulse API showed it already exposes exactly the query shape
  needed (date range + lat/lng/radius) — reusing GeoPulse's own proxy is one fewer credential to
  manage and matches what a GeoPulse user has already configured there.

## Implementation

Filed as #160 (rewritten with this ADR's decisions), milestone v1.4.0 (#28). Backend + a web entry
point for v1; Android explicitly deferred (not required by gate #1118, unlike the Contact Map
ticket).
