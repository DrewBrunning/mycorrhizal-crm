package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsCircleDescriptionColor covers 000080 (issue #1995): existing
// circles keep their name and gain empty description/colour defaults, NOT NULL
// holds, explicit values survive, and the down migration drops only the two
// columns.
func TestMigrationAddsCircleDescriptionColor(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "circle-description-color.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(79))

	const stamp = "2020-01-02 03:04:05"
	_, err = sqlDB.Exec("INSERT INTO users (id, created_at, updated_at, username, password, email) VALUES (1, ?, ?, 'u', 'x', 'u@example.com')", stamp, stamp)
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO circles (id, created_at, updated_at, user_id, name) VALUES ('c1', ?, ?, 1, 'College friends')", stamp, stamp)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(80))

	var name, desc, color string
	require.NoError(t, sqlDB.QueryRow("SELECT name, description, color FROM circles WHERE id = 'c1'").Scan(&name, &desc, &color))
	assert.Equal(t, "College friends", name)
	assert.Empty(t, desc, "existing circles get an empty description")
	assert.Empty(t, color, "existing circles get no colour")

	_, err = sqlDB.Exec("UPDATE circles SET description = 'Uni crowd', color = 'blue' WHERE id = 'c1'")
	require.NoError(t, err)
	require.NoError(t, sqlDB.QueryRow("SELECT description, color FROM circles WHERE id = 'c1'").Scan(&desc, &color))
	assert.Equal(t, "Uni crowd", desc)
	assert.Equal(t, "blue", color)
	_, err = sqlDB.Exec("UPDATE circles SET description = NULL WHERE id = 'c1'")
	require.Error(t, err, "description is NOT NULL")
	_, err = sqlDB.Exec("UPDATE circles SET color = NULL WHERE id = 'c1'")
	require.Error(t, err, "color is NOT NULL")

	require.NoError(t, m.Migrate(79))
	var cols int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('circles') WHERE name IN ('description','color')").Scan(&cols))
	assert.Zero(t, cols)
	require.NoError(t, sqlDB.QueryRow("SELECT name FROM circles WHERE id = 'c1'").Scan(&name))
	assert.Equal(t, "College friends", name, "the circle itself survives the rollback")
}
