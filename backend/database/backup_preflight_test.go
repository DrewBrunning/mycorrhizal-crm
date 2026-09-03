package database

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/diskspace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #498: BackupSnapshot preflights the output filesystem's free space
// before it starts writing a full second copy of the database, so a too-full
// disk is a clear refusal with the source untouched rather than an ENOSPC
// mid-VACUUM. These run against the real migrated schema (CLAUDE.md trap 1).

// migratedDBWithUser builds a small real database at the latest schema.
func migratedDBWithUser(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	require.NoError(t, MigrateUpTo(dbPath, latest))
	seedUser(t, dbPath, "backup")
	return dbPath
}

func TestBackupSnapshot_RefusesWhenDiskTooFull(t *testing.T) {
	src := migratedDBWithUser(t)
	out := filepath.Join(t.TempDir(), "snapshot.db")

	restore := diskspace.StubForTest(1 << 20) // 1 MiB free — below the estimate
	t.Cleanup(restore)

	err := BackupSnapshot(src, out)
	require.Error(t, err, "a backup onto a too-full disk must be refused")

	var ins *diskspace.ErrInsufficientSpace
	require.ErrorAs(t, err, &ins, "the refusal is the typed, assertable preflight error")
	assert.Contains(t, err.Error(), "backup preflight")

	_, statErr := os.Stat(out)
	assert.True(t, os.IsNotExist(statErr), "no snapshot file may appear when the preflight refuses")

	// The source is completely untouched.
	got, icErr := IntegrityCheck(src)
	require.NoError(t, icErr)
	assert.Equal(t, "ok", got, "the refused backup must not have touched the source")
}

func TestBackupSnapshot_ProceedsWhenDiskHasRoom(t *testing.T) {
	src := migratedDBWithUser(t)
	out := filepath.Join(t.TempDir(), "snapshot.db")

	restore := diskspace.StubForTest(10 << 30) // 10 GiB free
	t.Cleanup(restore)

	require.NoError(t, BackupSnapshot(src, out))
	got, err := IntegrityCheck(out)
	require.NoError(t, err)
	assert.Equal(t, "ok", got)
}

func TestBackupSnapshot_DoesNotBlockWhenStatfsUnreadable(t *testing.T) {
	src := migratedDBWithUser(t)
	out := filepath.Join(t.TempDir(), "snapshot.db")

	restore := diskspace.StubErrorForTest(errors.New("statfs unavailable"))
	t.Cleanup(restore)

	// A broken statfs must not be the thing that fails a backup — the
	// temp-then-link path is still the fail-closed backstop.
	require.NoError(t, BackupSnapshot(src, out), "a preflight that cannot read statfs falls through to the real operation")
}

func TestInitDB_PreMigrationBackupFailsClosedOnFullDisk(t *testing.T) {
	require.Greater(t, mustLatestVersion(t), SupportedUpgradeFloorVersion)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	restore := diskspace.StubForTest(1 << 20) // 1 MiB free
	t.Cleanup(restore)

	_, err := InitDB(dbPath)
	require.Error(t, err, "an upgrade on a too-full disk must refuse rather than migrate unbacked")

	var backupErr *ErrPreMigrationBackupFailed
	require.ErrorAs(t, err, &backupErr, "the refusal is the typed pre-migration-backup failure")
	var ins *diskspace.ErrInsufficientSpace
	require.ErrorAs(t, err, &ins, "and it carries the disk-space cause")

	// The database is untouched: still at the floor, still clean.
	version, dirty, ok, verErr := MigrationVersion(dbPath)
	require.NoError(t, verErr)
	require.True(t, ok)
	assert.EqualValues(t, SupportedUpgradeFloorVersion, version, "a refused pre-migration backup migrates nothing")
	assert.False(t, dirty)
}

func TestBackupSpaceEstimate(t *testing.T) {
	// Missing source → 0 (the preflight becomes a no-op rather than a guess).
	assert.Zero(t, backupSpaceEstimate(filepath.Join(t.TempDir(), "nope.db")))

	src := migratedDBWithUser(t)
	fi, err := os.Stat(src)
	require.NoError(t, err)
	est := backupSpaceEstimate(src)
	assert.Greater(t, est, uint64(fi.Size()), "the estimate exceeds the raw file size (margin added)")
	assert.GreaterOrEqual(t, est, uint64(fi.Size())+16<<20, "the margin is at least 16 MiB")
}
