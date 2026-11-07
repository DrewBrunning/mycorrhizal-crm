package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationShareGrantLifecycle covers 000092 (issue #2291, ADR 0042
// decision 11): the audit_events CHECK is rebuilt to accept the four new
// share-grant operations with every existing row, index and the append-only
// trigger preserved; share_grant_notifications is created with its CHECK constraints and
// FK cascade; and the down migration is reversible (new-op audit rows are
// discarded, everything else survives, the notifications table is dropped).
func TestMigrationShareGrantLifecycle(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "share-grant-lifecycle.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(91))

	const stamp = "2020-01-02 03:04:05"
	for _, u := range []struct {
		id   int
		name string
	}{{1, "alice"}, {2, "bob"}} {
		_, err = sqlDB.Exec("INSERT INTO users (id, created_at, updated_at, username, password, email) VALUES (?, ?, ?, ?, 'x', ?)",
			u.id, stamp, stamp, u.name, u.name+"@example.com")
		require.NoError(t, err)
	}
	_, err = sqlDB.Exec(`INSERT INTO audit_events (id, created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot, hash, prev_hash)
		VALUES (7, ?, ?, 'contact', 'c-1', 'update', 2, '{"a":1}', 'h2', 'h1')`, stamp, stamp)
	require.NoError(t, err)

	// The old CHECK refuses the new operations.
	_, err = sqlDB.Exec(`INSERT INTO audit_events (entity_type, entity_id, operation, user_id) VALUES ('share_grant', 'g', 'accept', 1)`)
	require.Error(t, err)

	require.NoError(t, m.Migrate(92))

	var snap, hash, prev string
	require.NoError(t, sqlDB.QueryRow("SELECT before_snapshot, hash, prev_hash FROM audit_events WHERE id = 7").Scan(&snap, &hash, &prev))
	assert.Equal(t, `{"a":1}`, snap)
	assert.Equal(t, "h2", hash)
	assert.Equal(t, "h1", prev, "the chain hashes survive the rebuild")

	for _, op := range []string{"accept", "decline", "expire", "reconfirm", "revoke", "create", "update"} {
		_, err = sqlDB.Exec(`INSERT INTO audit_events (entity_type, entity_id, operation, user_id) VALUES ('share_grant', 'g', ?, 2)`, op)
		require.NoError(t, err, op)
	}
	_, err = sqlDB.Exec(`INSERT INTO audit_events (entity_type, entity_id, operation, user_id) VALUES ('share_grant', 'g', 'bogus', 2)`)
	require.Error(t, err, "the CHECK still rejects unknown operations")

	_, err = sqlDB.Exec("UPDATE audit_events SET entity_id = 'x' WHERE id = 7")
	require.Error(t, err, "the append-only trigger is recreated")
	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name IN ('idx_audit_events_entity','idx_audit_events_user','idx_audit_events_created')").Scan(&n))
	assert.Equal(t, 3, n)

	// Notifications: CHECK constraints, FK cascade, starts empty.
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM share_grant_notifications").Scan(&n))
	assert.Zero(t, n)
	ins := func(kind, etype string) error {
		_, err := sqlDB.Exec(`INSERT INTO share_grant_notifications (user_id, from_user_id, kind, entity_type, created_at, updated_at) VALUES (2, 1, ?, ?, ?, ?)`, kind, etype, stamp, stamp)
		return err
	}
	require.NoError(t, ins("pending", "contact"))
	require.NoError(t, ins("revoked", "circle"))
	require.Error(t, ins("other", "contact"))
	require.Error(t, ins("pending", "tag"))
	_, err = sqlDB.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = sqlDB.Exec("DELETE FROM users WHERE id = 1")
	require.NoError(t, err)
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM share_grant_notifications").Scan(&n))
	assert.Zero(t, n, "deleting the sender cascades their notifications")

	// Down: new-op rows go, the rest stays, the table is dropped.
	require.NoError(t, m.Migrate(91))
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE operation IN ('accept','decline','expire','reconfirm')").Scan(&n))
	assert.Zero(t, n)
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE id = 7 AND hash = 'h2'").Scan(&n))
	assert.Equal(t, 1, n)
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE entity_type = 'share_grant' AND operation IN ('create','update','revoke')").Scan(&n))
	assert.Equal(t, 3, n)
	err = sqlDB.QueryRow("SELECT COUNT(*) FROM share_grant_notifications").Scan(&n)
	require.Error(t, err)
	_, err = sqlDB.Exec("UPDATE audit_events SET entity_id = 'x' WHERE id = 7")
	require.Error(t, err, "the append-only trigger is back after the down")
}
