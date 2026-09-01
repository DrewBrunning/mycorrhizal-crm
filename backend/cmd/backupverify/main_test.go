package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSet builds a real migrated database plus empty (already-consistent)
// photo/attachment directories, and returns their paths.
func setupSet(t *testing.T) (dbPath, photoDir, attachmentsDir string) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "x.db")
	photoDir = filepath.Join(dir, "photos")
	attachmentsDir = filepath.Join(dir, "attachments")
	require.NoError(t, os.MkdirAll(photoDir, 0o750))
	require.NoError(t, os.MkdirAll(attachmentsDir, 0o750))

	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return dbPath, photoDir, attachmentsDir
}

func TestRunCompleteSetExitsZero(t *testing.T) {
	dbPath, photoDir, attachmentsDir := setupSet(t)
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "backup set is complete")
}

func TestRunIncompleteSetExitsOneAndNamesTheFile(t *testing.T) {
	dbPath, photoDir, attachmentsDir := setupSet(t)
	db, err := database.OpenMigratedFile(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, email, password) VALUES (1, 'u', 'u@example.com', 'x')`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO attachments (user_id, contact_vcard_uid, stored_name, original_name, content_type, size_bytes)
		 VALUES (1, 'c', 'missing-file', 'd.pdf', 'application/pdf', 1)`).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	// deliberately no file written for "missing-file"

	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "missing-file")
	assert.Contains(t, out.String(), "INCOMPLETE")
}

func TestRunPositionalArgOverridesEnv(t *testing.T) {
	dbPath, photoDir, attachmentsDir := setupSet(t)
	t.Setenv("SQLITE_DB_PATH", filepath.Join(t.TempDir(), "wrong.db"))
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run([]string{dbPath}, &out, &errOut)

	assert.Equal(t, 0, code)
}

func TestRunBackupPhotosDirOverridesProfilePhotoDir(t *testing.T) {
	dbPath, _, attachmentsDir := setupSet(t)
	otherPhotoDir := t.TempDir()
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", "/nonexistent-should-be-overridden")
	t.Setenv("BACKUP_PHOTOS_DIR", otherPhotoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), otherPhotoDir)
}

func TestRunMissingDirEnvExitsTwoAsAUsageError(t *testing.T) {
	dbPath, photoDir, _ := setupSet(t)
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", "")
	t.Setenv("BACKUP_ATTACHMENTS_DIR", "")

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 2, code)
	assert.Contains(t, errOut.String(), "backupverify:")
	assert.Empty(t, out.String())
}

func TestRunTooManyArgsExitsTwoAsAUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"a", "b"}, &out, &errOut)

	assert.Equal(t, 2, code)
	assert.Contains(t, errOut.String(), "unexpected extra argument")
}

func TestRunVerifyErrorExitsTwo(t *testing.T) {
	_, photoDir, attachmentsDir := setupSet(t)
	t.Setenv("SQLITE_DB_PATH", filepath.Join(t.TempDir(), "does-not-exist.db"))
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 2, code)
	assert.NotEmpty(t, errOut.String())
}
