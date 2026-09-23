# ADR 0023: Relationship health score (facets, weights, thresholds)

- **Status:** accepted
- **Date:** 2026-09-22
- **Depends on:** `Activity.Qualifying()`/`CadencePolicy.Qualifies` (T19), `TraverseGraph` (T10),
  `User.SelfContactVCardUID` (T90), the sensitivity model (`normal|private|secret`)
- **Implements:** issue #383

## Context

Issue #383 asks for a per-contact "relationship health score" that colors graph nodes moss
(healthy), chanterelle (warning), or russula (neglected) — the app's existing fungus-palette visual
language (`frontend/src/theme.ts`'s `success`/`warning`/`error`, Android's `MycorrhizalColors`). The
issue is explicit that it is a design pass, not an implementation ticket: the facet set, weights,
and thresholds were left open. This ADR makes those decisions.

Five candidate facets from the issue were kept; two were deliberately dropped (see below). The
scoring function lives in `backend/internal/scoring` as a pure function of already-gathered raw
facts (`RawInputs`) and a tunable `Config` (`backend/internal/scoring/testdata/weights.json`) — no
database access, no wall-clock read, fully unit-testable with seeded fixtures
(`backend/internal/scoring/scoring_test.go`). `backend/services/contact_score_service.go` is the
DB-facing layer that gathers `RawInputs` in bulk and calls `scoring.Compute`.

## Decision

### Facets and weights (sum to 100, enforced by `validate()`)

| Facet | Weight | Signal |
|---|---|---|
| Recency | 35 | Days since the last *qualifying* interaction, normalized against a target interval |
| Frequency | 20 | Rolling qualifying-interaction count vs. expected count over a trailing window |
| Closeness | 20 | Relationship-type tier of the closest structural edge to the self-contact, or hop distance |
| ReachOut | 15 | An un-dismissed `ReachOutSuggestion` (org/title/address change) → fixed low score |
| LastUpdated | 10 | Contact-record edit recency — the weakest signal, capped influence by design |

Recency is weighted highest because it is the single strongest, least gameable signal. LastUpdated
is weighted lowest and floored (never reaches 0) because editing a contact's phone number is weak
evidence of an actual relationship-maintaining interaction — see "Known accepted gaming vector"
below.

### Thresholds

moss ≥ 70, chanterelle 40–69, russula < 40 — tunable in `weights.json`, not hardcoded.

### Facet derivation and degradation rules

**Recency.** Uses the contact's real `CadencePolicy.TargetIntervalDays` when the user created one;
otherwise falls back to a default interval derived from the Closeness tier (a closer relationship
defaults to a shorter expected check-in window). The curve is linear: 100 at zero days since the
last qualifying interaction, 50 at exactly the target interval, 0 at double the interval. A contact
with **no qualifying interaction ever** gets a fixed constant (45 — chanterelle territory)
deliberately chosen to be neither 0 (indistinguishable from "wildly overdue") nor derived from the
interval (which would make a same-day-created contact coincidentally match "1 day overdue" — an
accident, not a decision).

**Frequency.** Qualifying-interaction count in a trailing 180-day window vs. the count "on pace"
given the same target interval Recency uses, capped at 100 so far-over-expected contact isn't
rewarded indefinitely.

**Closeness.** A bulk direct-edge query between `User.SelfContactVCardUID` and the contact
(`status=confirmed`, `sensitivity != secret`, both directions) finds every structural relation type
directly connecting them. When more than one exists, the **highest-tier type wins,
deterministically** — not "whichever the database or graph traversal happened to return first".
This matters because `RelationshipEdge`'s unique key is `(user_id, source_id, target_id, type)`, so
two distinct edge types between the same pair (e.g. `friend_of` *and* `conflicts_with`) are legal
storage, and `TraverseGraph`'s own depth-tie dedup resolves via Go map iteration order — reading its
resolved relation label directly would make the closeness tier flaky.

Two overlay/affinity edge types — `gets_along_with` and `conflicts_with`
(`models/relationship_type_registry.go`'s "Affinity edges" section) — are **deliberately never
given a tier**. They describe pairwise compatibility, not a structural bond, and both are still
`status=confirmed`/non-secret edges that `TraverseGraph` happily traverses — so a contact reachable
*only* through a `conflicts_with` edge must score identically to an unconnected contact, never as
"closer" than a stranger. `scoring.Compute` enforces this structurally: any relation-type token with
no entry in `Config.RelationCloseness` is silently ignored, and `validate()` rejects a config that
tries to tier an affinity type or that leaves a structural type untiered — a drift check against the
live registry (`models.KnownRelationTypes()`), so a newly-added relation type must get a deliberate
tier assignment before it can ship.

No direct structural edge but reachable via a structural-only path (every hop's relation type
tiered, never an affinity-only hop) → hop-count decay, floored so a distant-but-connected contact
never reads as fully unknown. No self-contact set, or unreachable → a flat neutral default (50) —
expected to be the **common** branch in practice: `EnsureSelfContact` runs at registration and is
lazily backfilled on `/users/me`, so most accounts *do* have a self-contact, but most users have
never manually wired themselves into the relationship graph with an edge to another contact.

**ReachOut.** A pending `ReachOutSuggestion` (the org/title/address-change detector — unrelated to
staleness) is a discrete, actionable warning: fixed at 30 (russula territory on its own), not a
gradient, since it's a binary "there's something you haven't followed up on" fact.

**LastUpdated.** Decays from 100 at day 0 to a floor of 30 over 180 days, never below the floor.

### Sensitivity

The Closeness facet's direct-edge query excludes `sensitivity=secret` in the query itself, matching
`TraverseGraph`'s own filter and the `contact_record.go` convention — a secret edge and a no-edge
case must produce byte-identical output
(`TestContactScore_SecretEdgeExcludedFromCloseness`). The score is a computed UI read model only —
it never touches `Card`/`CRMEnvelope`, exports, or sync, so it cannot leak through those surfaces by
construction.

### Deliberately excluded facets

- **Upcoming occasions** (birthday/anniversary proximity): the issue ties this to "the
  occasions/event-planning surface", which doesn't exist yet as a feature. Inventing parallel
  date-proximity logic ahead of that surface would create a second, disconnected system rather than
  composing with one — deferred until that work lands.
- **Response rate**: no inbound/outbound or bidirectionality data model exists on `Activity` today.
  Inventing one is out of scope for a coloring feature.

### Known accepted gaming vector

A contact whose vCard fields were edited today, with zero real interaction ever, scores full marks
on LastUpdated while Recency and Frequency both sit at their "no interaction" defaults. At weight
10, this cannot flip the band by itself in the general case but can nudge a borderline score across
a threshold. This is accepted, not fixed: LastUpdated is deliberately the lowest-weighted facet
specifically because it's the weakest, most gameable signal, and the alternative (weight 0, i.e.
dropping it entirely) would throw away a genuinely-informative-if-weak signal for no benefit.

## Consequences

- Every number in `weights.json` carries a mandatory `reason` string (mirroring
  `internal/perfbench/testdata/budgets.json`'s pattern) — changing a weight or threshold is a
  deliberate, reviewed diff, not a silent magic-number edit.
- `validate()`'s drift check means adding a new relation type to `relationship_type_registry.go`
  without also tiering it in `weights.json` fails at config-load time (every `LoadConfig()` call,
  and `TestLoadConfig_Valid`), not silently defaulting to "unknown".
- The score is exposed via `GET /contacts/:id/score` (full breakdown) and decorates both `GET
  /graph` (`GraphNode`) and `GET /graph/connections` (`GraphChain`), since the web canvas graph and
  Android's ego-network list consume different endpoints/DTOs.
