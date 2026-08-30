package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/faults"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrityCheck opens a fresh connection to dbPath and reports the PRAGMA
// integrity_check result. Every injection test ends with this: "no injected
// fault leaves the database in a state PRAGMA integrity_check flags" is a
// TEST-06 (issue #434) acceptance criterion.
func integrityCheck(t *testing.T, dbPath string) string {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	var result string
	require.NoError(t, sqlDB.QueryRow("PRAGMA integrity_check").Scan(&result))
	return result
}

// dirtyState reads the schema_migrations version/dirty pair straight off disk.
func dirtyState(t *testing.T, dbPath string) (version uint, dirty bool) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	var v int
	require.NoError(t, sqlDB.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&v, &dirty))
	return uint(v), dirty
}

// tableExists reports whether a table with the given name exists in the
// database, read straight off disk.
func tableExists(t *testing.T, dbPath, name string) bool {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	var n int
	require.NoError(t, sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n))
	return n > 0
}

// TestInjectedMigrationFaultFailsClosedAndRecovers is the flagship TEST-06
// (issue #434) injection test. An armed error fault at the migration seam
// fails the very first run right after the baseline migration commits but
// before it is marked clean — the crash signature of a process killed between
// a migration's commit and its clean-mark — and asserts the defined outcome at
// every stage:
//
//   - the failure is a returned error naming the failing version and file
//     (the v0.6.2 diagnostic gate, issue #532, must not be bypassed);
//   - the failed migration's schema actually landed (the fault is
//     post-commit), so recovery has a consistent base to continue from;
//   - the database is left dirty at that version (a real crash leaves the
//     same mark);
//   - PRAGMA integrity_check is ok both immediately after the fault and after
//     recovery (no torn or partial state);
//   - with the fault disarmed, the next startup run REFUSES on the dirty flag
//     (MIG-04, issue #439 / #546 fail-closed) with a typed ErrDirtyMigration —
//     never a warning followed by a forced boot;
//   - the operator-only recovery (`database.MigrateForce`, what the migrate
//     CLI's prompted `force` command calls) recovers deterministically to the
//     latest schema, not dirty.
//
// Hand-verify (CLAUDE.md): delete the dirty refusal in checkMigrationPreflight
// and this test fails on the second MigrateUp (it would recover instead of
// refusing), then restores. The fail-closed refusal is what this test pins.
func TestInjectedMigrationFaultFailsClosedAndRecovers(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	dbPath := filepath.Join(t.TempDir(), "inject.db")

	faults.ArmError(faultMigrationStatement, &faults.ErrInjected{Name: faultMigrationStatement})
	defer faults.Disarm(faultMigrationStatement)

	buf := captureMigrationLogger(t)
	err := MigrateUp(dbPath)
	require.Error(t, err, "an armed migration fault must fail the run")
	assert.Contains(t, err.Error(), "1", "returned error must name the failing migration version")
	assert.Contains(t, err.Error(), migrationFileForVersion(1), "returned error must name the failing migration file")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines, "a structured migration_failed log line must be emitted for an injected fault")
	var line map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &line))
	assert.Equal(t, "migration_failed", line["event"])

	version, dirty := dirtyState(t, dbPath)
	assert.Equal(t, uint(1), version, "the fault leaves the DB dirty at the just-committed migration")
	assert.True(t, dirty, "the fault must leave the database dirty (the crash signature)")
	assert.True(t, tableExists(t, dbPath, "users"), "the post-commit fault leaves the migration's schema applied")
	assert.Equal(t, "ok", integrityCheck(t, dbPath), "a failed migration must not corrupt the database")

	// Fail-closed: with the fault disarmed the next startup run must REFUSE on
	// the dirty flag and name the recovery — not force the version and boot.
	// This is the MIG-04 (issue #439 / #546) posture the injection pins.
	faults.Disarm(faultMigrationStatement)
	err = MigrateUp(dbPath)
	require.Error(t, err, "a dirty database must refuse to migrate on the next run")
	var dirtyErr *ErrDirtyMigration
	require.ErrorAs(t, err, &dirtyErr, "the refusal must be a typed ErrDirtyMigration, not a generic failure")
	assert.EqualValues(t, 1, dirtyErr.Version, "the refusal must name the dirty version")
	assert.Contains(t, err.Error(), "Restore the pre-migration backup", "the refusal must state the recovery path (restore-from-backup)")
	version, dirty = dirtyState(t, dbPath)
	assert.True(t, dirty, "the refused database must still be dirty — a refusal must not alter it")

	// Operator-only recovery: MigrateForce is what the migrate CLI's prompted
	// `force` command calls. It asserts the dirty version, clears the flag, and
	// re-runs pending migrations deterministically.
	require.NoError(t, MigrateForce(dbPath), "the operator-only force must recover the dirty database")

	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	version, dirty = dirtyState(t, dbPath)
	assert.Equal(t, latest, version, "recovery must land on the latest schema")
	assert.False(t, dirty, "recovery must clear the dirty flag")
	assert.Equal(t, "ok", integrityCheck(t, dbPath))
}

// TestInjectedMigrationFaultMidFlightRecordsEvent covers the same seam against
// a database that is mostly migrated, so the injected failure lands on a late
// migration (past 000038) where system_events exists — asserting the
// migration_failed event row is persisted, and that the fail-closed refusal
// (dirty -> refuse) plus the operator-only force still recover cleanly.
func TestInjectedMigrationFaultMidFlightRecordsEvent(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	dbPath := filepath.Join(t.TempDir(), "inject-late.db")
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	require.Greater(t, latest, uint(1), "this test needs a migration beyond the squashed baseline")

	// Migrate everything except the last migration, then let the injected
	// fault be the failure — no table surgery required.
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(int(latest)-1))
	closeMigrator(m)

	faults.ArmError(faultMigrationStatement, errors.New("injected mid-flight failure"))
	defer faults.Disarm(faultMigrationStatement)

	err = MigrateUp(dbPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", latest), "must name the last migration")
	assert.Contains(t, err.Error(), migrationFileForVersion(latest))

	version, dirty := dirtyState(t, dbPath)
	assert.Equal(t, latest, version)
	assert.True(t, dirty)

	// system_events exists from 000038, so the diagnostic row lands.
	checkDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer checkDB.Close()
	var count int
	require.NoError(t, checkDB.QueryRow("SELECT COUNT(*) FROM system_events WHERE event_type = 'migration_failed'").Scan(&count))
	assert.Equal(t, 1, count, "an injected migration failure must be recorded as a system event")
	assert.Equal(t, "ok", integrityCheck(t, dbPath))

	// Fail-closed: the next startup run refuses on the dirty flag, and the
	// operator-only force recovers deterministically.
	faults.Disarm(faultMigrationStatement)
	err = MigrateUp(dbPath)
	require.Error(t, err, "a dirty database must refuse to migrate on the next run")
	var dirtyErr *ErrDirtyMigration
	require.ErrorAs(t, err, &dirtyErr)
	assert.EqualValues(t, latest, dirtyErr.Version)

	require.NoError(t, MigrateForce(dbPath), "the operator-only force must recover the dirty database")
	_, dirty = dirtyState(t, dbPath)
	assert.False(t, dirty)
	assert.Equal(t, "ok", integrityCheck(t, dbPath))
}

// TestInjectedMigrationPauseBlocksThenCompletes pins the pause seam in-process
// (the external-fault CI job uses the same seam with SIGKILL): a pause fault
// makes Run block for its duration, log its marker, and then return nil — the
// migration still completes and the database stays clean.
func TestInjectedMigrationPauseBlocksThenCompletes(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	dbPath := filepath.Join(t.TempDir(), "inject-pause.db")
	faults.ArmPause(faultMigrationStatement, 25*time.Millisecond)

	start := time.Now()
	require.NoError(t, MigrateUp(dbPath), "a pause fault delays but must not fail the migration")
	assert.GreaterOrEqual(t, time.Since(start), 25*time.Millisecond, "the pause must actually block inside Run")

	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	version, dirty := dirtyState(t, dbPath)
	assert.Equal(t, latest, version)
	assert.False(t, dirty)
	assert.Equal(t, "ok", integrityCheck(t, dbPath))
}
