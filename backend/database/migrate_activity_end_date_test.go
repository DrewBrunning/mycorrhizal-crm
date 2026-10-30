package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsActivityEndDate covers 000083 (issue #2006): existing
// activities gain a NULL end_date (no backfill, so they stay single-day), an
// explicit value round-trips, and the down migration drops only that column.
func TestMigrationAddsActivityEndDate(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "activity-end-date.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(81))

	const stamp = "2020-01-02 03:04:05"
	_, err = sqlDB.Exec("INSERT INTO users (id, created_at, updated_at, username, password, email) VALUES (1, ?, ?, 'u', 'x', 'u@example.com')", stamp, stamp)
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO activities (id, created_at, updated_at, user_id, title, date) VALUES (1, ?, ?, 1, 'Coffee', '2020-03-04 00:00:00+00:00')", stamp, stamp)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(83))

	var title string
	var end sql.NullString
	require.NoError(t, sqlDB.QueryRow("SELECT title, end_date FROM activities WHERE id = 1").Scan(&title, &end))
	assert.Equal(t, "Coffee", title)
	assert.False(t, end.Valid, "existing activities stay single-day: end_date is NULL, not backfilled")

	_, err = sqlDB.Exec("UPDATE activities SET end_date = '2020-03-06 00:00:00+00:00' WHERE id = 1")
	require.NoError(t, err)
	require.NoError(t, sqlDB.QueryRow("SELECT end_date FROM activities WHERE id = 1").Scan(&end))
	assert.True(t, end.Valid)
	assert.Contains(t, end.String, "2020-03-06")

	require.NoError(t, m.Migrate(81))
	var cols int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('activities') WHERE name = 'end_date'").Scan(&cols))
	assert.Zero(t, cols)
	require.NoError(t, sqlDB.QueryRow("SELECT title FROM activities WHERE id = 1").Scan(&title))
	assert.Equal(t, "Coffee", title, "the activity itself survives the rollback")
}
