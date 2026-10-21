package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationTargetCloseness covers 000076 (issue #1796): the nullable
// contacts.target_closeness column (every existing row reads NULL, the CHECK
// pins the vocabulary) and the data-preserving reach_out_suggestions rebuild
// that widens the kind CHECK. Every existing row, of every kind and status,
// survives up byte-for-byte; down deletes ONLY target_behind rows.
func TestMigrationTargetCloseness(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "target-closeness.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(75))

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'tc', 'x', 'tc@example.com')")
	require.NoError(t, err)
	const stamp = "2020-01-02 03:04:05"
	res, err := sqlDB.Exec("INSERT INTO contacts (created_at, updated_at, firstname, user_id, is_favorite) VALUES (?, ?, 'Ada', 1, 1)", stamp, stamp)
	require.NoError(t, err)
	contactID, err := res.LastInsertId()
	require.NoError(t, err)

	// Pre-existing suggestions: every kind x status, with and without a reminder.
	type sug struct {
		id, kind, status string
		reminder         any
	}
	existing := []sug{
		{"s-org-pending", "organization", "pending", 7},
		{"s-org-dismissed", "organization", "dismissed", nil},
		{"s-title-pending", "title", "pending", nil},
		{"s-title-dismissed", "title", "dismissed", 9},
		{"s-addr-pending", "address", "pending", 11},
		{"s-addr-dismissed", "address", "dismissed", nil},
	}
	for i, s := range existing {
		_, err := sqlDB.Exec(`INSERT INTO reach_out_suggestions
			(id, created_at, updated_at, user_id, contact_vcard_uid, kind, old_value, new_value, audit_event_id, reminder_id, status)
			VALUES (?, ?, ?, 1, 'uid-1', ?, ?, ?, ?, ?, ?)`,
			s.id, stamp, stamp, s.kind, "old-"+s.id, "new-"+s.id, 100+i, s.reminder, s.status)
		require.NoError(t, err)
	}
	// target_behind is not yet a legal kind.
	_, err = sqlDB.Exec(`INSERT INTO reach_out_suggestions (id, created_at, updated_at, user_id, contact_vcard_uid, kind, audit_event_id)
		VALUES ('early', ?, ?, 1, 'uid-1', 'target_behind', 0)`, stamp, stamp)
	require.Error(t, err)

	snapshot := func() map[string]string {
		t.Helper()
		rows, err := sqlDB.Query(`SELECT id, created_at, updated_at, user_id, contact_vcard_uid, kind, old_value, new_value,
			audit_event_id, COALESCE(reminder_id, -1), status FROM reach_out_suggestions WHERE kind != 'target_behind' ORDER BY id`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var id, c, u, uid, kind, oldV, newV, status string
			var userID, audit, reminder int
			require.NoError(t, rows.Scan(&id, &c, &u, &userID, &uid, &kind, &oldV, &newV, &audit, &reminder, &status))
			out[id] = fmt.Sprint(c, u, userID, uid, kind, oldV, newV, audit, reminder, status)
		}
		require.NoError(t, rows.Err())
		return out
	}
	before := snapshot()
	require.Len(t, before, len(existing))

	require.NoError(t, m.Migrate(76))

	assert.Equal(t, before, snapshot(), "the rebuild copies every existing suggestion verbatim")

	// The new column reads NULL on the existing contact; nothing else changed.
	var target sql.NullString
	var first string
	var fav int
	require.NoError(t, sqlDB.QueryRow("SELECT target_closeness, firstname, is_favorite FROM contacts WHERE id = ?", contactID).Scan(&target, &first, &fav))
	assert.False(t, target.Valid)
	assert.Equal(t, "Ada", first)
	assert.Equal(t, 1, fav)

	// The CHECK pins the vocabulary; NULL and each tier are legal.
	for _, tier := range []string{"immediate", "close", "casual", "distant"} {
		_, err := sqlDB.Exec("UPDATE contacts SET target_closeness = ? WHERE id = ?", tier, contactID)
		require.NoError(t, err, tier)
	}
	_, err = sqlDB.Exec("UPDATE contacts SET target_closeness = 'bestie' WHERE id = ?", contactID)
	require.Error(t, err)
	_, err = sqlDB.Exec("UPDATE contacts SET target_closeness = NULL WHERE id = ?", contactID)
	require.NoError(t, err)

	// target_behind is now legal, an unknown kind still is not, and the kind's
	// status CHECK and defaults survived the rebuild.
	_, err = sqlDB.Exec(`INSERT INTO reach_out_suggestions (id, created_at, updated_at, user_id, contact_vcard_uid, kind, old_value, new_value, audit_event_id)
		VALUES ('tb-1', ?, ?, 1, 'uid-1', 'target_behind', 'close', '2030-01-01T00:00:00Z', 0)`, stamp, stamp)
	require.NoError(t, err)
	var status, oldV string
	require.NoError(t, sqlDB.QueryRow("SELECT status, old_value FROM reach_out_suggestions WHERE id = 'tb-1'").Scan(&status, &oldV))
	assert.Equal(t, "pending", status)
	assert.Equal(t, "close", oldV)
	_, err = sqlDB.Exec(`INSERT INTO reach_out_suggestions (id, created_at, updated_at, user_id, contact_vcard_uid, kind, audit_event_id)
		VALUES ('bad', ?, ?, 1, 'uid-1', 'nonsense', 0)`, stamp, stamp)
	require.Error(t, err)
	_, err = sqlDB.Exec(`INSERT INTO reach_out_suggestions (id, created_at, updated_at, user_id, contact_vcard_uid, kind, audit_event_id, status)
		VALUES ('bad2', ?, ?, 1, 'uid-1', 'title', 0, 'bogus')`, stamp, stamp)
	require.Error(t, err)

	// The three indexes are recreated.
	for _, idx := range []string{"idx_reach_out_suggestions_user_status", "idx_reach_out_suggestions_contact", "idx_reach_out_suggestions_reminder"} {
		var n int
		require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", idx).Scan(&n))
		assert.Equal(t, 1, n, idx)
	}

	// Down: deletes only target_behind rows, keeps everything else, drops the column.
	require.NoError(t, m.Migrate(75))
	assert.Equal(t, before, snapshot(), "rollback keeps every organization/title/address row")
	var tb int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM reach_out_suggestions WHERE id = 'tb-1'").Scan(&tb))
	assert.Zero(t, tb, "target_behind rows are system-generated and deleted on rollback")
	var total int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM reach_out_suggestions").Scan(&total))
	assert.Equal(t, len(existing), total)
	var cols int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('contacts') WHERE name = 'target_closeness'").Scan(&cols))
	assert.Zero(t, cols)
	require.NoError(t, sqlDB.QueryRow("SELECT firstname FROM contacts WHERE id = ?", contactID).Scan(&first))
	assert.Equal(t, "Ada", first)
	_, err = sqlDB.Exec(`INSERT INTO reach_out_suggestions (id, created_at, updated_at, user_id, contact_vcard_uid, kind, audit_event_id)
		VALUES ('late', ?, ?, 1, 'uid-1', 'target_behind', 0)`, stamp, stamp)
	require.Error(t, err, "the narrowed CHECK is back")
	for _, idx := range []string{"idx_reach_out_suggestions_user_status", "idx_reach_out_suggestions_contact", "idx_reach_out_suggestions_reminder"} {
		var n int
		require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", idx).Scan(&n))
		assert.Equal(t, 1, n, idx)
	}

	// And forward again works on the rolled-back database.
	require.NoError(t, m.Migrate(76))
}
