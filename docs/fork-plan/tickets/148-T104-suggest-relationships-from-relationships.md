# T104 — Suggest relationships from relationships (adding a sibling should suggest the other siblings and the parents)

| | |
|---|---|
| **Platform** | Backend |
| **Rating** | 3 |
| **Size** | M–L, unknown until designed |
| **Depends on** | Nothing |
| **Status** | **NOT READY — needs a design pass before implementation.** Filed deliberately unready; see below. |
| **Source** | Beta testing note, 2026-08-13: *"Suggest relationships from relationships — adding siblings should suggest other siblings and parents, etc."* |

## Why this exists

Nothing transitive is ever *written*. The two mechanisms that look like they might do this both don't:

**Household inference is pairwise within one household, and skips the case the report asks about.**
`GenerateHouseholdSuggestions` (`backend/services/household_service.go:66-154`) loads the members,
classifies each as adult/child/pet (`:81`, `classifyMember`), and runs a pairwise switch:
`HouseholdTypeFamilyUnit` (`:97-132`) emits adult↔adult `spouse_of` (`:106`), adult→child `parent_of`
(`:110`/`:114`) and human→pet `owned_by` (`:121`/`:125`) at confidence 0.8; `HouseholdTypeRoommates`
(`:134-144`) emits `roommate_of` at 0.4; `default` (`:146-151`) emits nothing. The comment at `:128` is
explicit: **"child<->child and pet<->pet: no rule in §91.4; skipped"** — so sibling suggestions are not
generated even inside a household. It is user-triggered only, via
`POST /households/:id/suggest-relationships` (`backend/routes/routes.go:212`), never a background job.

**Graph traversal infers at read time and never persists.** `TraverseGraph`
(`backend/services/graph_traversal.go:53+`) is a recursive CTE bounded by `maxTraversalDepth = 5` (`:36`),
exposed as `GET /graph/connections` ([T10](23-T10-graph-traversal.md),
`backend/controllers/graph_controller.go:136-196`). Its doc comment at `graph_traversal.go:15-18` states
the policy: *inferred relations are computed at query time, never stored*. Only `status: confirmed` edges
participate (`:57-60`).

[T40](49-T40-household-suggestions-shared-address.md) is a *household* suggester and its landing note
(`:50-53`) says it deliberately does not produce `RelationshipEdge`s.

So the plumbing to store a suggestion exists — `RelationshipEdge` with `Status = "suggested"`
(`backend/models/relationship_edge.go:27`, validated `oneof=confirmed suggested` at `:102`) and
`Source = "household-inferred"` (`:96`), with review surfaces on web
(`frontend/src/components/RelationshipEdgeList.tsx:145-175`, `useRelationshipEdges.ts:84`/`:94`) and Android
([M21](103-M21-android-relationships-depth.md), `RelationshipsScreen.kt:141-153`). What's missing is
anything that generates them from the edge graph itself.

## Why this is filed unready

The rules are the whole ticket, and they are not obvious. Writing them down is a design pass, not an
implementation detail. Open questions that must be answered *before* anyone writes code:

1. **Which closures?** Sibling-of-sibling is symmetric-transitive and safe-ish. Sibling's-parent-is-my-parent
   is not — half-siblings and step-families break it, and this app models real families. `parent_of` +
   `parent_of` → grandparent has no token in the registry at all. Decide the exact rule set, with the
   families it gets wrong stated explicitly.
2. **Confidence, and how it decays with hops.** Household inference uses 0.8 and 0.4 as flat constants
   (`household_service.go:100`, `:143`). A two-hop inference should not carry the same weight as a directly
   observed one.
3. **When does it run?** On edge creation (cheap, incremental, but fires mid-form), on demand behind a
   button like the household suggester, or as a job? There is no background job infrastructure for this
   today.
4. **What stops it from flooding?** A family of six generates 15 sibling pairs from one edge. The review
   surface is a flat list; 15 suggestions from one action will read as spam. Batching, capping, or grouping
   is required, and that is a UI decision as much as a backend one.
5. **Rejection has no memory.** `suggestEdgeIfNew` (`household_service.go:167+`) dedupes against existing
   edges in either direction using `models.InverseRelationType` (`:168`), but a *rejected* suggestion is
   hard-deleted and leaves nothing behind — documented at
   `docs/fork-plan/tickets/49-T40-household-suggestions-shared-address.md:24-27`. A transitive suggester
   re-proposes rejected edges on every run. This must be solved first or the feature is unusable; it is the
   same persisted-dismissal problem [T93](137-T93-duplicate-scan-endpoint-and-review.md) has to solve, and
   the two should share a mechanism.

## What the design pass should produce

A rule table (source edge pair → inferred type → confidence), a decision on trigger and cap, a dismissal
model, and a size estimate. Reuse `suggestEdgeIfNew` (`household_service.go:167`) and
`models.InverseRelationType` — the storage and dedupe halves already work.

Note `Source = "ai-suggested"` is a valid enum value (`backend/models/relationship_edge.go:15`) that **no
code produces**. A rules-based transitive suggester is not AI and should not claim that source; add a new
one (`graph-inferred`) rather than reusing it.

## Done when

*(Not implementable yet — the design pass above is the next step, not a checklist.)*
