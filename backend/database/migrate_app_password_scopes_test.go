package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsApiTokenScopesAndDAVPasswordPolicy covers 000078 (issue
// #2010): existing tokens get a scope set derived from their legacy scope,
// existing users keep the account-password-on-DAV behaviour they have today,
// users created after the migration start with it off, and the down migration
// drops only the two new columns.
func TestMigrationAddsApiTokenScopesAndDAVPasswordPolicy(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "app-password-scopes.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(77))

	const stamp = "2020-01-02 03:04:05"
	insertUser := func(name string) int64 {
		t.Helper()
		res, err := sqlDB.Exec(
			"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (?, ?, ?, 'x', ?)",
			stamp, stamp, name, name+"@example.com")
		require.NoError(t, err)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	live := insertUser("live")
	dead := insertUser("dead")
	twoFactor := insertUser("two-factor")
	_, err = sqlDB.Exec("UPDATE users SET deleted_at = ? WHERE id = ?", stamp, dead)
	require.NoError(t, err)
	_, err = sqlDB.Exec("UPDATE users SET totp_enabled = 1 WHERE id = ?", twoFactor)
	require.NoError(t, err)

	insertToken := func(userID int64, name, scope string) int64 {
		t.Helper()
		res, err := sqlDB.Exec(
			"INSERT INTO api_tokens (created_at, updated_at, user_id, name, token_hash, scope) VALUES (?, ?, ?, ?, ?, ?)",
			stamp, stamp, userID, name, "hash-"+name, scope)
		require.NoError(t, err)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	fullToken := insertToken(live, "full", "full")
	carddavToken := insertToken(live, "carddav", "carddav")

	require.NoError(t, m.Migrate(78))

	readToken := func(id int64) (scope, scopes string) {
		t.Helper()
		require.NoError(t, sqlDB.QueryRow(
			"SELECT scope, scopes FROM api_tokens WHERE id = ?", id).Scan(&scope, &scopes))
		return
	}
	scope, scopes := readToken(fullToken)
	assert.Equal(t, "full", scope)
	assert.JSONEq(t, `["carddav","caldav","api:read","api:write","mcp"]`, scopes)
	scope, scopes = readToken(carddavToken)
	assert.Equal(t, "carddav", scope)
	assert.JSONEq(t, `["carddav","caldav"]`, scopes)

	readPolicy := func(id int64) bool {
		t.Helper()
		var allowed bool
		require.NoError(t, sqlDB.QueryRow(
			"SELECT allow_dav_password FROM users WHERE id = ?", id).Scan(&allowed))
		return allowed
	}
	assert.True(t, readPolicy(live), "existing users keep password-on-DAV working")
	assert.True(t, readPolicy(dead), "soft-deleted users get the same backfill")
	assert.False(t, readPolicy(twoFactor), "an existing second factor keeps account-password DAV off")

	newUser := insertUser("new-after-migration")
	assert.False(t, readPolicy(newUser), "new users opt in explicitly")

	require.NoError(t, m.Migrate(77))

	var cols int
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('api_tokens') WHERE name = 'scopes'").Scan(&cols))
	assert.Zero(t, cols)
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'allow_dav_password'").Scan(&cols))
	assert.Zero(t, cols)

	var users, tokens int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&users))
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM api_tokens").Scan(&tokens))
	assert.Equal(t, 4, users)
	assert.Equal(t, 2, tokens)
}
