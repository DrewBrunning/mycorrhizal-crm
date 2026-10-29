package services

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func selfApplyFixture(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "applyme", Password: "password123!A", Email: "applyme@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

func selfApplyContact(t *testing.T, db *gorm.DB, userID uint, name string, ambient bool) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: name, Ambient: ambient}
	require.NoError(t, db.Create(&c).Error)
	return c
}

func TestApplySelfContact_UnknownUserErrors(t *testing.T) {
	db, _ := selfApplyFixture(t)
	require.Error(t, ApplySelfContact(db, 99999, ""))
}

// A pointer that outlived its contact (hard-gone row) is not an error: the
// pointer simply moves and no provenance is claimed.
func TestApplySelfContact_MissingContactsAreTolerated(t *testing.T) {
	db, user := selfApplyFixture(t)
	ghost := "11111111-2222-4333-8444-555555555555"
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{"self_contact_vcard_uid": ghost, "self_contact_ambient_set": true}).Error)

	require.NoError(t, ApplySelfContact(db, user.ID, ghost), "re-marking a vanished Me")
	require.NoError(t, ApplySelfContact(db, user.ID, ""), "clearing a vanished Me")

	var got models.User
	require.NoError(t, db.First(&got, user.ID).Error)
	assert.Nil(t, got.SelfContactVCardUID)
	assert.False(t, got.SelfContactAmbientSet)
}

func TestApplySelfContact_ContactLookupErrorsAbort(t *testing.T) {
	cases := map[string]func(t *testing.T, db *gorm.DB, user models.User, a, b models.Contact){
		"marking a new Me": func(t *testing.T, db *gorm.DB, user models.User, a, b models.Contact) {
			require.Error(t, ApplySelfContact(db, user.ID, a.VCardUID))
		},
		"re-marking the current Me": func(t *testing.T, db *gorm.DB, user models.User, a, b models.Contact) {
			require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).
				Update("self_contact_vcard_uid", a.VCardUID).Error)
			require.Error(t, ApplySelfContact(db, user.ID, a.VCardUID))
		},
		"un-meing a contact marking made ambient": func(t *testing.T, db *gorm.DB, user models.User, a, b models.Contact) {
			require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).
				Updates(map[string]any{"self_contact_vcard_uid": a.VCardUID, "self_contact_ambient_set": true}).Error)
			require.Error(t, ApplySelfContact(db, user.ID, b.VCardUID))
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			db, user := selfApplyFixture(t)
			a := selfApplyContact(t, db, user.ID, "A", false)
			b := selfApplyContact(t, db, user.ID, "B", false)
			dbtest.HideTable(t, db, "contacts")
			run(t, db, user, a, b)
		})
	}
}

func TestApplySelfContact_WritesAreAllOrNothingInATransaction(t *testing.T) {
	db, user := selfApplyFixture(t)
	a := selfApplyContact(t, db, user.ID, "A", false)

	// Fail after the contact write by making the user write impossible.
	require.NoError(t, db.Exec(`CREATE TRIGGER block_self_write BEFORE UPDATE OF self_contact_vcard_uid ON users
		BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)

	err := db.Transaction(func(tx *gorm.DB) error { return ApplySelfContact(tx, user.ID, a.VCardUID) })
	require.Error(t, err)

	var got models.Contact
	require.NoError(t, db.First(&got, a.ID).Error)
	assert.False(t, got.Ambient, "the ambient write rolled back with the failed pointer write")
}

func TestClearSelfAmbientProvenanceIfMe_OnlyTouchesTheMeContact(t *testing.T) {
	db, user := selfApplyFixture(t)
	a := selfApplyContact(t, db, user.ID, "A", false)
	b := selfApplyContact(t, db, user.ID, "B", false)
	require.NoError(t, ApplySelfContact(db, user.ID, a.VCardUID))

	require.NoError(t, ClearSelfAmbientProvenanceIfMe(db, user.ID, b.VCardUID))
	var got models.User
	require.NoError(t, db.First(&got, user.ID).Error)
	assert.True(t, got.SelfContactAmbientSet, "editing some other contact leaves the provenance")

	require.NoError(t, ClearSelfAmbientProvenanceIfMe(db, user.ID, a.VCardUID))
	require.NoError(t, db.First(&got, user.ID).Error)
	assert.False(t, got.SelfContactAmbientSet)
}
