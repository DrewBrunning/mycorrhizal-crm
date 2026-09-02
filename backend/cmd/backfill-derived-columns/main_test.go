package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func migratedDBFile(t *testing.T, seed func(db *gorm.DB)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backfill.db")
	db := dbtest.NewAt(t, path)
	if seed != nil {
		seed(db)
	}
	require.NoError(t, db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return path
}

func TestRun_RebuildsDrift(t *testing.T) {
	var contactID uint
	path := migratedDBFile(t, func(db *gorm.DB) {
		u := models.User{Username: "cli", Email: "cli@example.com", Password: "x"}
		require.NoError(t, db.Create(&u).Error)
		c := models.Contact{UserID: u.ID, Firstname: "Ada", Lastname: "Lovelace"}
		require.NoError(t, db.Create(&c).Error)
		contactID = c.ID
		require.NoError(t, db.Exec("UPDATE contacts SET sort_name = 'zzz' WHERE id = ?", c.ID).Error)
	})

	var out, errOut bytes.Buffer
	code := run([]string{"-db", path}, &out, &errOut)
	require.Equal(t, 0, code, errOut.String())
	assert.Contains(t, out.String(), "scanned=1 updated=1")
	assert.Contains(t, out.String(), "sort_name: 1")

	db := dbtest.NewAt(t, path)
	var got models.Contact
	require.NoError(t, db.First(&got, contactID).Error)
	assert.Equal(t, "lovelace", got.SortName)
}

func TestRun_EmptyDBIsACleanNoOp(t *testing.T) {
	// database.InitDB creates and migrates a fresh file, so a fresh path is a
	// valid empty database, not an error.
	var out, errOut bytes.Buffer
	code := run([]string{"-db", filepath.Join(t.TempDir(), "fresh.db")}, &out, &errOut)
	require.Equal(t, 0, code, errOut.String())
	assert.Contains(t, out.String(), "scanned=0 updated=0")
}

func TestRun_BadFlagIsExit1(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 1, run([]string{"-nonsense"}, &out, &errOut))
}
