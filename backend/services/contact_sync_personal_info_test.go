package services

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1738 inbound-sync half: reconcileContactSync is a full overwrite,
// but the remote never received above-normal personalInfo entries (the
// CardDAV projection withholds them), so their absence is not a delete. A
// remote update must preserve them and carry metadata forward for entries it
// still has, while a normal entry the remote deleted really is deleted.
func TestReconcileContactSync_PreservesSensitivePersonalInfo(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	href := "/addressbooks/test/alice.vcf"
	remote := func(fn string) carddav.AddressObject {
		return carddav.AddressObject{Path: href, ETag: "\"etag-" + fn + "\"", Card: parseTestCard(t,
			"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:alice-uid\r\nFN:"+fn+"\r\nN:Wonderland;Alice;;;\r\nHOBBY:Chess\r\nEND:VCARD\r\n")}
	}

	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{remote("Alice")}, nil, false, "")
	require.NoError(t, err)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&contact).Error)
	require.Len(t, contact.Card.PersonalInfo, 1)
	chessID := contact.Card.PersonalInfo[0].ID
	require.NotEmpty(t, chessID)

	// The server holds two more entries the client never saw: a private hobby
	// and a normal one. Metadata is attached to all three.
	contact.Card.PersonalInfo = append(contact.Card.PersonalInfo,
		contactmodel.PersonalInfo{ID: "private-hobby", Kind: "hobby", Value: "Secret Poetry", Level: "high"},
		contactmodel.PersonalInfo{ID: "normal-hobby", Kind: "hobby", Value: "Old Normal Hobby"},
	)
	contact.CRM.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{
		chessID:         {Favorite: true, Notes: "plays weekly"},
		"private-hobby": {Sensitivity: models.RelationshipSensitivityPrivate, Notes: "not for the world"},
		"normal-hobby":  {Sensitivity: models.RelationshipSensitivityNormal, Notes: "will be deleted"},
	}
	require.NoError(t, db.Save(&contact).Error)

	// The remote changes an unrelated field and still only knows about Chess.
	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{remote("Alice Updated")}, nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Updated)

	var stored models.Contact
	require.NoError(t, db.First(&stored, contact.ID).Error)
	assert.Equal(t, "Alice Updated", stored.FN, "the remote's ordinary edit is applied")

	values := map[string]contactmodel.PersonalInfo{}
	for _, entry := range stored.Card.PersonalInfo {
		values[entry.Value] = entry
	}
	require.Contains(t, values, "Chess")
	assert.Equal(t, "high", values["Secret Poetry"].Level,
		"the private hobby is re-attached with its content intact")
	assert.NotContains(t, values, "Old Normal Hobby",
		"a normal entry the remote deleted is still authoritative")

	assert.Equal(t, contactmodel.PersonalInfoMeta{
		Favorite: true, Notes: "plays weekly",
	}, stored.CRM.PersonalInfoMeta[values["Chess"].ID],
		"metadata for an entry the remote still has is carried forward")
	assert.Equal(t, models.RelationshipSensitivityPrivate,
		stored.CRM.PersonalInfoMeta[values["Secret Poetry"].ID].Sensitivity)
	assert.NotContains(t, stored.CRM.PersonalInfoMeta, "normal-hobby",
		"metadata for an entry that no longer exists is dropped")
}

// TestReconcileContactSync_RemoteHobbyNeverBecomesHobbyPreference is the
// inverse direction: an incoming HOBBY lands in Card.PersonalInfo, which is
// the canonical home, and reconcile never invents a deprecated hobby
// Preference row.
func TestReconcileContactSync_RemoteHobbyNeverBecomesHobbyPreference(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	obj := carddav.AddressObject{Path: "/alice.vcf", ETag: "\"e1\"", Card: parseTestCard(t,
		"BEGIN:VCARD\r\nVERSION:4.0\r\nUID:alice-uid\r\nFN:Alice\r\nN:Wonderland;Alice;;;\r\nHOBBY;LEVEL=medium:Pottery\r\nEND:VCARD\r\n")}
	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{obj}, nil, false, "")
	require.NoError(t, err)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&contact).Error)
	require.Len(t, contact.Card.PersonalInfo, 1)
	assert.Equal(t, "Pottery", contact.Card.PersonalInfo[0].Value)
	assert.Equal(t, "medium", contact.Card.PersonalInfo[0].Level)

	var prefs int64
	require.NoError(t, db.Model(&models.Preference{}).Where("user_id = ?", user.ID).Count(&prefs).Error)
	assert.Zero(t, prefs, "a remote HOBBY must not create a deprecated hobby Preference")
}
