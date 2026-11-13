package services

import (
	"testing"

	"gorm.io/gorm"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2562: a contact delete soft-deletes the contact's notes, so it must
// also revoke the standing-share grants on those notes (ADR 0042 decision 10).
// Otherwise a trash restore re-arms access the owner already ended.

func seedNoteWithGrant(t *testing.T, f grantRevokeFixture, contact models.Contact, content string, status string) (models.Note, models.ShareGrant) {
	t.Helper()
	note := models.Note{UserID: f.owner.ID, ContactID: &contact.ID, Content: content, Date: f.nowRef}
	require.NoError(t, f.db.Create(&note).Error)
	require.NotEmpty(t, note.UUID)
	return note, f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityNote, note.UUID, status)
}

func TestContactDelete_RevokesGrantsOnTheCascadedNotes(t *testing.T) {
	f := newGrantRevokeFixture(t)
	contact := models.Contact{UserID: f.owner.ID, Firstname: "Del"}
	require.NoError(t, f.db.Create(&contact).Error)
	keep := models.Contact{UserID: f.owner.ID, Firstname: "Keep"}
	require.NoError(t, f.db.Create(&keep).Error)

	_, active := seedNoteWithGrant(t, f, contact, "gone", models.ShareGrantStatusActive)
	_, pending := seedNoteWithGrant(t, f, contact, "gone2", models.ShareGrantStatusPending)
	_, kept := seedNoteWithGrant(t, f, keep, "stays", models.ShareGrantStatusActive)
	loose := models.Note{UserID: f.owner.ID, Content: "no contact", Date: f.nowRef}
	require.NoError(t, f.db.Create(&loose).Error)
	looseGrant := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityNote, loose.UUID, models.ShareGrantStatusActive)

	require.NoError(t, DeleteContact(f.db, contact, f.owner.ID, f.nowRef))

	for _, g := range []models.ShareGrant{active, pending} {
		got := f.reload(t, g.ID)
		assert.Equal(t, models.ShareGrantStatusRevoked, got.Status)
		require.NotNil(t, got.RevokedReason)
		assert.Equal(t, models.ShareGrantReasonEntityDeleted, *got.RevokedReason)
	}
	assert.Equal(t, models.ShareGrantStatusActive, f.reload(t, kept.ID).Status, "another contact's note grant is untouched")
	assert.Equal(t, models.ShareGrantStatusActive, f.reload(t, looseGrant.ID).Status, "a contactless note grant is untouched")
}

func TestContactDelete_NoteGrantsStayRevokedAfterRestoreAndAreReported(t *testing.T) {
	f := newGrantRevokeFixture(t)
	contact := models.Contact{UserID: f.owner.ID, Firstname: "Del"}
	require.NoError(t, f.db.Create(&contact).Error)
	_, g := seedNoteWithGrant(t, f, contact, "shared body", models.ShareGrantStatusActive)

	require.NoError(t, DeleteContact(f.db, contact, f.owner.ID, f.nowRef))
	res, err := RestoreContact(f.db, f.owner.ID, contact.ID, f.nowRef.Add(1))
	require.NoError(t, err)

	assert.Equal(t, models.ShareGrantStatusRevoked, f.reload(t, g.ID).Status, "restore must not re-arm note access")
	require.Len(t, res.RevokedGrants, 1, "the restore response lists the revoked note grant")
	assert.Equal(t, g.ID, res.RevokedGrants[0].ID)
	assert.Equal(t, f.rcpt[0].ID, res.RevokedGrants[0].ToUserID)
}

func TestContactMerge_NoteGrantsFollowTheNotesToTheKeeper(t *testing.T) {
	f := newGrantRevokeFixture(t)
	keeper := models.Contact{UserID: f.owner.ID, Firstname: "Keeper"}
	loser := models.Contact{UserID: f.owner.ID, Firstname: "Loser"}
	require.NoError(t, f.db.Create(&keeper).Error)
	require.NoError(t, f.db.Create(&loser).Error)
	note, g := seedNoteWithGrant(t, f, loser, "moves", models.ShareGrantStatusActive)

	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		if _, err := RepointContactAssociations(tx, f.owner.ID, &keeper, &loser, nil, map[string]string{}); err != nil {
			return err
		}
		return DeleteContactAssociations(tx, loser, f.owner.ID, f.nowRef)
	}))

	var moved models.Note
	require.NoError(t, f.db.First(&moved, note.ID).Error)
	require.NotNil(t, moved.ContactID)
	assert.Equal(t, keeper.ID, *moved.ContactID, "the note survives under the keeper")
	assert.Equal(t, models.ShareGrantStatusActive, f.reload(t, g.ID).Status, "a surviving note keeps its grant")
}

// A contact trashed before the fix still carries live note grants; restoring
// it must end them rather than re-arm access.
func TestRestoreContact_RevokesNoteGrantsLeftLiveByAnEarlierDelete(t *testing.T) {
	f := newGrantRevokeFixture(t)
	contact := models.Contact{UserID: f.owner.ID, Firstname: "Old"}
	require.NoError(t, f.db.Create(&contact).Error)
	_, g := seedNoteWithGrant(t, f, contact, "legacy", models.ShareGrantStatusActive)
	require.NoError(t, DeleteContact(f.db, contact, f.owner.ID, f.nowRef))
	require.NoError(t, f.db.Table("share_grants").Where("id = ?", g.ID).
		UpdateColumns(map[string]any{"status": models.ShareGrantStatusActive, "revoked_reason": nil, "revoked_at": nil}).Error)

	res, err := RestoreContact(f.db, f.owner.ID, contact.ID, f.nowRef.Add(1))
	require.NoError(t, err)
	assert.Equal(t, models.ShareGrantStatusRevoked, f.reload(t, g.ID).Status)
	require.Len(t, res.RevokedGrants, 1)
}
