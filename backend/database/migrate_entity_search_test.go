package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ftsEntityIDs(t *testing.T, db *sql.DB, match string) []string {
	t.Helper()
	rows, err := db.Query("SELECT entity_id FROM entities_fts WHERE entities_fts MATCH ? ORDER BY entity_id", match)
	require.NoError(t, err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

// TestMigrationEntitySearchFTS covers 000094 (issue #2472, part of #2174): the
// entities_fts index is backfilled from the live entities that already exist
// (production data), leaves every entities row untouched, and its triggers
// keep it in step with the base table.
func TestMigrationEntitySearchFTS(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "entsearch.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(93))

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'es', 'x', 'es@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'es'").Scan(&userID))
	for _, e := range [][]any{
		{"e-live1", userID, "organisation", "Acme Corporation", nil},
		{"e-live2", userID, "pet", "Rex", nil},
		{"e-dead", userID, "place", "Acmeville Cottage", "2026-01-01 00:00:00"},
	} {
		_, err = sqlDB.Exec("INSERT INTO entities (id, user_id, kind, name, deleted_at) VALUES (?, ?, ?, ?, ?)", e...)
		require.NoError(t, err)
	}

	require.NoError(t, m.Steps(1)) // 000094

	// Backfill: exactly the live rows, with the right columns.
	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM entities_fts").Scan(&n))
	assert.Equal(t, 2, n, "only live entities are backfilled")
	var uid int64
	var kind string
	require.NoError(t, sqlDB.QueryRow("SELECT user_id, kind FROM entities_fts WHERE entity_id = 'e-live1'").Scan(&uid, &kind))
	assert.Equal(t, userID, uid)
	assert.Equal(t, "organisation", kind)
	assert.Equal(t, []string{"e-live1"}, ftsEntityIDs(t, sqlDB, `"acme"*`), "the soft-deleted Acmeville is not indexed")

	// Existing rows are untouched.
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM entities").Scan(&n))
	assert.Equal(t, 3, n)

	// Triggers.
	_, err = sqlDB.Exec("INSERT INTO entities (id, user_id, kind, name) VALUES ('e-new', ?, 'place', 'Zebra Lodge')", userID)
	require.NoError(t, err)
	assert.Equal(t, []string{"e-new"}, ftsEntityIDs(t, sqlDB, `"zebra"*`), "insert indexes a live row")
	_, err = sqlDB.Exec("INSERT INTO entities (id, user_id, kind, name, deleted_at) VALUES ('e-new-dead', ?, 'place', 'Zebra Annex', datetime('now'))", userID)
	require.NoError(t, err)
	assert.Equal(t, []string{"e-new"}, ftsEntityIDs(t, sqlDB, `"zebra"*`), "insert of a soft-deleted row does not index it")

	_, err = sqlDB.Exec("UPDATE entities SET name = 'Quokka Lodge' WHERE id = 'e-new'")
	require.NoError(t, err)
	assert.Empty(t, ftsEntityIDs(t, sqlDB, `"zebra"*`), "rename drops the old token")
	assert.Equal(t, []string{"e-new"}, ftsEntityIDs(t, sqlDB, `"quokka"*`))

	_, err = sqlDB.Exec("UPDATE entities SET deleted_at = datetime('now') WHERE id = 'e-new'")
	require.NoError(t, err)
	assert.Empty(t, ftsEntityIDs(t, sqlDB, `"quokka"*`), "soft delete removes the row")
	_, err = sqlDB.Exec("UPDATE entities SET deleted_at = NULL WHERE id = 'e-new'")
	require.NoError(t, err)
	assert.Equal(t, []string{"e-new"}, ftsEntityIDs(t, sqlDB, `"quokka"*`), "restore re-indexes")
	_, err = sqlDB.Exec("DELETE FROM entities WHERE id = 'e-new'")
	require.NoError(t, err)
	assert.Empty(t, ftsEntityIDs(t, sqlDB, `"quokka"*`), "hard delete removes the row")

	// Down drops only the derived index; the base rows survive; up round-trips.
	require.NoError(t, m.Steps(-1))
	var gone int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'entities_fts%'").Scan(&gone))
	assert.Zero(t, gone, "down drops the table and its triggers")
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM entities").Scan(&n))
	assert.Equal(t, 4, n, "down keeps every entities row") // e-live1, e-live2, e-dead, e-new-dead
	require.NoError(t, m.Steps(1))
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM entities_fts").Scan(&n))
	assert.Equal(t, 2, n, "re-applying backfills again")
}
