package database

import (
	"path/filepath"
	"strings"
	"testing"

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

// TestSquashedSchemaHasNoLegacyRelationshipsTable verifies the squashed
// baseline (T22) never creates the legacy `relationships` table — it was
// dropped in §3d WP5 and does not belong in the clean baseline.
func TestSquashedSchemaHasNoLegacyRelationshipsTable(t *testing.T) {
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

// TestMigrationsAddCadencePolicies pins the T19 migration's shape: the
// cadence_policies table must carry the partial unique index on
// (user_id, entity_id) WHERE deleted_at IS NULL, so a soft-deleted policy
// never blocks re-creating a cadence for the same contact (the same T26
// pattern as idx_contacts_vcard_uid_user).
func TestMigrationsAddCadencePolicies(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cadence-policies.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	assert.True(t, columnExists(t, dbPath, "cadence_policies", "target_interval_days"))
	assert.True(t, columnExists(t, dbPath, "cadence_policies", "qualifying_types"))

	var sql string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_cadence_policies_user_entity'",
	).Scan(&sql).Error)
	assert.NotEmpty(t, sql)
	assert.Contains(t, sql, "WHERE deleted_at IS NULL",
		"T19 partial unique index must not block re-creating a cadence after soft delete")
}

// TestMigrationsAddConversationAgenda pins the T21 migration's shape: the
// conversation_agenda table must carry the discussed/activity resolution
// columns and be keyed to the subject contact by entity_id (Contact.VCardUID),
// never by a date — the agenda is contextual, not scheduled.
func TestMigrationsAddConversationAgenda(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "conversation-agenda.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	assert.True(t, columnExists(t, dbPath, "conversation_agenda", "entity_id"))
	assert.True(t, columnExists(t, dbPath, "conversation_agenda", "content"))
	assert.True(t, columnExists(t, dbPath, "conversation_agenda", "reference_url"))
	assert.True(t, columnExists(t, dbPath, "conversation_agenda", "discussed_at"))
	assert.True(t, columnExists(t, dbPath, "conversation_agenda", "activity_id"))

	assert.False(t, columnExists(t, dbPath, "conversation_agenda", "remind_at"),
		"the agenda must carry NO scheduling column — that is what distinguishes it from a Reminder")

	var sql string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_conversation_agenda_entity_id'",
	).Scan(&sql).Error)
	assert.NotEmpty(t, sql)
}

// TestSquashedSchemaHasNoLegacyFoodPreference verifies the squashed baseline
// (T22) excludes the legacy contacts.food_preference column — it was retired
// by T20a and removed from the baseline during the migration squash.
func TestSquashedSchemaHasNoLegacyFoodPreference(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "no-food.db")
	assert.False(t, columnExists(t, dbPath, "contacts", "food_preference"),
		"food_preference column must not exist in the squashed baseline")
}

// TestSquashedSchemaHasT26PartialIndex verifies the squashed baseline (T22)
// carries T26's partial unique index on (user_id, vcard_uid) with the
// WHERE vcard_uid IS NOT NULL AND deleted_at IS NULL clause, so a
// soft-deleted contact never blocks re-import of the same vcard_uid.
func TestSquashedSchemaHasT26PartialIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "partial-idx.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	var sql string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_contacts_vcard_uid_user'",
	).Scan(&sql).Error)
	assert.NotEmpty(t, sql)
	assert.Contains(t, sql, "WHERE vcard_uid IS NOT NULL AND deleted_at IS NULL",
		"T26 partial unique index must be in the baseline")
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
