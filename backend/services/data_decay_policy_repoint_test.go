package services

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRepointDataDecayPolicy exercises repointDataDecayPolicy directly
// (package services, so the unexported function is reachable), covering the
// cases ComputeDataDecayPolicyConflict's own tests don't: the actual
// row-level outcome once a resolution is applied, not just what conflict is
// surfaced. Mirrors the shape of TestComputeDataDecayPolicyConflict's setup
// helpers.
func TestRepointDataDecayPolicy(t *testing.T) {
	t.Run("only the keeper has a policy: untouched, no-op", func(t *testing.T) {
		db, user := setupDataDecayConflictTestDB(t)
		keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
		loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
		require.NoError(t, db.Create(&keeper).Error)
		require.NoError(t, db.Create(&loser).Error)
		keeperPolicy := createDataDecayPolicy(t, db, user.ID, keeper.VCardUID, 30, true)

		require.NoError(t, repointDataDecayPolicy(db, user.ID, &keeper, &loser, nil))

		var stillThere models.DataDecayPolicy
		require.NoError(t, db.Where("id = ?", keeperPolicy.ID).First(&stillThere).Error)
		assert.Equal(t, keeper.VCardUID, stillThere.EntityID, "the keeper's own policy must not be moved or altered")
		assert.Equal(t, 30, stillThere.IntervalDays)
	})

	t.Run("only the loser has a policy: silently adopted onto the keeper", func(t *testing.T) {
		db, user := setupDataDecayConflictTestDB(t)
		keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
		loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
		require.NoError(t, db.Create(&keeper).Error)
		require.NoError(t, db.Create(&loser).Error)
		loserPolicy := createDataDecayPolicy(t, db, user.ID, loser.VCardUID, 45, true)

		require.NoError(t, repointDataDecayPolicy(db, user.ID, &keeper, &loser, nil))

		var adopted models.DataDecayPolicy
		require.NoError(t, db.Where("id = ?", loserPolicy.ID).First(&adopted).Error)
		assert.Equal(t, keeper.VCardUID, adopted.EntityID, "the loser's policy must be repointed onto the keeper")
	})

	t.Run("both sides identical: loser's row is dropped, keeper's survives unchanged", func(t *testing.T) {
		db, user := setupDataDecayConflictTestDB(t)
		keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
		loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
		require.NoError(t, db.Create(&keeper).Error)
		require.NoError(t, db.Create(&loser).Error)
		keeperPolicy := createDataDecayPolicy(t, db, user.ID, keeper.VCardUID, 60, true)
		loserPolicy := createDataDecayPolicy(t, db, user.ID, loser.VCardUID, 60, true)

		require.NoError(t, repointDataDecayPolicy(db, user.ID, &keeper, &loser, nil))

		var survivors []models.DataDecayPolicy
		require.NoError(t, db.Where("user_id = ?", user.ID).Find(&survivors).Error)
		require.Len(t, survivors, 1, "exactly one policy must survive an identical-pair merge")
		assert.Equal(t, keeperPolicy.ID, survivors[0].ID, "the keeper's own row must be the survivor, not a new one")
		assert.Equal(t, keeper.VCardUID, survivors[0].EntityID)

		var loserGone int64
		require.NoError(t, db.Model(&models.DataDecayPolicy{}).Where("id = ?", loserPolicy.ID).Count(&loserGone).Error)
		assert.EqualValues(t, 0, loserGone)
	})

	t.Run("both differ, resolved toward the keeper: loser dropped, keeper untouched", func(t *testing.T) {
		db, user := setupDataDecayConflictTestDB(t)
		keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
		loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
		require.NoError(t, db.Create(&keeper).Error)
		require.NoError(t, db.Create(&loser).Error)
		keeperPolicy := createDataDecayPolicy(t, db, user.ID, keeper.VCardUID, 10, true)
		loserPolicy := createDataDecayPolicy(t, db, user.ID, loser.VCardUID, 20, true)

		resolutions := map[string]string{dataDecayPolicyConflictField: "Every 10 days (active)"}
		require.NoError(t, repointDataDecayPolicy(db, user.ID, &keeper, &loser, resolutions))

		var survivor models.DataDecayPolicy
		require.NoError(t, db.Where("id = ?", keeperPolicy.ID).First(&survivor).Error)
		assert.Equal(t, 10, survivor.IntervalDays)

		var loserGone int64
		require.NoError(t, db.Model(&models.DataDecayPolicy{}).Where("id = ?", loserPolicy.ID).Count(&loserGone).Error)
		assert.EqualValues(t, 0, loserGone)
	})

	t.Run("both differ, unresolved: rejected, neither row touched", func(t *testing.T) {
		db, user := setupDataDecayConflictTestDB(t)
		keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
		loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
		require.NoError(t, db.Create(&keeper).Error)
		require.NoError(t, db.Create(&loser).Error)
		createDataDecayPolicy(t, db, user.ID, keeper.VCardUID, 10, true)
		createDataDecayPolicy(t, db, user.ID, loser.VCardUID, 20, true)

		err := repointDataDecayPolicy(db, user.ID, &keeper, &loser, nil)
		require.Error(t, err)

		var count int64
		require.NoError(t, db.Model(&models.DataDecayPolicy{}).Where("user_id = ?", user.ID).Count(&count).Error)
		assert.EqualValues(t, 2, count, "an unresolved conflict must leave both rows in place")
	})

	t.Run("both differ, resolution matches neither side: rejected", func(t *testing.T) {
		db, user := setupDataDecayConflictTestDB(t)
		keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
		loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
		require.NoError(t, db.Create(&keeper).Error)
		require.NoError(t, db.Create(&loser).Error)
		createDataDecayPolicy(t, db, user.ID, keeper.VCardUID, 10, true)
		createDataDecayPolicy(t, db, user.ID, loser.VCardUID, 20, true)

		resolutions := map[string]string{dataDecayPolicyConflictField: "Every 999 days (active)"}
		err := repointDataDecayPolicy(db, user.ID, &keeper, &loser, resolutions)
		require.Error(t, err)

		var count int64
		require.NoError(t, db.Model(&models.DataDecayPolicy{}).Where("user_id = ?", user.ID).Count(&count).Error)
		assert.EqualValues(t, 2, count)
	})
}

// TestRepointDataDecayPolicy_DBErrors fault-injects by hiding the
// data_decay_policies table (dbtest.HideTable) so the initial lookups in
// repointDataDecayPolicy and ComputeDataDecayPolicyConflict hit a real "no
// such table" error, not gorm.ErrRecordNotFound -- pinning that a genuine DB
// failure propagates instead of being swallowed as "nothing to repoint."
func TestRepointDataDecayPolicy_DBErrors(t *testing.T) {
	db, user := setupDataDecayConflictTestDB(t)
	keeper := models.Contact{UserID: user.ID, Firstname: "Keeper"}
	loser := models.Contact{UserID: user.ID, Firstname: "Loser"}
	require.NoError(t, db.Create(&keeper).Error)
	require.NoError(t, db.Create(&loser).Error)

	dbtest.HideTable(t, db, "data_decay_policies")

	err := repointDataDecayPolicy(db, user.ID, &keeper, &loser, nil)
	require.Error(t, err)

	_, err = ComputeDataDecayPolicyConflict(db, user.ID, keeper.VCardUID, loser.VCardUID)
	require.Error(t, err)
}
