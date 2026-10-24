# Contact data flow

Synthesis only; `docs/data-model.md` and ADR 0001 are canonical.

## Hops

1. **Input** — REST JSON, vCard 3/4, JSContact, CSV, or an integration import.
   Sanitization happens here; never trust a field's type or size.
2. **Neutral model** — `backend/models/contact_record.go` builds a
   `models.Card`. `ApplyRecordToContact` writes it through `BeforeSave`, which
   derives the flat denormalized columns. Direct field mutation skips that.
3. **Persistence** — flat `contacts` columns plus child rows (emails, phones,
   addresses, tags, field values). Anything with no flat home lives only in the
   serialized card.
4. **Read-back** — `RecordForContact` reads what is actually persisted,
   including `SpeakToAs`, `PersonalInfo`, projections and merge results.
   `RecordFromContact` rebuilds from flat fields and silently drops the rest.
5. **Merge** — `backend/models/contact_card_merge.go` merges an incoming card
   with the stored one. `mergeMedia` deliberately keeps a remote `http(s)` photo
   URI when the flat photo is empty (ADR 0012 INV-D8); a `data:` URI with no flat
   photo is a real delete.
6. **Projection** — `backend/models/field_selection.go` and the relationship
   projection in `contact_record.go` decide what a neutral `Card` exposes.
7. **Export** — `backend/vcard3`, `backend/vcard4` and `backend/jscontact`
   consume the same neutral `Card`. Filtering the record before the exporter
   applies to all three at once.

## Sensitivity

`normal|private|secret` is filtered **in the query** for external sync, contact
shares and neutral-`Card` exports, and re-includable only with
`?include_sensitive=true`. The CSV export, account bundle and contact map are
deliberate full-fidelity exceptions; do not "fix" them.

## Where the tests live

- Neutral model and projections: `backend/models/contact_record_reverse_test.go`,
  `backend/models/contact_record_reverse_test.go`.
- Merge and media: `backend/models/contact_card_merge_test.go`.
- Cross-format oracle: `backend/correspondence/`, plus
  `backend/internal/rfctest/` and `docs/golden-fixtures/`.
- Exporters: `backend/vcard3/`, `backend/vcard4/`, `backend/jscontact/`.

Canonical: `docs/data-model.md`, `docs/adrs/0001-neutral-hub-and-spoke-contact-model.md`,
`docs/adrs/0012-canonical-database-invariants.md`.
