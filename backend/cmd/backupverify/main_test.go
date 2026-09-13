package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/atrest"
	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "backupverify-test-secret-key-at-least-32-chars"

// setupSet builds a real migrated database plus empty (already-consistent)
// photo/attachment directories, and returns their paths. It points the signing
// key resolution at a fixed test secret.
func setupSet(t *testing.T) (dbPath, photoDir, attachmentsDir string) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "x.db")
	photoDir = filepath.Join(dir, "photos")
	attachmentsDir = filepath.Join(dir, "attachments")
	require.NoError(t, os.MkdirAll(photoDir, 0o750))
	require.NoError(t, os.MkdirAll(attachmentsDir, 0o750))

	t.Setenv("JWT_SECRET_KEY", testJWTSecret)
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
	t.Setenv("BACKUP_ALLOW_UNSIGNED", "")

	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return dbPath, photoDir, attachmentsDir
}

// signSet signs the database piece with the same key cmd/backup would derive
// from the test secret.
func signSet(t *testing.T, dbPath string) {
	t.Helper()
	key, err := atrest.BackupSigningKey()
	require.NoError(t, err)
	require.NotEmpty(t, key)
	require.NoError(t, database.SignBackup(dbPath, key))
}

func TestRunCompleteSetExitsZero(t *testing.T) {
	dbPath, photoDir, attachmentsDir := setupSet(t)
	signSet(t, dbPath)
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 0, code, "stderr: %s", errOut.String())
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "backup set is complete")
	assert.Contains(t, out.String(), "signature:   verified (hmac-sha256)")
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
	// Sign after the row is inserted so the snapshot verifies, isolating the
	// completeness failure this test exercises.
	signSet(t, dbPath)
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
	signSet(t, dbPath)
	t.Setenv("SQLITE_DB_PATH", filepath.Join(t.TempDir(), "wrong.db"))
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run([]string{dbPath}, &out, &errOut)

	assert.Equal(t, 0, code, "stderr: %s", errOut.String())
}

func TestRunBackupPhotosDirOverridesProfilePhotoDir(t *testing.T) {
	dbPath, _, attachmentsDir := setupSet(t)
	signSet(t, dbPath)
	otherPhotoDir := t.TempDir()
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", "/nonexistent-should-be-overridden")
	t.Setenv("BACKUP_PHOTOS_DIR", otherPhotoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 0, code, "stderr: %s", errOut.String())
	assert.Contains(t, out.String(), otherPhotoDir)
}

func TestRunMissingDirEnvExitsTwoAsAUsageError(t *testing.T) {
	dbPath, photoDir, _ := setupSet(t)
	signSet(t, dbPath)
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

// TestRunUnsignedSetFailsUnlessExplicitlyAllowed covers issue #943's fail-closed
// default for an unauthenticated snapshot and the documented opt-out for
// reconciling a legacy set.
func TestRunUnsignedSetFailsUnlessExplicitlyAllowed(t *testing.T) {
	dbPath, photoDir, attachmentsDir := setupSet(t)
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "unsigned")
	assert.Contains(t, errOut.String(), "BACKUP_ALLOW_UNSIGNED=1")

	out.Reset()
	errOut.Reset()
	t.Setenv("BACKUP_ALLOW_UNSIGNED", "1")
	code = run(nil, &out, &errOut)
	assert.Equal(t, 0, code, "stderr: %s", errOut.String())
	assert.Contains(t, out.String(), "unsigned (allowed by BACKUP_ALLOW_UNSIGNED=1)")
}

// TestRunTamperedManifestFailsClosed: the opt-out tolerates a *missing*
// manifest, never one that is present and does not verify.
func TestRunTamperedManifestFailsClosed(t *testing.T) {
	dbPath, photoDir, attachmentsDir := setupSet(t)
	signSet(t, dbPath)
	t.Setenv("BACKUP_ALLOW_UNSIGNED", "1")
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)

	// Forge a manifest that claims a different digest.
	m, err := database.ReadBackupManifest(dbPath)
	require.NoError(t, err)
	m.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	data, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(database.ManifestPath(dbPath), data, 0o600))

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "not authentic")
}

// TestRunNoSigningKeyExitsTwo: with a manifest present but no resolvable key,
// verification cannot even be attempted.
func TestRunNoSigningKeyExitsTwo(t *testing.T) {
	dbPath, photoDir, attachmentsDir := setupSet(t)
	signSet(t, dbPath)
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("PROFILE_PHOTO_DIR", photoDir)
	t.Setenv("ATTACHMENTS_DIR", attachmentsDir)
	t.Setenv("JWT_SECRET_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut.String(), "no at-rest master key configured")
}
