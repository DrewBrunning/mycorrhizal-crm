# ADR 0009: REST write-conflict policy (reject-and-return, per entity shape)

- **Status:** accepted
- **Date:** 2026-09-01
- **Depends on:** ADR 0006 (revision-token schema), ADR 0008 (conditional-write enforcement)
- **Implements:** issue #458 (CON-03, merge/conflict semantics). ADR 0008 decided *how* a client
  attaches a precondition and *that* a stale one is rejected; this ADR decides *what the conflict
  resolution model is* — per entity shape, and why it is not an automatic merge.
- **Contract behaviour** — DOC-03 (#488) documents it for API consumers and MAINT-02 (#491) treats a
  change to it as breaking.
- **Coordinates with:** #479 (ANDROID-02 offline sync) for the reconnection case.

## Context

Two write-conflict domains already exist in this codebase and resolve differently. That is fine as
long as each is deliberate and written down — which is the point of this ADR for the second one.

- **CardDAV sync** — decided. `reconcileContactSync` performs a full overwrite and discards local
  edits on remote change; migration `000032_contact_sync_conflicts` then records every overwritten
  field (local value + remote value) so the UI can tell the user "your edit to X was overwritten"
  and offer it back. This is a **detect-and-surface** policy, not a merge, and it is reasonable for a
  protocol whose wire model is whole-vCard replacement.
- **Interactive REST clients** — until CON-01 (#456), nothing was decided; last writer won silently.
  CON-01 added an opt-in `If-Match` check that returns `412` on a stale write. This ADR makes that
  the deliberate, documented conflict model and defines it for every entity shape.

CLAUDE.md (T13) is explicit that the CardDAV full-overwrite policy must **not** be copied into new
two-way paths without a deliberate decision. This is that decision for REST.

## Decision

### The model: reject-and-return. No silent merge. No automatic merge in v0.6.7.

On a REST write whose `If-Match` revision does not match the row's current revision, the server
**rejects the write** (`412`, `details.expected_revision`), changes nothing, and returns. The client
re-reads the current representation — which it can then diff against its own pending edit and resolve,
in the UI, with the user — and retries with the fresh revision.

The server never merges two versions of a row. It never silently applies "the parts that don't
conflict." A write that the server accepts is exactly the representation the client sent, at the
revision it claimed; a write the server rejects leaves the stored row byte-identical.

Why reject rather than auto-merge:

- **A wrong merge is worse than a rejection.** The realistic conflicts here (a renamed contact, a
  corrected birthday, a rewritten "how we met") are ones where "combine both" has no correct answer —
  one edit is right and the other is stale. Rejecting hands that judgement to the person who has both
  versions in front of them.
- **Auto-merge needs a visibility surface REST does not have yet.** Item 3 of the ticket is
  non-negotiable: an automatically resolved conflict the user cannot see is silent corruption. The
  CardDAV side pays for `contact_sync_conflicts` + a restore UI to satisfy exactly this. REST has no
  equivalent surface, and building one is out of scope for a milestone whose job is to *prove data
  cannot be silently lost*. Reject-and-return needs no such surface: nothing was resolved, so there
  is nothing to disclose.
- **The client already has everything a merge needs.** After a `412` the client holds its own pending
  edit and can `GET` the current record; a field-level union (see repeatable fields below) is a
  client-side operation on two complete representations. Moving it server-side buys nothing and adds a
  policy that MAINT-02 then freezes.

### Per entity shape

| Shape | Policy | Reasoning |
|---|---|---|
| **Scalar fields on a revision-bearing entity** (`Contact` name/birthday/org/…, `Activity`, `LifeEvent`, `Note`, `Reminder` fields) | Conditional write; stale → `412`, nothing changed. No merge. | The classic conflict with no correct automatic answer. The user resolves it with both versions visible. |
| **Repeatable fields** (`Card` phones, emails, addresses, URLs, …) | Same conditional write — a stale write is rejected **whole**, not partially applied, so no entry is ever silently dropped. A union of two clients' additive changes is a **client-side** step after the `412` (re-read, merge the lists, re-PUT). | `PUT /contacts/{id}` replaces the list wholesale, so without the `If-Match` guard client B's single-phone PUT would silently drop client A's phone — which is the bug the guard prevents. Server-side auto-union is deferred, not rejected: it needs a "these entries were auto-added by conflict resolution" disclosure the REST surface does not have. Prerequisite for revisiting: a REST conflict-surface analogous to `contact_sync_conflicts`. |
| **Edge- and join-shaped rows** (`RelationshipEdge`, `CircleMember`, `ContactTag`, `HouseholdMember`, …) | Last-write-wins. No revision token, no conditional write. | ADR 0006 §"Which entities are excluded" and CLAUDE.md trap #7: these are hard-deleted, natural-keyed, and re-pulled wholesale by clients. A per-row version token has no consumer. The blast radius of a lost update is one cheap, re-fetchable row. |
| **Relationship direction** (`RelationshipEdge.Type` + `source_id`/`target_id`) | Not a policy — an **invariant**. An update persists exactly the `(source, target, type)` the client sent. The server never derives the inverse and never writes it. | `RelationshipEdge.Type` describes the source's role relative to the target; only one direction is ever stored and the inverse is derived from `relationship_type_registry.go`, never persisted. A merge that inverted an edge would be silent corruption with no recovery. `UpdateRelationshipEdge` is full-replace of the input DTO, so LWW here cannot invert — it can only replace the row with another correctly-oriented row. Pinned by test, not left to policy. |

### Offline Android reconnection (#479)

A client returning from offline with many stale changes replays **each change as its own conditional
write**. Each is independently accepted (revision advanced) or `412`'d (client resolves that one and
retries). There is no batch-merge, no "apply all my offline edits over whatever is there," and no
second conflict model. The reconnection UX — showing the user which of their queued edits collided —
is #479's to design on top of this per-write contract; this ADR only fixes that the transport
contract is N independent conditional writes, not one bulk overwrite.

### CardDAV: unchanged, and deliberately the opposite of REST

CardDAV keeps detect-and-surface: full overwrite on remote change, every discarded local field
recorded in `contact_sync_conflicts`, restore offered. It is **not** reconciled with the REST policy
because the two are solving different problems — CardDAV's wire model is whole-object replacement with
no per-field precondition, so "reject the sync" is not a usable option the way "reject one REST PUT"
is. The REST policy is *not* copied from here (T13); it is the reverse: REST rejects and keeps the
stored row, CardDAV overwrites and records what it displaced.

## Consequences

- Concurrent modification of **disjoint** fields on one entity still produces a `412` for the second
  writer — any concurrent write bumps the revision, and the model is row-level, not field-level. The
  second client re-reads (now seeing the first client's change) and re-applies its own; the net
  result is both changes, applied in sequence by clients that each saw the other's. This is the
  documented outcome, tested.
- Concurrent modification of the **same** field: identical mechanism, `412`, and the user chooses.
- A repeatable-field conflict never silently drops an entry, because the losing write is rejected
  whole rather than merged.
- A concurrent relationship-edge update cannot invert direction; there is no code path that writes a
  derived inverse.
- Nothing is auto-resolved, so the "automatically resolved conflicts must be visible" requirement is
  met vacuously on the REST surface — there is nothing to surface. If a future ticket adds server-side
  union for repeatable fields, it must ship the disclosure surface in the same change; this ADR is
  the citation MAINT-02 uses to make that a breaking-change conversation.
- The CardDAV `contact_sync_conflicts` detect-and-surface path is untouched and still the way
  overwrites are disclosed on that surface.
