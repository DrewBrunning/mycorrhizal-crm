package schemafixture

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/attachments"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// BACKUP-01 (issue #453) — cross-version restore testing.
//
// A backup is only a backup if the software you will be running when you need
// it can restore it. #530 makes this load-bearing: rollback IS
// restore-from-backup, so the cross-version restore matrix is the rollback
// mechanism. This suite is that matrix.
//
// The cells, per the ticket, for supported releases from the #529 floor:
//
//   - M == N — a snapshot taken by release N, restored and run by that same
//     release. Must work: the snapshot already presents N's schema, so a
//     same-version binary runs no migration and serves it as-is.
//     (TestCrossVersionRestoreSameRelease)
//
//   - M > N — a snapshot taken by an older release N, restored onto a newer
//     release M (the common disaster-recovery case: restore onto a current
//     install). Must work: restore, then startup migrations bring the schema
//     forward, and the data must still MEAN what it meant (MIG-03 / #438
//     semantic comparison, not just "the server booted").
//     (TestCrossVersionRestoreOntoNewerRelease)
//
//   - M < N — a snapshot taken by a NEWER release N, an operator tries to
//     restore it under an OLDER binary M. Must be REFUSED, not attempted: per
//     #530 downgrade is unsupported, and per #439 an ahead-of-binary schema
//     fails closed. The refusal must name the recovery path.
//     (TestCrossVersionRestoreUnderOlderBinaryRefused)
//
// "Restore" here is the three-piece operation docs/deployment.md documents:
// the SQLite file PLUS the photo directory (PROFILE_PHOTO_DIR) PLUS the
// attachments directory (ATTACHMENTS_DIR). The database rows for photos and
// attachments are metadata; the bytes live outside the .db, so a test that
// restored only the .db would test a fiction (the ticket's action 2). Every
// non-deleted attachment/photo row must resolve to a real file after the
// restore.

// backupOf takes a VACUUM INTO snapshot of the database at srcPath — the exact
// call `make backup` and the pre-migration hook (#530) use — and returns the
// snapshot path. The snapshot lands in its own directory so the three-piece
// restore can treat it as the "backup" alongside the copied file dirs.
func backupOf(t *testing.T, srcPath string) string {
	t.Helper()
	snapDir := filepath.Join(t.TempDir(), "backup")
	require.NoError(t, os.MkdirAll(snapDir, 0o750))
	snapPath := filepath.Join(snapDir, "mycorrhizal.db")
	require.NoError(t, database.BackupSnapshot(srcPath, snapPath),
		"BackupSnapshot of %s must succeed", srcPath)
	return snapPath
}

// seededFilePieces is the on-disk photo/attachment state that the canonical
// manifest deliberately does not carry (it stores metadata rows only). The
// cross-version restore suite materialises it so "restore the three pieces"
// and "every attachment row resolves to a real file" are real assertions and
// not vacuous.
type seededFilePieces struct {
	photoDir  string
	attachDir string
}

// seedFilePieces writes a real file for every live attachment row's
// stored_name and stamps a photo file onto a couple of live contacts, so the
// fixture has the two out-of-database pieces a real instance keeps beside the
// SQLite file. It returns the source photo/attachment directories (these are
// what a three-piece backup copies).
//
// Files are written with the schema the fixture already has — raw SQL for the
// contacts.photo stamp, since a historical fixture may predate model fields
// and must not go through GORM hooks (mirrors seedMigrationScopeData).
func seedFilePieces(t *testing.T, f *Fixture) seededFilePieces {
	t.Helper()
	base := t.TempDir()
	photoDir := filepath.Join(base, "photos")
	attachDir := filepath.Join(base, "attachments")
	require.NoError(t, os.MkdirAll(photoDir, 0o700))
	require.NoError(t, os.MkdirAll(attachDir, 0o700))

	// Attachments: one file per live stored_name. attachments.Save is not used
	// (it invents a fresh UUID); the file must match the row already loaded
	// from the manifest, so the stored_name from the row is the filename.
	var storedNames []string
	require.NoError(t, f.DB.Raw(
		"SELECT stored_name FROM attachments WHERE deleted_at IS NULL AND stored_name != ''").
		Scan(&storedNames).Error)
	require.NotEmpty(t, storedNames,
		"the %s fixture must carry live attachment rows for the completeness check to exercise", f.Release.Tag)
	for _, name := range storedNames {
		p, err := attachments.StoredPath(attachDir, name)
		require.NoError(t, err, "manifest stored_name %q must be a safe on-disk name", name)
		require.NoError(t, os.WriteFile(p, []byte("attachment bytes for "+name), 0o600))
	}

	// Photos: stamp a filename onto the first two live contacts and write the
	// file. The value goes into contacts.photo (a column present since well
	// before the v0.6.0 floor), so a restore that drops the photo directory
	// leaves these rows pointing at nothing — which is exactly what the
	// hand-verify in the ticket breaks on.
	var contactIDs []uint
	require.NoError(t, f.DB.Raw(
		"SELECT id FROM contacts WHERE deleted_at IS NULL ORDER BY id LIMIT 2").
		Scan(&contactIDs).Error)
	require.Len(t, contactIDs, 2, "the fixture must have at least two live contacts")
	for _, id := range contactIDs {
		name := fmt.Sprintf("contact-%d.jpg", id)
		require.NoError(t, os.WriteFile(filepath.Join(photoDir, name), []byte("jpeg bytes"), 0o600))
		require.NoError(t, f.DB.Exec("UPDATE contacts SET photo = ? WHERE id = ?", name, id).Error)
	}

	return seededFilePieces{photoDir: photoDir, attachDir: attachDir}
}

// restoreThreePieces models docs/deployment.md → Restore: into a fresh data
// directory it drops the snapshot .db in place and recursively copies the
// photo and attachment directories (the `rsync -a --delete` into a clean
// target is a plain copy). It returns the restored database path and the two
// restored directories. Passing skipPhotos=true reproduces the ticket's
// hand-verify: a restore that forgot the photo directory.
func restoreThreePieces(t *testing.T, snapPath, srcPhotoDir, srcAttachDir string, skipPhotos bool) (dbPath, photoDir, attachDir string) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "restored")
	require.NoError(t, os.MkdirAll(dst, 0o750))

	dbPath = filepath.Join(dst, "mycorrhizal.db")
	copyFile(t, snapPath, dbPath)

	photoDir = filepath.Join(dst, "photos")
	if !skipPhotos {
		copyTree(t, srcPhotoDir, photoDir)
	}
	attachDir = filepath.Join(dst, "attachments")
	copyTree(t, srcAttachDir, attachDir)
	return dbPath, photoDir, attachDir
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o750))
	require.NoError(t, os.WriteFile(dst, b, 0o600))
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dst, 0o700))
	for _, e := range entries {
		if e.IsDir() {
			copyTree(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
			continue
		}
		copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
	}
}

// assertRestoreCompleteness is the ticket's "every attachment row resolving to
// a real file" — extended to photos, since the photo directory is the second
// out-of-database piece and the hand-verify skips exactly it. Every live
// attachment stored_name and every live contacts.photo must name a file that
// exists under the restored directories.
func assertRestoreCompleteness(t *testing.T, db *gorm.DB, photoDir, attachDir string) {
	t.Helper()

	var storedNames []string
	require.NoError(t, db.Raw(
		"SELECT stored_name FROM attachments WHERE deleted_at IS NULL AND stored_name != ''").
		Scan(&storedNames).Error)
	require.NotEmpty(t, storedNames, "restored database must still carry live attachment rows")
	for _, name := range storedNames {
		p, err := attachments.StoredPath(attachDir, name)
		require.NoError(t, err)
		assert.FileExistsf(t, p, "attachment row %q must resolve to a real file after restore", name)
	}

	var photos []string
	require.NoError(t, db.Raw(
		"SELECT photo FROM contacts WHERE deleted_at IS NULL AND photo != ''").
		Scan(&photos).Error)
	require.NotEmpty(t, photos, "restored database must still carry contacts with photos")
	for _, name := range photos {
		assert.FileExistsf(t, filepath.Join(photoDir, name),
			"contact photo %q must resolve to a real file after restore", name)
	}
}

// operationalBookkeepingTables are the tables a real server-startup restore
// legitimately writes that are NOT user data: the migration-completed events
// RunMigrations appends on every schema advance, plus the scheduled-job /
// health bookkeeping the same startup wires up. They are exactly the set the
// restore drill (issue #275, services.excludedFromRestoreDrill) and the
// migration row-count guard (migrationDiagnosticTables) already exclude for
// the same reason — the MIG-03 comparison is about whether USER data still
// means what it meant, and a restore that boots the server is expected to log
// that it happened.
var operationalBookkeepingTables = map[string]bool{
	"system_events":             true,
	"job_executions":            true,
	"operational_check_results": true,
	"alert_states":              true,
}

// withoutOperationalTables clones snap with the operational bookkeeping tables
// dropped, so the semantic content comparison sees only user data.
func withoutOperationalTables(snap *contentSnapshot) *contentSnapshot {
	out := cloneSnapshot(snap)
	for table := range operationalBookkeepingTables {
		delete(out.columns, table)
		delete(out.rows, table)
	}
	return out
}

// openRestoredDB opens a restored .db the way a server of the RESTORING
// release would at startup: InitDB runs every pending migration (the M > N
// "restore, then migrations bring the schema forward" step). It returns the
// handle and the applied version.
func openRestoredDB(t *testing.T, dbPath string) (*gorm.DB, uint) {
	t.Helper()
	db, err := database.InitDB(dbPath)
	require.NoError(t, err, "a restored database must open and migrate")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	version, dirty, ok, err := database.AppliedMigrationVersion(db)
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, dirty, "a restored database must not be dirty")
	return db, version
}

// TestCrossVersionRestoreSameRelease is the M == N cell: a snapshot taken by
// release N is restored and run by that same release. The snapshot already
// presents N's schema, so nothing migrates; the three pieces come back intact
// and the content is byte-for-byte the fixture's.
func TestCrossVersionRestoreSameRelease(t *testing.T) {
	for _, from := range SupportedReleases {
		from := from
		t.Run(from.Tag, func(t *testing.T) {
			f := Load(t, from)
			pieces := seedFilePieces(t, f)

			before := captureContentSnapshot(t, f.DB)
			beforeContacts := contactRecords(t, f.DB, f.Dataset)
			closeFixtureDB(t, f.DB)

			snap := backupOf(t, f.Path)

			// The snapshot is a self-contained database at exactly N's schema,
			// clean, integrity ok — no migration, no WAL sidecar.
			assertFixtureVersion(t, snap, from.Version)
			assertIntegrity(t, snap, "ok")

			dbPath, photoDir, attachDir := restoreThreePieces(t, snap, pieces.photoDir, pieces.attachDir, false)

			// A same-version binary runs no migration on restore.
			db, err := database.OpenMigratedFile(dbPath)
			require.NoError(t, err)
			t.Cleanup(func() {
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close()
				}
			})
			version, dirty, ok, err := database.AppliedMigrationVersion(db)
			require.NoError(t, err)
			require.True(t, ok)
			assert.EqualValues(t, from.Version, version, "M == N must not migrate the restored snapshot")
			assert.False(t, dirty)

			assertContentIdentical(t, from.Tag, before, captureContentSnapshot(t, db))
			assertContactsSemanticallyEqual(t, from.Tag, beforeContacts, contactRecords(t, db, f.Dataset))
			assertRestoreCompleteness(t, db, photoDir, attachDir)
		})
	}
}

// TestCrossVersionRestoreOntoNewerRelease is the M > N cell and the common
// real case: a snapshot from an older release N is restored onto a current
// install, which migrates it forward on startup. Row counts survive, the
// three pieces resolve, and — the point the ticket makes — the data still
// MEANS what it meant, checked with the MIG-03 semantic comparison rather than
// "the server started".
func TestCrossVersionRestoreOntoNewerRelease(t *testing.T) {
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	expectDir := committedMigrationsDir(t)

	for _, from := range SupportedReleases {
		from := from
		t.Run(from.Tag+"->current", func(t *testing.T) {
			f := Load(t, from)
			seedMigrationScopeData(t, f) // audit history + the etag/e_tag sync-state trap
			pieces := seedFilePieces(t, f)
			assertCriteriaPopulated(t, f)

			before := withoutOperationalTables(captureContentSnapshot(t, f.DB))
			beforeContacts := contactRecords(t, f.DB, f.Dataset)
			beforeCounts := tableCounts(t, f.DB)
			closeFixtureDB(t, f.DB)

			snap := backupOf(t, f.Path)
			assertFixtureVersion(t, snap, from.Version) // the backup captured the PRE-upgrade schema

			dbPath, photoDir, attachDir := restoreThreePieces(t, snap, pieces.photoDir, pieces.attachDir, false)

			db, version := openRestoredDB(t, dbPath)
			assert.EqualValues(t, latest, version, "restore onto a newer release must migrate forward to current")
			assertIntegrity(t, dbPath, "ok")

			assertRowCountsPreserved(t, from.Tag+" restore->current", beforeCounts, tableCounts(t, db))

			after := withoutOperationalTables(captureContentSnapshot(t, db))
			afterContacts := contactRecords(t, db, f.Dataset)
			declared := loadExpectationFiles(t, expectDir, from.Version, latest)
			if len(declared) == 0 {
				assertContentIdentical(t, from.Tag, before, after)
				assertContactsSemanticallyEqual(t, from.Tag, beforeContacts, afterContacts)
			} else {
				assertContentMatchesExpectations(t, from.Tag, before, after, declared)
			}

			assertRestoreCompleteness(t, db, photoDir, attachDir)
		})
	}
}

// TestCrossVersionRestoreOntoNewerReleaseMissingPhotoDirIsIncomplete is the
// ticket's hand-verify wired in: a restore that forgot the photo directory
// must fail the completeness check (not pass quietly). It runs the same M > N
// path as above but with skipPhotos=true and asserts the photo-resolution
// half of assertRestoreCompleteness would fail — the attachment half still
// passes, proving the check is specific.
func TestCrossVersionRestoreOntoNewerReleaseMissingPhotoDirIsIncomplete(t *testing.T) {
	from := SupportedReleases[0]
	f := Load(t, from)
	pieces := seedFilePieces(t, f)
	closeFixtureDB(t, f.DB)

	snap := backupOf(t, f.Path)
	dbPath, photoDir, attachDir := restoreThreePieces(t, snap, pieces.photoDir, pieces.attachDir, true /* skip photos */)

	db, _ := openRestoredDB(t, dbPath)

	// Attachments still resolve — they were restored.
	var storedNames []string
	require.NoError(t, db.Raw(
		"SELECT stored_name FROM attachments WHERE deleted_at IS NULL AND stored_name != ''").Scan(&storedNames).Error)
	require.NotEmpty(t, storedNames)
	for _, name := range storedNames {
		p, err := attachments.StoredPath(attachDir, name)
		require.NoError(t, err)
		assert.FileExists(t, p, "attachments were restored, so they must still resolve")
	}

	// Photos do not — the directory was skipped. This is the failure the
	// completeness assertion exists to catch.
	var photos []string
	require.NoError(t, db.Raw(
		"SELECT photo FROM contacts WHERE deleted_at IS NULL AND photo != ''").Scan(&photos).Error)
	require.NotEmpty(t, photos, "the fixture stamped photos onto contacts")
	missing := 0
	for _, name := range photos {
		if _, err := os.Stat(filepath.Join(photoDir, name)); err != nil {
			missing++
		}
	}
	assert.Equal(t, len(photos), missing,
		"a restore that skipped the photo directory must leave every photo row unresolved")
}

// TestCrossVersionRestoreUnderOlderBinaryRefused is the M < N cell: a snapshot
// taken by a NEWER release is handed to an OLDER binary. Downgrade is
// unsupported (#530); an ahead-of-binary schema fails closed (#439). The
// refusal must be the typed sentinel and must name the recovery path.
//
// Modelling note. The refusal is version-comparison based, not release-tag
// based: an older release M perceives a newer snapshot as exactly "schema
// version > my latest known migration". A Go test always links the current
// binary, so "an older binary M" is modelled by a snapshot whose recorded
// schema_migrations version sits ABOVE what this binary knows — the same
// construction issue #439's own fail-closed tests use. Every concrete
// (N, M) pair with M < N reduces to this one state.
func TestCrossVersionRestoreUnderOlderBinaryRefused(t *testing.T) {
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)

	// A real, current-schema instance is backed up...
	livePath := filepath.Join(t.TempDir(), "live.db")
	liveDB, err := database.InitDB(livePath)
	require.NoError(t, err)
	seedLiveInstance(t, liveDB, 3)
	closeDB(t, liveDB)

	for _, ahead := range []uint{latest + 1, latest + 7} {
		ahead := ahead
		t.Run(fmt.Sprintf("snapshot_at_v%d_binary_at_v%d", ahead, latest), func(t *testing.T) {
			snap := backupOf(t, livePath)

			// ...then a NEWER release migrated the source further, so the
			// snapshot an operator now tries to restore records a schema this
			// (older) binary does not know.
			bumpRecordedSchemaVersion(t, snap, ahead)

			recorded, _, _, err := database.MigrationVersion(snap)
			require.NoError(t, err)
			require.EqualValues(t, ahead, recorded)

			for _, restore := range []struct {
				name string
				run  func(string) error
			}{
				{"startup (InitDB)", func(p string) error { _, e := database.InitDB(p); return e }},
				{"make migrate-up (MigrateUp)", database.MigrateUp},
			} {
				t.Run(restore.name, func(t *testing.T) {
					// Fresh copy per attempt: a refusal must not have touched
					// the previous one.
					attempt := filepath.Join(t.TempDir(), "attempt.db")
					copyFile(t, snap, attempt)

					err := restore.run(attempt)
					require.Error(t, err, "restoring a newer snapshot under an older binary must be refused")

					var aheadErr *database.ErrSchemaAheadOfBinary
					require.ErrorAs(t, err, &aheadErr,
						"the refusal must be the typed ErrSchemaAheadOfBinary sentinel, not a generic boot failure")
					assert.EqualValues(t, ahead, aheadErr.Version)
					assert.EqualValues(t, latest, aheadErr.BinaryVersion)

					// It must name the recovery path (#453 action 1): deploy a
					// binary that knows the migration, or restore the
					// pre-upgrade backup.
					msg := err.Error()
					assert.Contains(t, msg, "Downgrade is unsupported")
					assert.Contains(t, msg, "Deploy a binary that knows migration")
					assert.Contains(t, msg, "restore the backup taken before the newer release ran")
					assert.Contains(t, msg, "docs/deployment.md")

					// Refused, not attempted: the database is exactly as it was.
					after, _, _, verr := database.MigrationVersion(attempt)
					require.NoError(t, verr)
					assert.EqualValues(t, ahead, after, "a refused restore must not migrate the database")
					assertIntegrity(t, attempt, "ok")
				})
			}
		})
	}
}

// bumpRecordedSchemaVersion rewrites the snapshot's schema_migrations row to
// version, clean — the on-disk signature of "a newer release migrated this
// database" as an older binary sees it. Mirrors the construction in
// database/migrate_failclosed_test.go.
func bumpRecordedSchemaVersion(t *testing.T, dbPath string, version uint) {
	t.Helper()
	db, err := database.OpenMigratedFile(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE schema_migrations SET version = ?, dirty = 0", version).Error)
	closeDB(t, db)
}

// TestBackupUnderConcurrentWriteLoadIsConsistent is the ticket's action 5: a
// snapshot taken WHILE writes are in flight must still be a transactionally
// consistent cut — VACUUM INTO reads through the WAL, so it should be, and
// this says so. Writers insert a (user, note) pair inside one transaction, so
// a torn snapshot would be observable as a note whose user is missing; the
// snapshot must show neither a broken foreign key nor a half-written pair, and
// must pass integrity_check and open cleanly.
func TestBackupUnderConcurrentWriteLoadIsConsistent(t *testing.T) {
	srcPath := filepath.Join(t.TempDir(), "live.db")
	db, err := database.InitDB(srcPath)
	require.NoError(t, err)

	seed := models.User{Username: "seed", Password: "hunter2ish", Email: "seed@example.com"}
	require.NoError(t, db.Create(&seed).Error)

	var (
		stop     atomic.Bool
		wg       sync.WaitGroup
		writes   atomic.Int64
		writeErr = make(chan error, 1)
	)
	recordErr := func(e error) {
		select {
		case writeErr <- e:
		default:
		}
	}
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				err := db.Transaction(func(tx *gorm.DB) error {
					u := models.User{
						Username: fmt.Sprintf("w%d-%d", id, i),
						Password: "hunter2ish",
						Email:    fmt.Sprintf("w%d-%d@example.com", id, i),
					}
					if err := tx.Create(&u).Error; err != nil {
						return err
					}
					n := models.Note{UserID: u.ID, Content: "paired note", Date: time.Now()}
					return tx.Create(&n).Error
				})
				if err != nil {
					recordErr(err)
					return
				}
				writes.Add(1)
			}
		}(w)
	}

	// Take several snapshots across the write window; each must be consistent.
	var snaps []string
	for i := 0; i < 3; i++ {
		time.Sleep(15 * time.Millisecond)
		snapPath := filepath.Join(t.TempDir(), fmt.Sprintf("snap-%d.db", i))
		require.NoError(t, database.BackupSnapshot(srcPath, snapPath),
			"an online snapshot under write load must succeed")
		snaps = append(snaps, snapPath)
	}

	stop.Store(true)
	wg.Wait()
	select {
	case err := <-writeErr:
		require.NoError(t, err, "the concurrent writers must not have errored")
	default:
	}
	require.Positive(t, writes.Load(), "the writers must have committed work during the snapshot window")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	for _, snapPath := range snaps {
		assertConsistentCut(t, snapPath)
	}
}

// assertConsistentCut verifies a snapshot is a whole-transaction cut: integrity
// ok, no foreign-key violations, and no note left without the user its writer
// created in the same transaction.
func assertConsistentCut(t *testing.T, dbPath string) {
	t.Helper()
	assertIntegrity(t, dbPath, "ok")

	db, err := database.OpenMigratedFile(dbPath)
	require.NoError(t, err)
	defer closeDB(t, db)

	var fkViolations int64
	require.NoError(t, db.Raw("SELECT count(*) FROM (SELECT 1 FROM pragma_foreign_key_check())").Scan(&fkViolations).Error)
	assert.Zero(t, fkViolations, "snapshot %s must have no foreign-key violations", filepath.Base(dbPath))

	var orphanNotes int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM notes n
		LEFT JOIN users u ON u.id = n.user_id
		WHERE u.id IS NULL`).Scan(&orphanNotes).Error)
	assert.Zero(t, orphanNotes,
		"snapshot %s must not contain a note whose paired user write was torn off", filepath.Base(dbPath))
}

// seedLiveInstance writes n contacts through a live GORM handle so a backup of
// the instance has real user data to lose (the M < N refusal must protect a
// non-empty database).
func seedLiveInstance(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	user := models.User{Username: "live", Password: "hunter2ish", Email: "live@example.com"}
	require.NoError(t, db.Create(&user).Error)
	for i := 0; i < n; i++ {
		c := models.Contact{UserID: user.ID, Firstname: fmt.Sprintf("Contact%d", i)}
		require.NoError(t, db.Create(&c).Error)
	}
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// TestCrossVersionRestoreAtScale is the ticket's action 4 (coordinating with
// #495): the cross-version restore path — an older-release snapshot restored
// onto a current install — proven at a realistic row count, not just on a
// handful of manifest rows. Gated by MYCORRHIZAL_LARGE_TESTS=1 like the other
// large-dataset migration tests; the file-directory pieces are omitted here
// deliberately (a recursive directory copy is linear and not SQLite-specific —
// its correctness is covered by the non-scale tests), so this cell isolates
// the part that scales: the VACUUM INTO snapshot and the forward migration.
func TestCrossVersionRestoreAtScale(t *testing.T) {
	if !largeTestsEnabled() {
		t.Skip("set MYCORRHIZAL_LARGE_TESTS=1 (the nightly/main-push large-dataset job does)")
	}
	floor := SupportedReleases[0]
	srcPath := buildLargeFixture(t, floor)

	beforeCounts := tableCountsAtPath(t, srcPath)
	require.Positive(t, beforeCounts["contacts"], "the large fixture must carry contacts")

	snap := backupOf(t, srcPath)
	assertFixtureVersion(t, snap, floor.Version)
	assertIntegrity(t, snap, "ok")

	dbPath, _, _ := restoreThreePieces(t, snap, t.TempDir(), t.TempDir(), true)

	db, version := openRestoredDB(t, dbPath)
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	assert.EqualValues(t, latest, version, "the large restore must migrate forward to current")
	assertIntegrity(t, dbPath, "ok")
	assertRowCountsPreserved(t, "large floor restore->current", beforeCounts, tableCounts(t, db))

	var fkViolations int64
	require.NoError(t, db.Raw("SELECT count(*) FROM (SELECT 1 FROM pragma_foreign_key_check())").Scan(&fkViolations).Error)
	assert.Zero(t, fkViolations, "the migrated large restore must have no foreign-key violations")
}
