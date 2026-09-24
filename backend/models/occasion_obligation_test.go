package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Direct models-package coverage for occasion_obligation.go's three GORM
// hooks (ADR 0024, issue #387) -- every other "occasion" test lives in
// controllers/ or services/, which exercise these hooks only indirectly
// through the ORM, and Go's per-package coverage mode doesn't attribute that
// back to this package. AutoMigrate here (not dbtest.New) mirrors
// gift_test.go's own setup for the same reason: models can't import dbtest
// without an import cycle (dbtest -> database -> models).
func setupOccasionObligationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}, &OccasionObligation{}))
	return db
}

func TestOccasionObligationTableName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "occasion_obligations", OccasionObligation{}.TableName())
}

func TestOccasionObligationBeforeCreateGeneratesUUID(t *testing.T) {
	t.Parallel()
	db := setupOccasionObligationTestDB(t)
	user := User{Username: "occ-model-tester", Password: "x", Email: "occ-model-tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	obligation := OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Sensitivity: RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	assert.NotEmpty(t, obligation.ID)
}

func TestOccasionObligationBeforeCreatePreservesExplicitID(t *testing.T) {
	t.Parallel()
	db := setupOccasionObligationTestDB(t)
	user := User{Username: "occ-model-tester2", Password: "x", Email: "occ-model-tester2@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	obligation := OccasionObligation{
		ID: "explicit-id", UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Sensitivity: RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	assert.Equal(t, "explicit-id", obligation.ID)
}

func TestOccasionObligationAfterDeleteAdvancesUpdatedAtOnSoftDelete(t *testing.T) {
	t.Parallel()
	db := setupOccasionObligationTestDB(t)
	user := User{Username: "occ-model-tester3", Password: "x", Email: "occ-model-tester3@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	obligation := OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Sensitivity: RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)
	originalUpdatedAt := obligation.UpdatedAt

	require.NoError(t, db.Delete(&obligation).Error)

	var reloaded OccasionObligation
	require.NoError(t, db.Unscoped().First(&reloaded, "id = ?", obligation.ID).Error)
	assert.True(t, reloaded.DeletedAt.Valid)
	assert.False(t, reloaded.UpdatedAt.Before(originalUpdatedAt), "AfterDelete must advance updated_at on a soft delete")
}

// GORM skips a struct field on Create whenever its Go value equals the zero
// value AND the field's tag carries a `default:` clause -- it defers to the
// SQL column default instead. Active's zero value is false, so a
// `default:true` tag on Active silently persisted true for every
// Active: false Create. Pinned here the same way TestDataDecayPolicy... (the
// same-shaped bug on DataDecayPolicy.Active) does: create explicitly false,
// reload, assert it stuck.
func TestOccasionObligationCreateWithActiveFalseSticks(t *testing.T) {
	t.Parallel()
	db := setupOccasionObligationTestDB(t)
	user := User{Username: "occ-model-tester4", Password: "x", Email: "occ-model-tester4@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	obligation := OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		Sensitivity: RelationshipSensitivityNormal,
		Active:      false,
	}
	require.NoError(t, db.Create(&obligation).Error)

	var reloaded OccasionObligation
	require.NoError(t, db.First(&reloaded, "id = ?", obligation.ID).Error)
	assert.False(t, reloaded.Active, "Active: false must survive db.Create, not silently become true via the column default")
}

// AfterDelete's own DeletedAt.Valid guard (occasion_obligation.go) must skip
// a no-op call rather than touch the DB -- called directly since GORM only
// invokes the hook on an actual (soft or hard) delete, and a hard delete on
// this table can't happen through the ORM in practice (no Unscoped().Delete
// call site exists for this entity).
func TestOccasionObligationAfterDeleteSkipsWhenNotSoftDeleted(t *testing.T) {
	t.Parallel()
	db := setupOccasionObligationTestDB(t)
	obligation := OccasionObligation{ID: "not-deleted"}
	assert.NoError(t, obligation.AfterDelete(db))
}
