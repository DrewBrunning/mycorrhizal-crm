package database_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordOperatorBackupCompletedWritesHeartbeat covers issue #943: a
// successful operator backup must leave a `backup_completed` event tagged
// component=backup so backup_stale measures the operator's cadence, not the
// restore drill.
func TestRecordOperatorBackupCompletedWritesHeartbeat(t *testing.T) {
	t.Parallel()
	dbPath := liveTestDB(t, "event.db")
	snap := filepath.Join(filepath.Dir(dbPath), "snap.db")
	require.NoError(t, os.WriteFile(snap, []byte("12345"), 0o640))

	require.NoError(t, database.RecordOperatorBackupCompleted(dbPath, snap))

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
	require.NotNil(t, events[0].Result)
	assert.Equal(t, "success", *events[0].Result)
	assert.Contains(t, events[0].Detail, "bytes=5")
	assert.Contains(t, events[0].Detail, "path=")
}

// TestRecordOperatorBackupCompletedErrorsWithoutSystemEventsTable pins the
// best-effort contract's observable half: on a pre-000038 schema the event
// cannot be written, so the caller gets an error it can warn about (unlike the
// pre-migration recorder, which swallows it because a boot must never fail on
// a diagnostic).
func TestRecordOperatorBackupCompletedErrorsWithoutSystemEventsTable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bare.db")
	raw, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	require.NoError(t, raw.Ping())
	require.NoError(t, raw.Close())

	snap := filepath.Join(dir, "snap.db")
	require.NoError(t, os.WriteFile(snap, []byte("x"), 0o640))

	err = database.RecordOperatorBackupCompleted(dbPath, snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert system event")
}

func TestRecordOperatorBackupCompletedMissingDatabaseErrors(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "no-such-dir", "db.db")
	err := database.RecordOperatorBackupCompleted(dbPath, "snap.db")
	require.Error(t, err)
}
