package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationsAddRevisionTokens covers 000044 (issue #591, ADR 0006): the
// monotonic per-row `revision` counter added to every user-authored soft-delete
// entity, plus the `etag` columns notes/reminders never had.
//
// Real production data exists, so the interesting assertions are preservation
// and backfill: rows that predate the migration must survive with their
// revision parsed from the old `e-{id}-{n}` etag suffix (so the counter starts
// ABOVE any historical token and no old etag value can ever recur), and rows
// with no parseable etag must land on the column default of 1.
func TestMigrationsAddRevisionTokens(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "revision-tokens.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000044, so the rows below genuinely
	// predate the new columns.
	require.NoError(t, m.Steps(43))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'revtokens', 'x', 'revtokens@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'revtokens'").Scan(&userID))

	// A contact with the old Unix()-derived etag shape.
	_, err = sqlDB.Exec(`
		INSERT INTO contacts (created_at, updated_at, user_id, firstname, vcard_uid, etag)
		VALUES (datetime('now'), datetime('now'), ?, 'Alice', 'vcard-alice', 'e-12-1787270921')`, userID)
	require.NoError(t, err)
	// A contact whose etag was never written (e.g. a pre-hook legacy row).
	_, err = sqlDB.Exec(`
		INSERT INTO contacts (created_at, updated_at, user_id, firstname, vcard_uid)
		VALUES (datetime('now'), datetime('now'), ?, 'Bob', 'vcard-bob')`, userID)
	require.NoError(t, err)
	// An activity with an old etag, and a life_event with a UUID-PK etag (its
	// suffix must survive the UUID's own dashes).
	_, err = sqlDB.Exec(`
		INSERT INTO activities (created_at, updated_at, title, date, user_id, uuid, etag)
		VALUES (datetime('now'), datetime('now'), 'Coffee', datetime('now'), ?, 'act-1', 'e-7-999')`, userID)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO life_events (id, created_at, updated_at, user_id, entity_id, type, etag)
		VALUES ('evt-1', datetime('now'), datetime('now'), ?, 'vcard-alice', 'graduated', 'e-458bc9ba-b9a7-4853-a3f8-d9cd907bbc9f-1787270921')`, userID)
	require.NoError(t, err)
	// A note and a reminder — neither had an etag column before 000044.
	_, err = sqlDB.Exec(`
		INSERT INTO notes (created_at, updated_at, content, date, contact_id, user_id)
		VALUES (datetime('now'), datetime('now'), 'a note', datetime('now'), (SELECT id FROM contacts WHERE vcard_uid='vcard-alice'), ?)`, userID)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO reminders (created_at, updated_at, message, remind_at, recurrence, contact_id, user_id)
		VALUES (datetime('now'), datetime('now'), 'a reminder', datetime('now'), 'once', (SELECT id FROM contacts WHERE vcard_uid='vcard-alice'), ?)`, userID)
	require.NoError(t, err)

	// Apply 000044.
	require.NoError(t, m.Steps(1))

	// All five tables carry the column.
	for _, table := range []string{"contacts", "activities", "life_events", "notes", "reminders"} {
		var colCount int64
		require.NoError(t, sqlDB.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'revision'", table,
		).Scan(&colCount))
		assert.Equal(t, int64(1), colCount, "%s.revision must be added by 000044", table)
	}
	// notes/reminders gained an etag column too.
	for _, table := range []string{"notes", "reminders"} {
		var colCount int64
		require.NoError(t, sqlDB.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'etag'", table,
		).Scan(&colCount))
		assert.Equal(t, int64(1), colCount, "%s.etag must be added by 000044", table)
	}

	// Backfill: the old etag suffix becomes the starting revision.
	var aliceRev, bobRev int64
	require.NoError(t, sqlDB.QueryRow("SELECT revision FROM contacts WHERE vcard_uid = 'vcard-alice'").Scan(&aliceRev))
	require.NoError(t, sqlDB.QueryRow("SELECT revision FROM contacts WHERE vcard_uid = 'vcard-bob'").Scan(&bobRev))
	assert.Equal(t, int64(1787270921), aliceRev, "revision must be parsed from the old etag suffix, so the counter starts above any historical token")
	assert.Equal(t, int64(1), bobRev, "a row with no etag keeps the column default of 1")

	var actRev int64
	require.NoError(t, sqlDB.QueryRow("SELECT revision FROM activities WHERE uuid = 'act-1'").Scan(&actRev))
	assert.Equal(t, int64(999), actRev)

	// LifeEvent: the suffix after the UUID's own dashes.
	var evtRev int64
	require.NoError(t, sqlDB.QueryRow("SELECT revision FROM life_events WHERE id = 'evt-1'").Scan(&evtRev))
	assert.Equal(t, int64(1787270921), evtRev)

	// Existing etag strings are unchanged (the migration only adds columns +
	// revision; it must not rewrite the etag values CardDAV clients hold).
	var aliceETag string
	require.NoError(t, sqlDB.QueryRow("SELECT etag FROM contacts WHERE vcard_uid = 'vcard-alice'").Scan(&aliceETag))
	assert.Equal(t, "e-12-1787270921", aliceETag)

	// Down drops the columns; the rows themselves survive.
	require.NoError(t, MigrateDown(dbPath))
	for _, table := range []string{"contacts", "activities", "life_events", "notes", "reminders"} {
		var colCount int64
		require.NoError(t, sqlDB.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'revision'", table,
		).Scan(&colCount))
		assert.Equal(t, int64(0), colCount, "%s.revision must be removed by the down migration", table)
	}
	for _, table := range []string{"notes", "reminders"} {
		var colCount int64
		require.NoError(t, sqlDB.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'etag'", table,
		).Scan(&colCount))
		assert.Equal(t, int64(0), colCount, "%s.etag must be removed by the down migration", table)
	}
	var survivor string
	require.NoError(t, sqlDB.QueryRow("SELECT firstname FROM contacts WHERE vcard_uid = 'vcard-alice'").Scan(&survivor))
	assert.Equal(t, "Alice", survivor, "a rollback must not destroy contact data")
}
