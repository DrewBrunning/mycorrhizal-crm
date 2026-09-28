package database

import (
	"database/sql"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uuidV4Pattern matches a canonical lowercase UUIDv4: the version nibble is 4
// and the variant nibble is 8/9/a/b.
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestMigrationBackfillsNoteReminderUUIDs covers 000067 (issue #1260): the
// three added `uuid` columns are backfilled with a genuine UUIDv4 for every
// pre-existing row (notes, reminders and reminder completions), the partial
// unique index enforces uniqueness only over live rows, and the down migration
// drops the indexes and the columns.
//
// The canonical release fixture has no notes/reminders with NULL uuid relative
// to its schema (the column does not exist there), so MIG-03's semantic suite
// sees this as added columns (outside its guarantee); this test seeds the rows
// directly, per EXPECTATION-FILES.md's "validate their backfill with their own
// migration test".
func TestMigrationBackfillsNoteReminderUUIDs(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "note-reminder-uuid.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000067, so the rows below genuinely
	// predate the uuid columns.
	require.NoError(t, m.Steps(66))

	mkUser := func(username string) int64 {
		t.Helper()
		_, err := sqlDB.Exec(
			"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), ?, 'x', ?)",
			username, username+"@example.com")
		require.NoError(t, err)
		var id int64
		require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&id))
		return id
	}
	mkContact := func(firstname string, userID int64) int64 {
		t.Helper()
		_, err := sqlDB.Exec(
			"INSERT INTO contacts (created_at, updated_at, firstname, user_id) VALUES (datetime('now'), datetime('now'), ?, ?)",
			firstname, userID)
		require.NoError(t, err)
		var id int64
		require.NoError(t, sqlDB.QueryRow("SELECT id FROM contacts WHERE firstname = ?", firstname).Scan(&id))
		return id
	}

	userA := mkUser("uuid-a")
	userB := mkUser("uuid-b")
	contactA := mkContact("ContactA", userA)
	contactB := mkContact("ContactB", userB)

	// Two notes per user, a reminder each, and a completion each — enough to
	// prove per-row UUIDs and per-user scoping.
	for _, pair := range []struct {
		contact int64
		user    int64
		label   string
	}{{contactA, userA, "a"}, {contactB, userB, "b"}} {
		_, err := sqlDB.Exec(
			"INSERT INTO notes (created_at, updated_at, content, date, contact_id, user_id) VALUES (datetime('now'), datetime('now'), ?, datetime('now'), ?, ?), (datetime('now'), datetime('now'), ?, datetime('now'), ?, ?)",
			"note-1-"+pair.label, pair.contact, pair.user, "note-2-"+pair.label, pair.contact, pair.user)
		require.NoError(t, err)

		_, err = sqlDB.Exec(
			"INSERT INTO reminders (created_at, updated_at, message, remind_at, recurrence, contact_id, user_id) VALUES (datetime('now'), datetime('now'), ?, datetime('now'), 'once', ?, ?)",
			"reminder-"+pair.label, pair.contact, pair.user)
		require.NoError(t, err)

		_, err = sqlDB.Exec(
			"INSERT INTO reminder_completions (created_at, updated_at, user_id, contact_id, message, completed_at) VALUES (datetime('now'), datetime('now'), ?, ?, ?, datetime('now'))",
			pair.user, pair.contact, "completion-"+pair.label)
		require.NoError(t, err)
	}

	// Apply exactly 000067.
	require.NoError(t, m.Steps(1))

	// Every pre-existing row has a distinct v4 UUID; no NULLs were left.
	// notes has 2 rows per user (4), reminders and completions 1 each (2).
	seen := map[string]bool{}
	for table, want := range map[string]int{"notes": 4, "reminders": 2, "reminder_completions": 2} {
		rows, err := sqlDB.Query("SELECT uuid FROM " + table + " ORDER BY id")
		require.NoError(t, err)
		var n int
		for rows.Next() {
			var u sql.NullString
			require.NoError(t, rows.Scan(&u))
			require.True(t, u.Valid && u.String != "", "%s row got no uuid", table)
			assert.Regexp(t, uuidV4Pattern, u.String, "%s uuid must be a v4 UUID", table)
			assert.False(t, seen[u.String], "%s uuid %q collided across the backfill", table, u.String)
			seen[u.String] = true
			n++
		}
		require.NoError(t, rows.Err())
		assert.Equal(t, want, n, "%s should have the seeded row count", table)
	}

	// The unique indexes exist.
	for _, idx := range []string{"idx_notes_user_uuid", "idx_reminders_user_uuid", "idx_reminder_completions_user_uuid"} {
		var count int64
		require.NoError(t, sqlDB.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&count))
		assert.Equal(t, int64(1), count, "index %s must exist after 000067", idx)
	}

	// The index is partial and per-user: a live duplicate is rejected, but a
	// soft-deleted row does not block re-creating the same UUID.
	var liveUUID string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT uuid FROM notes WHERE user_id = ? ORDER BY id LIMIT 1", userA).Scan(&liveUUID))
	_, err = sqlDB.Exec(
		"INSERT INTO notes (created_at, updated_at, content, date, contact_id, user_id, uuid) VALUES (datetime('now'), datetime('now'), 'dup', datetime('now'), ?, ?, ?)",
		contactA, userA, liveUUID)
	require.Error(t, err, "a live duplicate (user_id, uuid) must be rejected")
	assert.Contains(t, err.Error(), "UNIQUE constraint failed: notes")

	// Another user may reuse the same UUID (per-user scoping).
	_, err = sqlDB.Exec(
		"INSERT INTO notes (created_at, updated_at, content, date, contact_id, user_id, uuid) VALUES (datetime('now'), datetime('now'), 'other', datetime('now'), ?, ?, ?)",
		contactB, userB, liveUUID)
	require.NoError(t, err, "a different user may hold the same uuid")

	// Soft-deleting the live row frees the UUID for re-creation.
	_, err = sqlDB.Exec("UPDATE notes SET deleted_at = datetime('now') WHERE user_id = ? AND uuid = ?", userA, liveUUID)
	require.NoError(t, err)
	_, err = sqlDB.Exec(
		"INSERT INTO notes (created_at, updated_at, content, date, contact_id, user_id, uuid) VALUES (datetime('now'), datetime('now'), 'recreated', datetime('now'), ?, ?, ?)",
		contactA, userA, liveUUID)
	require.NoError(t, err, "the partial index must ignore soft-deleted rows")

	// The down migration drops the indexes and the columns.
	require.NoError(t, MigrateDown(dbPath))
	for _, idx := range []string{"idx_notes_user_uuid", "idx_reminders_user_uuid", "idx_reminder_completions_user_uuid"} {
		var count int64
		require.NoError(t, sqlDB.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&count))
		assert.Equal(t, int64(0), count, "the down migration must drop %s", idx)
	}
	for _, table := range []string{"notes", "reminders", "reminder_completions"} {
		var colCount int64
		require.NoError(t, sqlDB.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'uuid'", table).Scan(&colCount))
		assert.Equal(t, int64(0), colCount, "the down migration must drop %s.uuid", table)
	}
}
