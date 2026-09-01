package database

// DEPLOY-03 (issue #452): interrupted startup & migration testing.
//
// Server startup runs every pending migration from the embedded FS
// (database.InitDB), so startup is a WRITE operation and an interruption at the
// wrong moment can leave a schema change half-done. MIG-04 (issues #439/#546)
// defines the fail-closed refusals and issue #530 the mandatory pre-migration
// backup; migrate_failclosed_test.go / fault_injection_test.go pin those with
// injected errors. This file is the interruption-shaped proof: kill the process
// at each point that matters, restart, and assert a DEFINED outcome — either
// the instance comes up correct and complete, or it refuses naming the state
// and the recovery. Nothing in between. Repeated kill/restart cycles must
// converge on that same state, never a worse one.
//
// The four kill points, and the crash signature each leaves:
//
//	before_migrations  — after the pre-migration backup, before the first
//	                     migration statement. Schema completely untouched.
//	during_migration   — between a migration body's commit and its clean-mark.
//	                     Dirty at N, migration N's DDL applied.
//	between_migrations — after migration N's clean-mark, before N+1 begins.
//	                     CLEAN at intermediate version N.
//	after_migrations   — migrations finished, server not yet serving. Clean at
//	                     latest.
//
// The real-SIGKILL analogue of every case runs in the chaos job
// (.github/workflows/chaos-tests.yml startup-interruption-kill-points); per the
// split-harness rule (docs/development/fault-injection.md) the deterministic
// state assertions live here, in-process.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/faults"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fileSHA256 returns the hex SHA-256 of a file's bytes — used to assert a
// rollback point is byte-for-byte unchanged across a crash loop.
func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// userCount reads users.COUNT straight off disk — the pre-upgrade data whose
// survival every recovery path must guarantee.
func userCount(t *testing.T, dbPath string) int {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()
	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&n))
	return n
}

// closeInitDB closes the *gorm.DB handle InitDB returns so the next InitDB on
// the same file is not fighting an open WAL connection.
func closeInitDB(t *testing.T, dbPath string) func() {
	t.Helper()
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	return func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}
}

// midChainVersion picks a migration version strictly between the upgrade floor
// and the latest — the "between two migrations" clean stopping point.
func midChainVersion(t *testing.T) uint {
	t.Helper()
	latest := mustLatestVersion(t)
	require.Greater(t, latest, SupportedUpgradeFloorVersion+1,
		"DEPLOY-03 between-migrations needs at least two migrations above the floor")
	return SupportedUpgradeFloorVersion + (latest-SupportedUpgradeFloorVersion)/2
}

// TestInterruptedStartupKillPoints walks the four kill points: drive a
// floor-version install (with one pre-upgrade user row) into each crash
// signature, assert the signature, restart via InitDB, and assert the declared
// outcome plus that the user row survived.
func TestInterruptedStartupKillPoints(t *testing.T) {
	latest := mustLatestVersion(t)

	t.Run("before_migrations", func(t *testing.T) {
		faults.Reset()
		t.Cleanup(faults.Reset)
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "live.db")
		floorDBWithUser(t, dbPath)

		// Killed after the pre-migration backup, before the first statement.
		faults.ArmError(faultMigrationBeforeBatch, &faults.ErrInjected{Name: faultMigrationBeforeBatch})
		err := MigrateUp(dbPath)
		require.Error(t, err, "an armed before-batch fault must abort the run")
		assert.Contains(t, err.Error(), "before applying any migration")
		faults.Disarm(faultMigrationBeforeBatch)

		// Crash signature: the schema is completely untouched.
		version, dirty := dirtyState(t, dbPath)
		assert.EqualValues(t, SupportedUpgradeFloorVersion, version, "no migration may have been applied")
		assert.False(t, dirty, "an interruption before the first statement leaves NO dirty flag")
		assert.Equal(t, "ok", integrityCheck(t, dbPath))
		assert.True(t, tableExists(t, dbPath, "users"), "sanity: the floor schema is still present")

		// The pre-migration backup was taken before the abort and is a valid
		// rollback point (issue #452 action 5).
		snaps := preMigrationSnapshots(t, dir)
		require.Len(t, snaps, 1, "the mandatory pre-migration backup is taken before the before-batch seam")
		res, err := IntegrityCheck(snaps[0])
		require.NoError(t, err)
		assert.Equal(t, "ok", res)

		// Defined outcome: a restart just migrates normally.
		closeInitDB(t, dbPath)()
		version, dirty = dirtyState(t, dbPath)
		assert.EqualValues(t, latest, version, "restart migrates to the latest schema")
		assert.False(t, dirty)
		assert.Equal(t, "ok", integrityCheck(t, dbPath))
		assert.Equal(t, 1, userCount(t, dbPath), "the pre-upgrade data survived")
	})

	t.Run("during_migration", func(t *testing.T) {
		faults.Reset()
		t.Cleanup(faults.Reset)
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "live.db")
		floorDBWithUser(t, dbPath)

		// Killed between the first pending migration's commit and its clean-mark.
		faults.ArmError(faultMigrationStatement, &faults.ErrInjected{Name: faultMigrationStatement})
		err := MigrateUp(dbPath)
		require.Error(t, err, "an armed statement fault must fail the run")
		faults.Disarm(faultMigrationStatement)

		firstPending := SupportedUpgradeFloorVersion + 1
		version, dirty := dirtyState(t, dbPath)
		assert.EqualValues(t, firstPending, version, "the fault leaves the DB dirty at the just-committed migration")
		assert.True(t, dirty, "the crash signature is a dirty flag")
		assert.Equal(t, "ok", integrityCheck(t, dbPath), "a failed migration must not corrupt the database")

		// Defined outcome: a restart REFUSES on the dirty flag (fail-closed),
		// naming the version and the recovery — never a forced boot.
		_, err = InitDB(dbPath)
		require.Error(t, err, "a dirty database must refuse to start")
		var dirtyErr *ErrDirtyMigration
		require.ErrorAs(t, err, &dirtyErr)
		assert.EqualValues(t, firstPending, dirtyErr.Version)
		assert.Contains(t, err.Error(), "Restore the pre-migration backup")
		version, dirty = dirtyState(t, dbPath)
		assert.True(t, dirty, "a refusal must not alter the database")

		// The operator-only recovery lands clean at the latest schema.
		require.NoError(t, MigrateForce(dbPath))
		version, dirty = dirtyState(t, dbPath)
		assert.EqualValues(t, latest, version)
		assert.False(t, dirty)
		assert.Equal(t, "ok", integrityCheck(t, dbPath))
		assert.Equal(t, 1, userCount(t, dbPath), "the pre-upgrade data survived the recovery")
	})

	t.Run("between_migrations", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "live.db")
		k := midChainVersion(t)

		// The clean intermediate state a process killed between two migrations'
		// clean-marks leaves: golang-migrate marks each step clean as it goes.
		require.NoError(t, MigrateUpTo(dbPath, k))
		seedUser(t, dbPath, "between")
		version, dirty := dirtyState(t, dbPath)
		require.EqualValues(t, k, version)
		require.False(t, dirty, "a kill between migrations leaves NO dirty flag")
		assert.Equal(t, "ok", integrityCheck(t, dbPath))

		// Defined outcome: a restart RESUMES from k+1 with no force needed.
		closeInitDB(t, dbPath)()
		version, dirty = dirtyState(t, dbPath)
		assert.EqualValues(t, latest, version, "restart resumes and completes the chain")
		assert.False(t, dirty)
		assert.Equal(t, "ok", integrityCheck(t, dbPath))
		assert.Equal(t, 1, userCount(t, dbPath))
	})

	t.Run("after_migrations", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "live.db")
		floorDBWithUser(t, dbPath)

		// Migrations finished; the process is killed before it binds its
		// listener. At the database level this is a completed upgrade.
		require.NoError(t, MigrateUp(dbPath))
		version, dirty := dirtyState(t, dbPath)
		require.EqualValues(t, latest, version)
		require.False(t, dirty)
		snapsAfterUpgrade := preMigrationSnapshots(t, dir)

		// Defined outcome: a restart is a migration no-op — no new backup, no
		// schema change, and the instance is serviceable.
		closeInitDB(t, dbPath)()
		version, dirty = dirtyState(t, dbPath)
		assert.EqualValues(t, latest, version)
		assert.False(t, dirty)
		assert.Equal(t, "ok", integrityCheck(t, dbPath))
		assert.Equal(t, 1, userCount(t, dbPath))
		assert.Len(t, preMigrationSnapshots(t, dir), len(snapsAfterUpgrade),
			"a restart of an up-to-date database takes no new pre-migration backup")
	})
}

// TestInterruptedStartupCrashLoopConverges is issue #452 action 3: a container
// that is killed, restarts, is killed again, and restarts must end in the SAME
// defined state as one kill — not a deeper one. Both the dirty (during) and the
// clean-intermediate (between) signatures are exercised.
func TestInterruptedStartupCrashLoopConverges(t *testing.T) {
	latest := mustLatestVersion(t)

	t.Run("dirty_signature_stays_put_across_restarts", func(t *testing.T) {
		faults.Reset()
		t.Cleanup(faults.Reset)
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "live.db")
		floorDBWithUser(t, dbPath)

		faults.ArmError(faultMigrationStatement, &faults.ErrInjected{Name: faultMigrationStatement})
		require.Error(t, MigrateUp(dbPath))
		faults.Disarm(faultMigrationStatement)

		want, wantDirty := dirtyState(t, dbPath)
		require.True(t, wantDirty)
		wantUsers := userCount(t, dbPath)

		// The orchestrator keeps restarting the container. Every restart must
		// refuse and leave the exact same state — the refusal is not a write.
		for i := 0; i < 5; i++ {
			_, err := InitDB(dbPath)
			require.Errorf(t, err, "restart %d must refuse the dirty database", i+1)
			var dirtyErr *ErrDirtyMigration
			require.ErrorAs(t, err, &dirtyErr)
			gotV, gotDirty := dirtyState(t, dbPath)
			assert.Equalf(t, want, gotV, "restart %d must not move the version", i+1)
			assert.Truef(t, gotDirty, "restart %d must not clear the dirty flag", i+1)
			assert.Equalf(t, "ok", integrityCheck(t, dbPath), "restart %d must not corrupt the database", i+1)
			assert.Equalf(t, wantUsers, userCount(t, dbPath), "restart %d must not touch the data", i+1)
		}

		// After any number of kills, ONE operator recovery still works.
		require.NoError(t, MigrateForce(dbPath))
		gotV, gotDirty := dirtyState(t, dbPath)
		assert.EqualValues(t, latest, gotV)
		assert.False(t, gotDirty)
		assert.Equal(t, wantUsers, userCount(t, dbPath))
	})

	t.Run("clean_intermediate_completes_and_then_is_stable", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "live.db")
		k := midChainVersion(t)
		require.NoError(t, MigrateUpTo(dbPath, k))
		seedUser(t, dbPath, "loop")

		// First restart resumes to latest; every subsequent restart is a clean
		// no-op that converges on the same state.
		for i := 0; i < 5; i++ {
			closeInitDB(t, dbPath)()
			gotV, gotDirty := dirtyState(t, dbPath)
			assert.EqualValuesf(t, latest, gotV, "restart %d must be at latest", i+1)
			assert.Falsef(t, gotDirty, "restart %d must be clean", i+1)
			assert.Equalf(t, "ok", integrityCheck(t, dbPath), "restart %d integrity", i+1)
			assert.Equalf(t, 1, userCount(t, dbPath), "restart %d data", i+1)
		}
	})
}

// TestInterruptedStartupPreMigrationBackupSurvivesAndRestores is issue #452
// action 5: the pre-migration backup (#530) taken during an interrupted upgrade
// must survive the interruption AND a crash loop (reused, not rewritten each
// restart), and must be a restorable recovery point.
func TestInterruptedStartupPreMigrationBackupSurvivesAndRestores(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)
	latest := mustLatestVersion(t)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	floorDBWithUser(t, dbPath)

	// The upgrade is interrupted mid-migration: the pre-migration snapshot was
	// written before the first statement, then the statement fault fired.
	faults.ArmError(faultMigrationStatement, &faults.ErrInjected{Name: faultMigrationStatement})
	require.Error(t, MigrateUp(dbPath))

	snaps := preMigrationSnapshots(t, dir)
	require.Len(t, snaps, 1, "exactly one pre-migration snapshot for the hop")
	snap := snaps[0]

	res, err := IntegrityCheck(snap)
	require.NoError(t, err)
	assert.Equal(t, "ok", res, "the snapshot survived the interruption intact")
	sv, sdirty, sok, err := MigrationVersion(snap)
	require.NoError(t, err)
	require.True(t, sok)
	assert.EqualValues(t, SupportedUpgradeFloorVersion, sv, "the snapshot captures the PRE-upgrade schema")
	assert.False(t, sdirty)

	// Hash the snapshot AFTER the inspection reads above (opening a WAL database
	// can rewrite its own file on close), then crash-loop and re-hash.
	sumBefore := fileSHA256(t, snap)

	// Crash loop: the interrupted upgrade is retried and interrupted again. The
	// dirty database is refused, so the snapshot is byte-for-byte untouched, and
	// the idempotent-per-hop path never rewrites the existing rollback point.
	for i := 0; i < 3; i++ {
		_, err := InitDB(dbPath)
		require.Error(t, err)
	}
	assert.Equal(t, sumBefore, fileSHA256(t, snap),
		"the pre-migration snapshot must be reused across restarts, not rewritten")
	require.Len(t, preMigrationSnapshots(t, dir), 1, "a crash loop must not pile up snapshots")

	faults.Disarm(faultMigrationStatement)

	// Recovery: restore the snapshot in place (overwrite the .db, clear stale
	// WAL sidecars) and restart. This is docs/operations/migration-recovery.md
	// → Dirty schema, exercised.
	for _, suffix := range []string{"-wal", "-shm"} {
		if e := os.Remove(dbPath + suffix); e != nil && !os.IsNotExist(e) {
			require.NoError(t, e)
		}
	}
	data, err := os.ReadFile(snap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dbPath, data, 0o644))

	closeInitDB(t, dbPath)()
	version, dirty := dirtyState(t, dbPath)
	assert.EqualValues(t, latest, version, "the restored database migrates cleanly to latest")
	assert.False(t, dirty)
	assert.Equal(t, "ok", integrityCheck(t, dbPath))
	assert.Equal(t, 1, userCount(t, dbPath), "the pre-upgrade data is back after the restore")
}

// TestInterruptedStartupIntegrityHoldsAfterEveryRecovery consolidates issue
// #452 action 6: PRAGMA integrity_check passes after every recovery path this
// file exercises. The DB-01 deep checker (mycorrhizal doctor, issue #460) is
// not built yet; when it lands its check slots in beside IntegrityCheck here.
func TestInterruptedStartupIntegrityHoldsAfterEveryRecovery(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)
	latest := mustLatestVersion(t)

	// Recovery path 1: refuse-then-force (during_migration).
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "a.db")
	floorDBWithUser(t, dbPath)
	faults.ArmError(faultMigrationStatement, &faults.ErrInjected{Name: faultMigrationStatement})
	require.Error(t, MigrateUp(dbPath))
	faults.Disarm(faultMigrationStatement)
	_, err := InitDB(dbPath)
	require.Error(t, err)
	require.NoError(t, MigrateForce(dbPath))
	assert.Equal(t, "ok", integrityCheck(t, dbPath))
	// TODO(#460): assert the mycorrhizal-doctor deep check passes here too.

	// Recovery path 2: resume-from-intermediate (between_migrations).
	dbPath = filepath.Join(dir, "b.db")
	require.NoError(t, MigrateUpTo(dbPath, midChainVersion(t)))
	closeInitDB(t, dbPath)()
	v, _ := dirtyState(t, dbPath)
	require.EqualValues(t, latest, v)
	assert.Equal(t, "ok", integrityCheck(t, dbPath))

	// Recovery path 3: retry-after-before-batch-abort.
	dbPath = filepath.Join(dir, "c.db")
	floorDBWithUser(t, dbPath)
	faults.ArmError(faultMigrationBeforeBatch, &faults.ErrInjected{Name: faultMigrationBeforeBatch})
	require.Error(t, MigrateUp(dbPath))
	faults.Disarm(faultMigrationBeforeBatch)
	closeInitDB(t, dbPath)()
	assert.Equal(t, "ok", integrityCheck(t, dbPath))
}

// TestInitDBReturnsNoHandleOnEveryInterruptedState pins the startup ordering
// guarantee behind issue #452 action 4: main.go runs database.InitDB (which
// runs migrations) BEFORE it builds the router or binds the HTTP listener, and
// InitDB returns (nil, err) — never a usable handle — for every interrupted /
// refused migration state. So a process that is mid-migration or that came up
// on a dirty database never reaches srv.ListenAndServe(): /health/live and
// /health/ready are physically unservable in that window, and an orchestrator
// probing during a migration sees connection-refused (= not ready), never a
// false "ready". The refusal is a logger.Fatal at main.go's
// "Failed to initialize database".
func TestInitDBReturnsNoHandleOnEveryInterruptedState(t *testing.T) {
	latest := mustLatestVersion(t)

	cases := map[string]func(t *testing.T, dbPath string){
		"dirty": func(t *testing.T, dbPath string) {
			dirtyDB(t, dbPath, 1)
		},
		"ahead_of_binary": func(t *testing.T, dbPath string) {
			require.NoError(t, MigrateUp(dbPath))
			sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
			require.NoError(t, err)
			defer sqlDB.Close()
			_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?", latest+3)
			require.NoError(t, err)
		},
		"sub_floor": func(t *testing.T, dbPath string) {
			require.NoError(t, MigrateUpTo(dbPath, SupportedUpgradeFloorVersion-1))
		},
		"interrupted_mid_migration": func(t *testing.T, dbPath string) {
			interruptedDB(t, dbPath, 1)
		},
	}

	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "live.db")
			setup(t, dbPath)

			db, err := InitDB(dbPath)
			require.Error(t, err, "InitDB must fail closed on the %s state", name)
			assert.Nil(t, db, "InitDB must not hand main() a usable DB handle for the %s state", name)
		})
	}
}

// TestMigrationBeforeBatchFaultLeavesSchemaUntouched is the focused unit for
// the new seam (and the coverage-gate test for its error branch): an armed
// before-batch fault returns from runPendingMigrations before m.Up(), so a
// brand-new database is left with no schema at all.
func TestMigrationBeforeBatchFaultLeavesSchemaUntouched(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)
	dbPath := filepath.Join(t.TempDir(), "fresh.db")

	faults.ArmError(faultMigrationBeforeBatch, &faults.ErrInjected{Name: faultMigrationBeforeBatch})
	err := MigrateUp(dbPath)
	require.Error(t, err)
	assert.ErrorIs(t, err, &faults.ErrInjected{Name: faultMigrationBeforeBatch})
	assert.Contains(t, err.Error(), "before applying any migration")

	_, _, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	assert.False(t, ok, "no migration may have been applied — not even the version row")
	assert.False(t, tableExists(t, dbPath, "users"), "the seam fires before the first statement")

	// Disarmed, the very next run migrates normally.
	faults.Disarm(faultMigrationBeforeBatch)
	require.NoError(t, MigrateUp(dbPath))
	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, mustLatestVersion(t), version)
	assert.False(t, dirty)
}
