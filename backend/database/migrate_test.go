package database

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
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

// TestMigrateDownRollsBackExactlyOneMigration pins the semantics of "down".
//
// cmd/migrate used to call golang-migrate's m.Down() — roll back EVERYTHING —
// while the Makefile documented the target as "Rollback the last migration".
// With the migrations squashed to a single initial schema, the documented
// command dropped the whole database. The CLI now delegates to MigrateDown,
// and this test is what keeps it a single step: if someone swaps Steps(-1)
// back to Down(), the version assertion below fails instead of a user's data
// disappearing.
func TestMigrateDownRollsBackExactlyOneMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "down-one.db")

	require.NoError(t, MigrateUp(dbPath))

	before, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok, "migrations must have been applied")
	require.False(t, dirty)
	require.Greater(t, before, uint(1), "need more than one migration for this test to mean anything")

	require.NoError(t, MigrateDown(dbPath))

	after, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok, "rolling back one migration must not unapply the whole chain")
	assert.False(t, dirty)
	assert.Equal(t, before-1, after, "down must step back exactly one migration, not to zero")

	// The baseline schema — and therefore the user's data — must survive a
	// single rollback. Under m.Down() these tables were gone.
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}()

	for _, table := range []string{"users", "contacts"} {
		var count int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&count).Error)
		assert.Equalf(t, int64(1), count, "table %q must survive a one-step rollback", table)
	}
}

// TestSquashedSchemaHasNoLegacyRelationshipsTable verifies the squashed
// baseline (T22) never creates the legacy `relationships` table — it was
// dropped and does not belong in the clean baseline.
func TestSquashedSchemaHasNoLegacyRelationshipsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "drop-relationships.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'relationships'",
	).Scan(&count).Error)
	assert.Zero(t, count, "relationships table should be dropped by migration 000035")

	// relationship_edges (its replacement) must still be present.
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

// TestForeignKeysEnforced is the regression test
// foreign_keys is a
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

// TestMigrationsAddGifts pins the T20b migration's shape: the gifts table must
// carry the three-state status column, the occasion, the explicit
// value/currency pair (value_cents + ISO-4217 currency), and the optional
// LifeEvent/Activity references — keyed to the subject contact by entity_id
// (Contact.VCardUID), never by a numeric ID.
func TestMigrationsAddGifts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gifts.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)

	assert.True(t, columnExists(t, dbPath, "gifts", "entity_id"))
	assert.True(t, columnExists(t, dbPath, "gifts", "status"))
	assert.True(t, columnExists(t, dbPath, "gifts", "occasion"))
	assert.True(t, columnExists(t, dbPath, "gifts", "description"))
	assert.True(t, columnExists(t, dbPath, "gifts", "date"))
	assert.True(t, columnExists(t, dbPath, "gifts", "value_cents"))
	assert.True(t, columnExists(t, dbPath, "gifts", "currency"))
	assert.True(t, columnExists(t, dbPath, "gifts", "life_event_id"))
	assert.True(t, columnExists(t, dbPath, "gifts", "activity_id"))

	var sql string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_gifts_entity_id'",
	).Scan(&sql).Error)
	assert.NotEmpty(t, sql)
}

// TestMigrationsAddGiftURLAndNotes covers T35's two additive columns and,
// more importantly, that adding them preserves gift rows that predate them.
// Real production data exists as of v0.2.0-alpha-candidate, so "the migration
// applies" is not the interesting assertion — "the migration applies and the
// user's existing gifts are still there, unchanged" is.
func TestMigrationsAddGiftURLAndNotes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t35-gift-url-notes.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000012, so the gift below genuinely
	// predates the new columns.
	require.NoError(t, m.Steps(11))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't35', 'x', 't35@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 't35'").Scan(&userID))

	_, err = sqlDB.Exec(`
		INSERT INTO gifts (id, created_at, updated_at, user_id, entity_id, status, occasion, description, value_cents, currency)
		VALUES ('gift-t35', datetime('now'), datetime('now'), ?, 'vcard-t35', 'given', 'birthday', 'The espresso machine', 12000, 'EUR')`,
		userID)
	require.NoError(t, err)

	// Apply exactly 000012 — Steps(1), not m.Up(), so the MigrateDown below
	// still rolls back this migration once another one lands after it.
	require.NoError(t, m.Steps(1))

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('gifts') WHERE name IN ('url', 'notes')",
	).Scan(&colCount))
	assert.Equal(t, int64(2), colCount, "gifts.url and gifts.notes must be added by the T35 migration")

	// The pre-existing gift survives untouched, with the new columns simply
	// null (nullable, no default, no backfill).
	var description, currency string
	var url, notes sql.NullString
	require.NoError(t, sqlDB.QueryRow(
		"SELECT description, currency, url, notes FROM gifts WHERE id = 'gift-t35'",
	).Scan(&description, &currency, &url, &notes))
	assert.Equal(t, "The espresso machine", description, "an additive migration must not disturb existing gift data")
	assert.Equal(t, "EUR", currency)
	assert.False(t, url.Valid, "an existing row's new url column must be null, not an empty-string backfill")
	assert.False(t, notes.Valid)

	// Writing the new columns works against the real migrated schema.
	_, err = sqlDB.Exec(
		"UPDATE gifts SET url = ?, notes = ? WHERE id = 'gift-t35'",
		"https://shop.example.com/machine", "Check the voltage")
	require.NoError(t, err)

	// Down drops both columns; the row itself and its original data survive.
	require.NoError(t, MigrateDown(dbPath))

	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('gifts') WHERE name IN ('url', 'notes')",
	).Scan(&colCount))
	assert.Equal(t, int64(0), colCount, "the down migration must remove both columns")

	require.NoError(t, sqlDB.QueryRow(
		"SELECT description FROM gifts WHERE id = 'gift-t35'",
	).Scan(&description))
	assert.Equal(t, "The espresso machine", description, "a rollback must not destroy the gift")
}

// TestMigrationsAddPreferenceNotes covers 000029's additive preferences.notes
// column, following TestMigrationsAddGiftURLAndNotes' exact template: a
// preference that predates the migration must survive it unchanged, with the
// new column simply null rather than backfilled.
func TestMigrationsAddPreferenceNotes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "preference-notes.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000029, so the preference below
	// genuinely predates the notes column.
	require.NoError(t, m.Steps(28))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'pref-notes', 'x', 'pref-notes@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'pref-notes'").Scan(&userID))

	_, err = sqlDB.Exec(`
		INSERT INTO preferences (id, created_at, updated_at, user_id, entity_id, category, key, value, sensitivity)
		VALUES ('pref-notes-1', datetime('now'), datetime('now'), ?, 'vcard-pref-notes', 'food', 'dislike', 'Alcohol', 'normal')`,
		userID)
	require.NoError(t, err)

	// Apply exactly 000029 — Steps(1), not m.Up(), so the MigrateDown below
	// still rolls back this migration once another one lands after it.
	require.NoError(t, m.Steps(1))

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('preferences') WHERE name = 'notes'",
	).Scan(&colCount))
	assert.Equal(t, int64(1), colCount, "preferences.notes must be added by the migration")

	var value string
	var notes sql.NullString
	require.NoError(t, sqlDB.QueryRow(
		"SELECT value, notes FROM preferences WHERE id = 'pref-notes-1'",
	).Scan(&value, &notes))
	assert.Equal(t, "Alcohol", value, "an additive migration must not disturb existing preference data")
	assert.False(t, notes.Valid, "an existing row's new notes column must be null, not an empty-string backfill")

	// Writing the new column works against the real migrated schema.
	_, err = sqlDB.Exec(
		"UPDATE preferences SET notes = ? WHERE id = 'pref-notes-1'",
		"Doesn't drink alcohol")
	require.NoError(t, err)

	// Down drops the column; the row itself and its original data survive.
	require.NoError(t, MigrateDown(dbPath))

	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('preferences') WHERE name = 'notes'",
	).Scan(&colCount))
	assert.Equal(t, int64(0), colCount, "the down migration must remove the notes column")

	require.NoError(t, sqlDB.QueryRow(
		"SELECT value FROM preferences WHERE id = 'pref-notes-1'",
	).Scan(&value))
	assert.Equal(t, "Alcohol", value, "a rollback must not destroy the preference")
}

// TestMigrationsBackfillMediaPreferences covers 000030's backfill of the
// legacy category='media' scheme (key=show/movie/music, no disposition) into
// the new medium-specific categories with key='favorite'. Also covers the
// catch-all branch for a row with an unrecognized key, and the down
// migration's reversal of the three deterministic branches.
func TestMigrationsBackfillMediaPreferences(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "preference-media-backfill.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000030, so these preferences
	// genuinely predate the backfill.
	require.NoError(t, m.Steps(29))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'media-backfill', 'x', 'media-backfill@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'media-backfill'").Scan(&userID))

	insertLegacyMedia := func(id, key string) {
		_, err := sqlDB.Exec(`
			INSERT INTO preferences (id, created_at, updated_at, user_id, entity_id, category, key, value, sensitivity)
			VALUES (?, datetime('now'), datetime('now'), ?, 'vcard-media-backfill', 'media', ?, 'The Bear', 'normal')`,
			id, userID, key)
		require.NoError(t, err)
	}
	insertLegacyMedia("media-show", "show")
	insertLegacyMedia("media-movie", "movie")
	insertLegacyMedia("media-music", "music")
	insertLegacyMedia("media-unrecognized", "podcast") // never written by the old UI, but never enum-enforced either

	// Apply exactly 000030.
	require.NoError(t, m.Steps(1))

	assertCategoryKey := func(id, wantCategory, wantKey string) {
		var category, key string
		require.NoError(t, sqlDB.QueryRow(
			"SELECT category, key FROM preferences WHERE id = ?", id,
		).Scan(&category, &key))
		assert.Equal(t, wantCategory, category, "id=%s category after backfill", id)
		assert.Equal(t, wantKey, key, "id=%s key after backfill", id)
	}
	assertCategoryKey("media-show", "media_tv", "favorite")
	assertCategoryKey("media-movie", "media_movie", "favorite")
	assertCategoryKey("media-music", "media_music_artist", "favorite")
	assertCategoryKey("media-unrecognized", "media_movie", "favorite")

	// Down reverses the three deterministic branches.
	require.NoError(t, MigrateDown(dbPath))

	assertCategoryKey("media-show", "media", "show")
	assertCategoryKey("media-movie", "media", "movie")
	assertCategoryKey("media-music", "media", "music")
	// The catch-all-branch row is documented as not perfectly reversible —
	// it comes back as media/movie alongside the genuine movie row, not its
	// original podcast key.
	assertCategoryKey("media-unrecognized", "media", "movie")
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

// TestMigrationsAddSearchAddressesFlat pins the T38 migration's shape: the
// denormalized addresses_flat column exists on contacts, and contacts_fts
// (dropped and recreated by 000010 because FTS5 cannot ALTER TABLE) indexes
// it so a street/city search can hit.
func TestMigrationsAddSearchAddressesFlat(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "search-addresses.db")
	assert.True(t, columnExists(t, dbPath, "contacts", "addresses_flat"),
		"contacts.addresses_flat must be added by the T38 migration")

	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}()

	var sql string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'contacts_fts'",
	).Scan(&sql).Error)
	assert.Contains(t, sql, "addresses_flat",
		"contacts_fts must index addresses_flat so address text is searchable")

	// The FTS triggers must reference the new column too — a trigger that
	// still used the old 8-column insert would silently never index addresses.
	var triggerSQL string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'contacts_fts_ai'",
	).Scan(&triggerSQL).Error)
	assert.Contains(t, triggerSQL, "addresses_flat",
		"the contacts insert trigger must populate addresses_flat")
}

// TestSearchAddressesMigrationBackfillsExistingRows is the T38 data-safety
// test: contacts whose addresses predate migration 000010 (rows that were
// never saved through GORM/BeforeSave) must have addresses_flat populated by
// the migration's own backfill — and become searchable through the FTS index
// the migration rebuilds — without waiting for their next edit.
func TestSearchAddressesMigrationBackfillsExistingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t38-backfill.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Apply everything up to but NOT including 000010, so the contact below
	// genuinely predates the new column.
	require.NoError(t, m.Steps(9))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't38', 'x', 't38@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 't38'").Scan(&userID))

	// A pre-existing contact with JSON addresses, written raw (no GORM, no
	// BeforeSave), exactly like a row created before this migration shipped.
	_, err = sqlDB.Exec(`
		INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid, addresses)
		VALUES (datetime('now'), datetime('now'), 'Pre', 'Existing', ?, 'vcard-t38', ?)`,
		userID, `[{"type":"home","street":"Clark St","city":"Springfield","region":"IL","postal":"62701","country":"USA"},{"type":"work","street":"","city":"Chicago"}]`)
	require.NoError(t, err)

	// Apply exactly 000010 — not m.Up(), which would also run every
	// migration after it (e.g. T36's 000011) and make the MigrateDown below
	// roll back the wrong one.
	require.NoError(t, m.Steps(1))

	// The backfill derived addresses_flat from the JSON array (mirroring
	// FormatAddress/FlattenAddresses: components joined with ", ", addresses
	// joined with a space).
	var flat string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT addresses_flat FROM contacts WHERE vcard_uid = 'vcard-t38'",
	).Scan(&flat))
	assert.Contains(t, flat, "Clark St", "backfilled street must be present")
	assert.Contains(t, flat, "Springfield", "backfilled city must be present")
	assert.Contains(t, flat, "Chicago", "the second address must be flattened too")

	// The migration's own index rebuild makes the address findable without
	// any additional re-index step.
	var ftsCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM contacts_fts WHERE contacts_fts MATCH 'clark' AND user_id = ?", userID,
	).Scan(&ftsCount))
	assert.Equal(t, int64(1), ftsCount, "the migration-rebuilt FTS index must find the backfilled address")

	// Down: rolling back exactly this migration drops the column and reverts
	// contacts_fts; the contact's data itself survives. Checked through the
	// still-open raw connection rather than columnExists, because columnExists
	// opens via InitDB, which would re-apply the up migration and mask the
	// rollback.
	require.NoError(t, MigrateDown(dbPath))

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('contacts') WHERE name = 'addresses_flat'",
	).Scan(&colCount))
	assert.Equal(t, int64(0), colCount, "the down migration must remove contacts.addresses_flat")

	var firstname string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT firstname FROM contacts WHERE vcard_uid = 'vcard-t38'",
	).Scan(&firstname))
	assert.Equal(t, "Pre", firstname, "a rollback must not destroy the contact")
}

// TestFlatAddressSubStreetMigrationBackfillsStrandedCardData is the T79
// (T79)
// data-recovery test: a contact whose card still holds postOfficeBox/
// apartment/floor components — imported from a VCF before the flat projection
// gained slots for them — must have that detail recovered into the flat
// addresses JSON AND the searchable addresses_flat column by migration 000022,
// without waiting for its next edit. Rows whose card has no such stranded
// data must be left untouched.
func TestFlatAddressSubStreetMigrationBackfillsStrandedCardData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t79-backfill.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Apply everything up to but NOT including 000022, so the contacts below
	// genuinely predate the backfill.
	require.NoError(t, m.Steps(21))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't79', 'x', 't79@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 't79'").Scan(&userID))

	// Stranded: the card carries postOfficeBox/apartment/floor components the
	// flat addresses JSON lacks (exactly what a VCF import produced before
	// T79 gave the projection slots for them).
	_, err = sqlDB.Exec(`
		INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid, addresses, card)
		VALUES (datetime('now'), datetime('now'), 'Stranded', 'Guy', ?, 'vcard-t79-stranded', ?, ?)`,
		userID,
		`[{"type":"home","street":"742 Clark St","city":"Springfield","region":"IL","postal":"62701","country":"USA"}]`,
		`{"name":{"components":[{"kind":"given","value":"Stranded"}]},"addresses":[{"components":[
			{"kind":"name","value":"742 Clark St"},
			{"kind":"postOfficeBox","value":"PO Box 42"},
			{"kind":"apartment","value":"Apt 3B"},
			{"kind":"floor","value":"Floor 2"},
			{"kind":"locality","value":"Springfield"},
			{"kind":"region","value":"IL"},
			{"kind":"postcode","value":"62701"},
			{"kind":"country","value":"USA"}],"contexts":["home"]}]}`,
	)
	require.NoError(t, err)

	// Untouched-by-design controls: a row with a card but no sub-street kinds,
	// and a row with no card at all.
	_, err = sqlDB.Exec(`
		INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid, addresses, card)
		VALUES (datetime('now'), datetime('now'), 'Plain', 'Guy', ?, 'vcard-t79-plain', ?, ?)`,
		userID,
		`[{"type":"home","street":"999 Oak Ave","city":"Shelbyville"}]`,
		`{"name":{"components":[{"kind":"given","value":"Plain"}]},"addresses":[{"components":[{"kind":"name","value":"999 Oak Ave"},{"kind":"locality","value":"Shelbyville"}],"contexts":["home"]}]}`,
	)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid, addresses)
		VALUES (datetime('now'), datetime('now'), 'NoCard', 'Guy', ?, 'vcard-t79-nocard', ?)`,
		userID,
		`[{"type":"home","street":"111 Pine Rd","city":"Ogdenville"}]`,
	)
	require.NoError(t, err)

	// Apply exactly 000022.
	require.NoError(t, m.Steps(1))

	// The stranded row's flat addresses JSON gained the three sub-street keys.
	var addresses string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT addresses FROM contacts WHERE vcard_uid = 'vcard-t79-stranded'",
	).Scan(&addresses))
	assert.Contains(t, addresses, "PO Box 42", "flat addresses must hold the recovered PO box")
	assert.Contains(t, addresses, "Apt 3B", "flat addresses must hold the recovered apartment")
	assert.Contains(t, addresses, "Floor 2", "flat addresses must hold the recovered floor")
	assert.Contains(t, addresses, `"street":"742 Clark St"`, "the projected street must be preserved")
	assert.Contains(t, addresses, `"type":"home"`, "the type derived from contexts[0] must be preserved")

	// The searchable column now carries the recovered parts (between street
	// and city, the FormatAddress ordering), and the FTS index (maintained by
	// the 000010 triggers) finds the apartment without any manual rebuild.
	var flat string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT addresses_flat FROM contacts WHERE vcard_uid = 'vcard-t79-stranded'",
	).Scan(&flat))
	assert.Contains(t, flat, "PO Box 42")
	assert.Contains(t, flat, "Apt 3B")

	var ftsCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM contacts_fts WHERE contacts_fts MATCH 'apt' AND user_id = ?", userID,
	).Scan(&ftsCount))
	assert.Equal(t, int64(1), ftsCount, "the recovered apartment must be findable through the FTS index")
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM contacts_fts WHERE contacts_fts MATCH 'box' AND user_id = ?", userID,
	).Scan(&ftsCount))
	assert.Equal(t, int64(1), ftsCount, "the recovered PO box must be findable through the FTS index")

	// The control rows were not rewritten.
	var plainAddresses string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT addresses FROM contacts WHERE vcard_uid = 'vcard-t79-plain'",
	).Scan(&plainAddresses))
	assert.Equal(t, `[{"type":"home","street":"999 Oak Ave","city":"Shelbyville"}]`, plainAddresses,
		"a card without sub-street kinds must not be rewritten")
	var noCardAddresses string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT addresses FROM contacts WHERE vcard_uid = 'vcard-t79-nocard'",
	).Scan(&noCardAddresses))
	assert.Equal(t, `[{"type":"home","street":"111 Pine Rd","city":"Ogdenville"}]`, noCardAddresses,
		"a row without a card must not be rewritten")
}

// TestMigrationsAddPhonesNormalized pins the T69 migration's shape: the
// denormalized phones_normalized column exists on contacts, and contacts_fts
// (dropped and recreated by 000020 because FTS5 cannot ALTER TABLE) indexes
// it so a cross-format phone search can hit.
func TestMigrationsAddPhonesNormalized(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "search-phones.db")
	assert.True(t, columnExists(t, dbPath, "contacts", "phones_normalized"),
		"contacts.phones_normalized must be added by the T69 migration")

	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}()

	var sql string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'contacts_fts'",
	).Scan(&sql).Error)
	assert.Contains(t, sql, "phones_normalized",
		"contacts_fts must index phones_normalized so cross-format phone search works")

	// The FTS triggers must reference the new column too — a trigger that
	// still used the old column list would silently never index phones.
	var triggerSQL string
	require.NoError(t, db.Raw(
		"SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'contacts_fts_ai'",
	).Scan(&triggerSQL).Error)
	assert.Contains(t, triggerSQL, "phones_normalized",
		"the contacts insert trigger must populate phones_normalized")
}

// TestPhonesNormalizedMigrationBackfillsExistingRows is the T69 data-safety
// test: contacts whose phones predate migration 000020 (rows that were never
// saved through GORM/BeforeSave) must have phones_normalized populated by the
// migration's own backfill — both the full digit string and the last-10
// PhoneKey — and become searchable through the rebuilt FTS index without
// waiting for their next edit.
func TestPhonesNormalizedMigrationBackfillsExistingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t69-backfill.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Apply everything up to but NOT including 000020, so the contact below
	// genuinely predates the new column.
	require.NoError(t, m.Steps(19))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't69', 'x', 't69@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 't69'").Scan(&userID))

	// A pre-existing contact with JSON phones, written raw (no GORM, no
	// BeforeSave), exactly like a row created before this migration shipped.
	// The +1-prefixed number exercises the >10-digit key token; the plain
	// second entry exercises a 10-digit number whose key equals its digits.
	_, err = sqlDB.Exec(`
		INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid, phones)
		VALUES (datetime('now'), datetime('now'), 'Pre', 'Existing', ?, 'vcard-t69', ?)`,
		userID, `[{"type":"cell","value":"+18005551234"},{"type":"home","value":"(800) 555-1234"}]`)
	require.NoError(t, err)

	// Apply exactly 000020 — not m.Up(), which would also run every migration
	// after it and make the MigrateDown below roll back the wrong one.
	require.NoError(t, m.Steps(1))

	// The backfill derived phones_normalized from the JSON array (mirroring
	// models.FlattenPhones: full digit string + last-10 key when it differs).
	var normalized string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT phones_normalized FROM contacts WHERE vcard_uid = 'vcard-t69'",
	).Scan(&normalized))
	assert.Contains(t, normalized, "18005551234", "backfilled full digit string must be present")
	assert.Contains(t, normalized, "8005551234", "backfilled PhoneKey token must be present")

	// The migration's own index rebuild makes the phone findable by its key
	// (the cross-format case) without any additional re-index step.
	var ftsCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM contacts_fts WHERE contacts_fts MATCH '8005551234' AND user_id = ?", userID,
	).Scan(&ftsCount))
	assert.Equal(t, int64(1), ftsCount, "the migration-rebuilt FTS index must find the backfilled phone by its key")

	// Down: rolling back exactly this migration drops the column and reverts
	// contacts_fts; the contact's data itself survives. Checked through the
	// still-open raw connection rather than columnExists, because columnExists
	// opens via InitDB, which would re-apply the up migration and mask the
	// rollback.
	require.NoError(t, MigrateDown(dbPath))

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('contacts') WHERE name = 'phones_normalized'",
	).Scan(&colCount))
	assert.Equal(t, int64(0), colCount, "the down migration must remove contacts.phones_normalized")

	var firstname string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT firstname FROM contacts WHERE vcard_uid = 'vcard-t69'",
	).Scan(&firstname))
	assert.Equal(t, "Pre", firstname, "a rollback must not destroy the contact")
}

// TestMigrationsAddContactSortName pins the T73 migration's shape: the
// denormalized sort_name column exists on contacts, and the (user_id,
// sort_name, id) composite index exists to serve the name-sorted cursor
// query (the same pattern as idx_contacts_feed).
func TestMigrationsAddContactSortName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-sort-name.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}()

	assert.True(t, columnExists(t, dbPath, "contacts", "sort_name"),
		"contacts.sort_name must be added by the T73 migration")

	var count int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_contacts_sort_name'",
	).Scan(&count).Error)
	assert.EqualValues(t, 1, count, "migration 000021 must create idx_contacts_sort_name")
}

// TestContactSortNameMigrationBackfillsExistingRows is the T73 data-safety
// test: contacts whose names predate migration 000021 (rows that were never
// saved through GORM/BeforeSave) must have sort_name populated by the
// migration's own backfill — lastname when non-empty, firstname otherwise,
// lowercased and trimmed — without waiting for their next edit.
func TestContactSortNameMigrationBackfillsExistingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t73-backfill.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Apply everything up to but NOT including 000021, so the contacts below
	// genuinely predate the new column.
	require.NoError(t, m.Steps(20))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't73', 'x', 't73@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 't73'").Scan(&userID))

	// Four pre-existing contacts, written raw (no GORM, no BeforeSave):
	// mixed-case lastname, whitespace-padded lastname, firstname-only, and
	// a blank-names edge case (should fall back to the firstname anyway).
	rows := [][2]string{
		{"Johnson", "Alice"}, // -> "johnson"
		{"  SMITH  ", "Bob"}, // -> "smith"
		{"", "Carol"},        // -> "carol"
		{"  ", "David"},      // -> "david"
	}
	for i, names := range rows {
		_, err := sqlDB.Exec(
			"INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid) VALUES (datetime('now'), datetime('now'), ?, ?, ?, ?)",
			names[1], names[0], userID, fmt.Sprintf("vcard-t73-%d", i))
		require.NoError(t, err)
	}
	// A NULL lastname — the schema allows it (nullable, unlike firstname) and
	// GORM scans it as "", so the backfill must treat it as empty and fall
	// back to the firstname rather than writing NULL into the NOT NULL column.
	_, err = sqlDB.Exec(
		"INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid) VALUES (datetime('now'), datetime('now'), 'Ellen', NULL, ?, 'vcard-t73-4')",
		userID)
	require.NoError(t, err)

	require.NoError(t, m.Steps(1))

	got := func(vcard string) string {
		var sortName string
		require.NoError(t, sqlDB.QueryRow(
			"SELECT sort_name FROM contacts WHERE vcard_uid = ?", vcard,
		).Scan(&sortName))
		return sortName
	}
	assert.Equal(t, "johnson", got("vcard-t73-0"), "backfill must lowercase and use the lastname")
	assert.Equal(t, "smith", got("vcard-t73-1"), "backfill must trim whitespace before lowercasing")
	assert.Equal(t, "carol", got("vcard-t73-2"), "a contact with no lastname must fall back to the firstname")
	assert.Equal(t, "david", got("vcard-t73-3"), "a blank lastname must fall back to the firstname")
	assert.Equal(t, "ellen", got("vcard-t73-4"), "a NULL lastname must be treated as empty and fall back to the firstname")

	// Down: rolling back exactly this migration drops the column; the
	// contacts themselves survive. Checked through the still-open raw
	// connection rather than columnExists, because columnExists opens via
	// InitDB, which would re-apply the up migration and mask the rollback.
	require.NoError(t, MigrateDown(dbPath))

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('contacts') WHERE name = 'sort_name'",
	).Scan(&colCount))
	assert.Zero(t, colCount, "the down migration must remove contacts.sort_name")

	var rowCount int64
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM contacts WHERE user_id = ?", userID).Scan(&rowCount))
	assert.EqualValues(t, 5, rowCount, "a rollback must not destroy the contacts")
}

// TestMigrationsAddLifeEventCategories pins the T36 migration's shape: an
// additive nullable life_events.category column.
func TestMigrationsAddLifeEventCategories(t *testing.T) {
	assert.True(t, columnExists(t, filepath.Join(t.TempDir(), "life-event-categories.db"), "life_events", "category"),
		"life_events.category must be added by the T36 migration")
}

// TestLifeEventCategoriesMigrationBackfillsExistingRows is T36's data-safety
// test: life_events rows written before this migration — whose Type matches
// one of the seven pre-existing LifeEventType* constants — must have
// category backfilled without waiting for their next edit. A row whose Type
// doesn't map onto any of the seven (either a legacy free-text value or one
// of T36's own 37 new tokens, since those never existed pre-migration
// either) must be left NULL rather than guessed, per the ticket's own trap
// note.
func TestLifeEventCategoriesMigrationBackfillsExistingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t36-backfill.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Apply everything up to but NOT including 000011, so the rows below
	// genuinely predate the new column.
	require.NoError(t, m.Steps(10))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't36', 'x', 't36@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 't36'").Scan(&userID))

	_, err = sqlDB.Exec(
		"INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid) VALUES (datetime('now'), datetime('now'), 'Pre', 'Existing', ?, 'vcard-t36')",
		userID)
	require.NoError(t, err)

	insertEvent := func(id, eventType string) {
		_, err := sqlDB.Exec(
			"INSERT INTO life_events (id, created_at, updated_at, user_id, entity_id, type) VALUES (?, datetime('now'), datetime('now'), ?, 'vcard-t36', ?)",
			id, userID, eventType)
		require.NoError(t, err)
	}
	// One per pre-existing constant, plus an unmapped legacy free-text value.
	insertEvent("evt-moved", "moved")
	insertEvent("evt-job-change", "job_change")
	insertEvent("evt-retired", "retired")
	insertEvent("evt-graduated", "graduated")
	insertEvent("evt-married", "married")
	insertEvent("evt-had-child", "had_child")
	insertEvent("evt-adopted-pet", "adopted_pet")
	insertEvent("evt-legacy-freetext", "started a podcast")

	require.NoError(t, m.Steps(1))

	category := func(id string) sql.NullString {
		var c sql.NullString
		require.NoError(t, sqlDB.QueryRow("SELECT category FROM life_events WHERE id = ?", id).Scan(&c))
		return c
	}

	assert.Equal(t, "home_living", category("evt-moved").String)
	assert.Equal(t, "work_education", category("evt-job-change").String)
	assert.Equal(t, "work_education", category("evt-retired").String)
	assert.Equal(t, "work_education", category("evt-graduated").String)
	assert.Equal(t, "family_relationships", category("evt-married").String)
	assert.Equal(t, "family_relationships", category("evt-had-child").String)
	assert.Equal(t, "family_relationships", category("evt-adopted-pet").String)
	assert.False(t, category("evt-legacy-freetext").Valid,
		"a legacy free-text type with no registry mapping must be left NULL, not guessed")

	// Down: rolling back exactly this migration drops the column; the events
	// themselves survive.
	require.NoError(t, MigrateDown(dbPath))

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('life_events') WHERE name = 'category'",
	).Scan(&colCount))
	assert.Equal(t, int64(0), colCount, "the down migration must remove life_events.category")

	var eventCount int64
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM life_events WHERE entity_id = 'vcard-t36'").Scan(&eventCount))
	assert.EqualValues(t, 8, eventCount, "a rollback must not destroy the life events")
}

// TestOpenDSN_PragmasArePresent verifies that openDSN appends the two pragmas
// the app requires for correctness:
//   - journal_mode(WAL): persisted once set, needed for safe hot-backup.
//   - foreign_keys(1): per-connection, needed for FK enforcement .
func TestOpenDSN_PragmasArePresent(t *testing.T) {
	dsn := openDSN("/path/to/db.sqlite")
	assert.True(t, strings.Contains(dsn, "_pragma=journal_mode(WAL)"),
		"openDSN must include the WAL journal pragma")
	assert.True(t, strings.Contains(dsn, "_pragma=foreign_keys(1)"),
		"openDSN must include the foreign_keys pragma")
	assert.True(t, strings.HasPrefix(dsn, "/path/to/db.sqlite?"),
		"openDSN must preserve the db path as the DSN prefix")
}

// TestGormLoggerDoesNotInterpolatePII pins issue #621: every connection opened
// through this package must use a logger that never echoes literal query
// values. GORM's default logger interpolates the WHERE/VALUES clause into
// error/slow-query logs, so a missing row or a constraint failure used to
// print `SELECT ... WHERE email = "<address>"` verbatim into an instance-wide
// log with no redaction layer (surfaced by the #510 privacy review).
// newGormLogger closes it two ways: ParameterizedQueries replaces values with
// `?`, and IgnoreRecordNotFoundError stops the benign not-found SELECTs
// entirely.
func TestGormLoggerDoesNotInterpolatePII(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gorm-logger.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}()

	// InitDB must wire the redacting logger onto its connection, not the bare
	// GORM default (gorm.Open defaults Logger to logger.Default when nil —
	// reverting the config to `&gorm.Config{}` makes this equal and fails).
	assert.NotEqual(t, gormLogger.Default, db.Logger,
		"InitDB must configure the parameterized/not-found-suppressing GORM logger")

	// Re-point the app's own logger at a buffer so we can assert on exactly
	// what it would emit for the real migrated schema.
	var buf bytes.Buffer
	db.Logger = newGormLogger(&buf)

	const sentinel = "ghost.unknown.sentinel@example.test"

	// A not-found lookup carrying PII: with IgnoreRecordNotFoundError it is
	// not logged at all.
	buf.Reset()
	var ghost models.User
	err = db.Where("email = ?", sentinel).First(&ghost).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Empty(t, buf.String(),
		"a not-found lookup must not be logged at all (IgnoreRecordNotFoundError)")

	// A constraint violation carrying PII: logged (Warn level reaches error
	// traces) but with `?` placeholders, never the interpolated value.
	buf.Reset()
	first := models.User{Username: "alice", Password: "x", Email: sentinel}
	require.NoError(t, db.Create(&first).Error)
	err = db.Create(&models.User{Username: "alice2", Password: "x", Email: sentinel}).Error
	require.Error(t, err, "the duplicate email must violate the unique index")

	out := buf.String()
	assert.NotEmpty(t, out, "the constraint failure must be logged at Warn level")
	assert.Contains(t, out, "INSERT INTO `users`", "the failed INSERT is the statement the test is about")
	assert.Contains(t, out, "?", "GORM must log `?` placeholders, not interpolated values")
	assert.NotContains(t, out, sentinel, "the PII value must never reach the log")
	assert.NotContains(t, out, `"`+sentinel+`"`, "the PII value must never reach the log")
}

// TestInitDBDoesNotLeakMigratorGoroutines pins a real leak found while
// investigating a CI timeout: newMigrator's golang-migrate instance parks a
// goroutine forever waiting to hand off the next migration until Close()
// runs, and InitDB never closed it. Every real-DB test (CLAUDE.md's "test
// against the real migrated schema" trap) calls InitDB, so one leaked
// goroutine per call piled up across the suite into exhausted file
// descriptors and a 20-minute CI job timeout. RunMigrations must close its
// migrator when done.
func TestInitDBDoesNotLeakMigratorGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	const calls = 20
	for i := 0; i < calls; i++ {
		db, err := InitDB(filepath.Join(t.TempDir(), fmt.Sprintf("leak-%d.db", i)))
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}

	// Give any genuinely-exiting goroutines a moment to unwind before
	// counting, so this isn't racy against normal scheduling jitter.
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	assert.LessOrEqual(t, after, before+5,
		"InitDB leaked goroutines: before=%d after=%d over %d calls", before, after, calls)
}

// TestNotificationChannelsMigration pins the N9 migration's shape AND its
// data-safety backfill. Real production data exists, so "the migration
// applies" is not the interesting assertion — "email_sent=true reminders get
// one backfilled 'sent' email delivery so the per-channel dispatch does not
// re-email yesterday's reminders" is.
func TestNotificationChannelsMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "n9-migration.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000013, so the rows below genuinely
	// predate the notification-channel tables.
	require.NoError(t, m.Steps(12))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'n9', 'x', 'n9@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 'n9'").Scan(&userID))

	_, err = sqlDB.Exec(
		"INSERT INTO contacts (created_at, updated_at, firstname, lastname, user_id, vcard_uid) VALUES (datetime('now'), datetime('now'), 'Pre', 'Existing', ?, 'vcard-n9')",
		userID)
	require.NoError(t, err)

	// Two pre-existing reminders: one already emailed (email_sent=1, with a
	// last_sent), one not. The backfill must cover exactly the first.
	_, err = sqlDB.Exec(`
		INSERT INTO reminders (created_at, updated_at, user_id, message, by_mail, remind_at, recurrence, completed, email_sent, last_sent, contact_id)
		VALUES (datetime('now'), datetime('now'), ?, 'sent', 1, datetime('now'), 'once', 0, 1, datetime('now'), 1)`,
		userID)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO reminders (created_at, updated_at, user_id, message, by_mail, remind_at, recurrence, completed, email_sent, last_sent, contact_id)
		VALUES (datetime('now'), datetime('now'), ?, 'unsent', 1, datetime('now'), 'once', 0, 0, NULL, 1)`,
		userID)
	require.NoError(t, err)

	require.NoError(t, m.Steps(1))

	// The new user toggles exist.
	var toggleCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('users') WHERE name IN ('notify_ntfy', 'notify_gotify', 'notify_push')",
	).Scan(&toggleCount))
	assert.Equal(t, int64(3), toggleCount, "users must gain the three N9 channel toggles")

	// Backfill: exactly one 'sent' email delivery, for the emailed reminder.
	var sentCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM notification_deliveries WHERE channel = 'email' AND status = 'sent'",
	).Scan(&sentCount))
	assert.Equal(t, int64(1), sentCount, "one backfilled sent email delivery per email_sent=true reminder")

	var (
		deliveryReminderID int64
		sentAt             sql.NullString
	)
	require.NoError(t, sqlDB.QueryRow(
		"SELECT reminder_id, sent_at FROM notification_deliveries WHERE channel = 'email' AND status = 'sent'",
	).Scan(&deliveryReminderID, &sentAt))
	var sentReminderID int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT id FROM reminders WHERE message = 'sent'",
	).Scan(&sentReminderID))
	assert.Equal(t, sentReminderID, deliveryReminderID, "the backfilled delivery must reference the emailed reminder")
	assert.True(t, sentAt.Valid, "a backfilled sent delivery must carry a sent_at")

	// The notification_configs partial unique index blocks a second active
	// row per user but allows re-creating after soft delete (T26 pattern).
	_, err = sqlDB.Exec(`
		INSERT INTO notification_configs (created_at, updated_at, user_id) VALUES (datetime('now'), datetime('now'), ?)`,
		userID)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO notification_configs (created_at, updated_at, user_id) VALUES (datetime('now'), datetime('now'), ?)`,
		userID)
	assert.Error(t, err, "a second active config row for the same user must violate the partial unique index")
	_, err = sqlDB.Exec("UPDATE notification_configs SET deleted_at = datetime('now') WHERE user_id = ?", userID)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO notification_configs (created_at, updated_at, user_id) VALUES (datetime('now'), datetime('now'), ?)`,
		userID)
	assert.NoError(t, err, "re-creating a config after soft delete must be allowed")

	// The reminder FK cascades: a hard-removed reminder takes its delivery
	// rows with it.
	_, err = sqlDB.Exec("DELETE FROM reminders WHERE message = 'sent'")
	require.NoError(t, err)
	var remaining int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM notification_deliveries WHERE reminder_id = ?", sentReminderID,
	).Scan(&remaining))
	assert.Zero(t, remaining, "notification_deliveries must be cascaded when their reminder is hard-deleted")

	// Down: rolling back 000013 drops the new tables/columns; the pre-existing
	// user/contact/reminders survive.
	require.NoError(t, MigrateDown(dbPath))
	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('notification_deliveries', 'notification_configs', 'push_subscriptions', 'server_settings')",
	).Scan(&tableCount))
	assert.Zero(t, tableCount, "the down migration must drop all four N9 tables")
	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('users') WHERE name IN ('notify_ntfy', 'notify_gotify', 'notify_push')",
	).Scan(&colCount))
	assert.Zero(t, colCount, "the down migration must remove the three user toggles")

	var reminderCount int64
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM reminders WHERE message = 'unsent'").Scan(&reminderCount))
	assert.EqualValues(t, 1, reminderCount, "a rollback must not destroy the reminder")
}

// TestAddressSuggestionDismissalMigration pins the T40 migration's shape:
// the dismissed_household_suggestions table exists with its natural-key
// composite unique index (so a duplicate dismissal is structurally
// impossible), and rolling back 000014 drops it without touching pre-existing
// tables.
func TestAddressSuggestionDismissalMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t40-migration.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// Everything up to but NOT including 000014, so a user row can be
	// created pre-migration and must survive the rollback.
	require.NoError(t, m.Steps(13))

	_, err = sqlDB.Exec(
		"INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't40', 'x', 't40@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM users WHERE username = 't40'").Scan(&userID))

	require.NoError(t, m.Steps(1))

	// The table and its unique index exist.
	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'dismissed_household_suggestions'",
	).Scan(&tableCount))
	assert.EqualValues(t, 1, tableCount, "000014 must create dismissed_household_suggestions")

	var indexCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_dismissed_household_suggestions_unique'",
	).Scan(&indexCount))
	assert.EqualValues(t, 1, indexCount, "000014 must create the natural-key unique index")

	// The unique index rejects a duplicate dismissal but allows a distinct one.
	_, err = sqlDB.Exec(`
		INSERT INTO dismissed_household_suggestions (created_at, updated_at, user_id, address_hash, member_hash)
		VALUES (datetime('now'), datetime('now'), ?, 'hash-a', 'members-1')`,
		userID)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO dismissed_household_suggestions (created_at, updated_at, user_id, address_hash, member_hash)
		VALUES (datetime('now'), datetime('now'), ?, 'hash-a', 'members-1')`,
		userID)
	assert.Error(t, err, "a duplicate (address_hash, member_hash) dismissal must violate the unique index")
	_, err = sqlDB.Exec(`
		INSERT INTO dismissed_household_suggestions (created_at, updated_at, user_id, address_hash, member_hash)
		VALUES (datetime('now'), datetime('now'), ?, 'hash-b', 'members-1')`,
		userID)
	assert.NoError(t, err, "a distinct address_hash must be insertable")

	// Down: rolling back 000014 drops the table; the pre-existing user survives.
	require.NoError(t, MigrateDown(dbPath))
	var afterCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'dismissed_household_suggestions'",
	).Scan(&afterCount))
	assert.Zero(t, afterCount, "the down migration must drop dismissed_household_suggestions")

	var userCount int64
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users WHERE username = 't40'").Scan(&userCount))
	assert.EqualValues(t, 1, userCount, "a rollback must not destroy the pre-existing user")
}

// TestCalendarTwoWayMigration pins migration 000015's shape: calendar_event_links
// gains the two nullable two-way sync columns (remote_etag, remote_path), and
// rolling back drops them without touching pre-existing data.
func TestCalendarTwoWayMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t13-migration.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(14))

	// Satisfy the calendar_event_links foreign keys before inserting a row.
	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't13', 'x', 't13@example.com')")
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO calendar_subscriptions (created_at, updated_at, user_id, name, url, sync_enabled) VALUES (datetime('now'), datetime('now'), 1, 's', 'http://x', 1)")
	require.NoError(t, err)
	_, err = sqlDB.Exec("INSERT INTO activities (created_at, updated_at, user_id, title, date) VALUES (datetime('now'), datetime('now'), 1, 'a', datetime('now'))")
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
		INSERT INTO calendar_event_links (created_at, updated_at, subscription_id, user_id, uid, activity_id, content_hash)
		VALUES (datetime('now'), datetime('now'), 1, 1, 'uid-1', 1, 'hash-1')`)
	require.NoError(t, err)

	require.NoError(t, m.Steps(1))

	var colCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('calendar_event_links') WHERE name IN ('remote_etag', 'remote_path')",
	).Scan(&colCount))
	assert.Equal(t, int64(2), colCount, "000015 must add both two-way columns")

	var rowCount int64
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM calendar_event_links").Scan(&rowCount))
	assert.EqualValues(t, 1, rowCount, "the migration must not touch existing rows")

	// Down: rolling back drops the columns; the existing row survives.
	require.NoError(t, MigrateDown(dbPath))
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('calendar_event_links') WHERE name IN ('remote_etag', 'remote_path')",
	).Scan(&colCount))
	assert.Zero(t, colCount, "the down migration must remove both columns")
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM calendar_event_links").Scan(&rowCount))
	assert.EqualValues(t, 1, rowCount, "a rollback must not destroy the row")
}

// TestAuditEventsMigration pins migration 000016's shape: the audit_events
// table exists with its immutability trigger (an UPDATE is rejected), and
// rolling back drops both without touching pre-existing data.
func TestAuditEventsMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t18-migration.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(15))

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't18', 'x', 't18@example.com')")
	require.NoError(t, err)

	require.NoError(t, m.Steps(1))

	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'audit_events'",
	).Scan(&tableCount))
	assert.EqualValues(t, 1, tableCount, "000016 must create audit_events")

	// The immutability trigger rejects UPDATE.
	_, err = sqlDB.Exec(`
		INSERT INTO audit_events (created_at, updated_at, entity_type, entity_id, operation, user_id)
		VALUES (datetime('now'), datetime('now'), 'contact', 'x', 'create', 1)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`UPDATE audit_events SET operation = 'delete' WHERE entity_id = 'x'`)
	assert.Error(t, err, "audit_events must reject UPDATE via its trigger")

	// Down: rolling back drops the table and trigger; the user survives.
	require.NoError(t, MigrateDown(dbPath))
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'audit_events'",
	).Scan(&tableCount))
	assert.Zero(t, tableCount, "the down migration must drop audit_events")
	var userCount int64
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users WHERE username = 't18'").Scan(&userCount))
	assert.EqualValues(t, 1, userCount, "a rollback must not destroy the pre-existing user")
}

// TestAttachmentsMigration pins migration 000017's shape: the attachments
// table exists with its columns/indexes, and rolling back drops it without
// touching pre-existing data.
func TestAttachmentsMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "n7-migration.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(16))

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'n7', 'x', 'n7@example.com')")
	require.NoError(t, err)

	require.NoError(t, m.Steps(1))

	var tableCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'attachments'",
	).Scan(&tableCount))
	assert.EqualValues(t, 1, tableCount, "000017 must create attachments")

	_, err = sqlDB.Exec(`
		INSERT INTO attachments (created_at, updated_at, user_id, contact_vcard_uid, stored_name, original_name, content_type, size_bytes)
		VALUES (datetime('now'), datetime('now'), 1, 'contact-1', 'uuid-1', 'cv.pdf', 'application/pdf', 123)`)
	require.NoError(t, err)

	// Down: rolling back drops the table; the user survives.
	require.NoError(t, MigrateDown(dbPath))
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'attachments'",
	).Scan(&tableCount))
	assert.Zero(t, tableCount, "the down migration must drop attachments")
	var userCount int64
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'n7'").Scan(&userCount))
	assert.EqualValues(t, 1, userCount, "a rollback must not destroy the pre-existing user")
}

// TestAuditHashChainMigration pins migration 000034's shape (issue #381): it
// adds hash/prev_hash, preserves pre-existing audit rows and their ids (the
// rebuild must be lossless — reach_out_suggestions.audit_event_id references
// them loosely), widens the operation CHECK to the auth/admin vocabulary, keeps
// the immutability trigger in force, and rolls back to the 000016 shape.
func TestAuditHashChainMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t381-migration.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	// 33 steps: migrations 000001-000032 plus 000033_at_rest_encryption
	// (issue #380), landing right before the hash-chain migration.
	require.NoError(t, m.Steps(33))

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't381', 'x', 't381@example.com')")
	require.NoError(t, err)

	// A pre-existing audit row (the 000016 shape has no hash/prev_hash columns).
	_, err = sqlDB.Exec(`
		INSERT INTO audit_events (created_at, updated_at, entity_type, entity_id, operation, user_id, before_snapshot)
		VALUES (datetime('now'), datetime('now'), 'contact', 'vcard-1', 'update', 1, '{"a":1}')`)
	require.NoError(t, err)
	var legacyID int64
	require.NoError(t, sqlDB.QueryRow("SELECT id FROM audit_events WHERE entity_id = 'vcard-1'").Scan(&legacyID))
	require.EqualValues(t, 1, legacyID, "the test's insert must be the first row")

	require.NoError(t, m.Steps(1)) // 000034

	// The columns exist and carry the default (backfill is a Go step).
	var n int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('audit_events') WHERE name = 'hash'").Scan(&n))
	assert.EqualValues(t, 1, n, "000034 must add the hash column")
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('audit_events') WHERE name = 'prev_hash'").Scan(&n))
	assert.EqualValues(t, 1, n, "000034 must add the prev_hash column")

	// The pre-existing row survives with its id and content intact.
	var survivedID int64
	var survivedOp string
	var survivedHash string
	require.NoError(t, sqlDB.QueryRow(
		"SELECT id, operation, hash FROM audit_events WHERE entity_id = 'vcard-1'").Scan(&survivedID, &survivedOp, &survivedHash))
	assert.EqualValues(t, legacyID, survivedID, "the rebuild must preserve row ids")
	assert.Equal(t, "update", survivedOp)
	assert.Equal(t, "", survivedHash, "backfill is a Go step, not part of the migration")

	// The widened CHECK accepts auth lifecycle tokens and still rejects garbage.
	_, err = sqlDB.Exec(`
		INSERT INTO audit_events (created_at, updated_at, entity_type, entity_id, operation, user_id)
		VALUES (datetime('now'), datetime('now'), 'auth', 'alice', 'login', 1)`)
	require.NoError(t, err, "the widened CHECK must accept auth lifecycle operations")
	_, err = sqlDB.Exec(`
		INSERT INTO audit_events (created_at, updated_at, entity_type, entity_id, operation, user_id)
		VALUES (datetime('now'), datetime('now'), 'auth', 'alice', 'bogus', 1)`)
	assert.Error(t, err, "an operation outside the vocabulary must be rejected")

	// The immutability trigger survives the rebuild.
	_, err = sqlDB.Exec(`UPDATE audit_events SET operation = 'delete' WHERE entity_id = 'vcard-1'`)
	assert.Error(t, err, "the immutability trigger must survive the rebuild")

	// The AUTOINCREMENT sequence continues after the preserved max id.
	var nextID int64
	require.NoError(t, sqlDB.QueryRow(`
		INSERT INTO audit_events (created_at, updated_at, entity_type, entity_id, operation, user_id)
		VALUES (datetime('now'), datetime('now'), 'contact', 'vcard-2', 'create', 1)
		RETURNING id`).Scan(&nextID))
	assert.Greater(t, nextID, legacyID, "new rows must keep allocating ids after the preserved ones")

	// Down: back to the 000016 shape — chain columns gone, CRUD rows kept.
	require.NoError(t, MigrateDown(dbPath))
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('audit_events') WHERE name = 'hash'").Scan(&n))
	assert.Zero(t, n, "the down migration must drop the hash column")
	var crudCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM audit_events WHERE operation = 'create'").Scan(&crudCount))
	assert.EqualValues(t, 1, crudCount, "CRUD rows must survive a rollback")
	var loginCount int64
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM audit_events WHERE operation = 'login'").Scan(&loginCount))
	assert.Zero(t, loginCount, "auth lifecycle rows are dropped on rollback (the restored CHECK cannot represent them)")
	_, err = sqlDB.Exec(`UPDATE audit_events SET operation = 'delete' WHERE entity_id = 'vcard-1'`)
	assert.Error(t, err, "the immutability trigger must be restored by the down migration")
}

// TestSyncHealthFieldsMigration pins migration 000039's shape (issue #390):
// both subscription tables gain the last-known-good columns, existing rows get
// their history backfilled from the single last_sync_status bit they already
// carried, and a rollback drops the columns without destroying rows.
func TestSyncHealthFieldsMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sync-health-migration.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(38)) // through 000038_system_events

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 't390', 'x', 't390@example.com')")
	require.NoError(t, err)

	// One healthy and one failing subscription of each kind, on the pre-000039
	// schema (last_synced_at + last_sync_status only).
	for _, table := range []string{"contact_subscriptions", "calendar_subscriptions"} {
		_, err = sqlDB.Exec(fmt.Sprintf(`
			INSERT INTO %s (created_at, updated_at, user_id, name, url, sync_enabled, last_synced_at, last_sync_status)
			VALUES (datetime('now'), datetime('now'), 1, 'ok', 'https://ok.example', 1, '2026-08-20 09:00:00', 'success')`, table))
		require.NoError(t, err)
		_, err = sqlDB.Exec(fmt.Sprintf(`
			INSERT INTO %s (created_at, updated_at, user_id, name, url, sync_enabled, last_synced_at, last_sync_status)
			VALUES (datetime('now'), datetime('now'), 1, 'bad', 'https://bad.example', 1, '2026-08-21 10:30:00', 'error')`, table))
		require.NoError(t, err)
	}

	require.NoError(t, m.Steps(1)) // 000039

	for _, table := range []string{"contact_subscriptions", "calendar_subscriptions"} {
		var cols int64
		require.NoError(t, sqlDB.QueryRow(fmt.Sprintf(
			`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name IN
			 ('last_attempt_at','last_success_at','last_failure_at','consecutive_failures','incident_first_failure_at','last_run_duration_ms','last_run_stats')`,
			table)).Scan(&cols))
		assert.EqualValues(t, 7, cols, "%s must gain all seven sync-health columns", table)

		// Healthy row: last_success_at / last_attempt_at backfilled to equal
		// the row's own last_synced_at, no incident.
		var okSynced, okSuccess, okAttempt sql.NullString
		var okConsec int64
		var okIncident sql.NullString
		require.NoError(t, sqlDB.QueryRow(fmt.Sprintf(
			"SELECT last_synced_at, last_success_at, last_attempt_at, consecutive_failures, incident_first_failure_at FROM %s WHERE name = 'ok'", table)).
			Scan(&okSynced, &okSuccess, &okAttempt, &okConsec, &okIncident))
		assert.Equal(t, okSynced.String, okSuccess.String, "%s healthy row: last_success_at backfilled from last_synced_at", table)
		assert.Equal(t, okSynced.String, okAttempt.String)
		assert.Zero(t, okConsec)
		assert.False(t, okIncident.Valid, "%s healthy row: no open incident", table)

		// Failing row: one failure deep into an incident that started at last_synced_at.
		var badSynced, badFailure, badIncident sql.NullString
		var badConsec int64
		var badSuccess sql.NullString
		require.NoError(t, sqlDB.QueryRow(fmt.Sprintf(
			"SELECT last_synced_at, last_failure_at, incident_first_failure_at, consecutive_failures, last_success_at FROM %s WHERE name = 'bad'", table)).
			Scan(&badSynced, &badFailure, &badIncident, &badConsec, &badSuccess))
		assert.Equal(t, badSynced.String, badFailure.String)
		assert.Equal(t, badSynced.String, badIncident.String)
		assert.EqualValues(t, 1, badConsec, "%s failing row: honest floor of one failure", table)
		assert.False(t, badSuccess.Valid, "%s failing row: never succeeded", table)
	}

	// Down: columns gone, rows kept.
	require.NoError(t, MigrateDown(dbPath))
	for _, table := range []string{"contact_subscriptions", "calendar_subscriptions"} {
		var cols, rows int64
		require.NoError(t, sqlDB.QueryRow(fmt.Sprintf(
			"SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = 'consecutive_failures'", table)).Scan(&cols))
		assert.Zero(t, cols, "%s: the down migration must drop the sync-health columns", table)
		require.NoError(t, sqlDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&rows))
		assert.EqualValues(t, 2, rows, "%s: a rollback must not destroy subscriptions", table)
	}
}

// captureMigrationLogger swaps the package logger for one writing JSON to buf
// and restores it afterwards. Mirrors logger/context_test.go's captureLogger —
// this package reaches the same seams (logger.Logger, zerolog.DefaultContextLogger,
// zerolog global level) so it can assert on the structured failure line.
func captureMigrationLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := logger.Logger
	oldDefault := zerolog.DefaultContextLogger
	oldLevel := zerolog.GlobalLevel()
	logger.Logger = zerolog.New(buf)
	zerolog.DefaultContextLogger = &logger.Logger
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	t.Cleanup(func() {
		logger.Logger = old
		zerolog.DefaultContextLogger = oldDefault
		zerolog.SetGlobalLevel(oldLevel)
	})
	return buf
}

// firstCreateTable extracts the table name of the first CREATE TABLE (or
// CREATE VIRTUAL TABLE) statement in an embedded migration file. Used to
// sabotage the last migration's table so a re-run fails in a controlled way.
//
// Transient rebuild scratch tables (a "<name>_new" that a CHECK-constraint
// migration creates, copies into, and renames away — e.g. 000046) are
// skipped: they do not exist after the migration completes, so there is
// nothing to drop-and-recreate, and the real persisted table is what the
// test means to sabotage.
func firstCreateTable(t *testing.T, filename string) string {
	t.Helper()
	raw, err := fs.ReadFile(migrationsFS, filepath.Join("migrations", filename))
	require.NoError(t, err)
	re := regexp.MustCompile(`(?i)CREATE\s+(?:VIRTUAL\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([\w_]+)`)
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		if !strings.HasSuffix(m[1], "_new") {
			return m[1]
		}
	}
	return ""
}

// TestMigrationFailureIdentifiesMigrationAndRecordsEvent pins the milestone
// v0.6.2 gate criterion "Migration failures provide actionable diagnostics"
// (issue #532): a failed migration must name the failing version and file,
// emit a structured migration_failed log line, and persist a migration_failed
// system event — not just echo the SQL error.
//
// Setup: fully migrate a temp database, then drop and re-create the last
// table-creating migration's table with a conflicting schema and roll
// schema_migrations back one. golang-migrate then re-applies that migration
// and fails — a failure that lands after 000038, so system_events exists and
// the event row is real.
//
// The tail migration is deliberately NOT assumed to create a table: 000044
// (revision tokens) only ALTERs, so the test walks back from the tip to the
// last migration whose up.sql has a CREATE TABLE to sabotage.
func TestMigrationFailureIdentifiesMigrationAndRecordsEvent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fail.db")
	require.NoError(t, MigrateUp(dbPath))

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	require.NotZero(t, latest, "need at least one migration")

	// Walk back from the tip to the last migration that creates a table (a
	// table to drop and re-create with a conflicting schema).
	var target uint
	for v := latest; v >= 1; v-- {
		name := migrationFileForVersion(v)
		if name == "" {
			continue
		}
		if table := firstCreateTable(t, name); table != "" {
			target = v
			break
		}
	}
	require.NotZero(t, target, "no migration in the chain creates a table to sabotage")
	name := migrationFileForVersion(target)
	table := firstCreateTable(t, name)
	require.NotEmpty(t, table)

	_, err = sqlDB.Exec("DROP TABLE " + table)
	require.NoError(t, err)
	_, err = sqlDB.Exec("CREATE TABLE " + table + " (sabotage INTEGER)")
	require.NoError(t, err)
	_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?, dirty = 0", target-1)
	require.NoError(t, err)

	buf := captureMigrationLogger(t)
	err = RunMigrations(sqlDB)
	require.Error(t, err, "sabotaged migration must fail")
	assert.Contains(t, err.Error(), strconv.FormatUint(uint64(target), 10), "returned error must name the failing migration version")
	assert.Contains(t, err.Error(), name, "returned error must name the failing migration file")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines, "a structured migration_failed log line must be emitted")
	var line map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &line))
	assert.Equal(t, "migration_failed", line["event"])
	assert.Equal(t, strconv.FormatUint(uint64(target), 10), fmt.Sprintf("%v", line["version"]))
	assert.Equal(t, name, line["migration"])

	var count int64
	// RunMigrations' closeMigrator closes sqlDB (migrate.go's note: Close()'s
	// database half closes the *sql.DB), so count on a fresh connection.
	checkDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer checkDB.Close()
	require.NoError(t, checkDB.QueryRow("SELECT COUNT(*) FROM system_events WHERE event_type = 'migration_failed'").Scan(&count))
	assert.EqualValues(t, 1, count, "a migration_failed system event must be recorded")
}
