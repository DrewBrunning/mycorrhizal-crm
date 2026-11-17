package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationBackfillsZeroNoteDates covers 000095 (issue #2663): the note an
// import wrote when it merged into an existing contact was saved with the zero
// time (0001-01-01), so every such note sorted last and rendered as 01.01.0001.
// The migration re-dates exactly those rows to their created_at (the moment the
// import ran), leaves every real date alone (soft-deleted rows included, since
// restoring a note from Trash must not bring the bad date back), and is a no-op
// the second time round.
func TestMigrationBackfillsZeroNoteDates(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "note-zero-date.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(94))

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'zd', 'x', 'zd@example.com')")
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO contacts (id, created_at, updated_at, firstname, user_id) VALUES (1, datetime('now'), datetime('now'), 'Ada', 1)")
	require.NoError(t, err)

	const zero = "0001-01-01 00:00:00+00:00" // how GORM stores a time.Time{}
	const importedAt = "2026-11-10 08:15:00.123456789+00:00"
	const realDate = "2024-03-05 00:00:00+00:00"
	const deletedAt = "2026-11-11 09:00:00+00:00"
	ins := func(id int, content, date string, deleted any) {
		t.Helper()
		_, err := sqlDB.Exec(`INSERT INTO notes (id, created_at, updated_at, deleted_at, user_id, content, date, contact_id)
			VALUES (?, ?, ?, ?, 1, ?, ?, 1)`, id, importedAt, importedAt, deleted, content, date)
		require.NoError(t, err)
	}
	ins(1, "CSV Import updated this contact.", zero, nil)
	ins(2, "a note the user dated", realDate, nil)
	ins(3, "VCF Import updated this contact.", zero, deletedAt) // in Trash
	ins(4, "a note dated at the very start of 0002", "0002-01-01 00:00:00+00:00", nil)

	require.NoError(t, m.Migrate(95))

	date := func(id int) string {
		t.Helper()
		var d string
		require.NoError(t, sqlDB.QueryRow("SELECT CAST(date AS TEXT) FROM notes WHERE id = ?", id).Scan(&d))
		return d
	}
	assert.Equal(t, importedAt, date(1), "a zero-dated merge note takes its created_at")
	assert.Equal(t, realDate, date(2), "a real date is untouched")
	assert.Equal(t, importedAt, date(3), "a soft-deleted zero-dated note is repaired too")
	assert.Equal(t, "0002-01-01 00:00:00+00:00", date(4), "only the zero year is rewritten")

	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM notes").Scan(&n))
	assert.Equal(t, 4, n, "no note is added or removed")
	var content string
	require.NoError(t, sqlDB.QueryRow("SELECT content FROM notes WHERE id = 1").Scan(&content))
	assert.Equal(t, "CSV Import updated this contact.", content)

	// down is a deliberate no-op (the zero date carried no information), and
	// re-applying changes nothing further.
	require.NoError(t, m.Migrate(94))
	assert.Equal(t, importedAt, date(1), "down does not put the bad date back")
	require.NoError(t, m.Migrate(95))
	assert.Equal(t, importedAt, date(1))
	assert.Equal(t, realDate, date(2))
}
