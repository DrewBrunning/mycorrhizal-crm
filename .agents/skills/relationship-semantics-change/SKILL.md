---
name: relationship-semantics-change
description: Use when changing relationship edge creation, traversal, labels, inverse derivation, household inference, or relationship queries.
license: MIT
---

# Relationship semantics change

Outcome: a relationship change that preserves one stored direction, a derived
inverse, and the confirmed-only rule everywhere an edge is read. Read
`backend/AGENTS.md` (Domain notes), `docs/adrs/0001-neutral-hub-and-spoke-contact-model.md`
and the [semantics reference](references/semantics.md) before editing.

## Steps

1. Decide what is stored: one edge direction from source to target. The inverse
   is derived from `backend/models/relationship_type_registry.go`, never stored
   twice.
2. Add or change the registry entry, not a second token list. The registry drives
   validation, inverse derivation and the vCard RELATED projection.
3. Keep `status: confirmed` as the only fact. `suggested` edges are review
   material and are never projected, graphed or treated as real.
4. Household inference reads current memberships only; it is an inference, not a
   stored relationship. Check the household paths you touched.
5. If traversal or projection changes, update the graph ceilings in
   `frontend/src/utils/graphBudget.ts` and its test in
   `frontend/src/utils/graphBudget.test.ts`.
6. If deletion reaches edges, register the cascade in
   `backend/services/contact_delete.go` (ADR 0035) and assert it with
   `Unscoped()` where hard-deleted rows must be distinguished.
7. Cover the new access path's ownership boundary; `backend/routes/ownership_matrix_test.go`
   is the cross-route tripwire.

## DoD

```bash
cd backend && go test ./models/... ./services/... ./controllers/...
cd frontend && npx vitest run src/utils/graphBudget.test.ts
```

Report any check you could not run; never imply it passed.

## Stop and ask

- The change would store both directions or infer the inverse from a label.
- An edge type has no RFC 6350 home and the proposal is to invent one.
- A query would traverse `suggested` or above-`normal` sensitivity.
- The change alters the health-score or household rules without an ADR update.

Canonical detail: `docs/adrs/0023-relationship-health-score.md`,
`backend/models/relationship_type_registry.go`, `backend/services/graph_traversal.go`.
