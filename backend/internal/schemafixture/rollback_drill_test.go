package schemafixture

import (
	"path/filepath"
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/require"
)

// Split from #923 into issue #997: docs/operations/migration-recovery.md's
// "Rolling back a bad release (N+1 -> N)" procedure was exercised only in
// pieces -- premigration_backup_test.go / cross_version_restore_test.go check
// the mandatory pre-migration snapshot (#530) is a valid, restorable database
// (assertPreMigrationBackupRestorable, issue #451 action 5), but nothing ever
// drove real HTTP traffic against the RESTORED rollback instance the way
// TestFullInstallUpgrade's exerciseUpgradedApp does for the forward-upgrade
// side. "The drill" table in migration-recovery.md had no row for this
// end-to-end scenario either.
//
// TestBadReleaseRollbackDrill closes both gaps: it plays out the runbook's
// own N+1 -> N scenario against a real three-piece install (deploy N+1 --
// the same in-place upgrade DEPLOY-02 exercises, which is also what writes
// the pre-migration backup -- then restore that backup and the file
// directories, exactly steps 1-4 of "Rolling back a bad release"), and then
// runs step 5 ("Start N and confirm...") as far as a single Go test binary
// can: driving the restored, NOT-migrated-further database through the real
// router (login, search, read+edit, export -- exerciseUpgradedApp, reused
// unchanged from DEPLOY-02).
//
// exerciseUpgradedApp's queries assume the CURRENT models/routes -- it is not
// a stand-in for the actual old release's binary, which this in-process test
// cannot invoke. Restoring all the way back to a schema many migrations
// behind current breaks routes that depend on tables/columns added since
// (hand-verified: v0.6.0 through v0.6.11 all 500 on login with "no such
// table: sessions", added later in the chain) -- not a rollback-drill bug,
// just the limit of testing "the old binary" with the new one. The drill
// therefore targets the most recent SupportedReleases entry still behind the
// current schema: the realistic "N" for "N+1 turned out bad, roll back to
// N" (an operator rolling back from an in-development state to their last
// deployed release), and the release closest to current is the one most
// likely to still be servable by today's code -- exactly what makes the HTTP
// exercise meaningful rather than a coin flip on how old the fixture is.
func TestBadReleaseRollbackDrill(t *testing.T) {
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)

	rel, ok := mostRecentSupportedReleaseBelow(latest)
	if !ok {
		t.Skip("every registered SupportedReleases entry is already at the current schema " +
			"(no unreleased migration exists right now) -- there is no real N to roll back to")
	}

	f := Load(t, rel)
	seedMigrationScopeData(t, f) // audit history + the etag/e_tag sync-state trap
	seedRealPasswordHash(t, f)
	pieces := seedFilePieces(t, f)
	username := f.Dataset.User.Username
	liveID := f.Dataset.Contacts[deploy02LiveContact].ID
	liveFirst := f.Dataset.Contacts[deploy02LiveContact].Firstname
	require.NotZero(t, liveID, "fixture must carry the %q contact", deploy02LiveContact)
	require.NotEmpty(t, liveFirst)
	closeFixtureDB(t, f.DB)

	// "You deployed release N+1 ... now N+1 is misbehaving." Lay out the
	// three-piece install and upgrade it in place -- the same DEPLOY-02 path,
	// which is also what fires the #530 pre-migration backup this drill then
	// rolls back to.
	base := t.TempDir()
	dbPath := filepath.Join(base, "mycorrhizal.db")
	require.NoError(t, database.BackupSnapshot(f.Path, dbPath))
	photoDir := filepath.Join(base, "photos")
	attachDir := filepath.Join(base, "attachments")
	copyTree(t, pieces.photoDir, photoDir)
	copyTree(t, pieces.attachDir, attachDir)
	preMigrationDir := filepath.Join(base, "pre-migration")

	_, err = database.InitDB(dbPath)
	require.NoError(t, err, "the %s -> current upgrade (the 'bad release' N+1) must succeed before it can be rolled back", rel.Tag)

	// "Downgrade is not a thing ... restore the snapshot taken before the
	// N->N+1 upgrade" -- steps 2-4 of "Rolling back a bad release": deploy N,
	// restore the database, restore the file directories. Landing here (NOT
	// migrated further, still at N's schema) is exactly the state the runbook
	// reaches right before step 5.
	rdb, restoredPhotos, restoredAttach := restorePreMigrationSnapshot(t, preMigrationDir, dbPath, rel.Version, latest, photoDir, attachDir)
	assertRestoreCompleteness(t, rdb, restoredPhotos, restoredAttach)

	// "5. Start N and confirm with After recovery ... /health/ready is
	// ready." This is the missing HTTP exercise the #997 finding named: not
	// just that the restored rollback instance is a structurally valid
	// database, but that it actually serves the same real workflow
	// exerciseUpgradedApp already proves for the forward-upgrade side.
	exerciseUpgradedApp(t, rdb, username, liveID, liveFirst, restoredPhotos, restoredAttach)
}

// mostRecentSupportedReleaseBelow returns the last release in
// SupportedReleases (release order, which is chronological) whose Version is
// strictly below latest -- the realistic "N" for a "N+1 turned out bad, roll
// back to N" drill: the most recently cut release before the schema in front
// of us right now, not an arbitrary older one. ok is false only when every
// registered release is already at the current schema (nothing to roll back
// FROM today).
func mostRecentSupportedReleaseBelow(latest uint) (rel Release, ok bool) {
	for _, r := range SupportedReleases {
		if r.Version < latest {
			rel, ok = r, true
		}
	}
	return rel, ok
}
