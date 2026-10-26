package models

// Issue #2046: vcard_uid is unique only per user, and a contact-share import
// deliberately reuses the sender's UID. Every projection query must therefore
// carry the owning user_id as well as the UID, or two accounts that legitimately
// hold the same UID project each other's tags, hobbies, edges, and custom
// fields into their records (and into the share payload built from them).

import (
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createScopeUser(t *testing.T, db *gorm.DB, username string) User {
	t.Helper()
	user := User{
		Username: username,
		Password: "password123!A",
		Email:    username + "@example.com",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestRecordForContact_ProjectsOnlyOwnUsersRowsOnUIDCollision(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)

	owner := createScopeUser(t, db, "scope-owner")
	other := createScopeUser(t, db, "scope-other")

	const sharedUID = "b1d6b0f0-3a96-4a5e-9f1e-6f8f2f0e1a11"
	ownerContact := Contact{UserID: owner.ID, Firstname: "Owner", VCardUID: sharedUID}
	otherContact := Contact{UserID: other.ID, Firstname: "Other", VCardUID: sharedUID}
	require.NoError(t, db.Create(&ownerContact).Error)
	require.NoError(t, db.Create(&otherContact).Error)

	// Tags, keyed by the same UID under each user.
	ownerTag := Tag{UserID: owner.ID, Name: "owner-tag"}
	otherTag := Tag{UserID: other.ID, Name: "other-tag"}
	require.NoError(t, db.Create(&ownerTag).Error)
	require.NoError(t, db.Create(&otherTag).Error)
	require.NoError(t, db.Create(&ContactTag{TagID: ownerTag.ID, UserID: owner.ID, ContactVCardUID: sharedUID}).Error)
	require.NoError(t, db.Create(&ContactTag{TagID: otherTag.ID, UserID: other.ID, ContactVCardUID: sharedUID}).Error)

	// Hobby preferences, keyed by the same UID under each user.
	require.NoError(t, db.Create(&Preference{
		UserID: owner.ID, EntityID: sharedUID, Category: PreferenceCategoryHobby,
		Value: "owner-hobby", Sensitivity: RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&Preference{
		UserID: other.ID, EntityID: sharedUID, Category: PreferenceCategoryHobby,
		Value: "other-hobby", Sensitivity: RelationshipSensitivityNormal,
	}).Error)

	// Confirmed relationship edges, one per user, both touching the shared UID.
	ownerTarget := Contact{UserID: owner.ID, Firstname: "OwnerTarget"}
	otherTarget := Contact{UserID: other.ID, Firstname: "OtherTarget"}
	require.NoError(t, db.Create(&ownerTarget).Error)
	require.NoError(t, db.Create(&otherTarget).Error)
	require.NoError(t, db.Create(&RelationshipEdge{
		UserID: owner.ID, SourceID: sharedUID, TargetID: ownerTarget.VCardUID,
		Type: "parent_of", Source: RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: RelationshipStatusConfirmed, Sensitivity: RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&RelationshipEdge{
		UserID: other.ID, SourceID: sharedUID, TargetID: otherTarget.VCardUID,
		Type: "parent_of", Source: RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: RelationshipStatusConfirmed, Sensitivity: RelationshipSensitivityNormal,
	}).Error)

	// vCard-projected custom fields, one definition/value per user, same entity UID.
	ownerDef := FieldDefinition{
		UserID: owner.ID, Label: "Owner field", Key: "owner_field",
		Target: FieldDefinitionTargetContact, Type: FieldTypeString,
		Projection: "vcard:X-OWNER-FIELD", Sensitivity: RelationshipSensitivityNormal,
	}
	otherDef := FieldDefinition{
		UserID: other.ID, Label: "Other field", Key: "other_field",
		Target: FieldDefinitionTargetContact, Type: FieldTypeString,
		Projection: "vcard:X-OTHER-FIELD", Sensitivity: RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&ownerDef).Error)
	require.NoError(t, db.Create(&otherDef).Error)
	require.NoError(t, db.Create(&FieldValue{
		FieldDefinitionID: ownerDef.ID, UserID: owner.ID, EntityID: sharedUID,
		Value: mustMarshal(t, "owner-field-value"),
	}).Error)
	require.NoError(t, db.Create(&FieldValue{
		FieldDefinitionID: otherDef.ID, UserID: other.ID, EntityID: sharedUID,
		Value: mustMarshal(t, "other-field-value"),
	}).Error)

	record := RecordForContact(&ownerContact, "", db)

	assert.Contains(t, record.Card.Keywords, "owner-tag")
	assert.NotContains(t, record.Card.Keywords, "other-tag",
		"another user's tag on the same vCard UID must not project")

	personalInfoValues := make([]string, len(record.Card.PersonalInfo))
	for i, entry := range record.Card.PersonalInfo {
		personalInfoValues[i] = entry.Value
	}
	assert.Contains(t, personalInfoValues, "owner-hobby")
	assert.NotContains(t, personalInfoValues, "other-hobby",
		"another user's hobby preference on the same vCard UID must not project")

	relatedTargets := make([]string, len(record.Card.RelatedTo))
	for i, rel := range record.Card.RelatedTo {
		relatedTargets[i] = rel.Target
	}
	assert.Contains(t, relatedTargets, "urn:uuid:"+ownerTarget.VCardUID)
	assert.NotContains(t, relatedTargets, "urn:uuid:"+otherTarget.VCardUID,
		"another user's confirmed relationship edge on the same vCard UID must not project")

	propNames := make([]string, len(record.Passthrough.VCard))
	for i, prop := range record.Passthrough.VCard {
		propNames[i] = prop.Name
	}
	assert.Contains(t, propNames, "X-OWNER-FIELD")
	assert.NotContains(t, propNames, "X-OTHER-FIELD",
		"another user's custom field value on the same vCard UID must not project")

	// The positive control: the other user's own record still projects their
	// rows, so the new scope is not simply suppressing every projection.
	otherRecord := RecordForContact(&otherContact, "", db)
	assert.Contains(t, otherRecord.Card.Keywords, "other-tag")
	assert.NotContains(t, otherRecord.Card.Keywords, "owner-tag")
}

// A contact with no owner (UserID 0) must not adopt rows owned by a real user
// that happen to share its UID.
func TestRecordForContact_ZeroUserIDProjectsNothingFromAnotherUser(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)

	other := createScopeUser(t, db, "scope-zero-other")

	const sharedUID = "c2e7c1f1-4b07-4b6f-8a2f-7f9f3f1e2b22"
	require.NoError(t, db.Create(&Contact{UserID: other.ID, Firstname: "Other", VCardUID: sharedUID}).Error)

	tag := Tag{UserID: other.ID, Name: "other-tag"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&ContactTag{TagID: tag.ID, UserID: other.ID, ContactVCardUID: sharedUID}).Error)
	require.NoError(t, db.Create(&Preference{
		UserID: other.ID, EntityID: sharedUID, Category: PreferenceCategoryHobby,
		Value: "other-hobby", Sensitivity: RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&RelationshipEdge{
		UserID: other.ID, SourceID: sharedUID, TargetID: "d3f8d2a2-5c18-4c7a-9b3a-8f0f4f2e3c33",
		Type: "parent_of", Source: RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: RelationshipStatusConfirmed, Sensitivity: RelationshipSensitivityNormal,
	}).Error)
	def := FieldDefinition{
		UserID: other.ID, Label: "Other field", Key: "zero_other_field",
		Target: FieldDefinitionTargetContact, Type: FieldTypeString,
		Projection: "vcard:X-ZERO-OTHER", Sensitivity: RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&FieldValue{
		FieldDefinitionID: def.ID, UserID: other.ID, EntityID: sharedUID,
		Value: mustMarshal(t, "other-field-value"),
	}).Error)

	unsaved := Contact{Firstname: "Unsaved", VCardUID: sharedUID}
	record := RecordForContact(&unsaved, "", db)

	assert.Empty(t, record.Card.Keywords)
	assert.Empty(t, record.Card.PersonalInfo)
	assert.Empty(t, record.Card.RelatedTo)
	assert.Empty(t, record.Passthrough.VCard)
}
