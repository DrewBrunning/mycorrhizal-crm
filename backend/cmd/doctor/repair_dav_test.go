package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"mycorrhizal/database"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedDavDeleted reproduces the pre-#2249 CardDAV bug: the contact is
// soft-deleted with a bare db.Delete, so its live note survives and no trash
// snapshot is written.
func seedDavDeleted(t *testing.T, db *gorm.DB) (models.User, models.Contact, models.Note) {
	t.Helper()
	u := models.User{Username: "davdeleted", Email: "dav@example.com", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	c := models.Contact{UserID: u.ID, Firstname: "Gone", Lastname: "Contact"}
	require.NoError(t, db.Create(&c).Error)
	n := models.Note{UserID: u.ID, ContactID: &c.ID, Content: "still live", Date: time.Now()}
	require.NoError(t, db.Create(&n).Error)
	require.NoError(t, db.Delete(&c).Error)
	return u, c, n
}

func TestDoctor_DavRepairDryRunReportsAndWritesNothing(t *testing.T) {
	var noteID uint
	path := migratedDBFile(t, func(db *gorm.DB) {
		_, _, n := seedDavDeleted(t, db)
		noteID = n.ID
	})

	code, out, _ := run(t, "-db", path, "-repair-dav-deletes")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "DRY RUN")
	assert.Contains(t, out, "would be repaired")
	assert.Contains(t, out, "-apply")
	assert.Contains(t, out, "notes=1")

	assertRowCount(t, path, "notes", "deleted_at IS NULL AND id = ?", noteID, 1)
	assertRowCount(t, path, "contact_deletion_snapshots", "id >= ?", 0, 0)
}

func TestDoctor_DavRepairApplyCascadesWithoutSnapshot(t *testing.T) {
	var noteID uint
	path := migratedDBFile(t, func(db *gorm.DB) {
		_, _, n := seedDavDeleted(t, db)
		noteID = n.ID
	})

	code, out, _ := run(t, "-db", path, "-repair-dav-deletes", "-apply")
	assert.Equal(t, 0, code)
	assert.NotContains(t, out, "DRY RUN")
	assert.Contains(t, out, "repaired")

	assertRowCount(t, path, "notes", "deleted_at IS NOT NULL AND id = ?", noteID, 1)
	assertRowCount(t, path, "contact_deletion_snapshots", "id >= ?", 0, 0)
}

func TestDoctor_DavRepairWriteSnapshotMakesContactRestorable(t *testing.T) {
	path := migratedDBFile(t, func(db *gorm.DB) {
		seedDavDeleted(t, db)
	})

	code, out, _ := run(t, "-db", path, "-repair-dav-deletes", "-apply", "-write-snapshot")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "snapshot(s) written")
	assert.Contains(t, out, "lossy")

	assertRowCount(t, path, "contact_deletion_snapshots", "id >= ?", 0, 1)
}

func TestDoctor_DavRepairSecondApplyIsANoop(t *testing.T) {
	path := migratedDBFile(t, func(db *gorm.DB) {
		seedDavDeleted(t, db)
	})

	code, _, _ := run(t, "-db", path, "-repair-dav-deletes", "-apply", "-write-snapshot")
	require.Equal(t, 0, code)

	code, out, _ := run(t, "-db", path, "-repair-dav-deletes", "-apply", "-write-snapshot")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "nothing to repair")
}

func TestDoctor_DavRepairJSONOutput(t *testing.T) {
	path := migratedDBFile(t, func(db *gorm.DB) {
		seedDavDeleted(t, db)
	})

	code, out, _ := run(t, "-db", path, "-repair-dav-deletes", "-json")
	assert.Equal(t, 0, code)
	var report struct {
		DryRun  bool `json:"dry_run"`
		Contact []struct {
			Action string `json:"action"`
		} `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.True(t, report.DryRun)
	require.Len(t, report.Contact, 1)
	assert.Equal(t, "cascade", report.Contact[0].Action)
}

func TestDoctor_DavRepairPerContactFailureExitsOne(t *testing.T) {
	path := migratedDBFile(t, func(db *gorm.DB) {
		seedDavDeleted(t, db)
		require.NoError(t, db.Exec(`CREATE TRIGGER fail_note_tombstone BEFORE UPDATE OF deleted_at ON notes
			BEGIN SELECT RAISE(ABORT, 'boom'); END`).Error)
	})

	code, out, _ := run(t, "-db", path, "-repair-dav-deletes", "-apply")
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "1 contact(s) failed")
	assert.Contains(t, out, "boom")
}

func TestDoctor_DavRepairRefusesCorruptDatabase(t *testing.T) {
	path := corruptDBWithOrphan(t)
	code, _, errOut := run(t, "-db", path, "-repair-dav-deletes", "-apply")
	assert.Equal(t, 3, code)
	assert.Contains(t, errOut, "refusing to repair")
}

func TestDoctor_DavRepairUsageErrors(t *testing.T) {
	path := migratedDBFile(t, nil)

	t.Run("apply without dav repair", func(t *testing.T) {
		code, _, errOut := run(t, "-db", path, "-apply")
		assert.Equal(t, 2, code)
		assert.Contains(t, errOut, "without -repair-dav-deletes")
	})

	t.Run("write snapshot without apply", func(t *testing.T) {
		code, _, errOut := run(t, "-db", path, "-repair-dav-deletes", "-write-snapshot")
		assert.Equal(t, 2, code)
		assert.Contains(t, errOut, "-write-snapshot requires -apply")
	})

	t.Run("combined with legacy repair", func(t *testing.T) {
		code, _, errOut := run(t, "-db", path, "-repair-dav-deletes", "-repair")
		assert.Equal(t, 2, code)
		assert.Contains(t, errOut, "cannot be combined")
	})
}

func TestDoctor_DavRepairScanFailureExitsOne(t *testing.T) {
	path := migratedDBFile(t, func(db *gorm.DB) {
		seedDavDeleted(t, db)
		dbtest.HideTable(t, db, "gifts")
	})

	code, _, errOut := run(t, "-db", path, "-repair-dav-deletes")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "CardDAV delete repair failed")
}

func TestDoctor_DavRepairJSONFailureExitsOne(t *testing.T) {
	path := migratedDBFile(t, func(db *gorm.DB) {
		seedDavDeleted(t, db)
		require.NoError(t, db.Exec(`CREATE TRIGGER fail_note_tombstone BEFORE UPDATE OF deleted_at ON notes
			BEGIN SELECT RAISE(ABORT, 'boom'); END`).Error)
	})

	code, out, _ := run(t, "-db", path, "-repair-dav-deletes", "-apply", "-json")
	assert.Equal(t, 1, code)
	var report struct {
		Failed int `json:"failed"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Equal(t, 1, report.Failed)
}

func TestDoctor_DavRepairRefusesWhenStorageCheckCannotRun(t *testing.T) {
	path := migratedDBFile(t, nil)
	db, err := database.OpenMigratedFile(path)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	var out, errOut bytes.Buffer
	code := runDavDeleteRepair(context.Background(), db, false, false, false, &out, &errOut)
	assert.Equal(t, 3, code)
	assert.Contains(t, errOut.String(), "storage integrity check could not run")
}
