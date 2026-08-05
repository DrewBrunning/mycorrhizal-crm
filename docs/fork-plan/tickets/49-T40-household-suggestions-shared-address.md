# T40 — Suggest new households from contacts sharing an address

| | |
|---|---|
| **Rating** | 3 — genuinely useful, but only once a user has enough address data entered |
| **Size** | M |
| **Depends on** | [T1](09-T1-households.md) (done — Household CRUD + suggestion review UI) |
| **Alpha** | n/a — real data exists. Read-only detection + a new suggestion surface; no schema change |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

`services.GenerateHouseholdSuggestions` (built for T1) only works **within** an existing
`Household`'s current membership — it proposes `RelationshipEdge`s (spouse_of, parent_of,
owned_by) among people already grouped together. It has no path for the other direction: contacts
who share an address but **aren't in a household together at all**. Confirmed by reading
`household_service.go` — it loads `HouseholdMember` rows for a given household ID and never scans
`Contact` addresses independently. Given the confirmed intent — detect contacts sharing an address
who aren't already co-members of a household, and surface a suggestion to create one — this is
new detection logic, not an extension of the existing suggestion engine's scope.

## Rejection persistence — decision locked 2026-08-04

Investigated the existing `RelationshipEdge` rejection path (`relationship_edge_controller.go:275-299`):
rejection hard-deletes the row, and `suggestEdgeIfNew` (`household_service.go:160-191`) checks only for
currently-existing edges — it has no memory of previously-deleted ones. There is no dismissed/rejected
persistence table anywhere in the codebase. **No existing pattern to copy — a new mechanism is needed.**

**Decision: new `dismissed_household_suggestions` join table.** Columns: `user_id` (uint), `address_hash`
(string — SHA-256 of the normalized address), `member_hash` (string — SHA-256 of sorted member VCardUIDs
joined), `dismissed_at` (timestamp). **Hard-delete** per T26 (join row with a natural-key composite
unique index on `(user_id, address_hash, member_hash)`). The detection query excludes any group whose
hash triple is in this table. No separate "clear all rejected" affordance needed in the first pass —
rejection is permanent and deterministic (same addresses always produce the same group, so rejecting
once is rejecting forever). If a user wants to re-offer a group, they can create the household manually;
if this proves annoying, a "clear rejected suggestions" button can be a follow-up.

## What to build

- A detection pass (on-demand, from the Households page — following the same "explicit trigger,
  not a background job" pattern `GenerateHouseholdSuggestions` itself uses) that groups the
  current user's contacts by matching address (a reasonable normalization: compare street +
  city/region + postal, not full string equality — trailing whitespace/casing/abbreviation
  differences will otherwise produce false negatives) and finds groups of 2+ contacts:
  - who share a matching address, **and**
  - who are not already co-members of any existing `Household`.
- Surface each such group as a suggestion on the Households page — reuse
  `HouseholdsPage.tsx`/`RelationshipEdgeList`'s existing suggested-item review pattern (accept →
  create the `Household` + `HouseholdMember` rows for the group; reject → dismiss, don't re-offer
  the same group again this session at minimum, ideally persistently).
- This is address-**matching**, not household-suggestion-generation in the WP-83 sense — it does
  not need to also generate `RelationshipEdge`s. Once the household is created from an accepted
  suggestion, the *existing* `GenerateHouseholdSuggestions` (triggered separately, already wired
  by T1) takes over for relationship suggestions within it — don't duplicate that logic here.

## Traps

- **Multiple households can legitimately share nothing in common except an address typo** — don't
  auto-create anything; this is propose-then-approve only, consistent with every other suggestion
  surface in this codebase (`92.7`'s standing requirement, and the precedent T1 itself set).
- **Address matching needs a defined normalization**, not naive string equality — decide and
  document it (e.g. lowercase, strip punctuation, compare street+city+postal, ignore unit/
  apartment sub-components) since T29's address work means addresses now carry many optional
  sub-components that could differ trivially between two otherwise-identical addresses.
- **Sensitivity/ownership scoping** — this only ever compares contacts within one user's own data;
  make sure the query is `user_id`-scoped like everything else, not global.
- **Rejecting a suggestion** uses the `dismissed_household_suggestions` table (see "Rejection
  persistence" above). The detection query must exclude any group whose hash triple is already in
  this table. `DELETE` is hard-delete per T26 — a join table with a natural-key composite unique
  index.
- **Archived contacts** — respect `include_archived` the way `GetContacts` already does; an
  archived contact sharing an address shouldn't generate a suggestion.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green, with a real-DB test
  covering: two contacts sharing a normalized-equal address (not already co-members) produce a
  suggestion; two contacts already in the same household do not; a rejected suggestion is not
  re-offered on a subsequent detection pass (dismissal table hit); re-running detection with the
  same data produces no new suggestions for an already-dismissed group; cross-user contacts never
  match; the `dismissed_household_suggestions` table's unique index prevents duplicate dismissals.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified: two contacts with the same address, not in any household, produce a visible
  suggestion on the Households page; accepting it creates a real `Household` with both as members.
