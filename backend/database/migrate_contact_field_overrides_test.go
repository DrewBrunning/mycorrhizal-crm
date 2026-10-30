package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsContactFieldOverrides covers 000084 (issue #1997): existing
// users gain hide_empty_fields = 0 (their display is unchanged), the override
// table starts empty, its natural key and state CHECK hold, deleting a user
// cascades its rows, and the down migration drops only the new objects.
func TestMigrationAddsContactFieldOverrides(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "contact-field-overrides.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(83))

	const stamp = "2020-01-02 03:04:05"
	_, err = sqlDB.Exec("INSERT INTO users (id, created_at, updated_at, username, password, email, enabled_contact_fields) VALUES (1, ?, ?, 'u', 'x', 'u@example.com', '[\"emails\"]')", stamp, stamp)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(84))

	var hide int
	require.NoError(t, sqlDB.QueryRow("SELECT hide_empty_fields FROM users WHERE id = 1").Scan(&hide))
	assert.Zero(t, hide, "existing users keep today's display (hide-empty off)")
	var enabled string
	require.NoError(t, sqlDB.QueryRow("SELECT enabled_contact_fields FROM users WHERE id = 1").Scan(&enabled))
	assert.Equal(t, `["emails"]`, enabled, "the account default is untouched")
	var rows int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM contact_field_overrides").Scan(&rows))
	assert.Zero(t, rows, "no backfill: every contact inherits")

	_, err = sqlDB.Exec("UPDATE users SET hide_empty_fields = NULL WHERE id = 1")
	require.Error(t, err, "hide_empty_fields is NOT NULL")

	_, err = sqlDB.Exec("INSERT INTO contact_field_overrides (user_id, contact_vcard_uid, field_key, state) VALUES (1, 'uid-a', 'emails', 'hide')")
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO contact_field_overrides (user_id, contact_vcard_uid, field_key, state) VALUES (1, 'uid-a', 'emails', 'show')")
	require.Error(t, err, "one row per (user, contact, field)")
	_, err = sqlDB.Exec("INSERT INTO contact_field_overrides (user_id, contact_vcard_uid, field_key, state) VALUES (1, 'uid-a', 'phones', 'inherit')")
	require.Error(t, err, "inherit is the absence of a row, never stored")
	assert.ErrorContains(t, err, "CHECK constraint failed")

	_, err = sqlDB.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = sqlDB.Exec("DELETE FROM users WHERE id = 1")
	require.NoError(t, err)
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM contact_field_overrides").Scan(&rows))
	assert.Zero(t, rows, "the FK cascades a deleted user's overrides")

	require.NoError(t, m.Migrate(83))
	var cols int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'hide_empty_fields'").Scan(&cols))
	assert.Zero(t, cols)
	var tables int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'contact_field_overrides'").Scan(&tables))
	assert.Zero(t, tables)
}
