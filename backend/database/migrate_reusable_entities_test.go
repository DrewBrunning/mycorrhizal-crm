package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migrateToBeforeEntities opens a fresh file database and migrates it to the
// version just before 000091_reusable_entities (every earlier migration).
func migrateToBeforeEntities(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "entities.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(90))
	return dbPath, sqlDB
}

func seedEntityUserAndDefs(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	_, err := db.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'ent', 'x', 'ent@example.com')")
	require.NoError(t, err)
	var userID int64
	require.NoError(t, db.QueryRow("SELECT id FROM users WHERE username = 'ent'").Scan(&userID))
	for _, d := range []string{"d1", "d2"} {
		_, err = db.Exec(`INSERT INTO field_definitions (id, created_at, updated_at, user_id, label, key, target, type, projection, sensitivity)
			VALUES (?, datetime('now'), datetime('now'), ?, ?, ?, 'contact', 'string', 'internal-only', 'normal')`, d, userID, "L"+d, "key_"+d)
		require.NoError(t, err)
	}
	return userID
}

// TestMigrationReusableEntities covers 000091 (issue #2295, ADR 0043): a
// populated database upgrades with every existing row intact, key uniqueness
// is relaxed to (user, target, key), and the down migration of an unused
// schema restores the previous shape exactly.
func TestMigrationReusableEntities(t *testing.T) {
	t.Parallel()
	_, sqlDB := migrateToBeforeEntities(t)
	defer sqlDB.Close()
	userID := seedEntityUserAndDefs(t, sqlDB)
	_, err := sqlDB.Exec(`INSERT INTO field_values (created_at, updated_at, field_definition_id, user_id, entity_id, value) VALUES (datetime('now'), datetime('now'), 'd1', ?, 'some-uid', '"v"')`, userID)
	require.NoError(t, err)

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(1)) // 000091

	var defs, values int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM field_definitions WHERE target = 'contact'").Scan(&defs))
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM field_values").Scan(&values))
	assert.Equal(t, 2, defs, "existing contact definitions are preserved")
	assert.Equal(t, 1, values, "existing field values are preserved")

	for _, table := range []string{"entities", "contact_entity_links", "entity_deletion_snapshots", "dismissed_entity_pairs"} {
		var n int
		require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?", table).Scan(&n))
		assert.Equal(t, 1, n, "%s exists", table)
	}

	// Uniqueness is per (user, target, key): the same key on a different target
	// is allowed, the same key on the same target is not.
	ins := func(id, target string) error {
		_, err := sqlDB.Exec(`INSERT INTO field_definitions (id, created_at, updated_at, user_id, label, key, target, type, projection, sensitivity)
			VALUES (?, datetime('now'), datetime('now'), ?, 'L', 'key_d1', ?, 'string', 'internal-only', 'normal')`, id, userID, target)
		return err
	}
	assert.NoError(t, ins("d3", "organisation"))
	assert.Error(t, ins("d4", "organisation"))
	assert.Error(t, ins("d5", "contact"), "an existing contact key is still unique per user")

	// Round trip on a database that holds no entity data: down then up again.
	_, err = sqlDB.Exec("DELETE FROM field_definitions WHERE target <> 'contact'")
	require.NoError(t, err)
	require.NoError(t, m.Steps(-1))
	var idxSQL string
	require.NoError(t, sqlDB.QueryRow("SELECT sql FROM sqlite_master WHERE name = 'idx_field_definitions_user_key'").Scan(&idxSQL))
	assert.Contains(t, idxSQL, "(user_id, key)", "down restores the old index")
	var gone int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name IN ('entities','contact_entity_links','entity_deletion_snapshots','dismissed_entity_pairs')").Scan(&gone))
	assert.Zero(t, gone, "down drops the four new tables")
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM field_definitions").Scan(&defs))
	assert.Equal(t, 2, defs, "down keeps the contact definitions")
	require.NoError(t, m.Steps(1))
}

// The down migration refuses, rather than deleting entity data, whenever an
// entity, a link or a non-contact field definition exists.
func TestMigrationReusableEntities_DownRefusesWhenEntityDataExists(t *testing.T) {
	t.Parallel()
	seeds := map[string]func(t *testing.T, db *sql.DB, userID int64){
		"entity": func(t *testing.T, db *sql.DB, userID int64) {
			_, err := db.Exec("INSERT INTO entities (id, user_id, kind, name) VALUES ('e1', ?, 'pet', 'Rex')", userID)
			require.NoError(t, err)
		},
		"soft-deleted entity": func(t *testing.T, db *sql.DB, userID int64) {
			_, err := db.Exec("INSERT INTO entities (id, user_id, kind, name, deleted_at) VALUES ('e1', ?, 'pet', 'Rex', datetime('now'))", userID)
			require.NoError(t, err)
		},
		"link": func(t *testing.T, db *sql.DB, userID int64) {
			_, err := db.Exec("INSERT INTO entities (id, user_id, kind, name) VALUES ('e1', ?, 'pet', 'Rex')", userID)
			require.NoError(t, err)
			_, err = db.Exec("INSERT INTO contact_entity_links (id, user_id, contact_vcard_uid, entity_id, kind, role) VALUES ('l1', ?, 'u', 'e1', 'pet', 'owner')", userID)
			require.NoError(t, err)
		},
		"non-contact field definition": func(t *testing.T, db *sql.DB, userID int64) {
			_, err := db.Exec(`INSERT INTO field_definitions (id, created_at, updated_at, user_id, label, key, target, type, projection, sensitivity)
				VALUES ('dx', datetime('now'), datetime('now'), ?, 'L', 'kx', 'place', 'string', 'internal-only', 'normal')`, userID)
			require.NoError(t, err)
		},
	}
	for name, seed := range seeds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, sqlDB := migrateToBeforeEntities(t)
			defer sqlDB.Close()
			userID := seedEntityUserAndDefs(t, sqlDB)
			m, err := newMigrator(sqlDB)
			require.NoError(t, err)
			require.NoError(t, m.Steps(1))
			seed(t, sqlDB, userID)

			err = m.Steps(-1)
			require.Error(t, err, "the down migration must refuse")
			assert.Contains(t, err.Error(), "cannot roll back 000091", "with a clear message")

			var tables int
			require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('entities','contact_entity_links')").Scan(&tables))
			assert.Equal(t, 2, tables, "nothing was dropped")
		})
	}
}

// A refused rollback must leave the database usable, not dirty: the guard
// fails before anything is dropped and the migrator reports the version as it
// was, so the operator is not forced into `migrate force`.
func TestMigrationReusableEntities_RefusedDownLeavesCleanVersion(t *testing.T) {
	t.Parallel()
	_, sqlDB := migrateToBeforeEntities(t)
	defer sqlDB.Close()
	userID := seedEntityUserAndDefs(t, sqlDB)
	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(1))
	_, err = sqlDB.Exec("INSERT INTO entities (id, user_id, kind, name) VALUES ('e1', ?, 'pet', 'Rex')", userID)
	require.NoError(t, err)

	require.Error(t, m.Steps(-1), "driving the migrator directly still hits the SQL guard")

	// Through MigrateDown (what `make migrate-down` and the CLI run) the refusal
	// is a pre-flight, so the version row is never marked dirty.
	dbPath, fileDB := migrateToBeforeEntities(t)
	defer fileDB.Close()
	fileUser := seedEntityUserAndDefs(t, fileDB)
	fm, err := newMigrator(fileDB)
	require.NoError(t, err)
	require.NoError(t, fm.Steps(1))
	_, err = fileDB.Exec("INSERT INTO entities (id, user_id, kind, name) VALUES ('e1', ?, 'pet', 'Rex')", fileUser)
	require.NoError(t, err)

	err = MigrateDown(dbPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot roll back 000091")
	var version int
	var dirty bool
	require.NoError(t, fileDB.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	assert.Equal(t, 91, version)
	assert.False(t, dirty, "a refused rollback must not leave the schema_migrations row dirty")
}

// The pre-flight reports a database it cannot inspect rather than guessing.
func TestMigrationReusableEntities_DownGuardReportsUnreadableDatabase(t *testing.T) {
	t.Parallel()
	sqlDB, err := sql.Open("sqlite", openDSN(filepath.Join(t.TempDir(), "empty.db")))
	require.NoError(t, err)
	defer sqlDB.Close()
	err = migrateDownGuards[91](sqlDB)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for entity data")
}
