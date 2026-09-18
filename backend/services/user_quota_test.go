package services

import (
	"testing"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func quotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return dbtest.New(t)
}

func mkNote(t *testing.T, db *gorm.DB, userID uint) models.Note {
	t.Helper()
	n := models.Note{UserID: userID, Content: "hello", Date: time.Now()}
	require.NoError(t, db.Create(&n).Error)
	return n
}

func mkAttachment(t *testing.T, db *gorm.DB, userID uint, uid string, size int64) models.Attachment {
	t.Helper()
	a := models.Attachment{
		UserID: userID, ContactVCardUID: uid, StoredName: "x", OriginalName: "x",
		ContentType: "application/pdf", SizeBytes: size,
	}
	require.NoError(t, db.Create(&a).Error)
	return a
}

// TestUserQuotaFromConfig pins the one place the config unit (MiB) becomes the
// enforcement unit (bytes), so the two cannot drift.
func TestUserQuotaFromConfig(t *testing.T) {
	q := UserQuotaFromConfig(config.Config{
		PerUserContactLimit:          10,
		PerUserNoteLimit:             20,
		PerUserRelationshipEdgeLimit: 30,
		PerUserAttachmentQuotaMB:     5,
	})
	assert.Equal(t, 10, q.Contacts)
	assert.Equal(t, 20, q.Notes)
	assert.Equal(t, 30, q.RelationshipEdges)
	assert.Equal(t, int64(5*1024*1024), q.AttachmentBytes)
	assert.True(t, q.Enabled())

	assert.False(t, UserQuotaFromConfig(config.Config{}).Enabled(), "all-zero limits are the disabled default")
}

// TestUserQuota_DisabledByDefault proves the shipped default (0 = unlimited)
// never blocks a create, so existing single-operator deployments are unaffected
// by the feature existing.
func TestUserQuota_DisabledByDefault(t *testing.T) {
	db := quotaTestDB(t)
	user := mkUser(t, db, "quota-default")
	q := UserQuotaFromConfig(config.Config{})

	for i := 0; i < 3; i++ {
		mkContact(t, db, user.ID, "C")
		mkNote(t, db, user.ID)
	}
	// A disabled attachment quota still permits any size.
	assert.Nil(t, q.CheckContactCreate(db, user.ID))
	assert.Nil(t, q.CheckNoteCreate(db, user.ID))
	assert.Nil(t, q.CheckRelationshipEdgeCreate(db, user.ID))
	assert.Nil(t, q.CheckAttachmentUpload(db, user.ID, 1<<40))
}

// TestUserQuota_ContactLimit pins the row-count semantics: the check refuses at
// the limit, not above it, and the refusal is a typed 507 naming the variable.
func TestUserQuota_ContactLimit(t *testing.T) {
	db := quotaTestDB(t)
	user := mkUser(t, db, "quota-contacts")
	q := UserQuota{Contacts: 2}

	assert.Nil(t, q.CheckContactCreate(db, user.ID), "0/2 is under the limit")
	mkContact(t, db, user.ID, "One")
	assert.Nil(t, q.CheckContactCreate(db, user.ID), "1/2 is under the limit")
	mkContact(t, db, user.ID, "Two")

	err := q.CheckContactCreate(db, user.ID)
	require.NotNil(t, err, "2/2 must be refused")
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, err.Code)
	assert.Contains(t, err.Message, "PER_USER_CONTACT_LIMIT")
}

func TestUserQuota_NoteLimit(t *testing.T) {
	db := quotaTestDB(t)
	user := mkUser(t, db, "quota-notes")
	q := UserQuota{Notes: 1}

	assert.Nil(t, q.CheckNoteCreate(db, user.ID))
	mkNote(t, db, user.ID)
	err := q.CheckNoteCreate(db, user.ID)
	require.NotNil(t, err)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, err.Code)
	assert.Contains(t, err.Message, "PER_USER_NOTE_LIMIT")
}

func TestUserQuota_RelationshipEdgeLimit(t *testing.T) {
	db := quotaTestDB(t)
	user := mkUser(t, db, "quota-edges")
	a := mkContact(t, db, user.ID, "A")
	b := mkContact(t, db, user.ID, "B")
	q := UserQuota{RelationshipEdges: 1}

	assert.Nil(t, q.CheckRelationshipEdgeCreate(db, user.ID))
	mkEdge(t, db, user.ID, a.VCardUID, b.VCardUID, "sibling_of", models.RelationshipStatusConfirmed)
	err := q.CheckRelationshipEdgeCreate(db, user.ID)
	require.NotNil(t, err)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, err.Code)
	assert.Contains(t, err.Message, "PER_USER_RELATIONSHIP_EDGE_LIMIT")
}

// TestUserQuota_AttachmentBytes pins that storage is summed, not counted: a
// single large upload can be refused even when no attachment exists yet, and an
// upload that exactly fills the quota is allowed.
func TestUserQuota_AttachmentBytes(t *testing.T) {
	db := quotaTestDB(t)
	user := mkUser(t, db, "quota-attach")
	q := UserQuota{AttachmentBytes: 100}

	c := mkContact(t, db, user.ID, "Files")
	assert.Nil(t, q.CheckAttachmentUpload(db, user.ID, 100), "exactly filling is allowed")
	mkAttachment(t, db, user.ID, c.VCardUID, 60)

	assert.Nil(t, q.CheckAttachmentUpload(db, user.ID, 40), "60+40 == 100 is allowed")
	err := q.CheckAttachmentUpload(db, user.ID, 41)
	require.NotNil(t, err, "60+41 > 100 must be refused")
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, err.Code)
	assert.Contains(t, err.Message, "PER_USER_ATTACHMENT_QUOTA_MB")

	// A single oversized upload is refused with an empty store.
	user2 := mkUser(t, db, "quota-attach-2")
	err = q.CheckAttachmentUpload(db, user2.ID, 101)
	require.NotNil(t, err, "one upload larger than the whole quota is refused")

	// A store already past its quota (the limit was lowered after files
	// existed) refuses even a zero-byte upload rather than underflowing.
	over := mkUser(t, db, "quota-attach-over")
	overContact := mkContact(t, db, over.ID, "Over")
	mkAttachment(t, db, over.ID, overContact.VCardUID, 101)
	err = q.CheckAttachmentUpload(db, over.ID, 0)
	require.NotNil(t, err, "an already-over-quota store refuses further uploads")
}

// TestUserQuota_IsolationBetweenUsers is the cross-user accounting guarantee
// (issue #558/#950): one user being at their limit must never block another
// user, on any resource. This is the per-user isolation the multi-user decision
// makes release-blocking.
func TestUserQuota_IsolationBetweenUsers(t *testing.T) {
	db := quotaTestDB(t)
	alice := mkUser(t, db, "quota-alice")
	bob := mkUser(t, db, "quota-bob")

	q := UserQuota{Contacts: 1, Notes: 1, RelationshipEdges: 1, AttachmentBytes: 10}

	// Drive alice to the limit on every dimension.
	aliceContact := mkContact(t, db, alice.ID, "Alice Contact")
	mkNote(t, db, alice.ID)
	aliceB := mkContact(t, db, alice.ID, "Alice B")
	// alice already has 2 contacts under a limit of 1 — created directly rather
	// than through the blocked path, which also proves the count is real.
	_ = aliceB
	mkEdge(t, db, alice.ID, aliceContact.VCardUID, aliceB.VCardUID, "sibling_of", models.RelationshipStatusConfirmed)
	mkAttachment(t, db, alice.ID, aliceContact.VCardUID, 10)

	require.NotNil(t, q.CheckContactCreate(db, alice.ID), "alice is over the contact limit")
	require.NotNil(t, q.CheckNoteCreate(db, alice.ID), "alice is over the note limit")
	require.NotNil(t, q.CheckRelationshipEdgeCreate(db, alice.ID), "alice is over the edge limit")
	require.NotNil(t, q.CheckAttachmentUpload(db, alice.ID, 1), "alice is over the attachment limit")

	// Bob has created nothing: every dimension is untouched by alice's usage.
	assert.Nil(t, q.CheckContactCreate(db, bob.ID), "alice's contacts must not count against bob")
	assert.Nil(t, q.CheckNoteCreate(db, bob.ID), "alice's notes must not count against bob")
	assert.Nil(t, q.CheckRelationshipEdgeCreate(db, bob.ID), "alice's edges must not count against bob")
	assert.Nil(t, q.CheckAttachmentUpload(db, bob.ID, 10), "alice's bytes must not count against bob")
}

// TestUserQuota_SoftDeletedRowsDoNotCount pins that a soft-deleted contact
// frees quota (it is the user's undo window, not a live row) while the row
// still exists for restore.
func TestUserQuota_SoftDeletedRowsDoNotCount(t *testing.T) {
	db := quotaTestDB(t)
	user := mkUser(t, db, "quota-softdelete")
	q := UserQuota{Contacts: 1}

	c := mkContact(t, db, user.ID, "Gone")
	require.NotNil(t, q.CheckContactCreate(db, user.ID), "the live row fills the quota")

	require.NoError(t, db.Delete(&c).Error)
	assert.Nil(t, q.CheckContactCreate(db, user.ID), "a soft-deleted row must not count")

	var unscoped int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&unscoped).Error)
	assert.Equal(t, int64(1), unscoped, "the row still exists for undo/restore")
}

// TestUserQuota_DatabaseErrorIsTyped proves a count failure surfaces as a
// database AppError rather than a panic or a silent pass. The table is dropped
// out from under the check to force the failure.
func TestUserQuota_DatabaseErrorIsTyped(t *testing.T) {
	db := quotaTestDB(t)
	user := mkUser(t, db, "quota-dberror")
	require.NoError(t, db.Exec("DROP TABLE notes").Error)

	err := (UserQuota{Notes: 1}).CheckNoteCreate(db, user.ID)
	require.NotNil(t, err)
	assert.Equal(t, apperrors.ErrCodeDatabase, err.Code)
}
