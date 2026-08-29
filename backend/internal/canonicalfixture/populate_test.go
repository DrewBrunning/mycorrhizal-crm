package canonicalfixture

import (
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestSoftDeletedContactCascade pins trap #7's cascade semantics: a
// soft-deleted contact's dependent user-authored content is soft-deleted
// (recoverable — the undo button) and its join/edge rows are hard-deleted
// (the client re-pulls them). The manifest declares gina's full surface
// precisely so this test can prove the loader applied DeleteContact's cascade,
// not just the contact-row tombstone.
func TestSoftDeletedContactCascade(t *testing.T) {
	_, ds, db := populatedDB(t)

	gina := ds.Contacts["gina"]
	require.True(t, gina.DeletedAt.Valid, "gina's contact row must be soft-deleted")
	uid := gina.VCardUID

	// Her user-authored content is tombstoned but recoverable (one row each,
	// all soft-deleted).
	softScopes := []struct {
		name  string
		count func(db *gorm.DB) int64
	}{
		{"notes", func(db *gorm.DB) int64 {
			var n int64
			db.Model(&models.Note{}).Unscoped().Where("contact_id = ?", gina.ID).Count(&n)
			return n
		}},
		{"life events", func(db *gorm.DB) int64 {
			var n int64
			db.Model(&models.LifeEvent{}).Unscoped().Where("entity_id = ?", uid).Count(&n)
			return n
		}},
		{"preferences", func(db *gorm.DB) int64 {
			var n int64
			db.Model(&models.Preference{}).Unscoped().Where("entity_id = ?", uid).Count(&n)
			return n
		}},
		{"gifts", func(db *gorm.DB) int64 {
			var n int64
			db.Model(&models.Gift{}).Unscoped().Where("entity_id = ?", uid).Count(&n)
			return n
		}},
		{"attachments", func(db *gorm.DB) int64 {
			var n int64
			db.Model(&models.Attachment{}).Unscoped().Where("contact_vcard_uid = ?", uid).Count(&n)
			return n
		}},
	}
	for _, row := range softScopes {
		assert.Equal(t, int64(1), row.count(db), "gina should have exactly one soft-deleted %s", row.name)
	}

	// The tombstones are real: the live (non-Unscoped) count is zero for each.
	var liveNotes, liveEvents, livePrefs, liveGifts, liveAttachments int64
	db.Model(&models.Note{}).Where("contact_id = ?", gina.ID).Count(&liveNotes)
	db.Model(&models.LifeEvent{}).Where("entity_id = ?", uid).Count(&liveEvents)
	db.Model(&models.Preference{}).Where("entity_id = ?", uid).Count(&livePrefs)
	db.Model(&models.Gift{}).Where("entity_id = ?", uid).Count(&liveGifts)
	db.Model(&models.Attachment{}).Where("contact_vcard_uid = ?", uid).Count(&liveAttachments)
	assert.Zero(t, liveNotes)
	assert.Zero(t, liveEvents)
	assert.Zero(t, livePrefs)
	assert.Zero(t, liveGifts)
	assert.Zero(t, liveAttachments)

	// Her join/edge rows are hard-deleted — gone entirely, not tombstoned.
	var liveEdges, liveMembers, liveCircleMembers, liveTagging, liveFieldVals, liveIdentities int64
	db.Unscoped().Model(&models.RelationshipEdge{}).Where("(source_id = ? OR target_id = ?)", uid, uid).Count(&liveEdges)
	db.Unscoped().Model(&models.HouseholdMember{}).Where("member_vcard_uid = ?", uid).Count(&liveMembers)
	db.Unscoped().Model(&models.CircleMember{}).Where("member_vcard_uid = ?", uid).Count(&liveCircleMembers)
	db.Unscoped().Model(&models.ContactTag{}).Where("contact_vcard_uid = ?", uid).Count(&liveTagging)
	db.Unscoped().Model(&models.FieldValue{}).Where("entity_id = ?", uid).Count(&liveFieldVals)
	db.Unscoped().Model(&models.ExternalIdentity{}).Where("entity_id = ?", uid).Count(&liveIdentities)
	assert.Zero(t, liveEdges, "gina's relationship edges must be hard-deleted")
	assert.Zero(t, liveMembers, "gina's household memberships must be hard-deleted")
	assert.Zero(t, liveCircleMembers, "gina's circle memberships must be hard-deleted")
	assert.Zero(t, liveTagging, "gina's taggings must be hard-deleted")
	assert.Zero(t, liveFieldVals, "gina's custom field values must be hard-deleted")
	assert.Zero(t, liveIdentities, "gina's external identities must be hard-deleted")

	// Her activity join rows are removed but the shared activity survives
	// (other contacts remain in it).
	var ginaJoin int64
	db.Model(&models.Activity{}).Where("user_id = ?", ds.User.ID).Joins(
		"JOIN activity_contacts ON activity_contacts.activity_id = activities.id AND activity_contacts.contact_id = ?", gina.ID,
	).Count(&ginaJoin)
	assert.Zero(t, ginaJoin, "gina's activity_contacts join rows must be removed")
	var brunch int64
	db.Model(&models.Activity{}).Where("title = ? AND user_id = ?", "Birthday brunch", ds.User.ID).Count(&brunch)
	assert.Equal(t, int64(1), brunch, "the shared activity itself must survive")
}

// TestUniqueIndexAllowsRecreatedVCardUID pins the partial unique index
// idx_contacts_vcard_uid_user (WHERE deleted_at IS NULL): a soft-deleted
// contact must not block re-creating a contact with the same vcard_uid —
// the re-import-after-delete case. Without the partial index the julie row
// could not exist, and Populate would have failed.
func TestUniqueIndexAllowsRecreatedVCardUID(t *testing.T) {
	_, ds, db := populatedDB(t)

	gina := ds.Contacts["gina"]
	julie := ds.Contacts["julie"]
	require.Equal(t, gina.VCardUID, julie.VCardUID)

	var byUID []models.Contact
	require.NoError(t, db.Unscoped().Where("vcard_uid = ? AND user_id = ?", gina.VCardUID, ds.User.ID).Find(&byUID).Error)
	require.Len(t, byUID, 2, "one tombstone plus one live row must both exist under the same vcard_uid")
	var tombstones, live int64
	for _, c := range byUID {
		if c.DeletedAt.Valid {
			tombstones++
		} else {
			live++
		}
	}
	assert.Equal(t, int64(1), tombstones)
	assert.Equal(t, int64(1), live)
}

// TestDuplicateConflictingRecords pins the deliberate duplicate pair: hugo
// and ida share an email, a phone number, and near-identical names, so
// downstream duplicate-detection suites have a real pair to find.
func TestDuplicateConflictingRecords(t *testing.T) {
	_, ds, _ := populatedDB(t)
	hugo := ds.Contacts["hugo"]
	ida := ds.Contacts["ida"]
	assert.Equal(t, hugo.Email, ida.Email)
	assert.Equal(t, hugo.Phone, ida.Phone)
	assert.NotEqual(t, hugo.VCardUID, ida.VCardUID, "duplicates are distinct rows")
}
