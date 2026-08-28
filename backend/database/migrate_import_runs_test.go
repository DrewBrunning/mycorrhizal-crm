package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationsAddImportRuns covers 000042: the import_runs table is created
// with its user index and format CHECK vocabulary, accepts a valid row,
// rejects an out-of-vocabulary format, and the down migration drops it
// cleanly on a populated database. import_runs holds only row counts +
// timestamps for completed imports, so there is no existing-data-preservation
// concern — the table is new (issue #651).
func TestMigrationsAddImportRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "import-runs.db")

	db, err := InitDB(dbPath)
	require.NoError(t, err, "full migration chain incl. 000042 must apply to an empty database")
	sqlDB, err := db.DB()
	require.NoError(t, err)

	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='import_runs'",
	).Scan(&tableCount))
	assert.Equal(t, int64(1), tableCount)

	var idxCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_import_runs_user'",
	).Scan(&idxCount))
	assert.Equal(t, int64(1), idxCount, "the (user_id, created_at) index must exist for the history query")

	// A valid row inserts.
	_, err = sqlDB.Exec(`
		INSERT INTO import_runs (user_id, format, total_processed, created, updated, skipped, error_count, created_at)
		VALUES (1, 'csv', 3, 2, 0, 1, 0, datetime('now'))`)
	require.NoError(t, err)

	// Each documented format token is accepted.
	for _, f := range []string{"vcf", "jscontact", "records"} {
		_, err = sqlDB.Exec(
			`INSERT INTO import_runs (user_id, format, created_at) VALUES (1, ?, datetime('now'))`, f)
		require.NoErrorf(t, err, "format %q must satisfy the CHECK constraint", f)
	}

	// An out-of-vocabulary format is rejected by the CHECK constraint.
	_, err = sqlDB.Exec(
		`INSERT INTO import_runs (user_id, format, created_at) VALUES (1, 'ldif', datetime('now'))`)
	require.Error(t, err, "CHECK constraint must reject an unknown format token")

	require.NoError(t, sqlDB.Close())

	// Down drops the table. 000042 is no longer the migration tip — 000043
	// sits on top — so roll that back first, then 000042's own down migration.
	require.NoError(t, MigrateDown(dbPath)) // rolls back 000043_storage_samples
	require.NoError(t, MigrateDown(dbPath)) // rolls back 000042_import_runs

	sqlDB2, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB2.Close()
	require.NoError(t, sqlDB2.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='import_runs'",
	).Scan(&tableCount))
	assert.Equal(t, int64(0), tableCount, "the down migration must drop import_runs")
}
