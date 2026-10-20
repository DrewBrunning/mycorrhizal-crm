# ADR 0023: Relationship health score (facets, weights, thresholds)

- **Status:** accepted
- **Date:** 2026-09-22
- **Depends on:** `Activity.Qualifying()`/`CadencePolicy.Qualifies` (T19), `TraverseGraph` (T10),
  `User.SelfContactVCardUID` (T90), the sensitivity model (`normal|private|secret`)
- **Implements:** issue #383
- **Amended:** 2026-10-16 — per-contact target closeness (issue #1737), see the amendment at the end
- **Amended:** 2026-10-20 — ambient contacts (issue #1733) and given gifts as interactions (issue #1736), see the amendment at the end

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

## Amendment, 2026-10-16 (target closeness per contact, issue #1737)

### Context

The Android UX audit (#1718) asked for a way to say "I want to be close to this person". The
Closeness facet above is *inferred* from relationship edges and hop distance, and it must stay
that way: it describes the graph, not the user's intent. This amendment adds a separate,
user-set **target closeness**. It is a goal that the computed Recency and Frequency facets are
measured against. It never overrides the inferred Closeness facet.

Two maintainer decisions dated 2026-10-16 are part of this amendment:

1. **Setting a target offers to create a cadence policy.** When the user sets a target on a
   contact with no cadence policy, the client offers to create one with the tier's
   `default_interval_days`. The user can accept or dismiss. Dismissing keeps the target with no
   policy. If a policy already exists, it is left alone and still wins.
2. **"Behind target" feeds reach-out suggestions automatically.** A contact whose target status
   is `behind` gets a reach-out suggestion through the existing reach-out engine
   (`backend/services/reach_out_trigger_service.go`). The suggestion is deduplicated against
   cadence-overdue, so one contact never gets two nudges for the same gap. Ambient contacts never
   get one.

### Decision

**Vocabulary.** The four tier names already defined under `tier_definitions` in
`backend/internal/scoring/testdata/weights.json`: `immediate`, `close`, `casual`, `distant`. "No
target" is the default and is stored as `NULL`. No new tier, weight or interval is added. The
target's expected interval is the tier's existing `default_interval_days` (14 / 30 / 45 / 60).
`scoring.Config` gains a `TargetTierIntervalDays map[string]int`, filled from `tier_definitions`
by `flatten`. `validate()` rejects a config whose `tier_definitions` keys are not exactly the
four tokens in a new `models.TargetClosenessTiers()`. This is the same drift-check pattern the
relation-type tiers use, so the API's `oneof` list, the DB `CHECK` and the tunables file cannot
disagree silently.

**Storage.** A new nullable column, `contacts.target_closeness TEXT NULL`, with
`CHECK (target_closeness IS NULL OR target_closeness IN ('immediate','close','casual','distant'))`.
There is no default and no backfill. Every existing row reads `NULL` ("no target"), so existing
data is unaffected. On the Go side:

- `models.Contact.TargetCloseness *string` (`gorm:"column:target_closeness"`, no `default:` tag;
  this is backend trap #1 and issue #1240).
- `contactmodel.CRMEnvelope.TargetCloseness *string` (`json:"target_closeness,omitempty"`).

The flat column is authoritative. `RecordFromContact` copies it into the envelope.
`mergeRecordFromFlat` takes it from `fresh`, like the other flat-owned envelope fields.

**The contact PUT does not write it.** `ApplyRecordToContact` deliberately ignores
`Envelope.TargetCloseness`, so `POST /contacts` and `PUT /contacts/:id` never change the stored
value, whatever the body says, including when the field is absent. Gender and Periods do not work
this way, and the difference is deliberate. The contact PUT is a full overwrite, and the Android
form rebuilds `crm` from its own model on every save (see the `periods` comment in the Android
`ContactRecord.kt`, a data loss that shipped once). A client built before this field existed
would otherwise wipe every target on its next save. Instead, the target has its own endpoints,
like favorite (`POST /contacts/:id/favorite`). That also gives decision 1's offer a natural home
in the response.

**API.**

- `PUT /api/v1/contacts/:id/target-closeness`, body `{"tier": "close"}`
  (`models.ContactTargetClosenessInput`, `Tier string`,
  `validate:"required,oneof=immediate close casual distant"`).
- `DELETE /api/v1/contacts/:id/target-closeness` clears the target. It is idempotent: clearing an
  already-`NULL` target returns 200.
- Both return 200 with `models.ContactTargetClosenessResponse`:

  ```json
  {
    "contact_id": 12,
    "target_closeness": "close",
    "default_interval_days": 30,
    "cadence_policy_offer": { "entity_id": "<contact vcard_uid>", "target_interval_days": 30 }
  }
  ```

  `target_closeness`, `default_interval_days` and `cadence_policy_offer` are never omitted. They
  are JSON `null` after a DELETE. `cadence_policy_offer` is non-null only when a tier was set
  **and** the contact has no live `cadence_policies` row (`user_id` and `entity_id` match,
  `deleted_at IS NULL`). Its shape is exactly `CadencePolicyInput`, so accepting the offer is a
  plain `POST /api/v1/cadence-policies` with that object. Dismissing it is a no-op on the server.
- The write is `db.Model(&contact).Update("target_closeness", v)`, the same hook-firing `Update`
  `FavoriteContact` uses. Hooks fire, so the contact's revision bumps. Like favorite, the route is
  a toggle-shaped action: it is exempt from If-Match (`reasonToggle`). It needs no
  Idempotency-Key, because PUT and DELETE are idempotent by method.
  After a success, clients re-read the revision exactly as they do after a favorite toggle.
- Ownership: the contact is loaded with `user_id = currentUserID`. Another user's contact or a
  missing one returns 404. Archived contacts can have a target. Their target is simply never
  evaluated for suggestions (below).
- DELETE also dismisses every `pending` `target_behind` suggestion for that contact in the same
  request, so a cleared target never leaves a stale nudge behind.

**Interval precedence (changes Recency and Frequency for targeted contacts).**
`resolveClosenessAndInterval` becomes: a real `CadencePolicy.TargetIntervalDays` wins, then the
target tier's `default_interval_days`, then the closeness-derived default (unchanged). The
Closeness facet's *value* is not touched. Only the interval that Recency and Frequency normalize
against changes. A contact with no target scores exactly as before.

**Target status.** `scoring.RawInputs` gains `TargetTier string` (`""` means no target). A token
with no entry in `Config.TargetTierIntervalDays` is treated as no target, the same way
`computeCloseness` ignores unrecognized tokens. With a target set, `scoring.Result` carries
`Target *TargetResult`:

- `Value = (Recency.Value*WeightRecency + Frequency.Value*WeightFrequency) / (WeightRecency + WeightFrequency)`,
  using the two facet values already computed against the resolved interval. With today's
  weights that is `(35·R + 20·F) / 55`.
- `Status` reuses the band thresholds, applied to `math.Round(Value)` the same way `band()` is
  applied to the rounded total: `on_track` when ≥ `moss_min` (70), `drifting` when ≥
  `chanterelle_min` (40), `behind` otherwise. No new threshold is introduced.
- `IntervalDays` is the resolved interval. `IntervalSource` is `cadence_policy` or `target_tier`.
- `Reason` is a plain-language English string, like every facet reason.

On the wire, `ContactScoreResponse` gains `target`, which is never omitted and is `null` when the
contact has no target:

```json
"target": {
  "tier": "close",
  "status": "drifting",
  "value": 65.9,
  "interval_days": 30,
  "interval_source": "target_tier",
  "reason": "Recency and frequency score 66 against a 30-day interval from the close target"
}
```

`status` is one of `on_track`, `drifting` or `behind`. Clients color it with the matching band:
on_track maps to moss, drifting to chanterelle and behind to russula. The total score and band
are computed exactly as before, and `target` does not feed back into them. The `GET /graph` and
`GET /graph/connections` decorations are unchanged.

**Reach-out integration (decision 2).** `ReachOutSuggestion.Kind` gains `target_behind`. The
`reach_out_suggestions.kind` `CHECK` is widened by a data-preserving table rebuild, the same
technique `000068_import_runs_mycorrhizal_format` used: copy every row verbatim, then recreate the
three indexes. The scheduled `reach_out_detection` job (`DetectReachOutSuggestions`) runs a second
pass after its audit-cursor scan, under the same job lock. That pass is a daily, state-driven
evaluation, not an audit diff. For every user with at least one non-archived contact whose
`target_closeness IS NOT NULL`, it evaluates the score in bulk through the same path
`computeContactScores` uses. That keeps the query count per user fixed, never one query per
contact. The pass creates a suggestion only when **all** of these hold:

1. The contact is not archived and not deceased (`Card.IsDeceased()`, issue #1193).
2. The contact has **no** live cadence policy. *This is the dedup rule against cadence-overdue.*
   When a policy exists, it wins the interval (above), and its own overdue path (the dashboard
   overdue list, plus the `cadence.overdue` webhook from `ProcessOverdueCadences`) is the one
   nudge for that gap. So a target suggestion and a cadence-overdue nudge can never both exist for
   one contact. Accepting decision 1's offer therefore moves the contact from target nudges to
   cadence nudges.
3. The contact is not ambient (issue #1733, `RawInputs.Ambient`). This is a belt-and-braces
   check, since ambient already forces status `on_track` (below).
4. The contact has at least one qualifying interaction. This mirrors T19's cadence rule that
   cadence never counts from the creation date (`CadenceHealth`), so a contact's nudge behavior is
   the same whether the user accepted or dismissed the policy offer.
5. The target status is `behind`.
6. No `target_behind` row exists, **in any status**, for the same
   `(user_id, contact_vcard_uid, old_value, new_value)`.

The row's fields: `old_value` is the tier token. `new_value` is the last qualifying interaction's
timestamp as `time.RFC3339` in UTC. `audit_event_id` is `0`, because the row is not
audit-derived and the column stays `NOT NULL`. Rule 6 therefore means **one nudge per gap**: after
the user dismisses a suggestion, the same gap never nudges again. A new qualifying interaction
starts a new gap, and so does a tier change. The row is created with the same companion one-off
Reminder and the same `reach_out_suggested` webhook as the org/title/address kinds, in
`createReachOutSuggestion`'s single transaction. The Reminder message is
`Reach out — behind your <tier> target: <name> (last interaction <YYYY-MM-DD>)`, with the date
taken in the reminder location. The webhook payload keys are unchanged, and `kind` carries the
new value.

The same pass **auto-dismisses** a `pending` `target_behind` row when its contact no longer
satisfies rules 1–5, or when the contact's current key (`old_value`, `new_value`) differs from
the row's. Examples: a newer interaction was logged, a cadence policy was created, the contact
became ambient or was archived. The companion Reminder is left as is, because it was delivered
when it fired. The org/title/address kinds are never auto-dismissed. That behavior is unchanged.

**No feedback loop.** The ReachOut facet's input (`pendingReachOutEntityIDs`) excludes
`kind = 'target_behind'`. Without this, being behind would create a suggestion, the suggestion
would drop the ReachOut facet to 30, and the total score would fall further. The ReachOut facet
keeps its original meaning: an org/title/address change the user hasn't followed up on.

**Surfaces.** The contact detail view shows "Target: close · drifting" next to the health badge,
and the score breakdown shows the target's reason. The dashboard shows `target_behind`
suggestions in its existing reach-out block. That block is ordered by `created_at DESC` as today,
and #1791 bounds it like every other dashboard section. A separate "ranked by gap to target" list
is **not** added in this round: the reach-out block plus the per-contact status covers the two
decisions, and a ranked view can be filed later if the block proves insufficient. Contacts with
no target behave exactly as today on every surface.

**Composition with #1733 (ambient) and #1736 (gifts as interactions).** Both change the *inputs*
that this amendment reads. Neither changes the target logic itself:

- An ambient contact (co-resident or flagged) is always `on_track`, with the reason "Ambient
  contact — always on track", and never produces a `target_behind` suggestion. With #1733's
  facet override (Recency and Frequency both 100), the computed value is already 100. The
  explicit short-circuit keeps the guarantee independent of the facet weights.
- A given gift is a qualifying interaction (#1736). It counts toward Recency, Frequency and the
  "last qualifying interaction" timestamp that rule 4 and the dedup key use, with no extra code
  here.
- Implementation order: #1791 (dashboard/graph scaling, which touches the same score and cadence
  services), then #1733, then #1736, then this amendment's backend issue. Each of them adds a
  migration and/or a `RawInputs` field. Landing them in that order keeps every migration number
  and every regenerated perf baseline a clean diff.

**Everything else the field touches.**

- **Account bundle** (ADR 0028 Decision 3): it rides `crm.target_closeness` in
  `AccountBundleContact.CRM`. Because `ApplyRecordToContact` ignores it, the bundle import sets it
  explicitly: `MappedContact` gains `TargetCloseness *string`, and the import uses it on create.
  On a merge into an existing contact, it is used only when the existing value is `NULL`. A token
  outside the vocabulary is dropped (stored as `NULL`).
- **CSV export** (the full-fidelity exception): a trailing `Target Closeness` column, empty for no
  target.
- **vCard/JSContact export**: the envelope is never serialized. The DATA-01 matrix gains a
  `crm.target_closeness` no-home row with a loss report, and `EnvelopeExportLossDiagnostics`
  names it when set.
- **Contact merge**: added to `mergeScalarFields` (the keeper wins when both are set, otherwise
  the non-empty side wins).
- **Sync and sensitivity**: not applicable. It is a CRM-local goal that never reaches CardDAV,
  shares or exports beyond the two full-fidelity ones above.

**Implementation issues:** #1796 (backend), #1797 (web) and #1798 (Android). Both clients
depend on #1796.

### Consequences

- The tier vocabulary has one source (`tier_definitions`), checked against the API `oneof` and the
  DB `CHECK` by `validate()` and a test.
- A targeted contact's total score can move when its target is set, because Recency and
  Frequency now use the target's interval when there is no policy. This is intended: the target
  *is* the expected interval.
- `reach_out_detection` now does a full score evaluation for users who have targets. That cost is
  bounded by the number of targeted contacts and runs once a day. The PERF-02 baseline is
  regenerated with the backend change.
- Additive to the `/api/v1` contract (new routes, a new field, a new enum value). The
  `reach_out_suggested` webhook's `kind` gains a value, so webhook consumers that switch on `kind`
  should treat unknown values as generic.


## Amendment, 2026-10-20 (ambient contacts and given gifts, issues #1733 and #1736)

### Context

Recency, Frequency and cadence only counted logged activities. That mis-scores two kinds of
real contact. The people you live with, or see every day, are in constant contact that nobody
logs, so their score decays and cadence calls them overdue. And handing someone a gift is an
interaction that no activity records.

### Decision: ambient contacts (#1733)

A contact is **ambient** when either of these holds:

1. **Co-resident (derived, nothing stored).** The user's self-contact (`User.SelfContactVCardUID`)
   and the contact are both **current** members of the same household. A membership is current
   on a day when `since` is empty or not after that day, and `until` is empty or not before it.
   The comparison is on the wall-clock date in the reminder location (ADR 0015) and respects the
   partial-date precision of `since`/`until`: an `until` of `2026-10` is still current throughout
   October, and a `since` of `2026-11` has not started in October. Both memberships must be
   current, so a user who has moved out is no longer co-resident with the flatmates who stayed.
   **A user with no self-contact has no co-residents.** The flag below still applies to them.
2. **Flag (stored).** The per-contact `contacts.ambient` boolean, set by the user for people they
   see daily but do not live with (a coworker at the next desk, a caregiver).

When both hold, the reported reason is `household`.

**Effect on the score.** `RawInputs.Ambient` and `AmbientReason` (`household` | `flag`). When set,
`computeRecency` and `computeFrequency` return `Value: 100` with the reason "Ambient contact
(lives with you)" or "Ambient contact (marked)", instead of the days/count math. Closeness,
ReachOut and LastUpdated are unchanged, and so is the interval resolution. `scoring.Result` and
`GET /contacts/:id/score` carry `ambient` (always present) and `ambient_reason` (absent when not
ambient), so a UI can explain the 100s.

**Effect on cadence.** An ambient contact is never overdue: `CadenceHealth.OverdueBy` is forced to
0 and `CadenceHealth.ambient` / `ambient_reason` are set, in the single-policy read, the
overdue list, the dashboard and the `cadence.overdue` webhook job. The truthful
`last_interaction` and `next_due` are still reported. The overdue list and the `cadence.overdue`
webhook are the cadence nudges, so an ambient contact generates none. (The #1737 amendment's
`target_behind` reach-out suggestions exclude ambient contacts separately.)

**Storage and the write rule for the flag.** `contacts.ambient BOOLEAN NOT NULL DEFAULT 0`
(migration `000075_contact_ambient`; no backfill needed, every existing row is non-ambient, which
is today's behavior). On the Go side `models.Contact.Ambient bool` carries
`gorm:"column:ambient"` with no `default:` tag (backend trap 1, issue #1240).

The clients (#1734, #1735) bind the toggle to `crm.ambient`, so the flag rides the contact save,
unlike `target_closeness`, which has its own sub-resource. The reason for that sub-resource, though,
applies here too: Android rebuilds `crm` from its own model on every save, so a build that predates
the field would send a body without it. A plain boolean would read that as "false" and silently
wipe every flag on the user's next save. **So `CRMEnvelope.Ambient` is a `*bool` with
preserve-when-absent semantics.** `ApplyRecordToContact` writes the flat column only when the
envelope value is non-nil: absent keeps the stored value, an explicit `true` or `false` sets it.
Reads emit `crm.ambient` only when true (a non-ambient contact's envelope is byte-identical to
before the flag existed), and clients treat absent as false. A client that wants to clear the flag
sends an explicit `false`. This keeps the issue's `crm.ambient` binding without a second
endpoint and without the wipe hazard. The flat column is authoritative: `BeforeSave` and the read
path mirror it into the envelope, and `mergeRecordFromFlat` takes it from the fresh derivation.

Everything that moves a contact carries it: the account bundle (via `crm`, restored on import by
`ApplyRecordToContact`; a bundle that predates the flag imports as non-ambient), the flat CSV
export (an `Ambient` column right after `Periods`), and a contact merge (the flag is a
union: either record being ambient makes the keeper ambient). vCard/JSContact exports never
serialize the envelope, so the DATA-01 matrix gains a `crm.ambient` no-home row with a loss report,
and `EnvelopeExportLossDiagnostics` names it when the flag is set.

**Cost.** One extra bulk query per score or cadence evaluation (`ambientByEntityID`: contacts
left-joined to the household rows they share with the self-contact, with the self-contact read in
a subquery). Never one query per contact.

### Decision: given gifts as interactions (#1736)

A gift with `status = given` and a non-null `date` is a **qualifying interaction for its recipient**
(`gifts.entity_id`) on its `date`, exactly like an activity of the synthetic type **`gift`**.
Recency (last qualifying interaction), Frequency (count in the window) and the cadence
last-interaction date all union it in. A cadence policy with empty `qualifying_types` counts it,
and a policy listing specific types counts it only if `gift` is listed (`gift` is already in the
frontend suggestion list). `received`, `purchased` and `idea` gifts do not count, and neither do
soft-deleted gifts or another user's rows. A given gift whose `activity_id` points at a live
activity linked to the same contact is not counted again, because that activity already is the
interaction.

**Date defaulting.** When a gift is saved as `given` (create or update) with no `date`, the server
stamps the request clock's now, so marking a gift given always counts. An explicit date is kept as
is. An update that omits the date on a gift that was **already** given keeps its stored date,
since the update is a full replace and an omitted date there is not a request to rewrite history.
The default applies on the REST write path only: gifts arriving through an import keep exactly the
date the source carried.

**Cost.** One extra bulk query per facet (last interaction, frequency window) and one in the
cadence batch path. The PERF-02 baseline is regenerated.

### Composition with target closeness (#1737)

An ambient contact is always on track (the explicit short-circuit in the #1737 amendment), and a
given gift is simply one more qualifying interaction for that amendment's Recency, Frequency and
"last qualifying interaction" inputs. Nothing in the target logic changes.

### Consequences

- The additive fields (`ambient` on the score and cadence-health responses, `crm.ambient`) pass the
  `/api/v1` baseline drift check; the baseline is regenerated.
- A user who flags a lot of people ambient will see their scores rise, by design. The flag is
  explicit and reversible, and the breakdown always says why.
- Cadence for an ambient contact is a deliberate no-op: a policy can still exist (it is the user's
  record of intent), but it never fires.
