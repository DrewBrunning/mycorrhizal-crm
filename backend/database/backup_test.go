package database_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveTestDB opens a real migrated database (never AutoMigrate — see
// CLAUDE.md trap 1) and seeds it exactly the way a running server would have,
// with the final write deliberately left uncheckpointed.
func liveTestDB(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, name)

	db, err := database.InitDB(dbPath)
	require.NoError(t, err, "migrations must apply before backup")

	user := models.User{Username: "backupuser", Password: "hunter2ish", Email: "backup@example.com"}
	require.NoError(t, db.Create(&user).Error)

	older := models.Note{UserID: user.ID, Content: "older note that predates the backup", Date: time.Now().Add(-time.Hour)}
	require.NoError(t, db.Create(&older).Error)

	act := models.Activity{UserID: user.ID, Title: "backup activity", Date: time.Now()}
	require.NoError(t, db.Create(&act).Error)

	// The most recent write: committed to the WAL, but nothing has checkpointed
	// since. A plain file copy of the .db would miss it; the documented
	// procedure must not.
	recent := models.Note{UserID: user.ID, Content: "recent note still only in the WAL", Date: time.Now()}
	require.NoError(t, db.Create(&recent).Error)

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	})
	return dbPath
}

// assertSeededData verifies data survived into a freshly-opened database at
// dbPath — InitDB runs migrations too, so a snapshot that is not a valid
// database fails here as loudly as it would on a real restore.
func assertSeededData(t *testing.T, dbPath string) {
	t.Helper()
	db, err := database.InitDB(dbPath)
	require.NoError(t, err, "database must open and migrate after restore")
	defer func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}()

	var notes int64
	require.NoError(t, db.Model(&models.Note{}).Count(&notes).Error)
	assert.Equal(t, int64(2), notes, "both notes (older + recent WAL-only) must survive")

	var activities int64
	require.NoError(t, db.Model(&models.Activity{}).Count(&activities).Error)
	assert.Equal(t, int64(1), activities, "the activity must survive")

	var users int64
	require.NoError(t, db.Model(&models.User{}).Count(&users).Error)
	assert.Equal(t, int64(1), users, "the user must survive")
}

// TestBackupSnapshotOnlineCapturesRecentWrites is the core of N6's "the
// procedure is actually tested": a live WAL database (recent writes still
// sitting in the -wal file, the server connection still open) is snapshotted,
// and every row — including the write that a naive `cp` would have lost —
// must be in the snapshot. It also proves the snapshot is standalone: it is
// copied to a fresh path and opened there, so no hidden -wal/-shm sidecar is
// doing any of the work.
func TestBackupSnapshotOnlineCapturesRecentWrites(t *testing.T) {
	t.Parallel()
	srcPath := liveTestDB(t, "live.db")
	dir := filepath.Dir(srcPath)
	backupPath := filepath.Join(dir, "backup.db")

	require.NoError(t, database.BackupSnapshot(srcPath, backupPath), "online backup of a live WAL database must succeed")

	// The backup is self-contained: copy only the single file, then open the
	// copy — if the snapshot depended on -wal/-shm sidecars this would fail.
	standalone := filepath.Join(dir, "restored.db")
	data, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(standalone, data, 0o644))

	assertSeededData(t, standalone)
}

// TestBackupSnapshotDoesNotDisturbSource: backing up must not corrupt or lock
// out the live database — the source still opens, still has every row, and
// still accepts writes afterwards.
func TestBackupSnapshotDoesNotDisturbSource(t *testing.T) {
	t.Parallel()
	srcPath := liveTestDB(t, "live.db")
	backupPath := filepath.Join(filepath.Dir(srcPath), "backup.db")

	require.NoError(t, database.BackupSnapshot(srcPath, backupPath))

	// Source still opens and still has its data after the snapshot.
	assertSeededData(t, srcPath)

	// And it still accepts writes — the source is the live DB, not a frozen copy.
	db, err := database.InitDB(srcPath)
	require.NoError(t, err)
	defer func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}()
	user := models.User{Username: "postbackup", Password: "hunter2ish", Email: "post@example.com"}
	require.NoError(t, db.Create(&user).Error)

	var count int64
	require.NoError(t, db.Model(&models.User{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

// TestBackupSnapshotRestoreRoundTrip is the full destroy-and-restore cycle the
// ticket demands: back up a populated instance, delete the database (and its
// WAL sidecars) exactly as a data-loss incident would, put the snapshot back
// in its place, and confirm every row is present.
func TestBackupSnapshotRestoreRoundTrip(t *testing.T) {
	t.Parallel()
	srcPath := liveTestDB(t, "live.db")
	backupPath := filepath.Join(filepath.Dir(srcPath), "backup.db")

	require.NoError(t, database.BackupSnapshot(srcPath, backupPath))

	// "Destroy the database": drop the .db and its WAL/SHM sidecars.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		require.NoError(t, os.Remove(srcPath+suffix))
	}

	// Restore: replace the .db from the snapshot.
	restored, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(srcPath, restored, 0o644))

	assertSeededData(t, srcPath)
}

// TestBackupSnapshotRefusesToOverwrite: a backup must never silently clobber
// an existing file — the operator is expected to pick a fresh path (or let the
// timestamped default do it). The pre-existing file must be left untouched.
func TestBackupSnapshotRefusesToOverwrite(t *testing.T) {
	t.Parallel()
	srcPath := liveTestDB(t, "live.db")
	backupPath := filepath.Join(filepath.Dir(srcPath), "existing.db")

	require.NoError(t, os.WriteFile(backupPath, []byte("precious data"), 0o644))

	err := database.BackupSnapshot(srcPath, backupPath)
	require.Error(t, err, "backup must refuse to overwrite an existing output")
	assert.Contains(t, err.Error(), "refusing to overwrite")

	kept, readErr := os.ReadFile(backupPath)
	require.NoError(t, readErr)
	assert.Equal(t, "precious data", string(kept), "the pre-existing file must be untouched")
}

// TestBackupSnapshotMissingSourceErrors: a typo'd SQLITE_DB_PATH must fail
// loudly, not silently produce an empty or partial backup.
func TestBackupSnapshotMissingSourceErrors(t *testing.T) {
	t.Parallel()
	err := database.BackupSnapshot(filepath.Join(t.TempDir(), "does-not-exist.db"), filepath.Join(t.TempDir(), "backup.db"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestDefaultBackupPath pins the timestamped-sibling naming the CLI falls back
// to when neither an argument nor BACKUP_PATH is set: the snapshot lands next
// to the source (inside the same Docker bind mount) and keeps the source's
// basename with a timestamp suffix.
func TestDefaultBackupPath(t *testing.T) {
	t.Parallel()
	p := database.DefaultBackupPath("/srv/data/mycorrhizal.db")
	assert.Equal(t, "/srv/data", filepath.Dir(p))
	assert.Equal(t, ".db", filepath.Ext(p))
	base := filepath.Base(p)
	assert.Regexp(t, `^mycorrhizal-\d{8}-\d{6}\.db$`, base, "default name must be <stem>-<ts>.db")

	// Relative sources (the CLI's default and the Docker host's ./data path)
	// must keep their directory and stem, only gaining the timestamp.
	rel := database.DefaultBackupPath("mycorrhizal.db")
	assert.Equal(t, ".", filepath.Dir(rel))
	assert.Regexp(t, `^mycorrhizal-\d{8}-\d{6}\.db$`, filepath.Base(rel))
	relDir := database.DefaultBackupPath("./data/mycorrhizal.db")
	assert.Equal(t, "data", filepath.Dir(relDir))
	assert.Regexp(t, `^mycorrhizal-\d{8}-\d{6}\.db$`, filepath.Base(relDir))
}

// TestBackupSnapshotFailureLeavesNoLitter: a failed backup must not leave a
// partial output file (or temp litter) behind, and must not block a later
// retry. The failure is forced deterministically by pointing the source at a
// file that is not a database: the pre-checks pass, then the checkpoint/VACUUM
// step fails, which is the moment the temp file may or may not have been
// created — so the no-litter contract is what is being pinned.
func TestBackupSnapshotFailureLeavesNoLitter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "not-a-db") // exists, but is not a database
	require.NoError(t, os.WriteFile(srcPath, []byte("this is not a sqlite database"), 0o644))
	outPath := filepath.Join(dir, "backup.db")

	err := database.BackupSnapshot(srcPath, outPath)
	require.Error(t, err, "backing up a non-database must fail")

	// Nothing may be left at the target path or anywhere else in the dir.
	assert.NoFileExists(t, outPath, "a failed backup must not leave an output file")
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp-", "no temp litter may remain after a failed backup")
	}

	// A subsequent real backup must not be blocked by the failed attempt.
	liveDB := liveTestDB(t, "live.db")
	require.NoError(t, database.BackupSnapshot(liveDB, outPath), "a retry after failure must succeed")
	require.FileExists(t, outPath)
}

// TestMakeBackupTarget exercises the real `make backup` Makefile target, not
// just the library call: it must pick up SQLITE_DB_PATH/BACKUP_PATH from the
// environment (the same plumbing the migrate targets use) and produce a
// restorable snapshot.
func TestMakeBackupTarget(t *testing.T) {
	t.Parallel()
	makeBin, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make not available; cannot exercise the Makefile target")
	}

	srcPath := liveTestDB(t, "live.db")
	dir := filepath.Dir(srcPath)
	backupPath := filepath.Join(dir, "make-backup.db")

	// Locate the backend directory from this test file's own path
	// (backend/database/backup_test.go -> backend/), so the test does not
	// depend on the process working directory.
	_, file, _, _ := runtime.Caller(0)
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))

	cmd := exec.Command(makeBin, "backup")
	cmd.Dir = backendDir
	cmd.Env = append(os.Environ(),
		"SQLITE_DB_PATH="+srcPath,
		"BACKUP_PATH="+backupPath,
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "make backup must succeed; output:\n%s", output)

	require.FileExists(t, backupPath, "make backup must write the snapshot to BACKUP_PATH")

	// The produced snapshot restores cleanly — copy it over a wiped database
	// and confirm the seeded rows are all there.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		require.NoError(t, os.Remove(srcPath+suffix))
	}
	restored, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(srcPath, restored, 0o644))
	assertSeededData(t, srcPath)
}
