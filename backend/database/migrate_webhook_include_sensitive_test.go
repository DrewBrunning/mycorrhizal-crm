package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationWebhookIncludeSensitive covers 000096 (issue #2888): the
// per-webhook include_sensitive choice. Existing webhooks must come through
// with every column intact and the new column NULL ("follow the server
// default", which is full fidelity in v1.x), an explicit 1/0 must be storable,
// and down must drop only the column while keeping the rows.
func TestMigrationWebhookIncludeSensitive(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "webhook-include-sensitive.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(95))

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'wh', 'x', 'wh@example.com')")
	require.NoError(t, err)
	_, err = sqlDB.Exec(`INSERT INTO webhooks (id, created_at, updated_at, user_id, name, url, events, secret, is_active)
		VALUES (1, datetime('now'), datetime('now'), 1, 'live', 'https://example.com/a', '["contact.created"]', 's1', 1),
		       (2, datetime('now'), datetime('now'), 1, 'paused', 'https://example.com/b', '[]', 's2', 0)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`UPDATE webhooks SET deleted_at = datetime('now') WHERE id = 2`)
	require.NoError(t, err)

	hasColumn := func() bool {
		t.Helper()
		rows, err := sqlDB.Query("PRAGMA table_info(webhooks)")
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull, pk int
			var dflt any
			require.NoError(t, rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk))
			if name == "include_sensitive" {
				return true
			}
		}
		require.NoError(t, rows.Err())
		return false
	}
	require.False(t, hasColumn())

	require.NoError(t, m.Migrate(96))
	require.True(t, hasColumn())

	type row struct {
		name, url, events, secret string
		active                    int
		deleted                   bool
		inc                       sql.NullInt64
	}
	read := func(id int) row {
		t.Helper()
		var r row
		require.NoError(t, sqlDB.QueryRow(`SELECT name, url, events, secret, is_active, deleted_at IS NOT NULL, include_sensitive
			FROM webhooks WHERE id = ?`, id).Scan(&r.name, &r.url, &r.events, &r.secret, &r.active, &r.deleted, &r.inc))
		return r
	}
	a, b := read(1), read(2)
	assert.Equal(t, row{"live", "https://example.com/a", `["contact.created"]`, "s1", 1, false, sql.NullInt64{}}, a)
	assert.Equal(t, row{"paused", "https://example.com/b", `[]`, "s2", 0, true, sql.NullInt64{}}, b,
		"a soft-deleted webhook survives and also follows the server default")

	// An explicit choice is storable in both directions.
	_, err = sqlDB.Exec("UPDATE webhooks SET include_sensitive = 0 WHERE id = 1")
	require.NoError(t, err)
	assert.Equal(t, sql.NullInt64{Int64: 0, Valid: true}, read(1).inc)
	_, err = sqlDB.Exec("UPDATE webhooks SET include_sensitive = 1 WHERE id = 1")
	require.NoError(t, err)
	assert.Equal(t, sql.NullInt64{Int64: 1, Valid: true}, read(1).inc)

	// down drops only the column; rows and their other fields stay.
	require.NoError(t, m.Migrate(95))
	assert.False(t, hasColumn())
	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM webhooks").Scan(&n))
	assert.Equal(t, 2, n)
	var name, secret string
	require.NoError(t, sqlDB.QueryRow("SELECT name, secret FROM webhooks WHERE id = 1").Scan(&name, &secret))
	assert.Equal(t, "live", name)
	assert.Equal(t, "s1", secret)

	// Re-applying is clean and the choice is back to the default.
	require.NoError(t, m.Migrate(96))
	assert.Equal(t, sql.NullInt64{}, read(1).inc)
}
