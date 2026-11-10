package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationImportRunIssues covers 000093 (issue #2463): import_runs gains
// the additive `issues` (JSON array) and `issue_count` columns. Every row that
// predates the migration is preserved and reads back as no issues; the down
// migration drops the two columns and leaves the rows.
func TestMigrationImportRunIssues(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "import-run-issues.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(92))

	const stamp = "2020-01-02 03:04:05"
	_, err = sqlDB.Exec(`INSERT INTO import_runs (id, user_id, format, total_processed, created, updated, skipped, error_count, created_at)
		VALUES (5, 1, 'csv', 4, 2, 1, 1, 0, ?), (6, 2, 'mycorrhizal', 9, 9, 0, 0, 0, ?)`, stamp, stamp)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(93))

	for _, id := range []int{5, 6} {
		var issues string
		var count, total int
		require.NoError(t, sqlDB.QueryRow(
			"SELECT issues, issue_count, total_processed FROM import_runs WHERE id = ?", id).Scan(&issues, &count, &total))
		assert.Equal(t, "[]", issues, "a pre-existing run reads back as no stored issues")
		assert.Zero(t, count)
		assert.NotZero(t, total, "existing counts are untouched")
	}

	_, err = sqlDB.Exec(`INSERT INTO import_runs (user_id, format, issues, issue_count, created_at)
		VALUES (1, 'vcf', '[{"row":1}]', 1, ?)`, stamp)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(92))
	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM import_runs").Scan(&n))
	assert.Equal(t, 3, n, "the down migration keeps every run")
	_, err = sqlDB.Exec("SELECT issues FROM import_runs")
	require.Error(t, err, "the issues column is gone after the down")
}
