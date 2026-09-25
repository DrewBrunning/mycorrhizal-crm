package services

import (
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A failed "Me" contact insert must abort the whole transaction: no pointer
// written, error surfaced — the orphan-avoidance contract in EnsureSelfContact.
func TestEnsureSelfContact_ContactCreateFailureLeavesUserUnpointed(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	user := models.User{Username: "selfless", Password: "password123!A", Email: "selfless@example.com"}
	require.NoError(t, db.Create(&user).Error)

	dbtest.HideTable(t, db, "contacts")
	require.Error(t, EnsureSelfContact(db, &user))
	assert.Nil(t, user.SelfContactVCardUID)

	var reloaded models.User
	require.NoError(t, db.First(&reloaded, user.ID).Error)
	assert.Nil(t, reloaded.SelfContactVCardUID)
}
