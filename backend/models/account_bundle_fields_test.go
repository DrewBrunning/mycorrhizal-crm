package models

import (
	"reflect"
	"sort"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/require"
)

// Field-level bundle completeness (issue #1318). The model-level net in
// account_bundle_completeness_test.go only notices a whole new *model*; a new
// column on Note, Activity, Reminder, ... would never be noticed there and a
// round trip would silently lose it. This test closes that gap: for every
// persisted column of every model the bundle carries, the column must either
//
//   - map to a field of that model's bundle DTO (same Go name, or an explicit
//     rename in `renamed`), or
//   - appear in `bundleFieldExcluded` with a reason.
//
// Adding a column to models.Note without touching the bundle therefore fails
// here until the author decides: carry it (add the DTO field, the export
// line, the import mapping) or exclude it with a written reason.

// bundleFieldDTO describes how one model's columns map onto its bundle DTO.
type bundleFieldDTO struct {
	dto any
	// renamed maps a model field name to the differently-named DTO field that
	// carries it (a uint FK becomes a stable-ID string, for example).
	renamed map[string]string
}

// Common reasons, so the per-model exclusion table stays readable.
const (
	reasonPK        = "uint row PK; the bundle carries the entity's stable portable ID instead"
	reasonTimestamp = "row timestamp regenerated on the destination; the bundle carries user-authored content, not row bookkeeping"
	reasonSoftDel   = "soft-delete marker; deleted rows are never exported"
	reasonUser      = "user scope; the destination assigns its own user"
	reasonRevision  = "per-instance write counter/ETag derived on the destination (ADR 0006)"
)

// bundleFieldMap lists, per bundle-covered model, the DTO that carries it.
var bundleFieldMap = map[string]bundleFieldDTO{
	"Activity": {dto: AccountBundleActivity{}},
	"Attachment": {dto: AccountBundleAttachment{}, renamed: map[string]string{
		"ContactVCardUID": "ContactVCardUID",
	}},
	"CadencePolicy": {dto: AccountBundleCadencePolicy{}},
	"Circle":        {dto: AccountBundleCircle{}},
	"Contact": {dto: AccountBundleContact{}, renamed: map[string]string{
		"VCardUID": "UID", "Card": "Card", "CRM": "CRM", "Passthrough": "Passthrough",
	}},
	"ConversationAgenda": {dto: AccountBundleAgendaItem{}, renamed: map[string]string{"ActivityID": "ActivityUUID"}},
	"DataDecayPolicy":    {dto: AccountBundleDataDecayPolicy{}},
	"FieldDefinition":    {dto: AccountBundleFieldDefinition{}},
	"FieldValue":         {dto: AccountBundleFieldValue{}},
	"Gift":               {dto: AccountBundleGift{}, renamed: map[string]string{"ActivityID": "ActivityUUID"}},
	"Household":          {dto: AccountBundleHousehold{}},
	"HouseholdMember":    {dto: AccountBundleHouseholdMember{}},
	"LifeEvent":          {dto: AccountBundleLifeEvent{}},
	"Note": {dto: AccountBundleNote{}, renamed: map[string]string{
		"ContactID": "ContactUID",
	}},
	"OccasionEvent":         {dto: AccountBundleOccasionEvent{}},
	"OccasionEventAttendee": {dto: AccountBundleAttendee{}},
	"OccasionObligation":    {dto: AccountBundleOccasion{}},
	"Preference":            {dto: AccountBundlePreference{}},
	"RelationshipEdge":      {dto: AccountBundleRelationship{}},
	"Reminder": {dto: AccountBundleReminder{}, renamed: map[string]string{
		"ContactID": "ContactUID",
	}},
	"ReminderCompletion": {dto: AccountBundleReminderCompletion{}, renamed: map[string]string{
		"ReminderID": "ReminderUUID", "ContactID": "ContactUID",
	}},
	"Tag": {dto: AccountBundleTag{}},
	// Pure join rows: their whole content is the pair of stable IDs that the
	// parent DTO's member/contact list carries (member_uids / contact_uids).
	"CircleMember": {dto: AccountBundleCircle{}, renamed: map[string]string{"MemberVCardUID": "MemberUIDs", "CircleID": "ID"}},
	"ContactTag":   {dto: AccountBundleTag{}, renamed: map[string]string{"ContactVCardUID": "ContactUIDs", "TagID": "ID"}},
}

// bundleUniversalExcluded are row-bookkeeping columns no DTO carries, for any
// model (a DTO that does carry a same-named field, e.g. Attachment.CreatedAt,
// simply matches first).
var bundleUniversalExcluded = map[string]string{
	"CreatedAt": reasonTimestamp,
	"UpdatedAt": reasonTimestamp,
	"DeletedAt": reasonSoftDel,
	"UserID":    reasonUser,
}

// bundleFieldExcluded maps model -> column (Go field name) -> the reason the
// bundle deliberately does not carry it. Each entry is a decision.
var bundleFieldExcluded = map[string]map[string]string{
	"Activity": {
		"ID": reasonPK, "Revision": reasonRevision, "ETag": reasonRevision,
	},
	"Attachment": {
		"StoredName": "server-generated on-disk filename; attachment bytes are not embedded in v1 (AccountBundleOmissions.Attachments)",
	},
	"Contact": {
		"ID":         reasonPK,
		"Revision":   reasonRevision,
		"VCardExtra": "legacy unmapped-vCard-property blob; RecordForContact folds it into Passthrough, which the bundle carries",
		"Firstname":  reasonFlatDerived, "Lastname": reasonFlatDerived, "MiddleName": reasonFlatDerived,
		"Prefix": reasonFlatDerived, "Suffix": reasonFlatDerived, "Nickname": reasonFlatDerived,
		"FN": reasonFlatDerived, "SortName": reasonFlatDerived,
		"Email": reasonFlatDerived, "Emails": reasonFlatDerived,
		"Phone": reasonFlatDerived, "Phones": reasonFlatDerived, "PhonesNormalized": reasonFlatDerived,
		"Address": reasonFlatDerived, "Addresses": reasonFlatDerived, "AddressesFlat": reasonFlatDerived,
		"URLs": reasonFlatDerived, "IMPPs": reasonFlatDerived,
		"Birthday": reasonFlatDerived, "Anniversary": reasonFlatDerived,
		"Organization": reasonFlatDerived, "Org": reasonFlatDerived, "Department": reasonFlatDerived,
		"JobTitle": reasonFlatDerived, "Role": reasonFlatDerived,
		"HowWeMet": reasonFlatEnvelope, "WorkInformation": reasonFlatEnvelope,
		"ContactInformation": reasonFlatEnvelope, "Circles": reasonFlatEnvelope,
		"Photo":          reasonFlatPhoto,
		"PhotoThumbnail": reasonFlatPhoto,
	},
	"FieldValue":            {"ID": reasonPK},
	"HouseholdMember":       {"ID": reasonPK, "HouseholdID": "parent link; the member sits inside its household's members[] list"},
	"OccasionEventAttendee": {"ID": reasonPK, "EventID": "parent link; the attendee sits inside its event's attendees[] list"},
	"LifeEvent":             {"Revision": reasonRevision, "ETag": reasonRevision},
	"Note":                  {"ID": reasonPK, "Revision": reasonRevision, "ETag": reasonRevision},
	"Reminder": {
		"ID": reasonPK, "Revision": reasonRevision, "ETag": reasonRevision,
		"EmailSent": "delivery bookkeeping for the current occurrence; the destination re-derives it when it next sends",
	},
	"ReminderCompletion": {"ID": reasonPK},
}

const (
	reasonFlatDerived  = "denormalized flat column derived from the Card (BeforeSave / ApplyRecordToContact); the bundle carries the Card"
	reasonFlatEnvelope = "flat mirror of a CRM-envelope field; the bundle carries the envelope (crm)"
	reasonFlatPhoto    = "flat photo column; the bundle carries the photo as a data: Card.Media entry and the import re-persists it (issue #1308)"
)

// TestAccountBundleFieldsAreCarriedOrExcluded is the field-level net.
func TestAccountBundleFieldsAreCarriedOrExcluded(t *testing.T) {
	db := dbtest.New(t)

	byName := map[string]any{}
	for _, m := range registeredModels {
		byName[parseModel(t, db, m).Name] = m
	}

	names := make([]string, 0, len(bundleFieldMap))
	for n := range bundleFieldMap {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		spec := bundleFieldMap[name]
		model, ok := byName[name]
		require.True(t, ok, "bundleFieldMap entry %q is not a registered model; remove it", name)
		require.Contains(t, bundleCovered, name, "bundleFieldMap entry %q must also be in bundleCovered", name)

		dtoType := reflect.TypeOf(spec.dto)
		dtoFields := map[string]bool{}
		for i := 0; i < dtoType.NumField(); i++ {
			dtoFields[dtoType.Field(i).Name] = true
		}

		s := parseModel(t, db, model)
		modelFields := map[string]bool{}
		for _, f := range s.Fields {
			if f.DBName == "" || f.IgnoreMigration {
				continue
			}
			modelFields[f.Name] = true
			if _, isExcluded := bundleFieldExcluded[name][f.Name]; isExcluded {
				continue
			}
			target := f.Name
			if renamed, ok := spec.renamed[f.Name]; ok {
				target = renamed
			}
			if _, universal := bundleUniversalExcluded[f.Name]; universal && !dtoFields[target] {
				continue
			}
			if !dtoFields[target] {
				t.Errorf("%s.%s (column %s) is neither carried by %s (looked for field %q) nor listed in "+
					"bundleFieldExcluded[%q]; carry it in the bundle (DTO + export + import mapping) or exclude it with a reason",
					name, f.Name, f.DBName, dtoType.Name(), target, name)
			}
		}

		// Anti-rot: every rename/exclusion names a real field, and an
		// exclusion is not also carried (a carried field needs no excuse).
		for field := range spec.renamed {
			require.True(t, modelFields[field], "bundleFieldMap[%q].renamed names %q, which is not a column of the model", name, field)
			require.True(t, dtoFields[spec.renamed[field]], "bundleFieldMap[%q].renamed[%q] -> %q is not a field of %s", name, field, spec.renamed[field], dtoType.Name())
		}
		for field, reason := range bundleFieldExcluded[name] {
			require.True(t, modelFields[field], "bundleFieldExcluded[%q] names %q, which is not a column of the model; remove it", name, field)
			require.NotEmpty(t, reason, "bundleFieldExcluded[%q][%q] needs a reason", name, field)
			if !modelFieldRenamed(spec, field) {
				require.False(t, dtoFields[field], "%s.%s is excluded but %s carries a field of that name; drop the exclusion", name, field, dtoType.Name())
			}
		}
	}

	// Every model the bundle covers with a DTO must be described here, so a
	// newly covered model cannot skip the field-level check.
	for name := range bundleCovered {
		_, ok := bundleFieldMap[name]
		require.True(t, ok, "bundleCovered model %q has no bundleFieldMap entry; describe its DTO", name)
	}
}

func modelFieldRenamed(spec bundleFieldDTO, field string) bool {
	_, ok := spec.renamed[field]
	return ok
}
