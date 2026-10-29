package services

import (
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2192: users.self_contact_ambient_set is cleared when the Me contact is
// deleted (the pointer is cleared with it) and travels through the trash, so a
// restore returns the pointer with the provenance it had.
func TestSelfContactAmbientSet_DeleteClearsAndRestoreReturnsIt(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", f.userID).
		Update("self_contact_ambient_set", true).Error)

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))

	var user models.User
	require.NoError(t, f.db.First(&user, f.userID).Error)
	assert.Nil(t, user.SelfContactVCardUID)
	assert.False(t, user.SelfContactAmbientSet, "a cleared pointer carries no provenance")

	_, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow.Add(time.Hour))
	require.NoError(t, err)

	require.NoError(t, f.db.First(&user, f.userID).Error)
	require.NotNil(t, user.SelfContactVCardUID)
	assert.True(t, user.SelfContactAmbientSet, "restore returns the pointer with its provenance")
}

func TestSelfContactAmbientSet_RestoreWithoutProvenanceStaysFalse(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	_, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow.Add(time.Hour))
	require.NoError(t, err)

	var user models.User
	require.NoError(t, f.db.First(&user, f.userID).Error)
	assert.False(t, user.SelfContactAmbientSet)
}
