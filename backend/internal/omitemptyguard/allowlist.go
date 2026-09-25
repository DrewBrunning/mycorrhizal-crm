package omitemptyguard

// Allowlist records every slice/map struct field in backend/models and
// backend/controllers that carries `omitempty` for a reason CLAUDE.md
// frontend trap #8 does not apply to. Keyed by "StructName.FieldName" (both
// packages are flat, single-directory packages in this repo, so the key is
// unambiguous). Each entry's value is the reason itself — a blank or
// missing reason does not exempt the field (see ScanFile).
//
// Categories, matching this WP's own allowlist guidance:
//   - request-only DTO: the client sends this field; a server response
//     never needs to distinguish absent from empty for it.
//   - deliberately absent unless requested: an `includes=`/`?include=`-style
//     opt-in, where "the client didn't ask" and "there was nothing to
//     return" are two different, both-legitimate reasons for absence.
//   - persisted column, not a Find-populated association: a single
//     record's own optional JSON-blob column (read back with the row that
//     owns it), never a has-many slice a separate Preload query can leave
//     nil while the rest of the struct is fully populated.
//   - internal parsing type: never marshaled into an HTTP response body at
//     all (a legacy on-disk blob's best-effort deserialization shape).
//   - deliberate absent-vs-empty semantic: the doc comment on the field
//     itself documents that absent and empty mean two different things,
//     and the frontend TypeScript type already mirrors that with an
//     optional (`field?:`) declaration.
var Allowlist = map[string]string{
	// Contact is the flat GORM model reused for persistence and several
	// distinct historical response shapes with no single owning
	// constructor. Preload("Notes")/Preload("Activities")/Preload("Reminders")
	// is not run on most of its serialization paths (GetContactsRandom,
	// Archive/UnarchiveContact) -- these fields are simply never populated
	// there, by design, not hidden after a Find left them nil. The two
	// shapes that DO preload-all (the detail view) already have their own
	// derived response DTOs -- ContactSummaryWithRelations and
	// ContactRecordResponse (models/contact_summary.go) -- which drop
	// omitempty and normalize nil to `[]` directly.
	"Contact.Activities": "GORM model reused across many response shapes without a single owning constructor; not Preloaded in most serialization paths (GetContactsRandom, Archive/UnarchiveContact); the preload-guaranteed detail-view shapes have their own fixed derived DTOs (ContactSummaryWithRelations/ContactRecordResponse)",
	"Contact.Notes":      "see Contact.Activities",
	"Contact.Reminders":  "see Contact.Activities",

	// Activity.Contacts is deliberately populated only when the caller asks
	// for it (GetActivities' `?include=contacts` query param, checked before
	// the Preload runs) -- "the client didn't ask" and "no contacts linked"
	// are both real, distinct reasons for absence.
	"Activity.Contacts": "deliberately absent unless requested via GetActivities' ?include=contacts query param, not a Find-then-hidden collection",

	// legacyVCardExtra/legacyVCardField are the on-disk shape of a legacy
	// carddav.VCardExtra blob, deserialized read-only for best-effort
	// passthrough migration (models/contact_record.go's buildPassthrough).
	// Neither type is ever marshaled into an HTTP response.
	"legacyVCardExtra.Properties": "internal parsing type for a legacy on-disk blob, deserialized read-only; never marshaled into an HTTP response",
	"legacyVCardField.Params":     "see legacyVCardExtra.Properties",

	// Request-only DTOs: the client sends these; nothing decodes them back
	// out of a response.
	"LifeEventInput.RelatedEntityIDs":    "request-only DTO (POST/PUT life-event body); never returned in a response",
	"RelationshipEdgeInput.Metadata":     "request-only DTO (POST/PUT relationship-edge body); never returned in a response",
	"ContactMergeRequest.Resolutions":    "request-only DTO (POST /contacts/merge body); never returned in a response",
	"ExternalIdentityInput.Metadata":     "request-only DTO (POST/PUT external-identity body); never returned in a response",
	"ImportConfirmRequest.FieldMappings": "request-only DTO -- the client's issue #514 custom-field promotion decisions, submitted at confirm; never returned in a response",
	"CadencePolicyInput.QualifyingTypes": "request-only DTO (POST/PUT cadence-policy body); never returned in a response",
	"ExternalActivityInput.Payload":      "request-only DTO (POST/PUT external-activity body); never returned in a response",

	// CadencePolicy.QualifyingTypes / LifeEvent.RelatedEntityIDs are a
	// single record's own persisted JSON-array column (`serializer:json`),
	// read back with the row that owns it -- never a has-many slice a
	// separate Preload query populates or leaves nil.
	"CadencePolicy.QualifyingTypes": "persisted column on the record's own row (serializer:json), not a Find-populated has-many association",
	"LifeEvent.RelatedEntityIDs":    "see CadencePolicy.QualifyingTypes",

	// Free-form JSON metadata/payload blob columns: each is a single
	// record's own optional column, not an association.
	"ExternalIdentity.Metadata": "persisted free-form metadata column on the record's own row, not a Find-populated has-many association",
	"RelationshipEdge.Metadata": "see ExternalIdentity.Metadata",
	"ExternalActivity.Payload":  "see ExternalIdentity.Metadata",

	// FieldConstraints.Values is genuinely absent for every FieldType other
	// than the enum type (models/field_definition.go's own doc comment:
	// "the allowed-value list for FieldTypeEnum") -- absence is meaningful
	// (not an enum field), not an artifact of an unpopulated association.
	"FieldConstraints.Values": "genuinely absent for every non-enum FieldType (see the field's own doc comment); absence is meaningful, not a Find-then-hidden collection",

	// ImportRowPreview's Diagnostics/CustomFieldCandidates document their
	// own absent-vs-empty distinction ("Absent (nil) for CSV-import rows...
	// Matches the Diagnostics field's omitempty convention: an absent key
	// means 'no candidates'") and the frontend already mirrors it with
	// optional fields (frontend/src/api/import.ts: `diagnostics?:` /
	// `custom_field_candidates?:`), not required ones.
	"ImportRowPreview.Diagnostics":           "documented absent-vs-empty semantic (see the field's own doc comment); frontend/src/api/import.ts already declares it optional (`diagnostics?:`), not required",
	"ImportRowPreview.CustomFieldCandidates": "documented absent-vs-empty semantic (see the field's own doc comment); frontend/src/api/import.ts already declares it optional (`custom_field_candidates?:`), not required",
}
