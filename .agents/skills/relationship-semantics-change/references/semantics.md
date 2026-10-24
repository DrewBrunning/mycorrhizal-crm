# Relationship semantics

Synthesis only; the ADRs are canonical.

## Stored direction vs derived inverse

A `RelationshipEdge` stores one direction: source → target. `Type` describes the
source's role relative to the target (`parent_of` from A to B means A is B's
parent). The inverse (`child_of`) is derived by
`backend/models/relationship_type_registry.go`; it is never persisted.

Registry rows carry the reciprocal token, whether the relation is symmetric, and
the RFC 6350 §6.6.6 TYPE token (empty when the relation has no standard home and
must stay internal). Extend that map; validation, projection and traversal read
it.

## Status and sensitivity

Only `status: confirmed` edges are fact. `suggested` edges must never be
projected, graphed, or treated as real outside a review surface. Above-`normal`
sensitivity is filtered in the query everywhere a copy leaves the instance.

## Household inference

Household membership is a join row, not an edge. When the self-contact and a
contact are both current members of the same household, the relationship-health
reason is `household`; the inference must not create edges or scores that the
stored data does not support.

## Projection and traversal

- `RecordForContact` projects confirmed, non-secret edges onto `Card.RelatedTo`
  for both sides (or one for asymmetric tokens).
- `backend/services/graph_traversal.go` filters before de-duplicating; a
  suggested/secret edge must not become a hop.
- The browser graph is budgeted by `frontend/src/utils/graphBudget.ts`; changing
  traversal ceilings is a frontend-visible change.

## Deletion

Edges and join rows are edge-shaped and hard-delete (backend trap #7). The
contact cascade registry (`backend/services/contact_delete.go`, ADR 0035) must
name every table that references a contact; a new relationship-shaped table
fails the completeness test until registered.

## Tests

- Inverse and registry consistency: `backend/models/relationship_edge_test.go`.
- Confirmed-only projection: same file, `TestRecordForContact_DoesNotProjectSuggestedEdge`.
- Traversal: `backend/services/graph_traversal_test.go`.
- Ownership across routes: `backend/routes/ownership_matrix_test.go`.
- Delete cascade: `backend/services/contact_delete_test.go`.

Canonical: `docs/adrs/0001-neutral-hub-and-spoke-contact-model.md`,
`docs/adrs/0023-relationship-health-score.md`.
