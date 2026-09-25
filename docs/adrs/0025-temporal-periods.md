# ADR 0025: Temporal periods — start/end ranges for addresses and employment

- **Status:** accepted
- **Date:** 2026-09-23
- **Implements:** issue #354 (temporal information — capture durations for fields that change over time)
- **Depends on:** ADR 0015 (temporal semantics — partial dates and the no-zone rule), ADR 0001 (neutral
  hub-and-spoke model), ADR 0002 (correspondence table as the locked oracle), ADR 0004 (soft vs hard
  delete)
- **Related:** issue #515 (the `CRMEnvelope` as the home for user-authored content with no standards
  home), issue #861 (the CSV export is the deliberate full-fidelity exception), issue #442 (DATA-02 loss
  reports)

## Context

The model records *where* someone lives or works but not *for how long*. A contact can hold several
addresses and several employers, and each one is a fact whose truth is qualified by a period: "lived at X
from 2019 to 2024", "worked at Y for three years", "studying at Z since 2023". Today the only temporal
qualifier anywhere in the record is a single `PartialDate`:

- `LifeEvent.Date` (`backend/models/life_event.go`) is one `contactmodel.PartialDate` — an event happens
  *on* a date, with no end.
- `HouseholdMember.Since` / `.Until` (`backend/models/household.go`) are loose, unvalidated strings; the
  membership edge is the one place a start/end pair already exists.
- `Birthday` / `Anniversary` / `Card.Anniversaries[].Date` are single calendar dates (ADR 0015
  categories 2/3).

The fields the issue names as needing a period — addresses, phone numbers, relationships, employers,
titles, schools, pets, household membership — all share one shape: **a value that was true over an
interval**, so a start/end pair rather than a point. A birthday is not one of them; a job is.

### The RFCs have no home for a period, and we may not invent one

This ADR starts by asking whether the neutral model already has somewhere to put this, because ADR 0002
locks the correspondence table and forbids adding a mapping that is not in RFC 9555.

- **RFC 9553 (JSContact)** defines no date or period on `Address`, `Organization`, `Title`, or `Phone`.
  The property lists are exhaustive and transcribed in `docs/specs/rfc9553-model.md` §1 — `Address` is
  components/countryCode/coordinates/timeZone/contexts/full/defaultSeparator/pref/phonetic\*; `Organization`
  is name/units/sortAs/contexts/pref/phonetic\*; `Title` is name/kind/organizationId. None carries a date.
  The only dated object is `Anniversary` (§2.8.1), and it is a *single* `PartialDate | Timestamp` with an
  optional place — not an interval, and not attached to any other entry.
- **RFC 9554 (vCard 4 extensions)** adds single-date *properties* (`CREATED`, `DEATHDATE`) and the
  `CREATED` *parameter* on another property (`docs/specs/rfc9554-vcard-extensions.md` §1, §2). There is
  no date-range property or parameter, and nothing that attaches a date to `ADR`/`ORG`/`TITLE`.
- **RFC 9555 (the correspondence oracle)** therefore has no row for a period
  (`docs/specs/rfc9555-correspondence.md` §1). Under ADR 0002, "missing or ambiguous mapping → escalate,
  never invent", so a period **cannot** be a `Card` field with an invented vCard/JSContact home.

What the standards *do* offer is an escape hatch — vCard `X-` properties, JSContact `vCardProps`,
`JSPROP`/`JSPTR`. ADR 0002 rule 2 rejects routing canonical user content through `Passthrough`: it is
reserved for spec-blessed unknowns, and putting a first-class CRM period there would misrepresent it as
standards-originated (the same reasoning issue #515 applied to `Gender`).

### Where user content with no standards home already lives

`CRMEnvelope` (`backend/contactmodel/envelope.go`) is the neutral record's CRM sibling — "Mycorrhizal-specific
data that is NOT part of any contact-exchange standard. Format adapters MUST ignore it entirely." Issue
#515 established the rules (`docs/adrs/0002-correspondence-table-locked-oracle.md` appendix): canonical
user content must have either a `Card` home (standards round trip) or a `CRMEnvelope` home (neutral-record
round trip), and an envelope-only field is a **named** loss on file export
(`models.EnvelopeExportLossDiagnostics`, `backend/models/contact_record.go`), never a silent drop. The CSV
export is the one deliberate full-fidelity exception (issue #861).

## Decision

### 1. One neutral primitive: `contactmodel.TemporalRange`

A period is a pair of optional calendar dates. It reuses `PartialDate` (ADR 0015 category 2/3), so a
start-of-`2019` or a month/day-only endpoint is representable and nothing is zone-converted.

```go
// contactmodel
type TemporalRange struct {
    Start *PartialDate `json:"start,omitempty"`
    End   *PartialDate `json:"end,omitempty"`
}
```

- Either endpoint may be absent (open-ended: "since 2023", "until 2024").
- **Both absent is invalid** — it asserts nothing and must not be persisted.
- **Duration is never stored.** It is derived from the endpoints on every read, so it can never drift
  and a partial/rounded duration is a presentation choice, not data.

### 2. Periods live in `CRMEnvelope`, attached to a `Card` entry by its neutral element ID

A period is not a property of the *card*; it is a property of *one entry within* the card. The envelope
carries an explicit reference:

```go
// contactmodel
type EntryPeriod struct {
    Kind    string        `json:"kind"`     // address | organization | title (open set)
    EntryID string        `json:"entry_id"` // the referenced Card element's ID
    Range   TemporalRange `json:"range"`
}

type CRMEnvelope struct {
    // ...existing fields...
    Periods []EntryPeriod `json:"periods,omitempty"`
}
```

- `EntryID` is the neutral element's `ID` — the JSContact map key / vCard `PROP-ID` (ADR 0001: element
  ID fields serialize precisely so this identity survives a save/reload). **An entry must carry an `ID`
  to carry a period.** The nested REST editor and the format importers already assign IDs to collection
  entries; the legacy flat-only write paths (CSV `BuildContactFromRow`, merge-by-flat) do not, and
  therefore do not express periods — no data is lost because those paths never had one.
- **No positional/parallel arrays.** Keying by `EntryID` survives reordering and unrelated entry edits;
  an index-keyed parallel slice would silently re-associate periods with the wrong entry the first time a
  user deletes a row.
- The `Kind` token names which Card collection `EntryID` indexes, because JSContact `Id` keys are unique
  only *within* a collection. The starting set is `address`, `organization`, `title`. It is not hard-enum'd
  in the type, but a `Kind` with no Card resolver is a `400` at the API boundary — a period that cannot
  point at an entry is dead data, not forward-compat content. Adding a resolver (phones, web/other entries)
  is a model change with no schema migration.
- An orphaned period (its entry was deleted) is dropped, and a period whose `EntryID` does not resolve is
  a `400` at the API boundary — never silently stored.

### 3. `LifeEvent` gains an optional end date

The existing timeline entity is the natural home for "this happened, and lasted until". `LifeEvent.Date`
stays the **start/anchor** (the existing field, with its existing annual-recurrence and reminder
semantics); a new optional `EndDate *PartialDate` makes it a span.

- A `LifeEvent` with `EndDate == nil` behaves exactly as today (a point).
- A reminder / annual recurrence anchors on `Date` (the start), never on `EndDate` — "started this job"
  is the anniversary, "left" is not.
- `EndDate` is meaningless for a year-only start that never recurs, exactly as `Date` already is.

This keeps durations out of prose: the timeline can say "worked at Acme 2019–2024" because one event has
both endpoints, rather than because a human typed the range into a description.

### 4. Duration arithmetic (ADR 0015 rules, no new ones)

Duration is computed, per endpoint precision:

- Both endpoints full `YYYY-MM-DD` → a whole-calendar-day count.
- Year precision → a whole-year span (`2024 - 2019 = 5 years`, inclusive/exclusive stated at the
  presentation layer).
- Month precision → a whole-month span; mixed precision → the **coarsest shared precision** (a year and a
  full date span whole years).
- An open end means "ongoing" / "as of"; an open start means "since"; both open is rejected (above).
- No zone, no time-of-day, no DST, no leap-second arithmetic — periods are calendar facts (ADR 0015
  Rules 2/6). A 29-Feb endpoint advances to 1 March in a non-leap year, exactly as every other calendar
  date in the product.

### 5. Surfaces

- **Timeline / detail**: a `LifeEvent` with `EndDate` renders as a range and sorts by `Date` (the
  existing sort key); an entry period renders against its address/employer/title and contributes a
  duration label. Neither is a new timeline *type*; period metadata rides the existing item.
- **Export (`vCard3`/`vCard4`/JSContact)**: envelope periods have no target-format home by construction
  (this ADR's opening survey), so the file drops them. The loss is **named**, not silent: `Periods` is
  added to `models.EnvelopeExportLossDiagnostics` as the concept `crm.periods`. `LifeEvent` (and its
  `EndDate`) is already CRM-only and relational, so it is unaffected by the single-contact file export.
- **CSV export**: full fidelity (issue #861), so periods and `LifeEvent.EndDate` are carried in dedicated
  columns, including sensitivity and `status: suggested` as that export already does.
- **CardDAV/CalDAV**: unchanged. CardDAV writes the vCard projection, which has no period; CalDAV already
  projects `LifeEvent` to a `VEVENT` and anchors on `Date`, so an `EndDate` does not change the event's
  calendar semantics without an explicit decision (out of scope here).
- **Data retention / lifecycle**: periods are stored inside the existing `contacts.crm` JSON column and
  the `life_events` row, so they inherit both entities' existing soft-delete, cascade, and backup
  behavior (ADR 0004) with no new copy to enumerate.

### 6. Sensitivity

Unchanged. `CRMEnvelope` is already excluded from external sync, contact shares, and the neutral `Card`
exports, and the CSV exception deliberately carries everything — a period is exactly as sensitive as the
address or employer it qualifies and needs no separate rule.

## Correspondence-table impact

**None.** No row is added, because RFC 9555 has no period mapping and ADR 0002 forbids inventing one. The
absence is the decision, and it is recorded here so a future reader does not "fix" it by adding a `Card`
field. The DATA-01 field-compatibility matrix gains one `audit-515` row for `crm.periods` (no standards
home, loss-reported), regenerated by `go run ./cmd/gencompatmatrix`.

## Consequences

- **No migration for the period storage**: `CRMEnvelope` is serialized into the existing `contacts.crm`
  column. Adding `Periods` is a JSON-shape change only.
- **One migration** adds `life_events.end_date` (nullable `TEXT`), following the existing
  `PartialDate`-as-JSON-column convention. Existing rows are unaffected (`NULL` = a point in time).
- The nested contact editor gains optional start/end inputs, and the address/org/title converters must
  preserve the element `ID` (they previously dropped it). This is a prerequisite, not an optimization:
  without the ID there is nothing to attach the period to. Address periods and the `LifeEvent` end date
  ship with this decision; attaching a period to a specific employer/title entry is the same mechanism on
  the professional section and is a follow-up UI change, not a model change (the envelope and API already
  accept `kind: organization`/`title`).
- `EnvelopeExportLossDiagnostics` grows `crm.periods`; the DATA-01 matrix and its drift test are
  regenerated in the same change.
- `LifeEventInput`/`LifeEvent` DTOs, the OpenAPI schema, the contract fixtures, and the API baseline are
  extended for `end_date`.

## Infer-and-suggest: candidate events from dated periods

A dated period often *looks like* a life event — an address start reads as "moved", an employer start as
"started a new job". The decision is to **suggest, not link and not auto-create**:

- **Periods and events stay independent.** Nothing infers an event into existence, and no accepted event
  is tied back to the period it came from. The app offers a *candidate*; the user accepts it (creating a
  normal, independent `LifeEvent`) or dismisses it, exactly the household-suggestion pattern
  (`RelationshipEdge{status: suggested}` + `DismissedHouseholdSuggestion`).
- **Candidates are computed on read, never stored.** `GET /contacts/:id/life-event-suggestions` derives
  them from the card's periods; nothing is written when a candidate is merely offered, so a bulk import
  or CardDAV reconcile cannot manufacture a narrative timeline. "Accepting" is the ordinary
  `POST /life-events` (the client may open the prefilled editor and change the inferred type — the field
  does not know whether a new address means "moved" or "bought a home"); the resolution is recorded by
  `POST /life-event-suggestions/resolve`.
- **A decision is permanent for that candidate.** Resolution memory
  (`life_event_suggestion_resolutions`) is keyed by the natural
  `(user, entity, source_kind, source_entry_id, event_type)` tuple: dismissing once never re-offers, and
  an accepted candidate is suppressed even if the user later edits or deletes the event. A candidate is
  also suppressed while a same-type event already anchors on the same date.
- **Initial rules are deliberately narrow**: address start → `moved` (the period's end rides along as the
  event's `EndDate`); organization start → `job_change`. An address *end* has no event type of its own.
  Titles, household membership, address-end departures, and any richer inference are a follow-up scoped
  in a separate issue — extending the rule table is a code change, not a schema change.
- **No linkage means no drift, and no cleanup.** Because the accepted event is plain user data, later
  period edits never rewrite it, and deleting the period never deletes it. The resolution row is
  hard-deleted with the contact (`deleteContactAssociations` / `deleteUserCascade`).

Schema impact: one table, `life_event_suggestion_resolutions` (migration 000060), hard-delete per T26.
It holds no contact-file data and has no standards surface, so the correspondence table and the DATA-01
matrix are unchanged.

### Follow-up: extended rules and employer/title period editing (issue #1233)

The "deliberately narrow" rules above were extended, still as code changes with no schema impact:

- **Title periods** (`kind: title`) start → `job_change`. When an organization and a title share an
  anchor date they produce a single candidate, attributed to the organization entry (deterministic
  kind priority), so the user is not offered two identical events.
- **Address end with no successor** → `moved_out`, a new predefined `LifeEventType*` token (the
  departure counterpart of `moved`). It is a distinct token rather than a second `moved` because the
  resolution key is `(entity, source_kind, source_entry_id, event_type)` — two `moved` candidates from
  the same address entry would collide. "No successor" means no other address period starts at or after
  this one's end (compared at shared precision, `contactmodel.ComparePartialDates`); when a successor
  exists, its own start already yields the `moved` and no departure is offered, which is the compound
  "an address change is a move at the new start" case.
- **Employer/title period editing** shipped on web and Android, on the single organization / job-title
  entry the professional section surfaces, preserving (and, where an imported entry has none, minting)
  the entry's element ID so the period has something to attach to.
- **Android parity also fixed a latent data loss**: Android's `CRMEnvelope` did not model `crm.periods`
  and its contact PUT is a full overwrite, so any Android edit silently deleted every period (and
  `LifeEvent.EndDate`). Both are now modelled and round-tripped.
- **Still deferred**: household-membership (`Since`/`Until`) rules need a second data source in the
  suggestion service (the membership edge, not a Card `EntryPeriod`); and "schools" cannot be
  distinguished from employers because `contactmodel.Organization` carries no `contexts` field (RFC
  9553 defines one, but the model does not).

## Alternatives considered

- **Put the range on the `Card` entry (`Address.Start`/`Address.End`).** The smallest code change, but it
  puts non-RFC data on the standards surface and either leaks a non-standard JSContact property or needs
  an invented correspondence row — both forbidden by ADR 0002. Rejected.
- **Carry the period as an `X-`/`JSPROP` escape hatch.** Would give vCard↔JSContact round-trip, but ADR
  0002 rule 2 reserves `Passthrough` for spec-blessed unknowns; a canonical CRM period is neither, so
  this misrepresents its origin (the #515 `Gender` ruling). Rejected.
- **Model every period as a `LifeEvent` only (no entry attachment).** Simpler, and already on the
  timeline, but it cannot say *which* of two addresses an interval belongs to; the association degrades
  into prose or a fragile `RelatedEntityIDs` convention. Rejected as the sole mechanism — kept as the
  spanning-event half of this design.
- **Positional parallel arrays in the envelope (index-aligned with `Card.Addresses`).** No IDs needed,
  but a single row deletion re-associates every later period. Rejected.
- **Store a `Duration` alongside the endpoints.** Derived data that drifts the moment an endpoint is
  edited, and unrepresentable for partial dates. Rejected.
- **Convert `HouseholdMember.Since`/`Until` to the primitive in this change.** Tempting (the fields are
  today unvalidated strings), but that is a separate, migration-bearing concern; the primitive is
  deliberately shaped so that conversion is later a type swap, not a redesign.
