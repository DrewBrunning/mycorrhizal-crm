package services

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupCadenceConflictTestDB opens a real migrated schema (the CadencePolicy
// table's partial unique index comes from migration 000002, which AutoMigrate
// cannot see) and creates a test user.
func setupCadenceConflictTestDB(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "cadenceuser", Password: "password123!A", Email: "cadence@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

func createCadencePolicy(t *testing.T, db *gorm.DB, userID uint, entityID string, interval int, qualifying []string) models.CadencePolicy {
	t.Helper()
	p := models.CadencePolicy{UserID: userID, EntityID: entityID, TargetIntervalDays: interval, QualifyingTypes: qualifying}
	require.NoError(t, db.Create(&p).Error)
	return p
}

func TestComputeCadencePolicyConflict(t *testing.T) {
	db, user := setupCadenceConflictTestDB(t)

	keeperUID := "00000000-0000-4000-8000-000000000001"
	loserUID := "00000000-0000-4000-8000-000000000002"

	// Neither side has a policy: no conflict.
	conflict, err := ComputeCadencePolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict)

	// Only the keeper has one: no conflict (silent adoption).
	createCadencePolicy(t, db, user.ID, keeperUID, 30, nil)
	conflict, err = ComputeCadencePolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict)

	// Both sides agree: no conflict (identical summaries).
	createCadencePolicy(t, db, user.ID, loserUID, 30, nil)
	conflict, err = ComputeCadencePolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict, "two identical policies must not surface as a conflict")
}

func TestComputeCadencePolicyConflict_GenuineDifference(t *testing.T) {
	db, user := setupCadenceConflictTestDB(t)

	keeperUID := "00000000-0000-4000-8000-000000000011"
	loserUID := "00000000-0000-4000-8000-000000000012"
	createCadencePolicy(t, db, user.ID, keeperUID, 30, nil)
	createCadencePolicy(t, db, user.ID, loserUID, 60, nil)

	conflict, err := ComputeCadencePolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	require.NotNil(t, conflict)
	assert.Equal(t, cadencePolicyConflictField, conflict.Field)
	assert.Equal(t, "Stay-in-touch cadence", conflict.Label)
	assert.Equal(t, "Every 30 days", conflict.KeeperValue)
	assert.Equal(t, "Every 60 days", conflict.LoserValue)
}

func TestComputeCadencePolicyConflict_SameIntervalDifferentQualifyingOrder(t *testing.T) {
	db, user := setupCadenceConflictTestDB(t)

	// Same interval and same qualifying set, different insertion order: the
	// summaries must compare equal (QualifyingTypes is sorted before
	// formatting), so this is NOT a conflict.
	keeperUID := "00000000-0000-4000-8000-000000000021"
	loserUID := "00000000-0000-4000-8000-000000000022"
	createCadencePolicy(t, db, user.ID, keeperUID, 14, []string{"call", "visit"})
	createCadencePolicy(t, db, user.ID, loserUID, 14, []string{"visit", "call"})

	conflict, err := ComputeCadencePolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	assert.Nil(t, conflict, "the same qualifying-type set in a different order must not conflict")
}

func TestComputeCadencePolicyConflict_QualifyingTypesDiffer(t *testing.T) {
	db, user := setupCadenceConflictTestDB(t)

	keeperUID := "00000000-0000-4000-8000-000000000031"
	loserUID := "00000000-0000-4000-8000-000000000032"
	createCadencePolicy(t, db, user.ID, keeperUID, 30, []string{"call"})
	createCadencePolicy(t, db, user.ID, loserUID, 30, []string{"call", "visit"})

	conflict, err := ComputeCadencePolicyConflict(db, user.ID, keeperUID, loserUID)
	require.NoError(t, err)
	require.NotNil(t, conflict)
	assert.Equal(t, "Every 30 days (call)", conflict.KeeperValue)
	assert.Equal(t, "Every 30 days (call, visit)", conflict.LoserValue)
}

func TestFormatCadencePolicySummary(t *testing.T) {
	cases := []struct {
		name     string
		policy   models.CadencePolicy
		expected string
	}{
		{name: "no qualifying types", policy: models.CadencePolicy{TargetIntervalDays: 30}, expected: "Every 30 days"},
		{name: "single qualifying type", policy: models.CadencePolicy{TargetIntervalDays: 14, QualifyingTypes: []string{"call"}}, expected: "Every 14 days (call)"},
		{name: "qualifying types sorted", policy: models.CadencePolicy{TargetIntervalDays: 7, QualifyingTypes: []string{"visit", "call"}}, expected: "Every 7 days (call, visit)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, formatCadencePolicySummary(tc.policy))
		})
	}
}

func TestDedupStrings(t *testing.T) {
	assert.Empty(t, dedupStrings(nil))
	assert.Equal(t, []string{"a", "b", "c"}, dedupStrings([]string{"a", "b", "a", "c", "b", "a"}))
	assert.Equal(t, []string{"a"}, dedupStrings([]string{"a", "a", "a"}))
	assert.Equal(t, []string{"", "a"}, dedupStrings([]string{"", "a", "", "a"}))
}

func TestRemoveString(t *testing.T) {
	assert.Empty(t, removeString(nil, "x"))
	assert.Equal(t, []string{"a", "c"}, removeString([]string{"a", "b", "b", "c"}, "b"))
	assert.Equal(t, []string{"a", "b"}, removeString([]string{"a", "b"}, "zzz"))
	assert.Empty(t, removeString([]string{"x", "x"}, "x"))
}
