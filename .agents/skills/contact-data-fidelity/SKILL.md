---
name: contact-data-fidelity
description: Use when changing contact fields, the neutral Card model, vCard/JSContact/CSV conversion, sanitization, or loss handling.
license: MIT
---

# Contact data fidelity

Outcome: a contact change that survives every representation without silently
dropping data. Read `backend/AGENTS.md` (traps #2 and #3) and
`docs/adrs/0001-neutral-hub-and-spoke-contact-model.md` first.

The canonical flow is: request/import → neutral `models.Card` → flat
`contacts` columns → projections (`RecordForContact`) → exporters. The e2e
setup in `frontend/e2e/global-setup.ts` hardcodes `http://localhost:7300`; it is
test infrastructure, not a production default.

## Steps

1. Open [data-flow](references/data-flow.md) and identify every hop your field
   touches. A field with no flat-column home must still round-trip through
   `RecordForContact`; `RecordFromContact` rebuilds from flat fields and drops it.
2. Write through `ApplyRecordToContact` (never set `Contact.Card`/`CRM` directly).
   On update, merge through `backend/models/contact_card_merge.go`; preserve its
   `mergeMedia` remote-photo rule.
3. Decide sensitivity (`normal|private|secret`). Anything above `normal` is
   filtered in the query for sync/shares/neutral-`Card` exports; CSV export,
   account bundle, and contact map are the three deliberate full-fidelity
   exceptions.
4. Update the correspondence oracle (`backend/correspondence/testdata/correspondence.tsv`)
   when the field's cross-format mapping changes, then regenerate the DATA-01
   matrix with `go run ./cmd/gencompatmatrix`.
5. Open [fidelity test matrix](references/fidelity-test-matrix.md) to choose the
   owning test rather than duplicating round-trip assertions in a new package.
6. Keep the web and Android clients on the same generated contract; run the
   frontend type check even when you only touched Go.

## DoD

```bash
cd backend && go test ./correspondence/... ./vcard3/... ./vcard4/... ./jscontact/... ./internal/rfctest/...
cd backend && go run ./cmd/gencompatmatrix
cd frontend && npx tsc --noEmit
```

Report any check you could not run; never imply it passed.

## Stop and ask

- The field has no neutral-model or flat-column home and the proposal is to
  expose it only in one exporter.
- A conversion would be lossy in a way the compatibility matrix cannot express.
- The change would weaken the sensitivity filter on a leaving copy.

Canonical detail: `docs/data-model.md`, `backend/AGENTS.md` (Domain notes),
`docs/data-01-field-compatibility-matrix.md`.
