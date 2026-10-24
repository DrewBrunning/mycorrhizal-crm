# Fidelity test ownership

Use the lowest layer that can catch the failure. Do not add a second round-trip
suite in a new package when one of these already owns the data class.

| Data class | Surface | Owning test / package |
|---|---|---|
| Names, nicknames, speak-to-as | neutral model ↔ vCard | `backend/models/` + `backend/vcard3/`, `backend/vcard4/` |
| Emails, phones, addresses, URLs | all formats + dedup/merge | `backend/correspondence/`, `backend/models/contact_card_merge_test.go` |
| Personal info / custom fields | neutral card, projections | `backend/models/contact_record_reverse_test.go`, `backend/models/contact_record_custom_field_test.go` |
| Preferences, tags, groups | projections and export | `backend/models/contact_record_preference_test.go`, `backend/models/field_selection_test.go` |
| Photos and other media | persistence, merge, export | `backend/models/contact_card_merge_test.go`, `backend/internal/rfctest/` |
| Relationship projections | neutral card ↔ vCard RELATED | `backend/models/relationship_edge_test.go` |
| Sensitivity filtering | every leaving copy | `backend/models/contact_record_reverse_test.go`, export tests |
| CSV / account bundle / contact map | full-fidelity exceptions | `backend/controllers/`, `backend/correspondence/` |
| Golden RFC examples | external oracle | `docs/golden-fixtures/`, `backend/internal/rfctest/oracle_parity_test.go` |

## Rules

- A round trip is **semantically equal**, not byte-identical (TEST-03).
- A field that exists only in one representation is a fidelity bug unless the
  compatibility matrix records the deliberate loss.
- Fuzz or property tests belong beside the package they exercise, not in a
  cross-cutting suite.
- A new field needs both a persistence test (real migrated schema) and an
  interop test if it projects to any format.

Canonical: `docs/development/testing.md` (Import/export interop),
`docs/data-01-field-compatibility-matrix.md`.
