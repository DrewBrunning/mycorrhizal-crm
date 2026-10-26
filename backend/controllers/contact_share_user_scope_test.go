package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateContactShare_OnlySenderRowsOnCollidingUID is the contact-share half
// of issue #2046: the stored payload is built from the sender's contact through
// the same Record projections as every other outward copy. When another account
// holds a contact with the same vCard UID, none of that account's tags, hobby
// preferences, confirmed edges, or custom fields may appear in the payload.
func TestCreateContactShare_OnlySenderRowsOnCollidingUID(t *testing.T) {
	db := dbtest.New(t)

	sender := models.User{Username: "share-scope-sender", Password: "password123!A", Email: "share-scope-sender@example.com"}
	recipient := models.User{Username: "share-scope-recipient", Password: "password123!A", Email: "share-scope-recipient@example.com"}
	other := models.User{Username: "share-scope-other", Password: "password123!A", Email: "share-scope-other@example.com"}
	require.NoError(t, db.Create(&sender).Error)
	require.NoError(t, db.Create(&recipient).Error)
	require.NoError(t, db.Create(&other).Error)

	const sharedUID = "a4c5d6e7-1111-4222-8333-444455556666"
	senderContact := models.Contact{UserID: sender.ID, Firstname: "Sender", VCardUID: sharedUID}
	otherContact := models.Contact{UserID: other.ID, Firstname: "Other", VCardUID: sharedUID}
	require.NoError(t, db.Create(&senderContact).Error)
	require.NoError(t, db.Create(&otherContact).Error)

	senderTag := models.Tag{UserID: sender.ID, Name: "sender-tag"}
	otherTag := models.Tag{UserID: other.ID, Name: "other-tag"}
	require.NoError(t, db.Create(&senderTag).Error)
	require.NoError(t, db.Create(&otherTag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: senderTag.ID, UserID: sender.ID, ContactVCardUID: sharedUID}).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: otherTag.ID, UserID: other.ID, ContactVCardUID: sharedUID}).Error)

	require.NoError(t, db.Create(&models.Preference{
		UserID: sender.ID, EntityID: sharedUID, Category: models.PreferenceCategoryHobby,
		Value: "sender-hobby", Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&models.Preference{
		UserID: other.ID, EntityID: sharedUID, Category: models.PreferenceCategoryHobby,
		Value: "other-hobby", Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	senderTarget := models.Contact{UserID: sender.ID, Firstname: "SenderTarget"}
	otherTarget := models.Contact{UserID: other.ID, Firstname: "OtherTarget"}
	require.NoError(t, db.Create(&senderTarget).Error)
	require.NoError(t, db.Create(&otherTarget).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: sender.ID, SourceID: sharedUID, TargetID: senderTarget.VCardUID,
		Type: "parent_of", Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: other.ID, SourceID: sharedUID, TargetID: otherTarget.VCardUID,
		Type: "parent_of", Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	senderDef := models.FieldDefinition{
		UserID: sender.ID, Label: "Sender field", Key: "share_sender_field",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeString,
		Projection: "vcard:X-SENDER-FIELD", Sensitivity: models.RelationshipSensitivityNormal,
	}
	otherDef := models.FieldDefinition{
		UserID: other.ID, Label: "Other field", Key: "share_other_field",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeString,
		Projection: "vcard:X-OTHER-FIELD", Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&senderDef).Error)
	require.NoError(t, db.Create(&otherDef).Error)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: senderDef.ID, UserID: sender.ID, EntityID: sharedUID,
		Value: []byte(`"sender-field-value"`),
	}).Error)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: otherDef.ID, UserID: other.ID, EntityID: sharedUID,
		Value: []byte(`"other-field-value"`),
	}).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	router := contactShareRouterFor(db, cfg, sender.ID)
	w := doJSON(t, router, http.MethodPost, "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID,
		VCardUID: sharedUID,
		Sections: models.FieldSections(),
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var share models.ContactShare
	require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", sender.ID, recipient.ID).First(&share).Error)

	assert.Contains(t, share.Payload, "sender-tag", "the sender's own tag must cross")
	assert.NotContains(t, share.Payload, "other-tag", "another user's tag must not enter the share")
	assert.Contains(t, share.Payload, "sender-hobby", "the sender's own hobby must cross")
	assert.NotContains(t, share.Payload, "other-hobby", "another user's hobby must not enter the share")
	assert.Contains(t, share.Payload, senderTarget.VCardUID, "the sender's own edge target must cross")
	assert.NotContains(t, share.Payload, otherTarget.VCardUID, "another user's edge target must not enter the share")
	assert.Contains(t, share.Payload, "sender-field-value", "the sender's own custom field must cross")
	assert.NotContains(t, share.Payload, "other-field-value", "another user's custom field must not enter the share")
}
