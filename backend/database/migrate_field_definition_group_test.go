package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationFieldDefinitionGroup covers 000074 (issue #1730): existing
// definitions survive the ADD COLUMN untouched and come out ungrouped (”), a
// grouped row can be written, and the down migration drops only the column.
func TestMigrationFieldDefinitionGroup(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "field-definition-group.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(73)) // everything before 000074

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'grp', 'x', 'grp@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'grp'").Scan(&userID))
	_, err = sqlDB.Exec(`
		INSERT INTO field_definitions (id, created_at, updated_at, user_id, label, key, target, type, projection, sensitivity)
		VALUES ('d1', datetime('now'), datetime('now'), ?, 'Airline', 'airline', 'contact', 'string', 'internal-only', 'normal')`, userID)
	require.NoError(t, err)

	require.NoError(t, m.Steps(1)) // 000074

	var label, group string
	require.NoError(t, sqlDB.QueryRow("SELECT label, group_label FROM field_definitions WHERE id = 'd1'").Scan(&label, &group))
	assert.Equal(t, "Airline", label, "existing definition is preserved")
	assert.Equal(t, "", group, "existing definitions come out ungrouped")

	_, err = sqlDB.Exec("UPDATE field_definitions SET group_label = 'Travel profile' WHERE id = 'd1'")
	require.NoError(t, err)

	require.NoError(t, sqlDB.Close())
	require.NoError(t, MigrateDown(dbPath)) // rolls back 000074

	reopened, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer reopened.Close()
	require.NoError(t, reopened.QueryRow("SELECT label FROM field_definitions WHERE id = 'd1'").Scan(&label))
	assert.Equal(t, "Airline", label, "down keeps the definition")
	var n int
	require.NoError(t, reopened.QueryRow("SELECT COUNT(*) FROM pragma_table_info('field_definitions') WHERE name = 'group_label'").Scan(&n))
	assert.Equal(t, 0, n, "down drops the column")
}
