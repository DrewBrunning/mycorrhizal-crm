package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationsAddSystemEvents covers 000037: the system_events table is
// created with its CHECK vocabularies and indexes, accepts a valid row,
// rejects an out-of-vocabulary event_type, and the down migration drops it
// cleanly. system_events holds only system-generated diagnostic data, so
// there is no existing-data-preservation concern — the table is new.
func TestMigrationsAddSystemEvents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sys-events.db")

	db, err := InitDB(dbPath)
	require.NoError(t, err, "full migration chain incl. 000037 must apply to an empty database")
	sqlDB, err := db.DB()
	require.NoError(t, err)

	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='system_events'",
	).Scan(&tableCount))
	assert.Equal(t, int64(1), tableCount)

	var idxCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_system_events_correlation_id'",
	).Scan(&idxCount))
	assert.Equal(t, int64(1), idxCount, "correlation_id index must exist for the timeline drill-down")

	// A valid row inserts.
	_, err = sqlDB.Exec(`
		INSERT INTO system_events (created_at, occurred_at, event_type, severity, component, correlation_id)
		VALUES (datetime('now'), datetime('now'), 'sync_completed', 'info', 'contact_sync', 'chain-1')`)
	require.NoError(t, err)

	// An out-of-vocabulary event_type is rejected by the CHECK constraint.
	_, err = sqlDB.Exec(`
		INSERT INTO system_events (created_at, occurred_at, event_type, severity)
		VALUES (datetime('now'), datetime('now'), 'totally_made_up', 'info')`)
	require.Error(t, err, "CHECK constraint must reject an unknown event_type token")

	// An out-of-vocabulary result is rejected too.
	_, err = sqlDB.Exec(`
		INSERT INTO system_events (created_at, occurred_at, event_type, severity, result)
		VALUES (datetime('now'), datetime('now'), 'job_completed', 'info', 'maybe')`)
	require.Error(t, err, "CHECK constraint must reject an unknown result token")

	require.NoError(t, sqlDB.Close())

	// Down drops the table.
	require.NoError(t, MigrateDown(dbPath))

	sqlDB2, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB2.Close()
	require.NoError(t, sqlDB2.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='system_events'",
	).Scan(&tableCount))
	assert.Equal(t, int64(0), tableCount, "the down migration must drop system_events")
}
