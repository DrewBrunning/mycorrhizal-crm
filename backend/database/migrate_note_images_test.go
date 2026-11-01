package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationsAddNoteImages covers 000087 (ADR 0040 slice b, issue
// #2004): the note_images table is created with its ownership indexes, a
// valid row inserts, and the down migration drops it without touching the
// pre-existing contact, note or attachment rows.
func TestMigrationsAddNoteImages(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "note-images.db")

	db, err := InitDB(dbPath)
	require.NoError(t, err, "full migration chain incl. 000087 must apply to an empty database")
	sqlDB, err := db.DB()
	require.NoError(t, err)

	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='note_images'",
	).Scan(&tableCount))
	assert.Equal(t, int64(1), tableCount)

	var idxCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN ('idx_note_images_uuid','idx_note_images_user_id','idx_note_images_note_id')",
	).Scan(&idxCount))
	assert.Equal(t, int64(3), idxCount)

	_, err = sqlDB.Exec(`INSERT INTO users (created_at, updated_at, username, password, email, is_admin) VALUES (datetime('now'), datetime('now'), 'migrate-images', 'x', 'migrate-images@example.com', 0)`)
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'migrate-images'").Scan(&userID))
	_, err = sqlDB.Exec(`INSERT INTO notes (created_at, updated_at, user_id, content, date, uuid, etag, revision) VALUES (datetime('now'), datetime('now'), ?, 'hello', datetime('now'), 'a-1', 'e-1-1', 1)`, userID)
	require.NoError(t, err)
	var noteID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM notes WHERE uuid = 'a-1'").Scan(&noteID))
	_, err = sqlDB.Exec(`INSERT INTO note_images (created_at, updated_at, user_id, uuid, note_id, stored_name, thumb_name, mime_type, thumb_mime_type, width, height, size_bytes, thumb_size_bytes, sensitivity) VALUES (datetime('now'), datetime('now'), ?, 'i-1', ?, 's.jpg', 's_t.jpg', 'image/jpeg', 'image/jpeg', 10, 10, 100, 10, 'normal')`, userID, noteID)
	require.NoError(t, err)

	require.NoError(t, sqlDB.Close())
	require.NoError(t, MigrateDown(dbPath))

	sqlDB2, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB2.Close()
	require.NoError(t, sqlDB2.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='note_images'",
	).Scan(&tableCount))
	assert.Zero(t, tableCount, "the down migration must drop note_images")

	// Existing production data is untouched by the down migration.
	var notes, users int64
	require.NoError(t, sqlDB2.QueryRow("SELECT COUNT(*) FROM notes").Scan(&notes))
	require.NoError(t, sqlDB2.QueryRow("SELECT COUNT(*) FROM users").Scan(&users))
	assert.Equal(t, int64(1), notes)
	assert.Equal(t, int64(1), users)
}
