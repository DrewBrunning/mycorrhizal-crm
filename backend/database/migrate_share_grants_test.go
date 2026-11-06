package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationAddsShareGrants covers 000090 (issue #2289, ADR 0042 decisions
// 2, 8 and 13): a populated database upgrades with every pre-existing row
// intact (including one-time contact_shares, which the new tables never
// touch), the new tables start empty, every CHECK / FK / index in the ADR
// holds, history rows coexist with one live grant, and the down migration
// drops only the new objects.
func TestMigrationAddsShareGrants(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "share-grants.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(89))

	const stamp = "2020-01-02 03:04:05"
	for _, u := range []struct {
		id   int
		name string
	}{{1, "alice"}, {2, "bob"}} {
		_, err = sqlDB.Exec("INSERT INTO users (id, created_at, updated_at, username, password, email) VALUES (?, ?, ?, ?, 'x', ?)",
			u.id, stamp, stamp, u.name, u.name+"@example.com")
		require.NoError(t, err)
	}
	_, err = sqlDB.Exec("INSERT INTO contact_shares (id, created_at, updated_at, from_user_id, to_user_id, contact_display_name, payload, status) VALUES ('cs-1', ?, ?, 1, 2, 'Carol', '{}', 'pending')", stamp, stamp)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(90))

	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM contact_shares WHERE id = 'cs-1' AND payload = '{}'").Scan(&n))
	assert.Equal(t, 1, n, "the frozen one-time share is untouched")
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&n))
	assert.Equal(t, 2, n)
	for _, tbl := range []string{"share_grants", "share_blocks"} {
		require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM "+tbl).Scan(&n))
		assert.Zero(t, n, "%s starts empty (no backfill)", tbl)
	}

	insert := func(id string, from, to int, etype, eid, status string, reason any) error {
		_, err := sqlDB.Exec(`INSERT INTO share_grants
			(id, from_user_id, to_user_id, entity_type, entity_id, status, revoked_reason, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, from, to, etype, eid, status, reason, stamp, stamp)
		return err
	}

	// Defaults.
	require.NoError(t, insert("g-live", 1, 2, "contact", "uid-a", "pending", nil))
	var sections, reconfirm string
	var revision int
	require.NoError(t, sqlDB.QueryRow("SELECT sections, reconfirm_sections, revision FROM share_grants WHERE id = 'g-live'").Scan(&sections, &reconfirm, &revision))
	assert.Equal(t, "[]", sections)
	assert.Equal(t, "[]", reconfirm)
	assert.Equal(t, 1, revision)

	// Check constraints.
	err = insert("g-self", 1, 1, "contact", "uid-a", "pending", nil)
	require.Error(t, err, "a user cannot grant to themselves")
	assert.ErrorContains(t, err, "CHECK constraint failed")
	err = insert("g-type", 1, 2, "reminder", "x", "pending", nil)
	require.Error(t, err, "entity_type is a closed set")
	assert.ErrorContains(t, err, "CHECK constraint failed")
	err = insert("g-status", 1, 2, "note", "x", "snoozed", nil)
	require.Error(t, err, "status is a closed set")
	assert.ErrorContains(t, err, "CHECK constraint failed")
	err = insert("g-reason", 1, 2, "note", "x", "revoked", "because")
	require.Error(t, err, "revoked_reason is a closed set")
	assert.ErrorContains(t, err, "CHECK constraint failed")
	for _, st := range []string{"pending", "active", "declined", "revoked", "expired", "blocked"} {
		require.NoError(t, insert("g-st-"+st, 1, 2, "note", "st-"+st, st, nil), st)
	}
	for _, r := range []string{"owner_revoked", "entity_deleted", "recipient_deleted"} {
		require.NoError(t, insert("g-r-"+r, 1, 2, "circle", "r-"+r, "revoked", r), r)
	}

	// Partial unique live-grant index: a second live grant for the same
	// (from, to, type, entity) is rejected for every live status...
	for _, st := range []string{"pending", "active", "blocked"} {
		err = insert("g-dup-"+st, 1, 2, "contact", "uid-a", st, nil)
		require.Error(t, err, "duplicate live grant (%s)", st)
		assert.ErrorContains(t, err, "UNIQUE constraint failed")
	}
	// ...but history rows coexist with the live one, and with each other.
	require.NoError(t, insert("g-h1", 1, 2, "contact", "uid-a", "revoked", "owner_revoked"))
	require.NoError(t, insert("g-h2", 1, 2, "contact", "uid-a", "declined", nil))
	require.NoError(t, insert("g-h3", 1, 2, "contact", "uid-a", "expired", nil))
	// A different recipient or direction is a different grant.
	require.NoError(t, insert("g-other-dir", 2, 1, "contact", "uid-a", "pending", nil))
	// Once the live grant ends, a new live one may be created.
	_, err = sqlDB.Exec("UPDATE share_grants SET status = 'revoked', revoked_reason = 'owner_revoked' WHERE id = 'g-live'")
	require.NoError(t, err)
	require.NoError(t, insert("g-new", 1, 2, "contact", "uid-a", "active", nil))

	// Indexes named in ADR 0042 decision 13.
	for _, idx := range []string{
		"idx_share_grants_to_user_status",
		"idx_share_grants_from_user_status",
		"idx_share_grants_entity_status",
		"idx_share_grants_live_unique",
	} {
		require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", idx).Scan(&n))
		assert.Equal(t, 1, n, idx)
	}

	// share_blocks: composite primary key, so a pair is unique.
	_, err = sqlDB.Exec("INSERT INTO share_blocks (blocker_user_id, blocked_user_id, created_at) VALUES (2, 1, ?)", stamp)
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO share_blocks (blocker_user_id, blocked_user_id, created_at) VALUES (2, 1, ?)", stamp)
	require.Error(t, err, "one block row per (blocker, blocked)")
	_, err = sqlDB.Exec("INSERT INTO share_blocks (blocker_user_id, blocked_user_id, created_at) VALUES (1, 2, ?)", stamp)
	require.NoError(t, err, "the reverse pair is a separate block")

	// FKs: a deleted user takes both tables' rows with it, either side.
	_, err = sqlDB.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO share_grants (id, from_user_id, to_user_id, entity_type, entity_id, status, created_at, updated_at) VALUES ('g-orphan', 1, 999, 'note', 'n', 'pending', ?, ?)", stamp, stamp)
	require.Error(t, err, "to_user_id must reference users")
	_, err = sqlDB.Exec("DELETE FROM users WHERE id = 2")
	require.NoError(t, err)
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM share_grants").Scan(&n))
	assert.Zero(t, n, "every grant involved user 2 as sender or recipient")
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM share_blocks").Scan(&n))
	assert.Zero(t, n)

	// Down migration drops only the new objects.
	require.NoError(t, m.Migrate(89))
	for _, tbl := range []string{"share_grants", "share_blocks"} {
		require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = ?", tbl).Scan(&n))
		assert.Zero(t, n, tbl)
	}
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'idx_share_%'").Scan(&n))
	assert.Zero(t, n, "indexes go with the tables")
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM contact_shares").Scan(&n))
	assert.Zero(t, n, "contact_shares rows cascaded with user 2; the table itself survives the down migration")
	require.NoError(t, m.Migrate(90), "up again after down round-trips")
}
