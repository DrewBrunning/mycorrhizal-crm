package atrest

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestReadKeyMaterialTimes_NoRow: a migrated database that has never armed
// encryption has no wrapped DEK, which is "nothing to age", not an error
// (issue #955).
func TestReadKeyMaterialTimes_NoRow(t *testing.T) {
	db := dbtest.New(t)

	times, ok, err := ReadKeyMaterialTimes(db)
	require.NoError(t, err)
	require.False(t, ok)
	require.True(t, times.CreatedAt.IsZero())
}

func TestReadKeyMaterialTimes_NilDB(t *testing.T) {
	_, ok, err := ReadKeyMaterialTimes(nil)
	require.Error(t, err)
	require.False(t, ok)
}

// TestReadKeyMaterialTimes_TracksRotation pins the whole point of migration
// 000057: created_at records the DEK's birth and does not move, while
// RotateMasterKey stamps rotated_at, so the surfaces report the age of the key
// material that actually protects the data.
func TestReadKeyMaterialTimes_TracksRotation(t *testing.T) {
	db, oldKEK := realDB(t)

	createdBefore := storedCreatedAt(t, db)
	require.False(t, createdBefore.IsZero())

	before, ok, err := ReadKeyMaterialTimes(db)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, before.CreatedAt.IsZero())
	require.True(t, before.RotatedAt.IsZero(), "a fresh DEK has not had its master key rotated")
	require.True(t, before.LastChanged().Equal(before.CreatedAt))

	// Back-date created_at so the rotation is observably later even though both
	// happen within the same test tick.
	require.NoError(t, db.Table("data_encryption_keys").
		Where("key_id = ?", keyID).
		Update("created_at", time.Now().UTC().Add(-2*time.Hour)).Error)
	createdAfterBackdate := storedCreatedAt(t, db)

	newKEK := make([]byte, keySize)
	for i := range newKEK {
		newKEK[i] = byte(i) ^ 0x5A
	}
	require.NoError(t, RotateMasterKey(db, oldKEK, newKEK))

	after, ok, err := ReadKeyMaterialTimes(db)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, createdAfterBackdate, storedCreatedAt(t, db), "created_at must not move on rotation")
	require.False(t, after.RotatedAt.IsZero(), "rotation must stamp rotated_at")
	require.True(t, after.LastChanged().Equal(after.RotatedAt), "LastChanged must prefer the newer rotation time")
	require.True(t, after.LastChanged().After(after.CreatedAt))
}

// TestKeyMaterialTimes_LastChanged covers the fallback in isolation.
func TestKeyMaterialTimes_LastChanged(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rotated := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	require.Equal(t, created, KeyMaterialTimes{CreatedAt: created}.LastChanged())
	require.Equal(t, rotated, KeyMaterialTimes{CreatedAt: created, RotatedAt: rotated}.LastChanged())
	// A rotated_at older than created_at (hand-edited database) does not win.
	require.Equal(t, created, KeyMaterialTimes{CreatedAt: created, RotatedAt: created.Add(-time.Hour)}.LastChanged())
}

// storedCreatedAt reads created_at back as a time for equality assertions.
func storedCreatedAt(t *testing.T, db *gorm.DB) time.Time {
	t.Helper()
	var row struct {
		CreatedAt time.Time `gorm:"column:created_at"`
	}
	require.NoError(t, db.Table("data_encryption_keys").Select("created_at").
		Where("key_id = ?", keyID).Scan(&row).Error)
	return row.CreatedAt.UTC()
}
