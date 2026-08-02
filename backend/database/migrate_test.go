package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// columnExists reports whether table has the named column, via SQLite's
// table_info pragma.
func columnExists(t *testing.T, dbPath, table, column string) bool {
	t.Helper()

	db, err := InitDB(dbPath)
	require.NoError(t, err)

	var count int64
	err = db.Raw(
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column,
	).Scan(&count).Error
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	return count == 1
}

// Applies the full migration chain to an empty database. Guards against a new
// migration that only works against a schema that already has the column, and
// against SQL that parses but fails on this SQLite build.
func TestMigrationsApplyToEmptyDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")

	db, err := InitDB(dbPath)
	require.NoError(t, err, "migrations must apply cleanly to an empty database")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// TestMigrationDropsLegacyRelationshipsTable is the regression test for §3d
// WP5 (docs/fork-plan/95-backlog-and-priorities.md): migration 000035 must
// actually remove the legacy `relationships` table against the real
// migration chain, not just leave a Go model with no backing table.
func TestMigrationDropsLegacyRelationshipsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "drop-relationships.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'relationships'",
	).Scan(&count).Error)
	assert.Zero(t, count, "relationships table should be dropped by migration 000035")

	// relationship_edges (its replacement, WP-80) must still be present.
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'relationship_edges'",
	).Scan(&count).Error)
	assert.Equal(t, int64(1), count, "relationship_edges table should still exist")
}

func TestMigrationsAddCredentialLifecycleColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "columns.db")

	assert.True(t, columnExists(t, dbPath, "users", "token_version"),
		"users.token_version is required to invalidate JWTs on password change")
	assert.True(t, columnExists(t, dbPath, "api_tokens", "expires_at"),
		"api_tokens.expires_at is required to bound API token lifetime")
}

// TestForeignKeysEnforced is the regression test for Tier 3c item 8
// (docs/fork-plan/95-backlog-and-priorities.md): foreign_keys is a
// per-connection SQLite setting, not persisted in the database file, so it
// must be supplied via the DSN on every InitDB call (openDSN) rather than a
// one-time PRAGMA statement.
func TestForeignKeysEnforced(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fk.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	var enabled int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Scan(&enabled).Error)
	assert.Equal(t, 1, enabled)
}

// TestForeignKeyCascadeDeletesOrphanedChildRows proves foreign_keys is not
// just set but actually enforced: deleting a circle now auto-cascades its
// circle_members rows at the SQLite level, closing a real orphan-row gap
// DeleteCircle's own code doesn't handle explicitly (household_members/
// contact_tags/field_values have the identical shape and rely on the same
// declared ON DELETE CASCADE).
func TestForeignKeyCascadeDeletesOrphanedChildRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fk-cascade.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	require.NoError(t, db.Exec(
		"INSERT INTO users (created_at, updated_at, username, email, password) VALUES (datetime('now'), datetime('now'), 'u', 'u@example.com', 'x')",
	).Error)
	var userID int64
	require.NoError(t, db.Raw("SELECT id FROM users WHERE username = 'u'").Scan(&userID).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO circles (id, created_at, updated_at, user_id, name) VALUES ('circle-1', datetime('now'), datetime('now'), ?, 'c')",
		userID,
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO circle_members (created_at, updated_at, circle_id, user_id, member_vcard_uid) VALUES (datetime('now'), datetime('now'), 'circle-1', ?, 'vcard-1')",
		userID,
	).Error)

	require.NoError(t, db.Exec("DELETE FROM circles WHERE id = 'circle-1'").Error)

	var remaining int64
	require.NoError(t, db.Table("circle_members").Where("circle_id = 'circle-1'").Count(&remaining).Error)
	assert.Zero(t, remaining, "circle_members should be auto-cascaded when its parent circle is deleted")
}

// TestOpenDSN_PragmasArePresent verifies that openDSN appends the two pragmas
// the app requires for correctness:
//   - journal_mode(WAL): persisted once set, needed for safe hot-backup.
//   - foreign_keys(1): per-connection, needed for FK enforcement (Tier 3c item 8).
func TestOpenDSN_PragmasArePresent(t *testing.T) {
	dsn := openDSN("/path/to/db.sqlite")
	assert.True(t, strings.Contains(dsn, "_pragma=journal_mode(WAL)"),
		"openDSN must include the WAL journal pragma")
	assert.True(t, strings.Contains(dsn, "_pragma=foreign_keys(1)"),
		"openDSN must include the foreign_keys pragma")
	assert.True(t, strings.HasPrefix(dsn, "/path/to/db.sqlite?"),
		"openDSN must preserve the db path as the DSN prefix")
}

// newMigrateForTest wires a golang-migrate instance over the same embed.FS
// and driver RunMigrations uses, so a test can stop the chain partway
// (m.Migrate(40)) to insert rows exactly as they existed before a
// migration, then resume (m.Up()).
func newMigrateForTest(t *testing.T, dbPath string) (*migrate.Migrate, *sql.DB) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	driver, err := withInstance(sqlDB, &sqliteConfig{})
	require.NoError(t, err)

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	require.NoError(t, err)

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", driver)
	require.NoError(t, err)
	return m, sqlDB
}

// TestMigration000041_BackfillsETag is the regression test for T12a's
// backfill requirement (docs/fork-plan/tickets/14-T12a-etag-primitives.md),
// mirroring 000030's own backfill precedent for activities.uuid: a row that
// predates the migration (here simulated by stopping the chain at 000040,
// inserting rows, then resuming) must come out of the migration with an
// ETag derived from its own id + updated_at — not wait for its next write —
// or CalDAV clients (T12b) cannot cache pre-existing resources.
func TestMigration000041_BackfillsETag(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "etag-backfill.db")
	m, sqlDB := newMigrateForTest(t, dbPath)

	// Apply everything through 000040 so the rows below are inserted into
	// tables that have not yet seen the etag columns.
	require.NoError(t, m.Migrate(40))

	_, err := sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, email, password) VALUES (datetime('now'), datetime('now'), 'u', 'u@example.com', 'x')",
	)
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'u'").Scan(&userID))

	// Pre-migration rows with fixed updated_at timestamps (the backfill must
	// key each ETag off its own row, not a shared value).
	_, err = sqlDB.Exec(
		"INSERT INTO activities (created_at, updated_at, title, date, user_id) VALUES ('2026-01-01 09:00:00', '2026-01-02 15:04:05', 'Coffee', '2026-01-02 15:00:00', ?)",
		userID,
	)
	require.NoError(t, err)
	var activityID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM activities WHERE title = 'Coffee'").Scan(&activityID))

	_, err = sqlDB.Exec(
		"INSERT INTO life_events (id, created_at, updated_at, user_id, entity_id, type) VALUES ('evt-1', '2026-01-01 09:00:00', '2026-01-03 10:11:12', ?, 'entity-1', 'graduated')",
		userID,
	)
	require.NoError(t, err)

	// Resume the chain: 000041 adds the columns and backfills them.
	require.NoError(t, m.Up())

	// The backfill's output must equal the same 'e-{id}-{updated_at_unix}'
	// derivation the migration itself ran and the model's AfterCreate/
	// AfterSave hooks produce, computed against the same stored updated_at.
	var activityETag, activityExpected string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT etag, 'e-' || id || '-' || CAST(strftime('%s', updated_at) AS TEXT) FROM activities WHERE id = ?", activityID,
	).Scan(&activityETag, &activityExpected))
	assert.Equal(t, activityExpected, activityETag, "a pre-existing Activity must come out of the migration backfilled")

	var eventETag, eventExpected string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT etag, 'e-' || id || '-' || CAST(strftime('%s', updated_at) AS TEXT) FROM life_events WHERE id = 'evt-1'",
	).Scan(&eventETag, &eventExpected))
	assert.Equal(t, eventExpected, eventETag, "a pre-existing LifeEvent (UUID PK) must come out of the migration backfilled")
}
