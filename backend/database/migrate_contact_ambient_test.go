package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsContactAmbient covers 000075 (issue #1733): the new
// contacts.ambient column is added with a false default on every existing row
// (live and soft-deleted), nothing else on those rows changes, an explicit
// true survives, and the down migration drops only the column and leaves the
// contacts themselves intact.
func TestMigrationAddsContactAmbient(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "contact-ambient.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(73)) // everything before 000075 (this PR's predecessor on the chain)

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'amb', 'x', 'amb@example.com')")
	require.NoError(t, err)
	const stamp = "2020-01-02 03:04:05"
	insert := func(name string, favorite int, deleted bool) int64 {
		t.Helper()
		var del any
		if deleted {
			del = "2021-01-01 00:00:00"
		}
		res, err := sqlDB.Exec(
			"INSERT INTO contacts (created_at, updated_at, deleted_at, firstname, user_id, is_favorite) VALUES (?, ?, ?, ?, 1, ?)",
			stamp, stamp, del, name, favorite)
		require.NoError(t, err)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	live := insert("live", 1, false)
	dead := insert("dead", 0, true)

	require.NoError(t, m.Migrate(75))

	type row struct {
		first    string
		favorite int
		ambient  int
		deleted  sql.NullString
	}
	read := func(id int64) row {
		t.Helper()
		var r row
		require.NoError(t, sqlDB.QueryRow("SELECT firstname, is_favorite, ambient, deleted_at FROM contacts WHERE id = ?", id).
			Scan(&r.first, &r.favorite, &r.ambient, &r.deleted))
		return r
	}
	l := read(live)
	assert.Equal(t, row{first: "live", favorite: 1, ambient: 0}, l, "existing rows get ambient=false and are otherwise untouched")
	d := read(dead)
	assert.Equal(t, "dead", d.first)
	assert.Equal(t, 0, d.ambient)
	assert.True(t, d.deleted.Valid, "a soft-deleted row stays soft-deleted (someone's undo button)")

	_, err = sqlDB.Exec("UPDATE contacts SET ambient = 1 WHERE id = ?", live)
	require.NoError(t, err)
	assert.Equal(t, 1, read(live).ambient)
	// NOT NULL: the column can never be null.
	_, err = sqlDB.Exec("UPDATE contacts SET ambient = NULL WHERE id = ?", live)
	require.Error(t, err)

	// Down drops the column, keeps the contacts.
	require.NoError(t, m.Migrate(73))
	var cols int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('contacts') WHERE name = 'ambient'").Scan(&cols))
	assert.Zero(t, cols)
	var first string
	var fav int
	require.NoError(t, sqlDB.QueryRow("SELECT firstname, is_favorite FROM contacts WHERE id = ?", live).Scan(&first, &fav))
	assert.Equal(t, "live", first)
	assert.Equal(t, 1, fav)
	var total int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM contacts").Scan(&total))
	assert.Equal(t, 2, total, "both rows, including the soft-deleted one, survive the rollback")
}
