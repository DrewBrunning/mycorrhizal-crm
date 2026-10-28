# Relationship start and end dates

Issue #2001. A `RelationshipEdge` carries an optional `start_date` and `end_date`, so "married
2010–2018" or "worked together until 2024" is a typed, validated, exportable fact rather than a
free-form `metadata` key.

## Shape

- Both are `contactmodel.PartialDate` (year, month, day — any component may be unknown; no time, no
  zone), stored as JSON text in `relationship_edges.start_date` / `end_date` (migration `000079`),
  the same convention as `life_events.date` / `end_date` ([ADR 0025](adrs/0025-temporal-periods.md),
  [ADR 0015](adrs/0015-temporal-semantics.md)).
- Both are nullable. No `end_date` means the relationship is open-ended.
- **Validation:** each date must carry at least one component, `month` 1–12, `day` 1–31, and the end
  must not precede the start, compared at the coarsest precision the two share. A pair that cannot be
  compared (one side has no year) is accepted. A violation is a `400` on `end_date`.
- `PUT /relationship-edges/:id` is full-replace: omitting the dates clears them (this is how a
  relationship is "reopened"). Clients that edit an edge must resend the dates.

## Backfill from `metadata.since` / `metadata.until`

Migration `000079` promotes `metadata.since` → `start_date` and `metadata.until` → `end_date` when
the value is exactly `YYYY`, `YYYY-MM` or `YYYY-MM-DD` with an in-range month/day. Anything else
(free text, an impossible date) stays in `metadata` and the typed column stays `NULL`; a promoted pair
whose end precedes its start keeps the start and drops only the promoted end. **`metadata` is never
modified**, so nothing is lost and the down migration only drops the two columns. The typed columns are
authoritative from then on; the legacy `metadata` keys are no longer read or written by the app.

## Behaviour: ended relationships

An end-dated edge **remains a confirmed fact**: it stays in the graph, the relationship list (with
"until YYYY[-MM[-DD]]"), traversal, the account bundle and the CSV backup. It does **not** count for
active reach-out: an edge whose `end_date` is on or before today (compared at the end date's own
precision — a year-only end of this year is still active until the year is over) is skipped when
computing a contact's closeness (`services.activeEdges`, used by the direct-relation and structural-hop
inputs). A former spouse or ex-colleague therefore no longer inherits that tier. A future end date is
still active. Nothing is deleted, so the history and a later reopen are free.

## Interaction with life events and deceased status

- The `ended_a_relationship` life event and the edge's `end_date` are **independent records**. Nothing
  mirrors one into the other (the same stance as deceased status, see below): the life event is the
  dated story ("we separated"), the edge date is the structured fact that drives reach-out. Recording
  one does not set the other; a user who wants both records both.
- Deceased status (`Card.Anniversaries[kind=death]`, issue #1193) is a property of a contact, not of an
  edge. A death does **not** end-date the edges that contact has; the relationship record is left as
  entered and deceased contacts are handled by their own reach-out rules. `end_date` is for a
  relationship that ended while both people are living (divorce, a job change).

## Projection and export

RFC 6350 `RELATED` has no date, and ADR 0002 forbids inventing a token, so the dates stay **internal**
and are not projected to vCard/JSContact (the confirmed edge itself still projects as before). They are
not silently dropped anywhere that is a backup: the account bundle carries `start_date` / `end_date` on
each relationship (full round-trip; re-import is idempotent through the `import_source_links` ledger),
and the flat CSV `RELATIONSHIPS` section has `Start Date` / `End Date` columns (`YYYY[-MM[-DD]]`).
The CSV export is not re-imported, so it has no import path to keep idempotent.
