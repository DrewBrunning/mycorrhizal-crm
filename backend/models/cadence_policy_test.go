package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCadencePolicyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &CadencePolicy{}))
	return db
}

func TestCadencePolicyBeforeCreateGeneratesUUID(t *testing.T) {
	db := setupCadencePolicyTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	policy := CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&policy).Error)

	assert.NotEmpty(t, policy.ID)
}

func TestCadencePolicyBeforeCreatePreservesExplicitID(t *testing.T) {
	db := setupCadencePolicyTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	policy := CadencePolicy{ID: "explicit-id", UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&policy).Error)

	assert.Equal(t, "explicit-id", policy.ID)
}

// Qualifies must delegate to Activity.Qualifying() (the single repo-wide
// definition) AND apply the policy's qualifying_types filter. photo is the
// only globally non-qualifying type — it can never be re-admitted by listing
// it in a policy.
func TestCadencePolicyQualifies_DelegatesToActivityQualifying(t *testing.T) {
	policy := CadencePolicy{TargetIntervalDays: 30, QualifyingTypes: nil}

	assert.True(t, policy.Qualifies(&Activity{Type: InteractionTypeCall}))
	assert.True(t, policy.Qualifies(&Activity{Type: InteractionTypeVisit}))
	assert.True(t, policy.Qualifies(&Activity{Type: InteractionTypePhoto}) == false,
		"photo is globally non-qualifying regardless of the empty filter")

	photoExplicit := CadencePolicy{TargetIntervalDays: 30, QualifyingTypes: []string{InteractionTypePhoto}}
	assert.False(t, photoExplicit.Qualifies(&Activity{Type: InteractionTypePhoto}),
		"listing a non-qualifying type must not re-admit it")
}

func TestCadencePolicyQualifies_Filter(t *testing.T) {
	policy := CadencePolicy{TargetIntervalDays: 30, QualifyingTypes: []string{InteractionTypeCall, InteractionTypeVisit}}

	assert.True(t, policy.Qualifies(&Activity{Type: InteractionTypeCall}))
	assert.True(t, policy.Qualifies(&Activity{Type: InteractionTypeVisit}))
	assert.False(t, policy.Qualifies(&Activity{Type: InteractionTypeMeal}),
		"a qualifying-but-unlisted type is filtered out")
}

func TestCadencePolicyQualifies_EmptyMeansAllDefaultQualifyingTypes(t *testing.T) {
	policy := CadencePolicy{TargetIntervalDays: 30}

	assert.True(t, policy.Qualifies(&Activity{Type: InteractionTypeMessage}),
		"an empty qualifying_types list must not filter — every default-qualifying type passes")
}
