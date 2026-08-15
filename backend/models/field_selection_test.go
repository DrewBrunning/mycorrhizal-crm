package models

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// buildFullRecord returns a Record with every picker-covered section
// populated, plus the always-exported identity data, so tests can assert
// that ApplyFieldSelection clears exactly the deselected sections.
func buildFullRecord() *contactmodel.Record {
	record := &contactmodel.Record{
		UID:  "rec-uid",
		ETag: `"etag-1"`,
		Card: contactmodel.Card{
			UID:                "card-uid",
			Kind:               "individual",
			Name:               &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Alice"}}},
			Nicknames:          []contactmodel.Nickname{{Name: "Ally"}},
			Emails:             []contactmodel.Email{{Address: "alice@example.com"}},
			Phones:             []contactmodel.Phone{{Number: "555-0100"}},
			Addresses:          []contactmodel.Address{{Full: "123 Main St"}},
			Organizations:      []contactmodel.Organization{{Name: "Acme"}},
			Titles:             []contactmodel.Title{{Name: "Engineer", Kind: "title"}},
			Anniversaries:      []contactmodel.Anniversary{{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Month: intPtr(5), Day: intPtr(10)}}}},
			Media:              []contactmodel.Resource{{Kind: "photo", URI: "data:image/png;base64,abc"}},
			ImppAddresses:      []contactmodel.OnlineService{{Service: "telegram", URI: "tg:@alice"}},
			Links:              []contactmodel.Resource{{URI: "https://example.com"}},
			Notes:              []contactmodel.Note{{Note: "met at conference"}},
			Keywords:           []string{"vegan"},
			RelatedTo:          []contactmodel.Relation{{Target: "urn:uuid:bob", Relations: []string{"spouse"}}},
			PersonalInfo:       []contactmodel.PersonalInfo{{Kind: "hobby", Value: "climbing"}},
			SpeakToAs:          &contactmodel.SpeakToAs{Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}}},
			Members:            []string{"urn:uuid:m1"},
			PreferredLanguages: []contactmodel.LanguagePref{{Language: "en"}},
		},
		Envelope: contactmodel.CRMEnvelope{
			Kind:               "individual",
			Circles:            []string{"Friends"},
			HowWeMet:           "conference",
			WorkInformation:    "Acme",
			ContactInformation: "prefers email",
		},
		Passthrough: contactmodel.Passthrough{
			VCard:     []contactmodel.JCardProp{{Name: "X-CUSTOM", Type: "text", Value: json.RawMessage(`"custom-value"`)}},
			JSContact: map[string]json.RawMessage{"/customKey": json.RawMessage(`"js-value"`)},
		},
	}
	return record
}

func intPtr(v int) *int { return &v }

func TestFieldSections(t *testing.T) {
	sections := FieldSections()
	require.NotEmpty(t, sections)

	// Every returned token must be valid and there must be no duplicates.
	seen := make(map[string]bool, len(sections))
	for _, s := range sections {
		require.False(t, seen[s], "duplicate section token %q", s)
		seen[s] = true
		require.NoError(t, NewFieldSelection().Enable(s), "token %q must be valid", s)
	}
}

// Only the three sensitivity-bearing sections may be marked sensitive — the
// frontend gates these behind the reveal action, and the backend threads the
// IncludeSensitive override through exactly these projection steps.
func TestIsSensitiveSection(t *testing.T) {
	for token, want := range map[string]bool{
		SectionEmails:         false,
		SectionPhones:         false,
		SectionAddresses:      false,
		SectionOrganizations:  false,
		SectionAnniversaries:  false,
		SectionMedia:          false,
		SectionOnlineServices: false,
		SectionLinks:          false,
		SectionNotes:          false,
		SectionKeywords:       false,
		SectionRelatedTo:      true,
		SectionPersonalInfo:   true,
		SectionSpeakToAs:      false,
		SectionMembers:        false,
		SectionLanguages:      false,
		SectionCustomFields:   true,
	} {
		assert.Equal(t, want, IsSensitiveSection(token), "section %q", token)
	}
}

func TestFieldSelectionAllSelectsEverything(t *testing.T) {
	sel := FieldSelectionAll()
	for _, s := range FieldSections() {
		assert.True(t, sel.Has(s), "FieldSelectionAll must select %q", s)
	}
	assert.False(t, sel.IncludeSensitive)
}

func TestFieldSelectionEnableRejectsUnknownToken(t *testing.T) {
	sel := NewFieldSelection()
	require.Error(t, sel.Enable("not_a_section"))
	assert.False(t, sel.Has("not_a_section"))
	require.NoError(t, sel.Enable(SectionEmails))
	assert.True(t, sel.Has(SectionEmails))
}

func TestApplyFieldSelection_NilSelectionReturnsRecordUnchanged(t *testing.T) {
	record := buildFullRecord()
	got := ApplyFieldSelection(record, nil)
	assert.Same(t, record, got, "nil selection must return the same pointer")
}

func TestApplyFieldSelection_AllSectionsKeepsEverything(t *testing.T) {
	record := buildFullRecord()
	got := ApplyFieldSelection(record, FieldSelectionAll())

	assert.Equal(t, record.Card, got.Card)
	assert.Equal(t, record.Passthrough, got.Passthrough)
	assert.Equal(t, record.Envelope, got.Envelope)
	assert.Equal(t, record.UID, got.UID)
	assert.Equal(t, record.ETag, got.ETag)
}

// The core filter contract: selecting a subset keeps only those sections
// (plus the always-exported identity data), clears the rest, and never
// mutates the caller's record.
func TestApplyFieldSelection_SubsetClearsOnlyDeselected(t *testing.T) {
	record := buildFullRecord()

	sel := NewFieldSelection()
	require.NoError(t, sel.Enable(SectionEmails))
	require.NoError(t, sel.Enable(SectionPhones))
	got := ApplyFieldSelection(record, sel)

	assert.Equal(t, []contactmodel.Email{{Address: "alice@example.com"}}, got.Card.Emails)
	assert.Equal(t, []contactmodel.Phone{{Number: "555-0100"}}, got.Card.Phones)

	// Everything else the picker covers is cleared...
	assert.Empty(t, got.Card.Addresses)
	assert.Empty(t, got.Card.Organizations)
	assert.Empty(t, got.Card.Titles)
	assert.Empty(t, got.Card.Anniversaries)
	assert.Empty(t, got.Card.Media)
	assert.Empty(t, got.Card.ImppAddresses)
	assert.Empty(t, got.Card.Links)
	assert.Empty(t, got.Card.Notes)
	assert.Empty(t, got.Card.Keywords)
	assert.Empty(t, got.Card.RelatedTo)
	assert.Empty(t, got.Card.PersonalInfo)
	assert.Nil(t, got.Card.SpeakToAs)
	assert.Empty(t, got.Card.Members)
	assert.Empty(t, got.Card.PreferredLanguages)
	assert.Empty(t, got.Passthrough.VCard)
	assert.Empty(t, got.Passthrough.JSContact)

	// ...but the always-exported identity and the CRM envelope survive.
	assert.Equal(t, record.Card.UID, got.Card.UID)
	assert.Equal(t, record.Card.Name, got.Card.Name)
	assert.Equal(t, record.Card.Nicknames, got.Card.Nicknames)
	assert.Equal(t, record.Card.Kind, got.Card.Kind)
	assert.Equal(t, record.Envelope, got.Envelope)

	// The caller's record is untouched.
	assert.Len(t, record.Card.Emails, 1)
	assert.Len(t, record.Card.Phones, 1)
	assert.Len(t, record.Card.RelatedTo, 1)
}

// organizations covers both Card.Organizations and Card.Titles.
func TestApplyFieldSelection_OrganizationsClearsTitlesToo(t *testing.T) {
	record := buildFullRecord()
	sel := NewFieldSelection()
	require.NoError(t, sel.Enable(SectionOrganizations))

	got := ApplyFieldSelection(record, sel)

	assert.Equal(t, record.Card.Organizations, got.Card.Organizations)
	assert.Equal(t, record.Card.Titles, got.Card.Titles)
	assert.Empty(t, got.Card.Emails)
}

// custom_fields clears Passthrough (vCardProps and jsContactProps), which is
// where vCard-projected custom fields ride.
func TestApplyFieldSelection_CustomFieldsKeepsPassthrough(t *testing.T) {
	record := buildFullRecord()
	sel := NewFieldSelection()
	require.NoError(t, sel.Enable(SectionCustomFields))

	got := ApplyFieldSelection(record, sel)
	assert.Equal(t, record.Passthrough, got.Passthrough)
	assert.Empty(t, got.Card.Emails)
}

func TestApplyFieldSelection_NilRecordIsSafe(t *testing.T) {
	assert.Nil(t, ApplyFieldSelection(nil, FieldSelectionAll()))
}

// --- Real-migrated-schema projection override tests --------------------------
//
// These run against database.InitDB (the hand-written migration SQL), per
// CLAUDE.md trap #1 — AutoMigrate-based tests cannot see a schema mismatch.

// buildSensitiveProjectionFixture seeds one contact with, around it:
//   - a secret-sensitivity confirmed relationship edge (spouse_of),
//   - a secret-sensitivity hobby Preference,
//   - a secret-sensitivity vcard:X- projected FieldDefinition + FieldValue.
func buildSensitiveProjectionFixture(t *testing.T) (*gorm.DB, User, Contact) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "field-selection-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := User{Username: "seltester", Password: "password123!A", Email: "seltester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := Contact{UserID: user.ID, Firstname: "Alice"}
	bob := Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	// Secret-sensitivity spouse edge between Alice and Bob.
	require.NoError(t, db.Create(&RelationshipEdge{
		UserID:      user.ID,
		SourceID:    alice.VCardUID,
		TargetID:    bob.VCardUID,
		Type:        "spouse_of",
		Directional: false,
		Source:      RelationshipSourceUserConfirmed,
		Confidence:  1.0,
		Status:      RelationshipStatusConfirmed,
		Sensitivity: RelationshipSensitivitySecret,
	}).Error)

	// Secret-sensitivity hobby preference.
	require.NoError(t, db.Create(&Preference{
		UserID:      user.ID,
		EntityID:    alice.VCardUID,
		Category:    PreferenceCategoryHobby,
		Value:       "sky-diving",
		Source:      PreferenceSourceUser,
		Sensitivity: RelationshipSensitivitySecret,
	}).Error)

	// Secret-sensitivity vcard-projected custom field.
	def := FieldDefinition{
		UserID:      user.ID,
		Label:       "Private Note",
		Key:         "private_note",
		Target:      FieldDefinitionTargetContact,
		Type:        FieldTypeString,
		Projection:  "vcard:X-PRIVATE-NOTE",
		Sensitivity: RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&FieldValue{
		FieldDefinitionID: def.ID,
		UserID:            user.ID,
		EntityID:          alice.VCardUID,
		Value:             json.RawMessage(`"eyes only"`),
	}).Error)

	return db, user, alice
}

// The ticket's binding sensitivity rule: a sensitive item is
// excluded by default and included only when the caller explicitly opts in —
// a plain RecordForContact (or an all-sections selection without the flag)
// must not leak it, and RecordForContactFiltered with IncludeSensitive must
// project all three sensitivity-bearing kinds (edges, preferences, custom
// fields).
func TestRecordForContactFiltered_SensitiveItemsRequireOptIn(t *testing.T) {
	db, user, alice := buildSensitiveProjectionFixture(t)
	_ = user

	// Default read: nothing sensitive projects.
	defaultRecord := RecordForContact(&alice, "", db)
	assert.Empty(t, defaultRecord.Card.RelatedTo, "secret edge must not project by default")
	assert.Empty(t, defaultRecord.Card.PersonalInfo, "secret hobby must not project by default")
	assert.Empty(t, defaultRecord.Passthrough.VCard, "secret custom field must not project by default")

	// Same thing through the filtered path with all sections but no opt-in:
	// checking a section is NOT enough.
	allSections := FieldSelectionAll()
	noOptIn := RecordForContactFiltered(&alice, "", db, allSections)
	assert.Empty(t, noOptIn.Card.RelatedTo)
	assert.Empty(t, noOptIn.Card.PersonalInfo)
	assert.Empty(t, noOptIn.Passthrough.VCard)

	// Explicit opt-in: all three sensitive items project.
	allSections.IncludeSensitive = true
	optedIn := RecordForContactFiltered(&alice, "", db, allSections)
	require.Len(t, optedIn.Card.RelatedTo, 1, "secret spouse edge must project with explicit opt-in")
	assert.Equal(t, []string{"spouse"}, optedIn.Card.RelatedTo[0].Relations)
	require.Len(t, optedIn.Card.PersonalInfo, 1)
	assert.Equal(t, "sky-diving", optedIn.Card.PersonalInfo[0].Value)
	require.Len(t, optedIn.Passthrough.VCard, 1)
	assert.Equal(t, "X-PRIVATE-NOTE", optedIn.Passthrough.VCard[0].Name)

	// The section filter still applies on top of the opt-in: opt-in for a
	// section the user did not select leaks nothing.
	sel := NewFieldSelection()
	require.NoError(t, sel.Enable(SectionEmails))
	sel.IncludeSensitive = true
	filtered := RecordForContactFiltered(&alice, "", db, sel)
	assert.Empty(t, filtered.Card.RelatedTo, "unselected sensitive section must stay empty")
	assert.Empty(t, filtered.Card.PersonalInfo)
	assert.Empty(t, filtered.Passthrough.VCard)
	assert.NotNil(t, filtered.Card.Name, "identity data is always present")
}
