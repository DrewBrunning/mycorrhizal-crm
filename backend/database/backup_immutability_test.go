package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests are the executable half of issue #505 (backup immutability /
// ransomware resistance). The invariant, stated in docs/security/asvs-l2.md P5
// and docs/deployment.md → "Backup immutability & ransomware resistance":
//
//	The application's own code and credentials cannot delete, overwrite,
//	truncate, re-encrypt, rotate, or expire an existing backup. The only
//	backup operation the app has is write-a-new-file.
//
// Genuine immutability against a compromised host is an operator-side property
// (off-host pull, or object-locked remote storage) — the app running as its
// own uid can always rm its own local files, and no software control changes
// that for a single-process self-hosted app. What is enforced *here* is the
// write-new-only half: prove, by trying, that nothing the app does touches an
// existing backup, and guard against a future in-app expirer creeping in.

// immutBackupTestDB returns a freshly migrated database holding one row. A
// brand-new path takes no pre-migration snapshot (fresh install), so the
// backup directory used by these tests stays uncluttered.
func immutBackupTestDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mycorrhizal.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	if sqlDB, e := db.DB(); e == nil {
		require.NoError(t, sqlDB.Close())
	}
	seedUser(t, dbPath, "immutable")
	return dbPath
}

// immutDigest returns the SHA-256 of a file's contents and its permission
// bits, so a test can assert a backup is byte-for-byte and mode-for-mode
// unchanged after some other operation ran.
func immutDigest(t *testing.T, path string) (digest string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	fi, err := os.Stat(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), fi.Mode().Perm()
}

// immutAssertRestores copies a snapshot to a scratch path, opens it (which
// runs migrations — a non-database would fail here), and confirms the seeded
// row is present: the ticket's "a restore from an untouched backup succeeds
// and produces a valid instance" check.
func immutAssertRestores(t *testing.T, snapshot string) {
	t.Helper()
	restored := filepath.Join(t.TempDir(), "restored.db")
	data, err := os.ReadFile(snapshot)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(restored, data, 0o600))

	db, err := InitDB(restored)
	require.NoError(t, err, "an untouched snapshot must still restore to a valid instance")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()

	var users int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&users))
	assert.GreaterOrEqual(t, users, 1, "the restored instance still holds its data")
}

// TestBackupImmutability_NewSnapshotLeavesPriorBackupsByteIdentical: taking new
// snapshots into a directory that already holds backups must not touch any of
// them — not the bytes, not the mode. This is the "overwrite / encrypt in
// place" attempt, run through the only primitive the app has.
func TestBackupImmutability_NewSnapshotLeavesPriorBackupsByteIdentical(t *testing.T) {
	t.Parallel()
	src := immutBackupTestDB(t)
	backupDir := t.TempDir()

	prior1 := filepath.Join(backupDir, "mycorrhizal-20260101-000000.db")
	prior2 := filepath.Join(backupDir, "mycorrhizal-20260102-000000.db")
	require.NoError(t, BackupSnapshot(src, prior1))
	require.NoError(t, BackupSnapshot(src, prior2))

	digest1, mode1 := immutDigest(t, prior1)
	digest2, mode2 := immutDigest(t, prior2)

	// Several more snapshots land in the same directory over time.
	for i := 3; i <= 6; i++ {
		fresh := filepath.Join(backupDir, fmt.Sprintf("mycorrhizal-2026010%d-000000.db", i))
		require.NoError(t, BackupSnapshot(src, fresh))
	}

	after1, afterMode1 := immutDigest(t, prior1)
	after2, afterMode2 := immutDigest(t, prior2)
	assert.Equal(t, digest1, after1, "prior backup 1 must be byte-identical after later snapshots")
	assert.Equal(t, digest2, after2, "prior backup 2 must be byte-identical after later snapshots")
	assert.Equal(t, mode1, afterMode1, "prior backup 1 mode must be unchanged")
	assert.Equal(t, mode2, afterMode2, "prior backup 2 mode must be unchanged")

	require.NoError(t, verifyBackup(prior1), "the untouched prior backup still passes integrity_check")
	immutAssertRestores(t, prior1)
}

// TestBackupImmutability_RefusesToOverwriteAnExistingBackup: pointing a new
// snapshot straight at an existing backup file is refused, and the target is
// left exactly as it was — the "overwrite / truncate" attempt, aimed directly.
func TestBackupImmutability_RefusesToOverwriteAnExistingBackup(t *testing.T) {
	t.Parallel()
	src := immutBackupTestDB(t)
	existing := filepath.Join(t.TempDir(), "mycorrhizal-20260101-000000.db")
	require.NoError(t, BackupSnapshot(src, existing))

	digest, mode := immutDigest(t, existing)

	err := BackupSnapshot(src, existing)
	require.Error(t, err, "a snapshot must never overwrite an existing backup")
	assert.Contains(t, err.Error(), "refusing to overwrite")

	afterDigest, afterMode := immutDigest(t, existing)
	assert.Equal(t, digest, afterDigest, "the existing backup must be byte-identical after a refused overwrite")
	assert.Equal(t, mode, afterMode, "the existing backup mode must be unchanged")
	immutAssertRestores(t, existing)
}

// TestBackupImmutability_FailedSnapshotTouchesNothingElse: a snapshot that
// fails after its pre-checks pass must delete only the temp it reserved — not
// a neighbouring backup, and not an unrelated file that merely *looks* like
// leftover backup temp (same prefix/suffix, different random middle). This is
// the "delete existing backups" attempt.
func TestBackupImmutability_FailedSnapshotTouchesNothingElse(t *testing.T) {
	t.Parallel()
	backupDir := t.TempDir()

	neighbour := filepath.Join(backupDir, "mycorrhizal-20260101-000000.db")
	require.NoError(t, BackupSnapshot(immutBackupTestDB(t), neighbour))
	neighbourDigest, neighbourMode := immutDigest(t, neighbour)

	// A file matching reserveTempPath's pattern (".<base>.tmp-*") for the
	// target below, but not the name this call will actually reserve.
	decoy := filepath.Join(backupDir, ".mycorrhizal-20260109-000000.db.tmp-DECOYDECOY")
	require.NoError(t, os.WriteFile(decoy, []byte("not the app's to delete"), 0o600))
	decoyDigest, decoyMode := immutDigest(t, decoy)

	// Force a deterministic failure: a source that exists (pre-checks pass) but
	// is not a database (checkpoint/VACUUM fails).
	notADB := filepath.Join(t.TempDir(), "not-a-db")
	require.NoError(t, os.WriteFile(notADB, []byte("this is not sqlite"), 0o600))
	require.Error(t, BackupSnapshot(notADB, filepath.Join(backupDir, "mycorrhizal-20260109-000000.db")))

	gotNeighbour, gotNeighbourMode := immutDigest(t, neighbour)
	assert.Equal(t, neighbourDigest, gotNeighbour, "a failed snapshot must not modify a neighbouring backup")
	assert.Equal(t, neighbourMode, gotNeighbourMode)

	require.FileExists(t, decoy, "a failed snapshot must not delete a file it did not create")
	gotDecoy, gotDecoyMode := immutDigest(t, decoy)
	assert.Equal(t, decoyDigest, gotDecoy, "the decoy temp-looking file must be untouched")
	assert.Equal(t, decoyMode, gotDecoyMode)
}

// TestBackupImmutability_PreMigrationRollbackPointSurvivesRoutineRotation
// pins issue #505 action 4 / issue #530 action 4: a routine backup-rotation
// sweep of the routine directory must not reach the automatic pre-migration
// rollback point. It lives in its own `pre-migration/` subdirectory precisely
// so a non-recursive sweep (the documented `find <dir> -maxdepth 1 -name
// 'mycorrhizal-*.db' -mtime +N -delete`) cannot see it — even though its
// filename does match that glob.
func TestBackupImmutability_PreMigrationRollbackPointSurvivesRoutineRotation(t *testing.T) {
	t.Parallel()
	require.Greater(t, mustLatestVersion(t), SupportedUpgradeFloorVersion, "need a pending migration to snapshot")

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mycorrhizal.db")
	floorDBWithUser(t, dbPath)

	// Startup migration writes the mandatory pre-migration snapshot (#530).
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	if sqlDB, e := db.DB(); e == nil {
		require.NoError(t, sqlDB.Close())
	}
	rollbackPoints := preMigrationSnapshots(t, dir)
	require.Len(t, rollbackPoints, 1, "exactly one pre-migration rollback point")
	rollbackPoint := rollbackPoints[0]

	// The operator also drops routine `make backup` snapshots beside the DB
	// (DefaultBackupPath's location) and lets them age.
	routine := filepath.Join(dir, "mycorrhizal-20250101-000000.db")
	require.NoError(t, BackupSnapshot(dbPath, routine))
	aged := time.Now().Add(-90 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(routine, aged, aged))

	// Model the documented rotation exactly: a NON-recursive glob of the
	// routine directory, deleting matches older than the retention window.
	matches, err := filepath.Glob(filepath.Join(dir, "mycorrhizal-*.db"))
	require.NoError(t, err)
	assert.NotContains(t, matches, rollbackPoint,
		"a non-recursive sweep of the routine directory must not even list the rollback point")

	swept := 0
	for _, m := range matches {
		fi, statErr := os.Stat(m)
		require.NoError(t, statErr)
		if time.Since(fi.ModTime()) > 30*24*time.Hour {
			require.NoError(t, os.Remove(m))
			swept++
		}
	}
	assert.Equal(t, 1, swept, "only the aged routine snapshot is swept")

	// The rollback point is untouched and still a valid database.
	require.FileExists(t, rollbackPoint)
	require.NoError(t, verifyBackup(rollbackPoint), "the surviving rollback point still passes integrity_check")

	// ...and it genuinely lives in its own directory, not as a sibling of the
	// routine snapshots.
	assert.Equal(t, filepath.Join(dir, "pre-migration"), preMigrationBackupDir(dbPath))
	rel, err := filepath.Rel(dir, rollbackPoint)
	require.NoError(t, err)
	assert.Equal(t, "pre-migration", filepath.Dir(rel), "the rollback point is one directory down from the routine store")
}

// TestBackupImmutability_NoBackupExpiryOrRotationCodeInTheApp is the guard
// against regression: walk every non-test .go file in the backend and fail if
// any declares a function whose name pairs a deletion verb with a backup noun
// (pruneBackups, rotateSnapshots, expireOldBackups, …). Retention is
// operator-owned by policy (issue #505) — the app growing its own expirer is
// the exact thing this test exists to catch in review.
func TestBackupImmutability_NoBackupExpiryOrRotationCodeInTheApp(t *testing.T) {
	t.Parallel()
	backendDir := immutBackendDir(t)
	deletionVerbs := []string{
		"rotate", "prune", "expire", "purge", "reap", "sweep",
		"unlink", "delete", "remove", "trim", "evict",
	}

	var offenders []string
	require.NoError(t, filepath.WalkDir(backendDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := strings.ToLower(fn.Name.Name)
			if !strings.Contains(name, "backup") && !strings.Contains(name, "snapshot") {
				continue
			}
			for _, verb := range deletionVerbs {
				if strings.Contains(name, verb) {
					rel, _ := filepath.Rel(backendDir, path)
					offenders = append(offenders, fmt.Sprintf("%s: func %s", rel, fn.Name.Name))
				}
			}
		}
		return nil
	}))

	assert.Empty(t, offenders,
		"issue #505: the application must have no code that deletes, rotates, or expires a backup — "+
			"retention is operator-owned, and an in-app expirer is the same capability a host attacker "+
			"running as the app inherits")
}

// TestBackupImmutability_SnapshotSourceOnlyRemovesItsOwnTemp pins the file
// operations in the two backup-writing source files: no recursive delete, no
// truncate, and every os.Remove targets the reserved temp path only.
func TestBackupImmutability_SnapshotSourceOnlyRemovesItsOwnTemp(t *testing.T) {
	t.Parallel()
	backendDir := immutBackendDir(t)
	for _, rel := range []string{"database/backup.go", "database/premigration_backup.go"} {
		data, err := os.ReadFile(filepath.Join(backendDir, rel))
		require.NoError(t, err)
		src := string(data)

		assert.NotContains(t, src, "os.RemoveAll(", rel+": must never recursively delete")
		assert.NotContains(t, src, "os.Truncate(", rel+": must never truncate a file")
		assert.NotContains(t, src, "os.Chmod(", rel+": must not re-mode an existing file")

		for i, line := range strings.Split(src, "\n") {
			if !strings.Contains(line, "os.Remove(") {
				continue
			}
			ok := strings.Contains(line, "os.Remove(tmpPath)") || strings.Contains(line, "os.Remove(name)")
			assert.Truef(t, ok, "%s:%d: os.Remove is only permitted on the reserved temp file, got: %s",
				rel, i+1, strings.TrimSpace(line))
		}
	}
}

// immutBackendDir resolves the backend/ directory from this test file's path
// (backend/database/backup_immutability_test.go -> backend/), independent of
// the process working directory.
func immutBackendDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}
