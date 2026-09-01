package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pre-migration backup (issue #530) is the load-bearing half of the
// rollback policy: downgrade is unsupported, so the server (InitDB) and
// `make migrate-up` (MigrateUp) take a verified SQLite snapshot before applying
// any pending migration and REFUSE to migrate if they cannot. These tests run
// against the real migrated schema (CLAUDE.md trap 1), not AutoMigrate.

func mustLatestVersion(t *testing.T) uint {
	t.Helper()
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	return latest
}

// seedUser inserts one row into users, which has existed since long before the
// upgrade floor — enough to prove a snapshot was taken before migrations ran
// and holds the pre-upgrade data.
func seedUser(t *testing.T, dbPath, username string) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	_, err = sqlDB.Exec(
		`INSERT INTO users (username, email, password, created_at, updated_at)
		 VALUES (?, ?, 'x', datetime('now'), datetime('now'))`,
		username, username+"@example.com",
	)
	require.NoError(t, err)
}

// floorDBWithUser builds a clean database at the supported upgrade floor
// (v0.6.0) holding one user — the state a real instance is in the moment before
// an in-place upgrade.
func floorDBWithUser(t *testing.T, dbPath string) {
	t.Helper()
	require.NoError(t, MigrateUpTo(dbPath, SupportedUpgradeFloorVersion))
	seedUser(t, dbPath, "premig")
}

func preMigrationSnapshots(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "pre-migration", "*.db"))
	require.NoError(t, err)
	return m
}

func TestInitDB_TakesVerifiedPreMigrationBackup(t *testing.T) {
	latest := mustLatestVersion(t)
	require.Greater(t, latest, SupportedUpgradeFloorVersion, "need at least one migration above the floor")

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	db, err := InitDB(dbPath)
	require.NoError(t, err)
	if sqlDB, e := db.DB(); e == nil {
		defer sqlDB.Close()
	}

	// Exactly one snapshot, named for the from->to hop.
	snaps := preMigrationSnapshots(t, dir)
	require.Len(t, snaps, 1)
	assert.Contains(t, filepath.Base(snaps[0]),
		fmt.Sprintf("live-pre-migration-%d-to-%d-", SupportedUpgradeFloorVersion, latest))

	// It is a valid database.
	res, err := IntegrityCheck(snaps[0])
	require.NoError(t, err)
	assert.Equal(t, "ok", res)

	// It was taken BEFORE migrations: floor schema, and the pre-upgrade user.
	snap, err := sql.Open("sqlite", openDSN(snaps[0]))
	require.NoError(t, err)
	defer snap.Close()
	var snapVersion uint
	require.NoError(t, snap.QueryRow("SELECT version FROM schema_migrations").Scan(&snapVersion))
	assert.EqualValues(t, SupportedUpgradeFloorVersion, snapVersion, "the snapshot is the pre-migration schema")
	var users int
	require.NoError(t, snap.QueryRow("SELECT COUNT(*) FROM users").Scan(&users))
	assert.Equal(t, 1, users, "the snapshot holds the pre-upgrade data")

	// The live database advanced to latest, clean.
	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, latest, version)
	assert.False(t, dirty)
}

func TestMigrateUp_TakesPreMigrationBackup(t *testing.T) {
	latest := mustLatestVersion(t)
	require.Greater(t, latest, SupportedUpgradeFloorVersion)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	require.NoError(t, MigrateUp(dbPath))

	assert.Len(t, preMigrationSnapshots(t, dir), 1, "`make migrate-up` is fail-closed on the backup exactly like startup")
	version, _, _, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	assert.EqualValues(t, latest, version)
}

func TestInitDB_FailsClosedWhenBackupTargetUnwritable(t *testing.T) {
	require.Greater(t, mustLatestVersion(t), SupportedUpgradeFloorVersion)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	// A plain file where a directory component is expected: os.MkdirAll fails
	// with ENOTDIR regardless of uid (a 0o500 dir would not stop root).
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	t.Setenv(preMigrationBackupDirEnvVar, filepath.Join(blocker, "pre-migration"))

	_, err := InitDB(dbPath)
	require.Error(t, err)
	var backupErr *ErrPreMigrationBackupFailed
	require.ErrorAs(t, err, &backupErr, "an unwritable backup target must be a typed, assertable refusal")
	assert.Contains(t, backupErr.Error(), preMigrationBackupDirEnvVar, "the refusal names the knob that moves the target")
	assert.Error(t, errors.Unwrap(backupErr), "the refusal wraps the underlying filesystem cause")

	// The database is untouched: still at the floor, still clean, latest not applied.
	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, SupportedUpgradeFloorVersion, version, "a failed pre-migration backup must migrate nothing")
	assert.False(t, dirty)
}

func TestInitDB_FreshDatabaseTakesNoBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fresh.db")

	db, err := InitDB(dbPath)
	require.NoError(t, err)
	if sqlDB, e := db.DB(); e == nil {
		defer sqlDB.Close()
	}

	_, statErr := os.Stat(filepath.Join(dir, "pre-migration"))
	assert.True(t, os.IsNotExist(statErr), "a clean install has no prior version to roll back to")
}

func TestInitDB_UpToDateDatabaseTakesNoNewBackup(t *testing.T) {
	require.Greater(t, mustLatestVersion(t), SupportedUpgradeFloorVersion)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	db1, err := InitDB(dbPath)
	require.NoError(t, err)
	if s, e := db1.DB(); e == nil {
		s.Close()
	}
	first := preMigrationSnapshots(t, dir)
	require.Len(t, first, 1)

	db2, err := InitDB(dbPath)
	require.NoError(t, err)
	if s, e := db2.DB(); e == nil {
		s.Close()
	}
	assert.Equal(t, first, preMigrationSnapshots(t, dir), "no pending migrations => no new snapshot")
}

func TestInitDB_DirtyDatabaseIsRefusedNotBackedUp(t *testing.T) {
	latest := mustLatestVersion(t)
	require.Greater(t, latest, SupportedUpgradeFloorVersion+1, "need a migration beyond the interrupted pair")

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	interruptedDB(t, dbPath, SupportedUpgradeFloorVersion+1)

	_, err := InitDB(dbPath)
	require.Error(t, err)
	var dirtyErr *ErrDirtyMigration
	require.ErrorAs(t, err, &dirtyErr)

	_, statErr := os.Stat(filepath.Join(dir, "pre-migration"))
	assert.True(t, os.IsNotExist(statErr), "a dirty database is refused before the backup step, not snapshotted")
}

func TestInitDB_SubFloorWithoutBridgeIsRefusedNotBackedUp(t *testing.T) {
	if SupportedUpgradeFloorVersion < 2 {
		t.Skip("no sub-floor version to build")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	require.NoError(t, MigrateUpTo(dbPath, SupportedUpgradeFloorVersion-1))

	_, err := InitDB(dbPath)
	require.Error(t, err)
	var subFloorErr *ErrSubFloorMigration
	require.ErrorAs(t, err, &subFloorErr)

	_, statErr := os.Stat(filepath.Join(dir, "pre-migration"))
	assert.True(t, os.IsNotExist(statErr), "a sub-floor database is refused before the backup step")
}

func TestInitDB_SubFloorBridgeStillSnapshots(t *testing.T) {
	if SupportedUpgradeFloorVersion < 2 {
		t.Skip("no sub-floor version to build")
	}
	t.Setenv(subFloorMigrationEnvVar, "1")

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	require.NoError(t, MigrateUpTo(dbPath, SupportedUpgradeFloorVersion-1))

	db, err := InitDB(dbPath)
	require.NoError(t, err)
	if s, e := db.DB(); e == nil {
		s.Close()
	}
	assert.Len(t, preMigrationSnapshots(t, dir), 1,
		"the one-time sub-floor bridge migrates real data, so it snapshots first")
}

func TestTakePreMigrationBackup_IdempotentPerHop(t *testing.T) {
	latest := mustLatestVersion(t)
	require.Greater(t, latest, SupportedUpgradeFloorVersion)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	first, err := takePreMigrationBackup(dbPath, SupportedUpgradeFloorVersion, latest)
	require.NoError(t, err)

	// A crash-loop retry: the same hop, snapshot already present. It must
	// reuse — not fail on the no-overwrite guard, not pile up a second copy.
	second, err := takePreMigrationBackup(dbPath, SupportedUpgradeFloorVersion, latest)
	require.NoError(t, err)
	assert.Equal(t, first, second, "the retry reuses the existing verified snapshot")
	assert.Len(t, preMigrationSnapshots(t, dir), 1, "no duplicate snapshot for the same hop")
}

func TestTakePreMigrationBackup_IgnoresCorruptLeftoverSnapshot(t *testing.T) {
	latest := mustLatestVersion(t)
	require.Greater(t, latest, SupportedUpgradeFloorVersion)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	// A leftover file that matches the hop's glob but is not a valid database
	// (a snapshot torn by an earlier crash): it must be skipped, not reused.
	junkDir := filepath.Join(dir, "pre-migration")
	require.NoError(t, os.MkdirAll(junkDir, 0o750))
	junk := filepath.Join(junkDir, fmt.Sprintf("live-pre-migration-%d-to-%d-00000000-000000.db",
		SupportedUpgradeFloorVersion, latest))
	require.NoError(t, os.WriteFile(junk, []byte("not a sqlite file"), 0o600))

	got, err := takePreMigrationBackup(dbPath, SupportedUpgradeFloorVersion, latest)
	require.NoError(t, err)
	assert.NotEqual(t, junk, got, "a leftover that fails integrity_check is not a rollback point")
	res, err := IntegrityCheck(got)
	require.NoError(t, err)
	assert.Equal(t, "ok", res)
}

func TestTakePreMigrationBackup_RecordsSystemEventWhenTableExists(t *testing.T) {
	latest := mustLatestVersion(t)
	// The pre-migration schema must already carry system_events (migration
	// 000038) for the operational-timeline row to land.
	from := latest - 1
	require.Greater(t, from, uint(38), "need a pre-migration schema with system_events")

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	require.NoError(t, MigrateUpTo(dbPath, from))
	seedUser(t, dbPath, "eventcheck")

	_, err := takePreMigrationBackup(dbPath, from, latest)
	require.NoError(t, err)

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	var n int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT COUNT(*) FROM system_events
		 WHERE event_type = 'backup_completed' AND operation = 'pre_migration_backup' AND component = 'migration'`,
	).Scan(&n))
	assert.Equal(t, 1, n, "the snapshot is recorded on the operational timeline")
}

func TestMigrateFileWithPreBackup_PropagatesVersionReadError(t *testing.T) {
	// A path whose directory component is a plain file: the migration-version
	// read fails before any backup or migration is attempted, and the failure
	// is surfaced as-is rather than mistaken for the backup refusal.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	_, err := InitDB(filepath.Join(blocker, "nested", "live.db"))
	require.Error(t, err)
	var backupErr *ErrPreMigrationBackupFailed
	assert.False(t, errors.As(err, &backupErr), "a version-read failure is not ErrPreMigrationBackupFailed")
	_, statErr := os.Stat(filepath.Join(dir, "pre-migration"))
	assert.True(t, os.IsNotExist(statErr))
}
