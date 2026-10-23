package carddav

// Issue #1738's CardDAV half, mirroring address_sensitivity_test.go:
//   - GET withholds personalInfo entries whose CRM metadata marks them
//     private/secret (RecordForContactForSync).
//   - PUT re-attaches them, because a WebDAV PUT is a full overwrite and the
//     client was never shown those entries (MergePersonalInfoFromRemote).
//
// The GET test fails with the filter removed; the PUT test fails with the
// merge stubbed to return the incoming card unchanged.

import (
	"context"
	"gorm.io/gorm"
	"mycorrhizal/internal/logtest"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedCardDAVUser creates the user row (ID 1) the backend's ContextWithUser
// writes as. Without it every contact write's audit event fails its users
// foreign key -- a swallowed warn that logtest.Guard reports.
func seedCardDAVUser(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&models.User{Model: gorm.Model{ID: 1}, Username: "tester", Email: "tester@example.com", Password: "password123"}).Error)
}

func newPersonalInfoContact(t *testing.T, uid string) (*Backend, models.Contact) {
	t.Helper()
	backend, db := newTestBackend(t)
	seedCardDAVUser(t, db)

	record := &contactmodel.Record{
		Card: contactmodel.Card{
			UID:  uid,
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			PersonalInfo: []contactmodel.PersonalInfo{
				{ID: "pi-normal", Kind: "hobby", Value: "Chess", Level: "high"},
				{ID: "pi-private", Kind: "hobby", Value: "Birdwatching", Level: "low"},
				{ID: "pi-secret", Kind: "hobby", Value: "Lockpicking"},
			},
		},
		Envelope: contactmodel.CRMEnvelope{PersonalInfoMeta: map[string]contactmodel.PersonalInfoMeta{
			"pi-normal":  {Favorite: true, Notes: "plays weekly"},
			"pi-private": {Sensitivity: models.RelationshipSensitivityPrivate, Notes: "secret spot"},
			"pi-secret":  {Sensitivity: models.RelationshipSensitivitySecret},
		}},
	}
	contact := models.Contact{UserID: 1}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, db.Create(&contact).Error)
	return backend, contact
}

func clientHobbyCard(uid string, hobbies ...string) vcard.Card {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	card.SetValue(vcard.FieldUID, uid)
	card.SetValue(vcard.FieldFormattedName, "Ada Edited")
	card.SetValue(vcard.FieldName, "Lovelace;Ada;;;")
	for _, hobby := range hobbies {
		card[vcard4PropHobby] = append(card[vcard4PropHobby], &vcard.Field{Value: hobby})
	}
	return card
}

// vcard4PropHobby is the RFC 6715 property name; kept local so the test does
// not depend on the exporter's constant.
const vcard4PropHobby = "HOBBY"

func TestCardDAV_GetAddressObject_ExcludesSensitivePersonalInfo(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	backend, contact := newPersonalInfoContact(t, "pi-secret-get")
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")

	obj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+contact.VCardUID+".vcf", nil)
	require.NoError(t, err)

	hobbies := obj.Card[vcard4PropHobby]
	require.Len(t, hobbies, 1, "only the normal hobby may be served")
	assert.Equal(t, "Chess", hobbies[0].Value)
	assert.Equal(t, "high", hobbies[0].Params.Get("LEVEL"))

	body := encodeCard(t, obj.Card)
	assert.NotContains(t, body, "Birdwatching")
	assert.NotContains(t, body, "Lockpicking")
	assert.Contains(t, body, "Ada", "identity data is still served")
}

func TestCardDAV_PutPreservesOmittedSensitivePersonalInfo(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	backend, contact := newPersonalInfoContact(t, "pi-secret-put")
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	// The client knows only the normal hobby and edits an unrelated field.
	_, err := backend.PutAddressObject(ctx, urlPath, clientHobbyCard(contact.VCardUID, "Chess"), nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)

	byID := map[string]contactmodel.PersonalInfo{}
	for _, entry := range stored.Card.PersonalInfo {
		byID[entry.ID] = entry
	}
	require.Contains(t, byID, "pi-private", "the private hobby must survive the full-overwrite PUT")
	assert.Equal(t, "Birdwatching", byID["pi-private"].Value)
	assert.Equal(t, "low", byID["pi-private"].Level)
	require.Contains(t, byID, "pi-secret", "the secret hobby must survive too")
	assert.Equal(t, "Lockpicking", byID["pi-secret"].Value)

	assert.Equal(t, models.RelationshipSensitivityPrivate, stored.CRM.PersonalInfoMeta["pi-private"].Sensitivity)
	assert.Equal(t, models.RelationshipSensitivitySecret, stored.CRM.PersonalInfoMeta["pi-secret"].Sensitivity)
	assert.Equal(t, contactmodel.PersonalInfoMeta{Favorite: true, Notes: "plays weekly"},
		stored.CRM.PersonalInfoMeta["pi-normal"], "metadata for the entry the client sent is carried forward")
	assert.Len(t, stored.Card.PersonalInfo, 3, "no duplicate normal hobby is created")
	assert.Equal(t, "Ada Edited", stored.FN, "the client's ordinary edit is applied")
}
