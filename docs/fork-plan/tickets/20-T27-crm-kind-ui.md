# T27 — Contact CRM.Kind UI (individual/pet/animal)

| | |
|---|---|
| **Rating** | 3 — household suggestions are broken without it |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | **before** — fixes a gap that silently breaks pet classification |
| **Source** | T22 pet-suggestion audit |

## Why this exists

`GenerateHouseholdSuggestions` (`services/household_service.go:30-38`) classifies household
members as pet vs. adult vs. child by reading `Contact.CRM.Kind` — if the value is `"pet"` or
`"animal"`, the member gets `classPet` and correctly receives `owned_by` edges rather than
`spouse_of` or `parent_of`. Without a value, the classifier falls through to `classAdult`.

**There is no UI to set `CRMEnvelope.Kind`.** The frontend's add-contact and edit-contact forms
(`AddContactDialog.tsx:218-222`, `ContactDetailPage.tsx`) only write `how_we_met`,
`work_information`, and `contact_information` to the CRM envelope. The `kind` field has existed
in the backend model (`contactmodel/envelope.go:6-14`) since the neutral model was introduced
but was never wired to any form.

## What to build

1. **Backend**: verify `CRMEnvelope.Kind` is included in the contact input DTO and properly
   round-tripped through `ApplyRecordToContact` (it should be already — confirm and add a
   test if missing).

2. **Frontend**: add a "Kind" dropdown to the contact create/edit form. Place it near the top
   of the contact form (in `AddContactDialog.tsx`'s initial fields and `ContactInformation.tsx`
   or `ContactHeader.tsx` for editing). Options:
   - `individual` (default — no special classification in the suggestion engine)
   - `pet` → sets `CRM.Kind = "pet"`
   - `animal` → sets `CRM.Kind = "animal"`

3. **Frontend**: include `kind` in `buildContactRecordInput()` when not empty.

## Done when

- A contact created/edited with Kind "pet" correctly receives `owned_by` suggestions (not
  `spouse_of` or `parent_of`) when added to a `family_unit` household.
- A contact with Kind "individual" (or no Kind set) behaves as before (default adult).
- `npx tsc --noEmit` clean, `npx vitest run` green.
- `cd backend && go build ./... && go vet ./... && go test ./...` green.
- Hand-verified: create a family household with two adults, one child, one pet — confirm
  the suggestion count and edge types match `household_service_test.go`'s expected counts.
