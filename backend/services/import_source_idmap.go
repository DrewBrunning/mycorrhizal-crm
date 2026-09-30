package services

import (
	"strings"

	"github.com/rs/zerolog/log"
)

// Issue #1355: the source-import engine never inserts a UUID-PK row under its
// source's primary key. Those keys are globally unique, so re-using them
// collided (and silently dropped ~12 entity types) whenever a bundle was
// imported into a second account on the same instance. Instead every row mints
// a fresh ID (the models' BeforeCreate) and this map records
// source ExternalID -> local ID so intra-bundle references (an occasion's
// linked life event, a gift's life event, a reminder's life event/occasion, a
// custom-field value's definition) are rewritten to the rows that were actually
// created in *this* account. A reference that does not resolve is dropped and
// reported -- it is never left pointing at the source's ID, which could be
// another account's row.

// Source-kind prefixes of the bundle's ExternalIDs ("<kind>/<sourceID>"; see
// bundleRef). They are the namespace importIDMap keys live in.
const (
	sourceKindLifeEvent       = "life_event"
	sourceKindOccasion        = "occasion"
	sourceKindFieldDefinition = "field_definition"
)

// importIDMap maps a source row's ExternalID to the local primary key it landed
// as.
type importIDMap map[string]string

// remember records that the source row ref landed as localID.
func (m importIDMap) remember(ref SourceRef, localID string) {
	m[ref.ExternalID] = localID
}

// resolve returns the local ID for the source row of the given kind and
// source ID.
func (m importIDMap) resolve(kind, sourceID string) (string, bool) {
	if sourceID == "" {
		return "", false
	}
	id, ok := m[kind+"/"+sourceID]
	return id, ok
}

// resolveOrReport resolves a reference or, when it does not resolve (the
// referenced row failed to import, or was never in the plan), reports a lossy
// issue and returns "" so the referencing row lands without the dangling link.
func (m importIDMap) resolveOrReport(report *ImportReport, record, field, kind, sourceID string) string {
	if id, ok := m.resolve(kind, sourceID); ok {
		return id
	}
	report.appendIssue(ImportIssue{
		Record:   record,
		Field:    field,
		Category: ImportIssueCategoryLossy,
		Message:  "references a " + strings.ReplaceAll(kind, "_", " ") + " that was not imported; the link was dropped",
	})
	return ""
}

// remapRelatedEntityIDs rewrites a life event's related contact UIDs to the
// UIDs those contacts landed under. A UID that resolves to no imported contact
// is dropped (and reported) rather than kept raw.
func remapRelatedEntityIDs(report *ImportReport, record string, related []string, refToUID map[string]string) []string {
	if len(related) == 0 {
		return related
	}
	out := make([]string, 0, len(related))
	for _, uid := range related {
		if local, ok := refToUID[bundleContactRef(uid).ExternalID]; ok {
			out = append(out, local)
			continue
		}
		report.appendIssue(ImportIssue{
			Record:   record,
			Field:    "life_event.related_entity_ids",
			Category: ImportIssueCategoryLossy,
			Message:  "references a contact that was not imported; the link was dropped",
		})
	}
	return out
}

// importWriteFailureMessage is the only text a user sees when a row cannot be
// written; the driver's error (SQL, constraint and table names) is logged, not
// surfaced.
const importWriteFailureMessage = "could not be saved"

// writeFailureIssue logs err and returns the user-facing issue for a row whose
// insert/update failed.
func writeFailureIssue(record, field string, err error) ImportIssue {
	log.Error().Err(err).Str("record", record).Str("field", field).Msg("import: row could not be saved")
	return ImportIssue{Record: record, Field: field, Category: ImportIssueCategoryInvalid, Message: importWriteFailureMessage}
}
