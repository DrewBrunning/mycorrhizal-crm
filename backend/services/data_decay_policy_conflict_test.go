package services

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupDataDecayConflictTestDB opens a real migrated schema (the
// DataDecayPolicy table's partial unique index comes from migration 000064,
// which AutoMigrate cannot see) and creates a test user. Mirrors
// setupCadenceConflictTestDB.
func setupDataDecayConflictTestDB(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "decayuser", Password: "password123!A", Email: "decay-conflict@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

func createDataDecayPolicy(t *testing.T, db *gorm.DB, userID uint, entityID string, interval int, active bool) models.DataDecayPolicy {
	t.Helper()
	p := models.DataDecayPolicy{UserID: userID, EntityID: entityID, IntervalDays: interval, Active: active}
	require.NoError(t, db.Create(&p).Error)
	return p
}

func TestComputeDataDecayPolicyConflict(t *testing.T) {
	db, user := setupDataDecayConflictTestDB(t)

	keeperUID := "00000000-0000-4000-8000-000000000001"
	loserUID := "00000000-0000-4000-8000-000000000002"

	// Neither side has a policy: no conflict.
	conflict, err := ComputeDataDecayPolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict)

	// Only the keeper has one: no conflict (silent adoption).
	createDataDecayPolicy(t, db, user.ID, keeperUID, 365, true)
	conflict, err = ComputeDataDecayPolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict)

	// Both sides agree: no conflict (identical summaries).
	createDataDecayPolicy(t, db, user.ID, loserUID, 365, true)
	conflict, err = ComputeDataDecayPolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict, "two identical policies must not surface as a conflict")
}

func TestComputeDataDecayPolicyConflict_GenuineDifference(t *testing.T) {
	db, user := setupDataDecayConflictTestDB(t)

	keeperUID := "00000000-0000-4000-8000-000000000011"
	loserUID := "00000000-0000-4000-8000-000000000012"
	createDataDecayPolicy(t, db, user.ID, keeperUID, 30, true)
	createDataDecayPolicy(t, db, user.ID, loserUID, 90, true)

	conflict, err := ComputeDataDecayPolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	require.NotNil(t, conflict)
	assert.Equal(t, dataDecayPolicyConflictField, conflict.Field)
	assert.Equal(t, "Data verification interval", conflict.Label)
	assert.Equal(t, "Every 30 days (active)", conflict.KeeperValue)
	assert.Equal(t, "Every 90 days (active)", conflict.LoserValue)
}

func TestComputeDataDecayPolicyConflict_ActiveStateDiffers(t *testing.T) {
	db, user := setupDataDecayConflictTestDB(t)

	keeperUID := "00000000-0000-4000-8000-000000000021"
	loserUID := "00000000-0000-4000-8000-000000000022"
	createDataDecayPolicy(t, db, user.ID, keeperUID, 30, true)
	createDataDecayPolicy(t, db, user.ID, loserUID, 30, false)

	conflict, err := ComputeDataDecayPolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	require.NotNil(t, conflict, "same interval but different active state must still conflict")
	assert.Equal(t, "Every 30 days (active)", conflict.KeeperValue)
	assert.Equal(t, "Every 30 days (paused)", conflict.LoserValue)
}

// TestComputeDataDecayPolicyConflict_LastVerifiedAtNotPartOfSummary pins a
// deliberate design decision: last_verified_at is a history fact, not a rule
// the user is choosing between, so two policies with the same interval and
// active state must never conflict just because one was verified more
// recently than the other.
func TestComputeDataDecayPolicyConflict_LastVerifiedAtNotPartOfSummary(t *testing.T) {
	db, user := setupDataDecayConflictTestDB(t)

	keeperUID := "00000000-0000-4000-8000-000000000031"
	loserUID := "00000000-0000-4000-8000-000000000032"
	keeper := createDataDecayPolicy(t, db, user.ID, keeperUID, 30, true)
	loser := createDataDecayPolicy(t, db, user.ID, loserUID, 30, true)

	require.NoError(t, db.Model(&keeper).Update("last_verified_at", "2026-01-01").Error)
	require.NoError(t, db.Model(&loser).Update("last_verified_at", "2026-06-01").Error)

	conflict, err := ComputeDataDecayPolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict, "differing last_verified_at alone must not surface as a conflict")
}

func TestFormatDataDecayPolicySummary(t *testing.T) {
	cases := []struct {
		name     string
		policy   models.DataDecayPolicy
		expected string
	}{
		{name: "active", policy: models.DataDecayPolicy{IntervalDays: 365, Active: true}, expected: "Every 365 days (active)"},
		{name: "paused", policy: models.DataDecayPolicy{IntervalDays: 90, Active: false}, expected: "Every 90 days (paused)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, formatDataDecayPolicySummary(tc.policy))
		})
	}
}
