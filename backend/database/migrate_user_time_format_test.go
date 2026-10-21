package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsUserTimeFormat covers 000076 (issue #1830): the new
// users.time_format column lands as 'auto' on every existing user (live and
// soft-deleted), nothing else on those rows changes, an explicit value
// survives, NOT NULL holds, and the down migration drops only the column.
func TestMigrationAddsUserTimeFormat(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "user-time-format.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(75)) // everything before 000076 (this PR)

	const stamp = "2020-01-02 03:04:05"
	insert := func(name, dateFormat string, deleted bool) int64 {
		t.Helper()
		var del any
		if deleted {
			del = "2021-01-01 00:00:00"
		}
		res, err := sqlDB.Exec(
			"INSERT INTO users (created_at, updated_at, deleted_at, username, password, email, date_format) VALUES (?, ?, ?, ?, 'x', ?, ?)",
			stamp, stamp, del, name, name+"@example.com", dateFormat)
		require.NoError(t, err)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	live := insert("live", "iso", false)
	dead := insert("dead", "us", true)

	require.NoError(t, m.Migrate(76))

	read := func(id int64) (name, dateFormat, timeFormat string, deleted sql.NullString) {
		t.Helper()
		require.NoError(t, sqlDB.QueryRow("SELECT username, date_format, time_format, deleted_at FROM users WHERE id = ?", id).
			Scan(&name, &dateFormat, &timeFormat, &deleted))
		return
	}
	n, df, tf, del := read(live)
	assert.Equal(t, "live", n)
	assert.Equal(t, "iso", df, "date_format is untouched")
	assert.Equal(t, "auto", tf, "existing users become auto")
	assert.False(t, del.Valid)
	_, df, tf, del = read(dead)
	assert.Equal(t, "us", df)
	assert.Equal(t, "auto", tf, "soft-deleted users get the default too")
	assert.True(t, del.Valid, "a soft-deleted user stays soft-deleted")

	_, err = sqlDB.Exec("UPDATE users SET time_format = '24h' WHERE id = ?", live)
	require.NoError(t, err)
	_, _, tf, _ = read(live)
	assert.Equal(t, "24h", tf)
	_, err = sqlDB.Exec("UPDATE users SET time_format = NULL WHERE id = ?", live)
	require.Error(t, err, "time_format is NOT NULL")

	// Down drops the column, keeps the users.
	require.NoError(t, m.Migrate(75))
	var cols int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'time_format'").Scan(&cols))
	assert.Zero(t, cols)
	var total int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&total))
	assert.Equal(t, 2, total, "both users, including the soft-deleted one, survive the rollback")
	var name, dateFormat string
	require.NoError(t, sqlDB.QueryRow("SELECT username, date_format FROM users WHERE id = ?", live).Scan(&name, &dateFormat))
	assert.Equal(t, "live", name)
	assert.Equal(t, "iso", dateFormat)
}
