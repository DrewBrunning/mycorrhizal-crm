# T37 — Contact created via pet relationship should default to animal kind

| | |
|---|---|
| **Rating** | 2 — small, narrow, but a real correctness gap |
| **Size** | S |
| **Depends on** | §3d `RelationshipEdge` (done), [T27](20-T27-crm-kind-ui.md) (done — `CRM.Kind` UI) |
| **Alpha** | n/a — real data exists. Backend logic change only, no schema change |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

`RelationshipEdgeDialog.tsx`'s "enter manually" path creates a new thin `Contact` via
`source_thin: { name, gender, birthday }` — no `kind`. When the relationship type is the pet
edge (`owned_by`/`owns`, `models/relationship_type_registry.go` — "Fork-invented (pets, §90 D3's
thin-entity graph invariant)"), the newly-created contact is a pet, but nothing sets
`Contact.CRM.Kind = "animal"` on it. The household-suggestion engine
(`services.household_service.go`'s `classifyMember`) already treats `CRM.Kind == "animal"` as
authoritative for pet classification — so a pet contact created this way silently gets treated as
a human adult by every kind-aware feature (household suggestions, and anything else T27 exposed)
until someone manually fixes its kind after the fact.

## What to build

When resolving a thin-contact endpoint (`resolveRelationshipEndpoint` in
`relationship_edge_controller.go`) for an edge whose type is `owned_by` (source is the pet) or
`owns` (target would be — but that direction implies the *existing* viewed contact owns a
newly-created pet as the *source*, so check which side is actually being created before assuming
which one is the pet), set the newly-created `Contact.CRM.Kind = "animal"` on the pet side.

Concretely: the pet is always the `owned_by` edge's **source** (per the registry: `owned_by`'s
semantics are "source is owned by target"). So: if the edge type being created is `owned_by` and
the **source** is a thin contact being created, default that contact's `CRM.Kind` to `"animal"`.
The reverse creation path (creating the *owner* as a thin contact on an `owned_by` edge) should
not set `animal` — the owner is presumably human.

## Traps

- Use `ApplyRecordToContact` to set `CRM.Kind`, not a direct struct field mutation before
  `Create` — `CLAUDE.md` backend trap 2. This has shipped broken before via direct mutation.
- Don't hardcode the check to the literal string `"owned_by"` without also considering
  `getEffectiveType`/`toBackendType`'s inversion logic on the frontend side already normalizes
  direction before it reaches the API — verify what the backend actually receives once the
  frontend has resolved `viewedIsSource`, so the direction check is against the right field.
- This should not touch the **linked** entry mode (`entryMode === 'linked'`) — that path selects
  an existing contact, whose kind is already whatever it already is.
- CRM.Kind is presumably not the same enum position as vCard's `Card.Kind` (individual/group/org/
  etc., added in T29 WP13) — confirm which field `classifyMember` actually reads
  (`Contact.CRM.Kind`) and set that one, not `Card.Kind`.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green, with a controller test
  asserting a thin contact created via an `owned_by` edge has `CRM.Kind == "animal"`, and a
  thin contact created via any other relationship type does not.
- Hand-verified: from a contact's page, add a relationship of type "pet" (or whatever the
  frontend renders `owned_by`'s synonym as) entering a new name manually, confirm the created
  contact shows as an animal (T27's UI) without manual correction.

## Landing note (2026-08-07)

Landed. `resolveRelationshipEndpoint` now defaults the pet side of an `owned_by`/`owns` pair
to `CRM.Kind = "animal"` through `ApplyRecordToContact` (never a direct field mutation —
`BeforeSave` would re-derive and discard it). The full pet/owner matrix (owned_by source,
owns target, and both owner-side negatives) plus a non-pet control is pinned by
`TestThinContactPetKind_RealMigratedSchema` against the real migrated schema, and an e2e
spec creates a pet from a contact's page and confirms the animal badge renders.
