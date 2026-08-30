package database

// MIG-04 (issue #439): the three fail-closed startup states, each refusal's
// typed error, the "leave the database untouched" property, and the
// operator-only force recovery. #546 is the concrete bug these tests close: a
// dirty database used to be force-cleared at boot and migrated on top of.

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dirtyDB migrates dbPath to the latest schema and then marks it dirty at the
// given version — the crash signature of a migration started but not finished.
func dirtyDB(t *testing.T, dbPath string, version uint) {
	t.Helper()
	require.NoError(t, MigrateUp(dbPath))
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?, dirty = 1", version)
	require.NoError(t, err)
}

// aheadDB migrates dbPath to the latest schema and then bumps the version row
// past the binary's newest migration — the signature of a binary rolled back
// onto a database a newer release migrated.
func aheadDB(t *testing.T, dbPath string) uint {
	t.Helper()
	require.NoError(t, MigrateUp(dbPath))
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	ahead := latest + 1
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?", ahead)
	require.NoError(t, err)
	return ahead
}

// TestDirtyDatabaseRefusesToStart is MIG-04 state 1 / issue #546: a database
// whose last migration started but did not finish must refuse to start, name
// the dirty version and the recovery path, and leave the database exactly as
// it was — never force the version, never migrate on top, never boot.
func TestDirtyDatabaseRefusesToStart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dirty.db")
	dirtyDB(t, dbPath, 1)

	_, err := InitDB(dbPath)
	require.Error(t, err, "a dirty database must refuse to start")

	var dirtyErr *ErrDirtyMigration
	require.ErrorAs(t, err, &dirtyErr, "the refusal must be a typed ErrDirtyMigration, not a generic boot failure")
	assert.EqualValues(t, 1, dirtyErr.Version, "the refusal must name the dirty version")

	msg := err.Error()
	assert.Contains(t, msg, "dirty migration state at version 1", "the refusal must name the condition and version")
	assert.Contains(t, msg, "did not finish", "the refusal must explain what dirty means")
	assert.Contains(t, msg, "Restore the pre-migration backup", "the refusal must name the recovery action")
	assert.Contains(t, msg, "force", "the refusal must point at the operator-only escape hatch")

	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, 1, version, "a refused database must not have been partially migrated")
	assert.True(t, dirty, "a refusal must not clear the dirty flag — the state is the signal")
}

// TestDirtyRefusalHoldsForEveryVersion makes the dirty refusal monotone across
// schema positions: sub-floor (a crashed initial install), at the floor, and
// at the latest version all refuse identically. A dirty flag means the schema
// state is unknown, and that is true at every version.
func TestDirtyRefusalHoldsForEveryVersion(t *testing.T) {
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)

	for _, version := range []uint{1, SupportedUpgradeFloorVersion, latest} {
		t.Run(fmt.Sprintf("dirty-at-%d", version), func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "dirty.db")
			dirtyDB(t, dbPath, version)

			_, err := InitDB(dbPath)
			require.Error(t, err, "a dirty database must refuse to start")
			var dirtyErr *ErrDirtyMigration
			require.ErrorAs(t, err, &dirtyErr)
			assert.EqualValues(t, version, dirtyErr.Version)
		})
	}
}

// TestDirtyRefusalCannotBeBypassedByConfig pins "no configuration setting can
// turn any of the three refusals into a warning": the documented sub-floor
// bridge override (MYCORRHIZAL_ALLOW_SUB_FLOOR_MIGRATION) exists to migrate a
// CLEAN pre-floor database; it must not also force-continue a dirty one. The
// one-time bridge is an exception to the floor policy, not to the fail-closed
// posture.
func TestDirtyRefusalCannotBeBypassedByConfig(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dirty-bridge.db")
	dirtyDB(t, dbPath, 1)

	t.Setenv(subFloorMigrationEnvVar, "1")
	_, err := InitDB(dbPath)
	require.Error(t, err, "the bridge env var must not bypass the dirty refusal")
	var dirtyErr *ErrDirtyMigration
	require.ErrorAs(t, err, &dirtyErr)
}

// TestSchemaAheadOfBinaryRefusesToStart is MIG-04 state 2: a database carrying
// migrations this binary does not know about is a rollback in progress, and
// starting anyway would have the binary misread columns it does not know —
// the same silent-corruption class as the dirty-force bug. It must refuse,
// name both versions, name the recovery, and leave the database untouched.
func TestSchemaAheadOfBinaryRefusesToStart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ahead.db")
	ahead := aheadDB(t, dbPath)

	_, err := InitDB(dbPath)
	require.Error(t, err, "a database ahead of the binary must refuse to start")

	var aheadErr *ErrSchemaAheadOfBinary
	require.ErrorAs(t, err, &aheadErr, "the refusal must be a typed ErrSchemaAheadOfBinary, not a generic boot failure")
	assert.EqualValues(t, ahead, aheadErr.Version, "the refusal must name the database's version")
	msg := err.Error()
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	assert.EqualValues(t, latest, aheadErr.BinaryVersion, "the refusal must name the binary's latest known migration")

	assert.Contains(t, msg, fmt.Sprintf("database schema version %d is ahead of this binary (latest known migration %d)", ahead, latest))
	assert.Contains(t, msg, "rolled back", "the refusal must explain what ahead-of-binary means")
	assert.Contains(t, msg, "Downgrade is unsupported", "the refusal must state the downgrade policy")
	assert.Contains(t, msg, "restore the backup", "the refusal must name the recovery action")

	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, ahead, version, "a refused database must not have been touched")
	assert.False(t, dirty)
}

// TestSchemaAheadRefusalHoldsForMultipleAheadVersions makes the ahead refusal
// hold one and several migrations past the binary's knowledge.
func TestSchemaAheadRefusalHoldsForMultipleAheadVersions(t *testing.T) {
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)

	for _, ahead := range []uint{latest + 1, latest + 5} {
		t.Run(fmt.Sprintf("ahead-by-%d", ahead-latest), func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "ahead.db")
			require.NoError(t, MigrateUp(dbPath))
			sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
			require.NoError(t, err)
			_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?", ahead)
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())

			_, err = InitDB(dbPath)
			require.Error(t, err)
			var aheadErr *ErrSchemaAheadOfBinary
			require.ErrorAs(t, err, &aheadErr)
			assert.EqualValues(t, ahead, aheadErr.Version)
		})
	}
}

// TestPreflightRefusalsLeaveDatabaseUntouched consolidates the "refuse, don't
// repair" property across every state: after each refusal the schema_migrations
// row (version + dirty) is byte-identical to before, so the refusal itself can
// never be the thing that loses data.
func TestPreflightRefusalsLeaveDatabaseUntouched(t *testing.T) {
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)

	cases := []struct {
		name        string
		setup       func(t *testing.T, dbPath string)
		wantVersion uint
		wantDirty   bool
	}{
		{
			name:        "dirty",
			setup:       func(t *testing.T, dbPath string) { dirtyDB(t, dbPath, 1) },
			wantVersion: 1,
			wantDirty:   true,
		},
		{
			name: "ahead-of-binary",
			setup: func(t *testing.T, dbPath string) {
				require.NoError(t, MigrateUp(dbPath))
				sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
				require.NoError(t, err)
				_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?", latest+2)
				require.NoError(t, err)
				require.NoError(t, sqlDB.Close())
			},
			wantVersion: latest + 2,
			wantDirty:   false,
		},
		{
			name: "sub-floor",
			setup: func(t *testing.T, dbPath string) {
				sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
				require.NoError(t, err)
				defer sqlDB.Close()
				m, err := newMigrator(sqlDB)
				require.NoError(t, err)
				require.NoError(t, m.Steps(int(SupportedUpgradeFloorVersion)-1))
				closeMigrator(m)
			},
			wantVersion: SupportedUpgradeFloorVersion - 1,
			wantDirty:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "untouched.db")
			tc.setup(t, dbPath)

			before, beforeDirty, okBefore, err := MigrationVersion(dbPath)
			require.NoError(t, err)
			require.True(t, okBefore)

			_, err = InitDB(dbPath)
			require.Error(t, err, "this state must refuse")

			after, afterDirty, okAfter, err := MigrationVersion(dbPath)
			require.NoError(t, err)
			require.True(t, okAfter)
			assert.Equal(t, before, after, "the version must be untouched by a refusal")
			assert.Equal(t, beforeDirty, afterDirty, "the dirty flag must be untouched by a refusal")
			assert.EqualValues(t, tc.wantVersion, after)
			assert.Equal(t, tc.wantDirty, afterDirty)
		})
	}
}

// TestMigrateForce_RefusesWhenClean pins that the operator-only escape hatch
// only acts on a dirty database: running it against a healthy database is an
// error, never a silent no-op.
func TestMigrateForce_RefusesWhenClean(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "force-clean.db")
	require.NoError(t, MigrateUp(dbPath))

	err := MigrateForce(dbPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not dirty", "force must explain why it refuses a clean database")
}

// TestMigrateForce_RefusesWhenNeverMigrated pins that force against a database
// with no migrations at all (nothing is dirty) is an error.
func TestMigrateForce_RefusesWhenNeverMigrated(t *testing.T) {
	err := MigrateForce(filepath.Join(t.TempDir(), "force-empty.db"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no migrations have been applied")
}

// TestMigrateForce_ReportsBrokenDB pins that force on an unopenable database
// surfaces the open/migrator failure rather than panicking.
func TestMigrateForce_ReportsBrokenDB(t *testing.T) {
	err := MigrateForce(filepath.Join(t.TempDir(), "no-such-dir", "x.db"))
	require.Error(t, err)
}

// TestMigrateForce_RefusesWhenAheadOfBinary pins that forcing an unknown
// version is refused: a database dirty at a version this binary does not know
// cannot be force-validated (nothing to re-run, and the binary cannot confirm
// the schema), so the recovery is roll-forward or restore, not force.
func TestMigrateForce_RefusesWhenAheadOfBinary(t *testing.T) {
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "force-ahead.db")
	dirtyDB(t, dbPath, latest+1)

	err = MigrateForce(dbPath)
	require.Error(t, err)
	var aheadErr *ErrSchemaAheadOfBinary
	require.ErrorAs(t, err, &aheadErr)
	assert.EqualValues(t, latest+1, aheadErr.Version)

	version, dirty, _, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	assert.EqualValues(t, latest+1, version, "a refused force must not touch the database")
	assert.True(t, dirty)
}

// interruptedDB builds a database whose schema genuinely matches version at
// (migrations 000001..at applied) but whose version row says at, dirty — the
// crash signature a process killed after a migration's commit but before its
// clean-mark leaves behind: the DDL is fully applied, and the dirty row is the
// fail-closed signal. The operator-only force can reason about this state:
// force `at` and re-run from at+1.
func interruptedDB(t *testing.T, dbPath string, at uint) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(int(at)))
	_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?, dirty = 1", at)
	require.NoError(t, err)
}

// TestMigrateForce_RecoversDirtyDatabase pins the operator-only recovery end
// to end at a mid-chain version: force clears the dirty flag at the interrupted
// version and re-runs every pending migration, landing clean at the latest
// schema.
func TestMigrateForce_RecoversDirtyDatabase(t *testing.T) {
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	require.Greater(t, latest, uint(1), "this test needs a migration beyond the baseline")

	dbPath := filepath.Join(t.TempDir(), "force-recover.db")
	interruptedDB(t, dbPath, 1)

	require.NoError(t, MigrateForce(dbPath))

	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, latest, version, "force must re-run every pending migration to the latest schema")
	assert.False(t, dirty, "force must leave the database clean")
}

// TestMigrateForce_DirtySubFloor pins the crashed-initial-install recovery: a
// fresh install killed mid-migration is dirty at a sub-floor version, and the
// operator-only force must recover it to the latest schema. A dirty database is
// not a stable pre-floor deployment, so the floor two-step does not apply — the
// operator's explicit confirmation is the gate.
func TestMigrateForce_DirtySubFloor(t *testing.T) {
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)

	// A real crashed initial install: the first two migrations applied (schema
	// genuinely at version 2), then a later one started and did not finish.
	dbPath := filepath.Join(t.TempDir(), "force-subfloor.db")
	interruptedDB(t, dbPath, 2)

	require.NoError(t, MigrateForce(dbPath))

	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, latest, version)
	assert.False(t, dirty)
}

// TestErrDirtyMigrationIsStableSentinel guards the exact message operators see
// in the dirty refusal so the documentation (which quotes it) cannot drift from
// the implementation.
func TestErrDirtyMigrationIsStableSentinel(t *testing.T) {
	err := &ErrDirtyMigration{Version: 43}
	msg := err.Error()
	assert.Contains(t, msg, "dirty migration state at version 43")
	assert.Contains(t, msg, "did not finish")
	assert.Contains(t, msg, "Refusing to start (fail-closed)")
	assert.Contains(t, msg, "Restore the pre-migration backup")
	assert.Contains(t, msg, "docs/deployment.md")
	assert.Contains(t, msg, "make migrate-force")
	assert.True(t, errors.As(err, new(*ErrDirtyMigration)))
}

// TestErrSchemaAheadOfBinaryIsStableSentinel guards the ahead-of-binary
// message the same way.
func TestErrSchemaAheadOfBinaryIsStableSentinel(t *testing.T) {
	err := &ErrSchemaAheadOfBinary{Version: 45, BinaryVersion: 44}
	msg := err.Error()
	assert.Contains(t, msg, "database schema version 45 is ahead of this binary (latest known migration 44)")
	assert.Contains(t, msg, "rolled back")
	assert.Contains(t, msg, "Downgrade is unsupported")
	assert.Contains(t, msg, "Deploy a binary that knows migration 45")
	assert.Contains(t, msg, "restore the backup taken before the newer release ran")
	assert.True(t, errors.As(err, new(*ErrSchemaAheadOfBinary)))
}

// TestMigrationBodyRollsBackAsOneTransaction pins the transactional property of
// individual migrations (MIG-04 action 4 / #546 action 5): each migration body
// runs inside ONE transaction, so a mid-body failure rolls back the earlier
// statements too. SQLite DDL is transactional, and this driver wraps each
// migration in a transaction by default (executeQuery, NoTxWrap=false), which
// narrows the dirty state from "any interruption leaves torn DDL" to "the DDL
// is all-or-nothing at exactly one version; the version row is dirty until a
// human resolves it".
func TestMigrationBodyRollsBackAsOneTransaction(t *testing.T) {
	db := openTestSQLite(t)
	m := &sqliteDriver{db: db, config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}

	err := m.executeQuery("CREATE TABLE tx_a (id INTEGER); CREATE TABLE tx_b (this is not sql);")
	require.Error(t, err, "a migration body with a failing statement must fail")

	var n int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('tx_a', 'tx_b')",
	).Scan(&n))
	assert.Zero(t, n, "the whole migration body must have rolled back as one transaction")
}

// TestSkippedMigrationFailureLeavesDirtyAndSchemaAtPreviousVersion pins the
// end-to-end consequence of the transactional property against the real chain:
// when a migration's SQL fails, the database is left dirty at that version
// (the fail-closed signal) but the DDL is NOT partially applied — the schema
// still matches the previous version, exactly what restore-from-backup or the
// operator-only force can reason about. The sabotage technique mirrors
// TestMigrationFailureIdentifiesMigrationAndRecordsEvent.
func TestSkippedMigrationFailureLeavesDirtyAndSchemaAtPreviousVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tx-fail.db")
	require.NoError(t, MigrateUp(dbPath))

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	latest, err := LatestMigrationVersion()
	require.NoError(t, err)

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
	table := firstCreateTable(t, migrationFileForVersion(target))
	require.NotEmpty(t, table)

	_, err = sqlDB.Exec("DROP TABLE " + table)
	require.NoError(t, err)
	_, err = sqlDB.Exec("CREATE TABLE " + table + " (sabotage INTEGER)")
	require.NoError(t, err)
	_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?, dirty = 0", target-1)
	require.NoError(t, err)

	err = RunMigrations(sqlDB)
	require.Error(t, err, "sabotaged migration must fail")

	// RunMigrations' closeMigrator closes sqlDB (migrate.go's note: Close()'s
	// database half closes the *sql.DB), so query on a fresh connection.
	checkDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer checkDB.Close()

	// The failed migration's DDL rolled back: the sabotaged table is still the
	// previous-version shape, not half-recreated.
	var ddl string
	require.NoError(t, checkDB.QueryRow("SELECT sql FROM sqlite_master WHERE name = ?", table).Scan(&ddl))
	assert.Contains(t, ddl, "sabotage INTEGER", "the failed migration must have rolled back, not partially applied")

	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, target, version, "the failed migration is left dirty at its own version")
	assert.True(t, dirty, "a failed migration must be the fail-closed dirty signal")
}
