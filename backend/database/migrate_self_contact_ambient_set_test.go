package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsSelfContactAmbientSet covers 000082 (issues #2193/#2192):
// existing users, with or without a "Me" contact, keep every field and read the
// flag as false (today's behaviour), NOT NULL holds, and the down migration
// drops only the column and leaves contacts.ambient alone.
func TestMigrationAddsSelfContactAmbientSet(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "self-contact-ambient-set.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(81))

	const stamp = "2020-01-02 03:04:05"
	_, err = sqlDB.Exec("INSERT INTO users (id, created_at, updated_at, username, password, email, self_contact_vcard_uid) VALUES (1, ?, ?, 'u', 'x', 'u@example.com', 'uid-me')", stamp, stamp)
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO users (id, created_at, updated_at, username, password, email) VALUES (2, ?, ?, 'v', 'x', 'v@example.com')", stamp, stamp)
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO contacts (id, created_at, updated_at, user_id, firstname, vcard_uid, ambient) VALUES (1, ?, ?, 1, 'Me', 'uid-me', 1)", stamp, stamp)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(82))

	for _, id := range []int{1, 2} {
		var set int
		require.NoError(t, sqlDB.QueryRow("SELECT self_contact_ambient_set FROM users WHERE id = ?", id).Scan(&set))
		assert.Zero(t, set, "existing users default to not-set")
	}
	var uid sql.NullString
	require.NoError(t, sqlDB.QueryRow("SELECT self_contact_vcard_uid FROM users WHERE id = 1").Scan(&uid))
	assert.Equal(t, "uid-me", uid.String, "the Me pointer survives")

	_, err = sqlDB.Exec("UPDATE users SET self_contact_ambient_set = 1 WHERE id = 1")
	require.NoError(t, err)
	_, err = sqlDB.Exec("UPDATE users SET self_contact_ambient_set = NULL WHERE id = 1")
	require.Error(t, err, "self_contact_ambient_set is NOT NULL")

	require.NoError(t, m.Migrate(81))
	var cols int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'self_contact_ambient_set'").Scan(&cols))
	assert.Zero(t, cols)
	require.NoError(t, sqlDB.QueryRow("SELECT self_contact_vcard_uid FROM users WHERE id = 1").Scan(&uid))
	assert.Equal(t, "uid-me", uid.String)
	var amb int
	require.NoError(t, sqlDB.QueryRow("SELECT ambient FROM contacts WHERE id = 1").Scan(&amb))
	assert.Equal(t, 1, amb, "rollback leaves contacts.ambient alone")
}
