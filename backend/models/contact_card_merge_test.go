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

// richCardOnlyRecord builds a neutral Record whose Card-only data has no
// flat-field home, mirroring what a VCF/JSContact import or the nested REST
// API produces: SpeakToAs (pronouns), PersonalInfo (hobbies), address
// components outside the flat projection (the nine kinds with no editor
// demand — room, building, block, etc.), rich per-entry phone/email metadata
// (pref, features, contexts), a CRMEnvelope.Kind, and an imported Passthrough
// property. (PO box / apartment / floor DO have a flat home since T79, but
// they stay in the fixture as unprojected members of a card that was never
// round-tripped through the flat shape, so the merge tests keep exercising
// the preservation path.)
func richCardOnlyRecord() *contactmodel.Record {
	return &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{
				Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}},
			},
			Emails: []contactmodel.Email{{
				Address:  "ada@example.com",
				Contexts: []string{"work"},
				Pref:     intPtr(1),
				Label:    "work",
			}},
			Phones: []contactmodel.Phone{{
				Number:   "+15550100100",
				Label:    "cell",
				Features: []string{"cell", "voice"},
				Pref:     intPtr(1),
			}},
			Addresses: []contactmodel.Address{{
				Components: []contactmodel.AddressComponent{
					{Kind: "name", Value: "123 Main St"},
					{Kind: "apartment", Value: "Apt 3B"},
					{Kind: "floor", Value: "4"},
					{Kind: "postOfficeBox", Value: "PO Box 42"},
					{Kind: "locality", Value: "Springfield"},
				},
				Contexts: []string{"home"},
			}},
			SpeakToAs: &contactmodel.SpeakToAs{
				Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}},
			},
			PersonalInfo: []contactmodel.PersonalInfo{{Kind: "hobby", Value: "sailing"}},
		},
		Envelope: contactmodel.CRMEnvelope{Kind: "pet"},
		Passthrough: contactmodel.Passthrough{
			VCard: []contactmodel.JCardProp{{Name: "X-CUSTOM", Type: "text", Value: json.RawMessage(`"keep-me"`)}},
		},
	}
}

// newT75TestDB is a real migrated schema (CLAUDE.md backend trap 1) for the
// T75 BeforeSave merge tests. The audit recorder is deliberately not
// registered: the hooks fire and skip silently, exactly like the other
// models tests that use database.InitDB (etag_real_db_test.go).
func newT75TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "t75-merge-test.db"))
	require.NoError(t, err)
	return db
}

// TestBeforeSave_PlainSavePreservesCardOnlyData is the T75 reproduction,
// pinned at the model layer against the real migrated schema: create a
// contact through ApplyRecordToContact carrying SpeakToAs, PersonalInfo,
// unprojected address components, rich phone/email metadata, a pet Kind and
// an imported Passthrough, reload it, mutate one unrelated flat field (the
// exact shape of the photo-upload handler), and call db.Save. Every
// Card-only member must survive — before T75's merge, the flat→Card
// rebuild in BeforeSave silently destroyed all of it.
func TestBeforeSave_PlainSavePreservesCardOnlyData(t *testing.T) {
	db := newT75TestDB(t)
	user := User{Username: "t75-preserve", Password: "password123!A", Email: "t75-preserve@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, richCardOnlyRecord(), "")
	require.NoError(t, db.Create(contact).Error)

	// Reload as the handlers do, mutate one flat field (the photo-upload
	// shape), and save.
	var loaded Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	loaded.Photo = "new-photo.jpg"
	require.NoError(t, db.Save(&loaded).Error)

	var persisted Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)

	if persisted.Card.SpeakToAs == nil || len(persisted.Card.SpeakToAs.Pronouns) != 1 || persisted.Card.SpeakToAs.Pronouns[0].Pronouns != "she/her" {
		t.Errorf("SpeakToAs = %+v, want the created pronouns preserved across a plain save", persisted.Card.SpeakToAs)
	}
	if len(persisted.Card.PersonalInfo) != 1 || persisted.Card.PersonalInfo[0].Value != "sailing" {
		t.Errorf("PersonalInfo = %+v, want the created hobby preserved across a plain save", persisted.Card.PersonalInfo)
	}
	require.Len(t, persisted.Card.Addresses, 1, "the address must survive the save")
	components := map[string]string{}
	for _, comp := range persisted.Card.Addresses[0].Components {
		components[comp.Kind] = comp.Value
	}
	for _, kind := range []string{"apartment", "floor", "postOfficeBox"} {
		if components[kind] == "" {
			t.Errorf("address component kind=%q missing after plain save (components=%v)", kind, components)
		}
	}
	if persisted.Card.Emails[0].Pref == nil || persisted.Card.Emails[0].Contexts[0] != "work" {
		t.Errorf("Card.Emails[0] = %+v, want the created pref/contexts preserved", persisted.Card.Emails[0])
	}
	if len(persisted.Card.Phones[0].Features) != 2 || persisted.Card.Phones[0].Pref == nil {
		t.Errorf("Card.Phones[0] = %+v, want the created features/pref preserved", persisted.Card.Phones[0])
	}
	if persisted.CRM.Kind != "pet" {
		t.Errorf("CRM.Kind = %q, want the created pet kind preserved across a plain save", persisted.CRM.Kind)
	}
	if len(persisted.Passthrough.VCard) != 1 || persisted.Passthrough.VCard[0].Name != "X-CUSTOM" {
		t.Errorf("Passthrough.VCard = %+v, want the imported X-CUSTOM property preserved across a plain save", persisted.Passthrough.VCard)
	}
}

// TestBeforeSave_FlatArrayAppendPreservesUntouchedEntries pins the per-entry
// dirty-comparison rule (T75 item 2, the generalization of the ticket's
// whole-array rule): when the caller appends a new entry to a flat array —
// exactly what MergeImportedContact's additive merge does (T49) — the
// untouched existing entries keep their Card-only data. The ticket's
// original whole-array rule would have rebuilt the entire array from flat
// and destroyed the existing apartment; the per-entry rule preserves it.
func TestBeforeSave_FlatArrayAppendPreservesUntouchedEntries(t *testing.T) {
	db := newT75TestDB(t)
	user := User{Username: "t75-append", Password: "password123!A", Email: "t75-append@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, richCardOnlyRecord(), "")
	require.NoError(t, db.Create(contact).Error)

	var loaded Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)

	// Simulate an import merge appending a different address (the lossy flat
	// shape; the incoming address has no apartment).
	loaded.Addresses = append(loaded.Addresses, ContactAddress{
		Type: "work", Street: "456 Oak Ave", City: "Shelbyville",
	})
	require.NoError(t, db.Save(&loaded).Error)

	var persisted Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	require.Len(t, persisted.Card.Addresses, 2, "the appended address must survive the save")

	components := map[string]string{}
	for _, comp := range persisted.Card.Addresses[0].Components {
		components[comp.Kind] = comp.Value
	}
	if components["apartment"] != "Apt 3B" || components["postOfficeBox"] != "PO Box 42" {
		t.Errorf("existing address unprojected components lost when a new flat address was appended (components=%v)", components)
	}
	newComponents := map[string]string{}
	for _, comp := range persisted.Card.Addresses[1].Components {
		newComponents[comp.Kind] = comp.Value
	}
	if newComponents["name"] != "456 Oak Ave" {
		t.Errorf("new address name = %q, want the appended street value in the card", newComponents["name"])
	}
	if persisted.Card.Addresses[1].Full == "" {
		t.Errorf("appended address = %+v, want the new flat address rebuilt into the card", persisted.Card.Addresses[1])
	}
}

// TestBeforeSave_EditedFlatEntryRebuildsFromFlat pins the per-entry dirty-
// comparison rule's "rebuilt from flat" half: when the caller edits a flat
// entry's projected field, the entry is rebuilt from flat. Since T79 the
// apartment has a flat home, so editing the street preserves it (the flat
// entry still carries it) — but a caller who deliberately clears the flat
// apartment expresses intent through the shape and gets it dropped from the
// card, exactly like any other projected field.
func TestBeforeSave_EditedFlatEntryRebuildsFromFlat(t *testing.T) {
	db := newT75TestDB(t)
	user := User{Username: "t75-edit", Password: "password123!A", Email: "t75-edit@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, richCardOnlyRecord(), "")
	require.NoError(t, db.Create(contact).Error)

	var loaded Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	// The reloaded flat address carries the apartment (T79 widened the flat
	// projection), so editing only the street must NOT destroy it.
	require.Equal(t, "Apt 3B", loaded.Addresses[0].Apartment, "the reloaded flat address must carry the apartment")
	loaded.Addresses[0].Street = "999 Changed St"
	require.NoError(t, db.Save(&loaded).Error)

	var persisted Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	require.Len(t, persisted.Card.Addresses, 1)
	components := map[string]string{}
	for _, comp := range persisted.Card.Addresses[0].Components {
		components[comp.Kind] = comp.Value
	}
	if components["name"] != "999 Changed St" {
		t.Errorf("street = %q, want the edited value reflected in the card", components["name"])
	}
	if components["apartment"] != "Apt 3B" {
		t.Errorf("apartment = %q, want it preserved — the flat shape owns it since T79 (components=%v)", components["apartment"], components)
	}

	// Deliberately clearing the flat apartment IS an intent expressed through
	// the (now lossy for that field) shape: the rebuilt entry drops it.
	var cleared Contact
	require.NoError(t, db.First(&cleared, contact.ID).Error)
	cleared.Addresses[0].Apartment = ""
	require.NoError(t, db.Save(&cleared).Error)

	var afterClear Contact
	require.NoError(t, db.First(&afterClear, contact.ID).Error)
	clearedComps := map[string]string{}
	for _, comp := range afterClear.Card.Addresses[0].Components {
		clearedComps[comp.Kind] = comp.Value
	}
	if clearedComps["apartment"] != "" {
		t.Errorf("apartment = %q, want dropped — the caller cleared it in the flat shape", clearedComps["apartment"])
	}
}

// TestRestoreFlatStateFrom_LeavesNonsnapshotFieldsAlone pins the undo stopgap
// helper: RestoreFlatStateFrom overwrites exactly the flat fields an audit
// before-snapshot records, and deliberately leaves the json:"-" columns
// (PhotoThumbnail, VCardUID, VCardExtra, ETag) — which no snapshot ever
// carried — untouched.
func TestRestoreFlatStateFrom_LeavesNonsnapshotFieldsAlone(t *testing.T) {
	current := &Contact{
		Firstname: "Changed", Lastname: "Now", Photo: "current.jpg", PhotoThumbnail: "data:thumb",
		VCardUID: "uid-current", VCardExtra: `{"properties":{"X-KEEP":[{"Value":"1"}]}}`, ETag: "e-1-1",
		Emails: []ContactEmail{{Type: "work", Value: "current@example.com"}},
	}
	before := &Contact{
		Firstname: "Original", Lastname: "Then", Photo: "before.jpg",
		Emails: []ContactEmail{{Type: "home", Value: "before@example.com"}},
	}

	current.RestoreFlatStateFrom(before)

	if current.Firstname != "Original" || current.Lastname != "Then" || current.Photo != "before.jpg" {
		t.Errorf("flat fields not restored: %+v", current)
	}
	if len(current.Emails) != 1 || current.Emails[0].Value != "before@example.com" || current.Emails[0].Type != "home" {
		t.Errorf("Emails = %+v, want the snapshot's array restored", current.Emails)
	}
	if current.PhotoThumbnail != "data:thumb" || current.VCardUID != "uid-current" ||
		current.VCardExtra != `{"properties":{"X-KEEP":[{"Value":"1"}]}}` || current.ETag != "e-1-1" {
		t.Errorf("json:- fields must be left untouched, got PhotoThumbnail=%q VCardUID=%q VCardExtra=%q ETag=%q",
			current.PhotoThumbnail, current.VCardUID, current.VCardExtra, current.ETag)
	}
}

// TestRestoreFlatStateFrom_ThenSavePreservesCardOnlyData runs the undo stopgap
// end to end at the model layer: restore the snapshot's flat state onto a
// live contact, save, and assert the Card-only data that no snapshot ever
// carried survives while the flat state is reverted.
func TestRestoreFlatStateFrom_ThenSavePreservesCardOnlyData(t *testing.T) {
	db := newT75TestDB(t)
	user := User{Username: "t75-undo", Password: "password123!A", Email: "t75-undo@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, richCardOnlyRecord(), "")
	require.NoError(t, db.Create(contact).Error)

	// Simulate a snapshot that could never have carried Card data: unmarshal
	// a flat-only JSON blob (json.Marshal of the loaded contact, which drops
	// Card/CRM/Passthrough via json:"-").
	snap, err := json.Marshal(contact)
	require.NoError(t, err)
	var before Contact
	require.NoError(t, json.Unmarshal(snap, &before))

	var current Contact
	require.NoError(t, db.First(&current, contact.ID).Error)
	current.Firstname = "Edited"
	require.NoError(t, db.Save(&current).Error)

	var currentEdited Contact
	require.NoError(t, db.First(&currentEdited, contact.ID).Error)
	require.Equal(t, "Edited", currentEdited.Firstname)

	// Undo: restore the snapshot's flat state, save.
	currentEdited.RestoreFlatStateFrom(&before)
	require.NoError(t, db.Save(&currentEdited).Error)

	var undone Contact
	require.NoError(t, db.First(&undone, contact.ID).Error)
	assert.Equal(t, "Ada", undone.Firstname, "the flat name must be reverted to the snapshot value")
	if undone.Card.SpeakToAs == nil || len(undone.Card.SpeakToAs.Pronouns) != 1 {
		t.Errorf("SpeakToAs = %+v, want the Card-only data preserved through undo", undone.Card.SpeakToAs)
	}
	if len(undone.Card.PersonalInfo) != 1 || undone.Card.PersonalInfo[0].Value != "sailing" {
		t.Errorf("PersonalInfo = %+v, want the created hobby preserved through undo", undone.Card.PersonalInfo)
	}
	components := map[string]string{}
	for _, comp := range undone.Card.Addresses[0].Components {
		components[comp.Kind] = comp.Value
	}
	if components["apartment"] != "Apt 3B" {
		t.Errorf("address apartment lost through undo (components=%v)", components)
	}
	if undone.CRM.Kind != "pet" {
		t.Errorf("CRM.Kind = %q, want preserved through undo", undone.CRM.Kind)
	}
}
