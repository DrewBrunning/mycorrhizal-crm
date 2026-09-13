package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/atrest"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "backup-cli-test-secret-key-at-least-32-chars"

// newMigratedDB opens a real migrated database (never AutoMigrate — CLAUDE.md
// trap 1) and returns its path.
func newMigratedDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "src.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return dbPath
}

// setSigningEnv points the CLI at a test snapshot key and clears the other key
// sources so the test is independent of the ambient environment.
func setSigningEnv(t *testing.T, dbPath string) {
	t.Helper()
	t.Setenv("SQLITE_DB_PATH", dbPath)
	t.Setenv("JWT_SECRET_KEY", testJWTSecret)
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEY_FILE", "")
	t.Setenv("BACKUP_PATH", "")
}

func TestRunSignsSnapshotAndRecordsHeartbeat(t *testing.T) {
	dbPath := newMigratedDB(t)
	setSigningEnv(t, dbPath)
	backupPath := filepath.Join(filepath.Dir(dbPath), "snap.db")
	t.Setenv("BACKUP_PATH", backupPath)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	require.Equal(t, 0, code, "stderr: %s", errOut.String())
	require.FileExists(t, backupPath)
	require.FileExists(t, database.ManifestPath(backupPath))
	assert.Contains(t, out.String(), "Backed up")

	// The snapshot verifies under the same key the CLI derived.
	key, err := atrest.BackupSigningKey()
	require.NoError(t, err)
	require.NoError(t, database.VerifyBackupSignature(backupPath, key))

	// And the freshness heartbeat landed as a backup_completed event.
	db, err := database.OpenMigratedFile(dbPath)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	var events []models.SystemEvent
	require.NoError(t, db.
		Where("component = ? AND event_type = ?", "backup", models.SysEventBackupCompleted).
		Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, "operator_backup", events[0].Operation)
}

func TestRunRefusesWithoutSigningKey(t *testing.T) {
	dbPath := newMigratedDB(t)
	setSigningEnv(t, dbPath)
	t.Setenv("JWT_SECRET_KEY", "")
	backupPath := filepath.Join(filepath.Dir(dbPath), "snap.db")
	t.Setenv("BACKUP_PATH", backupPath)

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 2, code)
	assert.Contains(t, errOut.String(), "no at-rest master key configured")
	assert.NoFileExists(t, backupPath, "a missing key must fail before an unsigned snapshot is written")
}

func TestRunRefusesToOverwriteExistingSnapshot(t *testing.T) {
	dbPath := newMigratedDB(t)
	setSigningEnv(t, dbPath)
	backupPath := filepath.Join(filepath.Dir(dbPath), "snap.db")
	t.Setenv("BACKUP_PATH", backupPath)

	var out, errOut bytes.Buffer
	require.Equal(t, 0, run(nil, &out, &errOut))

	out.Reset()
	errOut.Reset()
	code := run(nil, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "refusing to overwrite")
}

func TestRunPositionalArgOverridesBackupPath(t *testing.T) {
	dbPath := newMigratedDB(t)
	setSigningEnv(t, dbPath)
	envPath := filepath.Join(filepath.Dir(dbPath), "from-env.db")
	positional := filepath.Join(filepath.Dir(dbPath), "from-arg.db")
	t.Setenv("BACKUP_PATH", envPath)

	var out, errOut bytes.Buffer
	require.Equal(t, 0, run([]string{positional}, &out, &errOut))

	require.FileExists(t, positional)
	assert.NoFileExists(t, envPath)
}

func TestRunTooManyArgsIsAUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"a", "b"}, &out, &errOut)
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut.String(), "unexpected extra argument")
	assert.Empty(t, out.String())
}

// TestRunHeartbeatFailureIsAWarningNotAFailure pins the best-effort contract:
// the snapshot is written and signed, so the backup itself succeeded even if
// the freshness row cannot be recorded.
func TestRunHeartbeatFailureIsAWarningNotAFailure(t *testing.T) {
	dbPath := newMigratedDB(t)
	setSigningEnv(t, dbPath)
	backupPath := filepath.Join(filepath.Dir(dbPath), "snap.db")
	t.Setenv("BACKUP_PATH", backupPath)

	// Remove the heartbeat's destination table; the snapshot and manifest
	// paths do not need it.
	db, err := database.OpenMigratedFile(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Exec("DROP TABLE system_events").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)

	assert.Equal(t, 0, code)
	assert.Contains(t, errOut.String(), "warning: snapshot written and signed")
	require.FileExists(t, backupPath)
	require.FileExists(t, database.ManifestPath(backupPath))
}

func TestRunMissingSourceFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "does-not-exist.db")
	setSigningEnv(t, dbPath)
	t.Setenv("BACKUP_PATH", filepath.Join(t.TempDir(), "snap.db"))

	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, errOut.String())
	assert.NoFileExists(t, os.Getenv("BACKUP_PATH"))
}
