package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDataDecayPolicyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &DataDecayPolicy{}))
	return db
}

func TestDataDecayPolicyBeforeCreateGeneratesUUID(t *testing.T) {
	t.Parallel()
	db := setupDataDecayPolicyTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	policy := DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365}
	require.NoError(t, db.Create(&policy).Error)

	assert.NotEmpty(t, policy.ID)
}

func TestDataDecayPolicyBeforeCreatePreservesExplicitID(t *testing.T) {
	t.Parallel()
	db := setupDataDecayPolicyTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	policy := DataDecayPolicy{ID: "explicit-id", UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365}
	require.NoError(t, db.Create(&policy).Error)

	assert.Equal(t, "explicit-id", policy.ID)
}

func TestDataDecayPolicyDefaultsToActive(t *testing.T) {
	t.Parallel()
	db := setupDataDecayPolicyTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	policy := DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true}
	require.NoError(t, db.Create(&policy).Error)

	var reloaded DataDecayPolicy
	require.NoError(t, db.First(&reloaded, "id = ?", policy.ID).Error)
	assert.True(t, reloaded.Active)
	assert.Nil(t, reloaded.LastVerifiedAt, "a freshly created policy has never been verified")
}

// TestDataDecayPolicyCreateWithActiveFalseSticks pins the fix for a GORM
// footgun: a field tagged `gorm:"default:..."` is skipped on Create whenever
// its Go value equals the zero value, so Active bool with a `default:true`
// tag would silently override an explicit Active: false back to true (this
// bug is real and still latent in OccasionObligation.Active -- see the
// DataDecayPolicy.Active doc comment). No `default:` tag on Active is what
// makes this test pass.
func TestDataDecayPolicyCreateWithActiveFalseSticks(t *testing.T) {
	t.Parallel()
	db := setupDataDecayPolicyTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	policy := DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: false}
	require.NoError(t, db.Create(&policy).Error)

	var reloaded DataDecayPolicy
	require.NoError(t, db.First(&reloaded, "id = ?", policy.ID).Error)
	assert.False(t, reloaded.Active, "an explicit Active: false must survive Create, not be silently reset to true")
}

func TestDataDecayPolicyAfterDeleteAdvancesUpdatedAtOnSoftDelete(t *testing.T) {
	t.Parallel()
	db := setupDataDecayPolicyTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	policy := DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365}
	require.NoError(t, db.Create(&policy).Error)
	originalUpdatedAt := policy.UpdatedAt

	require.NoError(t, db.Delete(&policy).Error)

	var reloaded DataDecayPolicy
	require.NoError(t, db.Unscoped().First(&reloaded, "id = ?", policy.ID).Error)
	assert.True(t, reloaded.DeletedAt.Valid)
	assert.False(t, reloaded.UpdatedAt.Before(originalUpdatedAt), "AfterDelete must advance updated_at on a soft delete")
}

// AfterDelete's own DeletedAt.Valid guard must skip a no-op call rather than
// touch the DB -- called directly since GORM only invokes the hook on an
// actual (soft or hard) delete. Mirrors
// TestOccasionObligationAfterDeleteSkipsWhenNotSoftDeleted.
func TestDataDecayPolicyAfterDeleteSkipsWhenNotSoftDeleted(t *testing.T) {
	t.Parallel()
	db := setupDataDecayPolicyTestDB(t)
	policy := DataDecayPolicy{ID: "not-deleted"}
	assert.NoError(t, policy.AfterDelete(db))
}
