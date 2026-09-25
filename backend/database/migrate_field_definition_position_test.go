package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationBackfillsFieldDefinitionPosition covers 000066 (issue #1210):
// the added `position` column is backfilled with a per-user contiguous order
// derived from each row's created_at (id breaking ties), so a user who never
// reorders keeps the relative order the list already showed. The canonical
// fixture's field_definitions are empty, so the semantic migration suite sees
// this as an added column (outside its guarantee); this test seeds rows
// directly, per EXPECTATION-FILES.md's "validate their backfill with their own
// migration test".
func TestMigrationBackfillsFieldDefinitionPosition(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "field-definition-position.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000066, so the rows below genuinely
	// predate the position column.
	require.NoError(t, m.Steps(65))

	mkUser := func(username string) int64 {
		t.Helper()
		_, err := sqlDB.Exec(
			"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), ?, 'x', ?)",
			username, username+"@example.com")
		require.NoError(t, err)
		var id int64
		require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&id))
		return id
	}
	userA := mkUser("field-pos-a")
	userB := mkUser("field-pos-b")

	insertDef := func(id string, userID int64, key, createdAt string) {
		t.Helper()
		_, err := sqlDB.Exec(`
			INSERT INTO field_definitions
				(id, created_at, updated_at, user_id, label, key, target, type, projection, sensitivity)
			VALUES (?, ?, ?, ?, ?, ?, 'contact', 'string', 'internal-only', 'normal')`,
			id, createdAt, createdAt, userID, key, key)
		require.NoErrorf(t, err, "seeding field definition %s", id)
	}

	// Out of insertion order, and two rows share a created_at so the id
	// tiebreak is exercised. Expected order by (created_at, id):
	//   a0 (01-01), a1 (01-02, id "a1"), a2 (01-02), a3 (01-03)
	//   b0 (01-04), b1 (01-05) — independent per user.
	insertDef("a3", userA, "a3", "2026-01-03 00:00:00")
	insertDef("a2", userA, "a2", "2026-01-02 00:00:00")
	insertDef("a1", userA, "a1", "2026-01-02 00:00:00")
	insertDef("a0", userA, "a0", "2026-01-01 00:00:00")
	// A second user's rows are numbered independently from 0.
	insertDef("b1", userB, "b1", "2026-01-05 00:00:00")
	insertDef("b0", userB, "b0", "2026-01-04 00:00:00")

	// Apply exactly 000066.
	require.NoError(t, m.Steps(1))

	positionOf := func(id string) int {
		t.Helper()
		var pos int
		require.NoError(t, sqlDB.QueryRow("SELECT position FROM field_definitions WHERE id = ?", id).Scan(&pos))
		return pos
	}
	assert.Equal(t, 0, positionOf("a0"), "earliest created_at is position 0")
	assert.Equal(t, 1, positionOf("a1"), "same created_at breaks ties by id (a1 before a2)")
	assert.Equal(t, 2, positionOf("a2"))
	assert.Equal(t, 3, positionOf("a3"))
	assert.Equal(t, 0, positionOf("b0"), "each user's positions start at 0 independently")
	assert.Equal(t, 1, positionOf("b1"))

	var idxCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_field_definitions_user_position'",
	).Scan(&idxCount))
	assert.Equal(t, int64(1), idxCount, "the display-order index must exist after 000066")

	// The down migration drops the index and the column.
	require.NoError(t, MigrateDown(dbPath))

	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_field_definitions_user_position'",
	).Scan(&idxCount))
	assert.Equal(t, int64(0), idxCount, "the down migration must drop the display-order index")

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('field_definitions') WHERE name = 'position'",
	).Scan(&colCount))
	assert.Equal(t, int64(0), colCount, "the down migration must drop the position column")
}
