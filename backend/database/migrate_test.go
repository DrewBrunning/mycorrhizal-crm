package database

import (
	"database/sql"
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
