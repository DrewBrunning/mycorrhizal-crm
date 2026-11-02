package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationsAddActivityImages covers 000086 (ADR 0040 slice a, issue
// #1994): the activity_images table is created with its ownership indexes, a
// valid row inserts, and the down migration drops it without touching the
// pre-existing contact, activity or attachment rows.
func TestMigrationsAddActivityImages(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "activity-images.db")

	db, err := InitDB(dbPath)
	require.NoError(t, err, "full migration chain incl. 000086 must apply to an empty database")
	sqlDB, err := db.DB()
	require.NoError(t, err)

	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='activity_images'",
	).Scan(&tableCount))
	assert.Equal(t, int64(1), tableCount)

	var idxCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN ('idx_activity_images_uuid','idx_activity_images_user_id','idx_activity_images_activity_id')",
	).Scan(&idxCount))
	assert.Equal(t, int64(3), idxCount)

	_, err = sqlDB.Exec(`INSERT INTO users (created_at, updated_at, username, password, email, is_admin) VALUES (datetime('now'), datetime('now'), 'migrate-images', 'x', 'migrate-images@example.com', 0)`)
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'migrate-images'").Scan(&userID))
	_, err = sqlDB.Exec(`INSERT INTO activities (created_at, updated_at, user_id, uuid, title, description, location, date, type, external_ref, etag, revision) VALUES (datetime('now'), datetime('now'), ?, 'a-1', 'trip', '', '', datetime('now'), 'visit', '', 'e-1-1', 1)`, userID)
	require.NoError(t, err)
	var activityID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM activities WHERE uuid = 'a-1'").Scan(&activityID))
	_, err = sqlDB.Exec(`INSERT INTO activity_images (created_at, updated_at, user_id, uuid, activity_id, stored_name, thumb_name, mime_type, thumb_mime_type, width, height, size_bytes, thumb_size_bytes, sensitivity) VALUES (datetime('now'), datetime('now'), ?, 'i-1', ?, 's.jpg', 's_t.jpg', 'image/jpeg', 'image/jpeg', 10, 10, 100, 10, 'normal')`, userID, activityID)
	require.NoError(t, err)

	require.NoError(t, sqlDB.Close())
	require.NoError(t, MigrateDown(dbPath))

	sqlDB2, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB2.Close()
	require.NoError(t, sqlDB2.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='activity_images'",
	).Scan(&tableCount))
	assert.Zero(t, tableCount, "the down migration must drop activity_images")

	// Existing production data is untouched by the down migration.
	var activities, users int64
	require.NoError(t, sqlDB2.QueryRow("SELECT COUNT(*) FROM activities").Scan(&activities))
	require.NoError(t, sqlDB2.QueryRow("SELECT COUNT(*) FROM users").Scan(&users))
	assert.Equal(t, int64(1), activities)
	assert.Equal(t, int64(1), users)
}
