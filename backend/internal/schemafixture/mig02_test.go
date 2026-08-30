package schemafixture

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestUpgradeFixtureToCurrent is MIG-02's headline upgrade test (issue #437,
// action 1): every supported-release fixture is migrated to the CURRENT schema
// through database.InitDB — the exact path the server boots through, not a
// reconstructed migration call (CLAUDE.md backend trap 1) — asserting the
// final version, a clean (non-dirty) flag, and that every table's row count
// survived. A migration that drops a table exits zero, so exit status alone
// proves nothing; the row counts are the "ran" half of the MIG-02/MIG-03
// boundary (MIG-03 proves the migration preserved *meaning*).
//
// Each release is a NAMED subtest so the CI matrix can run one release per
// job (`-run TestUpgradeFixtureToCurrent/v0.6.1`) and a failure names the
// release that broke. The v0.6.0 entry IS the longest supported skip
// (v0.6.0 -> current, issue #529) — a single additional matrix entry that is
// the higher-value shape, run here through the whole chain at once.
func TestUpgradeFixtureToCurrent(t *testing.T) {
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)

	for _, r := range SupportedReleases {
		t.Run(r.Tag, func(t *testing.T) {
			f := Load(t, r)
			before := tableCounts(t, f.DB)
			closeFixtureDB(t, f.DB)

			db, err := database.InitDB(f.Path)
			require.NoError(t, err, "InitDB must migrate the %s fixture to the current schema", r.Tag)
			t.Cleanup(func() {
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close()
				}
			})

			version, dirty, ok, err := database.AppliedMigrationVersion(db)
			require.NoError(t, err)
			require.True(t, ok, "the upgraded %s fixture must carry a schema_migrations row", r.Tag)
			assert.EqualValuesf(t, latest, version, "%s -> current must land on the current schema", r.Tag)
			assert.False(t, dirty)

			assertRowCountsPreserved(t, fmt.Sprintf("%s -> current", r.Tag), before, tableCounts(t, db))
		})
	}
}

// migrationDiagnosticTables are system-generated bookkeeping tables that a
// migration run itself legitimately appends to — RunMigrations records a
// migration_completed row into system_events on every schema advance — so
// their row counts are not stable across an upgrade and must not be asserted
// equal. Every other table is user data and must survive exactly.
var migrationDiagnosticTables = map[string]bool{
	"system_events": true,
}

// assertRowCountsPreserved asserts that every table in before still holds the
// same row count in after, skipping migrationDiagnosticTables. The step name
// identifies the failing leg in the message.
func assertRowCountsPreserved(t *testing.T, step string, before, after map[string]int64) {
	t.Helper()
	for table, want := range before {
		if migrationDiagnosticTables[table] {
			continue
		}
		assert.Equalf(t, want, after[table],
			"%s must preserve %s row counts", step, table)
	}
}

// TestEveryMigrationRoundTripsUpDownUp is MIG-02's down-direction gate (issue
// #437 action 6): every migration in the chain must round-trip up -> down ->
// up against a POPULATED fixture — an empty-schema round trip proves nothing
// (issue #530 keeps .down.sql required), and the populated fixture is what
// shows a down migration that destroys pre-existing data. Each migration v is
// tested on a fixture built at version v-1 from the canonical manifest data
// intersected to that schema, so:
//
//   - the up migration runs over real rows;
//   - MigrateDown (exactly one step, the migration this test is about) must
//     reverse it AND leave the version v-1 rows intact;
//   - the re-up must apply cleanly again on top of the restored rows.
//
// Row counts are asserted after every leg, in both directions. The schema is
// asserted byte-exact too (MIG-05, issue #440 action 4): the fingerprint after
// a down must equal the fingerprint before its up, so "what `make migrate-down`
// destroys" is specified by the migration itself and pinned by CI — exactly the
// objects and columns its up created, and nothing pre-existing — rather than
// discovered by an operator at 2am. The re-up leg independently guards this
// (a leftover table/column/index breaks it), but only the fingerprint catches a
// down that removes MORE than its up created.
func TestEveryMigrationRoundTripsUpDownUp(t *testing.T) {
	data := extractManifestData(t)

	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)

	for v := uint(1); v <= latest; v++ {
		t.Run(fmt.Sprintf("%06d", v), func(t *testing.T) {
			path := buildVersionFixturePath(t, data, v-1)
			before := tableCountsAtPath(t, path)
			beforeSchema := schemaFingerprintAtPath(t, path)

			require.NoError(t, database.MigrateUpTo(path, v), "migration %06d up must apply", v)
			assertFixtureVersion(t, path, v)
			assertRowCountsPreserved(t, fmt.Sprintf("up %06d", v), before, tableCountsAtPath(t, path))

			require.NoError(t, database.MigrateDown(path), "migration %06d down must apply", v)
			assertFixtureVersion(t, path, v-1)
			assertRowCountsPreserved(t, fmt.Sprintf("down %06d", v), before, tableCountsAtPath(t, path))
			assert.Equal(t, beforeSchema, schemaFingerprintAtPath(t, path),
				"down %06d must restore the pre-up schema exactly: a down migration destroys exactly what its up created, and nothing more", v)

			require.NoError(t, database.MigrateUpTo(path, v), "migration %06d re-up must apply", v)
			assertFixtureVersion(t, path, v)
			assertRowCountsPreserved(t, fmt.Sprintf("re-up %06d", v), before, tableCountsAtPath(t, path))
		})
	}
}

// schemaFingerprintAtPath opens path, returns the canonical schema fingerprint
// of the database at it, and closes the handle again — the same pattern as
// tableCountsAtPath: a migration step must never run against a stale open
// connection.
func schemaFingerprintAtPath(t *testing.T, path string) string {
	t.Helper()
	db, err := database.OpenMigratedFile(path)
	require.NoError(t, err)
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	return schemaFingerprint(t, db)
}

// schemaFingerprint returns a canonical, sorted text encoding of the database's
// schema: every non-internal sqlite_master object (tables, virtual tables,
// indexes, triggers) with its CREATE statement, plus every table's full column
// definition (PRAGMA table_info) — the latter because SQLite never rewrites the
// original CREATE TABLE statement in sqlite_master on ALTER TABLE ADD/DROP
// COLUMN, so a column-level destruction would be invisible to the sqlite_master
// text alone. Two databases with identical fingerprints hold the same schema
// objects with the same columns.
func schemaFingerprint(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var objects []struct {
		Type    string
		Name    string
		SQL     string
		TblName string
	}
	// schema_migrations is golang-migrate's own bookkeeping, not application
	// schema: it is created lazily and left behind (empty) after the final
	// down, so a version-0 fixture has it and an unmigrated file does not.
	// Filtered by tbl_name so its version_unique index is excluded too.
	require.NoError(t, db.Raw(`
		SELECT type, name, COALESCE(sql, '') AS sql, tbl_name FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%' AND tbl_name != 'schema_migrations'
		ORDER BY type, name`).Scan(&objects).Error)

	var b strings.Builder
	for _, o := range objects {
		// SQLite rewrites the CREATE statement's table name with double quotes
		// when a table is recreated via ALTER TABLE ... RENAME (migration
		// 000034's audit_events rebuild does this in both directions). The
		// quotes are identifier quoting, never literal content in this DDL
		// (CHECK/DEFAULT bodies use single quotes), so stripping them makes the
		// fingerprint compare semantics rather than quoting cosmetics.
		fmt.Fprintf(&b, "%s|%s|%s\n", o.Type, o.Name, strings.ReplaceAll(strings.TrimSpace(o.SQL), `"`, ""))
		if o.Type != "table" {
			continue
		}
		var cols []struct {
			CID     int
			Name    string
			Type    string
			NotNull int
			Default *string
			PK      int
		}
		require.NoError(t, db.Raw(`PRAGMA table_info("`+strings.ReplaceAll(o.Name, `"`, `""`)+`")`).Scan(&cols).Error)
		sort.Slice(cols, func(i, j int) bool { return cols[i].CID < cols[j].CID })
		for _, c := range cols {
			dflt := ""
			if c.Default != nil {
				dflt = *c.Default
			}
			fmt.Fprintf(&b, "  col|%d|%s|%s|%d|%s|%d\n", c.CID, c.Name, c.Type, c.NotNull, dflt, c.PK)
		}
	}
	return b.String()
}

// extractManifestData populates one current-schema scratch database from the
// canonical manifest and extracts its data, so the down-direction test can
// build every version's fixture from a single population (the expensive part
// of Load) instead of repopulating per version.
func extractManifestData(t *testing.T) map[string]tableData {
	t.Helper()
	data, _, err := populateCurrentSchema(t, Release{})
	if err != nil {
		t.Fatalf("schemafixture: populating manifest dataset: %v", err) // # pragma: no cover — a committed manifest against a migrated scratch DB always populates
	}
	return data
}

// buildVersionFixturePath builds a populated fixture DATABASE FILE whose
// schema is exactly migration version v (via MigrateUpTo over the frozen
// embedded chain — equivalent to a release whose highest applied migration is
// v), copies the pre-extracted manifest data into it (columns intersected to
// that version), and returns the CLOSED file path. The connection is closed
// so the caller can migrate the file without a stale handle, mirroring
// closeFixtureDB in the release-fixture tests.
//
// Version 0 (no schema) is special-cased: MigrateUpTo(0) is not a real
// migration target, and the initial-schema migration's fixture is empty by
// definition — the copy is a no-op because no tables exist yet.
func buildVersionFixturePath(t *testing.T, data map[string]tableData, version uint) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.db")
	if version > 0 {
		if err := database.MigrateUpTo(path, version); err != nil {
			t.Fatalf("schemafixture: building version-%d schema: %v", version, err) // # pragma: no cover — the frozen chain always migrates a fresh file
		}
	}

	conn, err := sqlOpenFixture(path)
	if err != nil {
		t.Fatalf("schemafixture: opening version-%d fixture: %v", version, err) // # pragma: no cover — a driver already registered cannot fail to open a file DSN
	}
	// The copy inserts in alphabetical table order, so foreign-key enforcement
	// must be off for the copying connection (openDSN enables it; the release
	// path relies on the dump's own PRAGMA foreign_keys=OFF for the same).
	if _, err := conn.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		_ = conn.Close()
		t.Fatalf("schemafixture: disabling foreign keys for version-%d copy: %v", version, err) // # pragma: no cover — a fresh connection accepts PRAGMA
	}
	if err := copyData(conn, data); err != nil {
		_ = conn.Close()
		t.Fatalf("schemafixture: populating version-%d schema with manifest data: %v", version, err) // # pragma: no cover — an INSERT against columns just read from this schema cannot fail
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("schemafixture: closing version-%d schema connection: %v", version, err) // # pragma: no cover — Close on an open handle does not fail
	}
	return path
}

// sqlOpenFixture opens a raw connection to a fixture database file with the
// app's standard DSN, for the raw data copy in buildVersionFixturePath.
func sqlOpenFixture(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
}

// tableCountsAtPath opens path read-only, returns every table's row count (the
// same shape tableCounts produces for an open GORM handle), and closes the
// handle again — a migration step must never run against a stale open
// connection (the closeFixtureDB pattern).
func tableCountsAtPath(t *testing.T, path string) map[string]int64 {
	t.Helper()
	db, err := database.OpenMigratedFile(path)
	require.NoError(t, err)
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	return tableCounts(t, db)
}

// assertFixtureVersion asserts the database at path is at exactly want,
// clean. Version 0 is represented by an absent schema_migrations row
// (golang-migrate deletes it on the final down), which MigrationVersion
// reports as ok=false.
func assertFixtureVersion(t *testing.T, path string, want uint) {
	t.Helper()
	version, dirty, ok, err := database.MigrationVersion(path)
	require.NoError(t, err)
	if want == 0 {
		assert.False(t, ok, "a database at version 0 must report no applied migration")
		return
	}
	require.Truef(t, ok, "a database at version %d must carry a schema_migrations row", want)
	assert.EqualValues(t, want, version)
	assert.False(t, dirty)
}
