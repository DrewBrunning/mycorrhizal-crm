package schemafixture

import (
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// closeFixtureDB closes a fixture's open GORM handle so a subsequent migration
// of the underlying file has no stale connection; callers reopen with
// database.OpenMigratedFile.
func closeFixtureDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// migrateFixtureTo migrates the fixture's database file to exactly target and
// returns a reopened GORM handle, asserting the fixture's row counts survive.
func migrateFixtureTo(t *testing.T, f *Fixture, target uint) *gorm.DB {
	t.Helper()
	before := tableCounts(t, f.DB)
	closeFixtureDB(t, f.DB)

	require.NoError(t, database.MigrateUpTo(f.Path, target),
		"upgrading the %s fixture to version %d must succeed", f.Release.Tag, target)

	db, err := database.OpenMigratedFile(f.Path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	version, dirty, ok, err := database.AppliedMigrationVersion(db)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, target, version)
	assert.False(t, dirty)

	after := tableCounts(t, db)
	for table, want := range before {
		assert.Equalf(t, want, after[table],
			"upgrading %s -> version %d must preserve %s row counts", f.Release.Tag, target, table)
	}
	return db
}

// TestUpgradeEachAdjacentHop is issue #529's "the longest supported skip is
// tested, not just adjacent hops" companion: every ADJACENT hop in the
// supported range runs and preserves data, so a failure identifies the exact
// release whose migration broke the chain. Each hop migrates a fixture of the
// older release only as far as the next release's schema (the delta a real
// operator stepping releases would hit), asserting row counts survive.
func TestUpgradeEachAdjacentHop(t *testing.T) {
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)

	for i := 0; i+1 < len(SupportedReleases); i++ {
		from := SupportedReleases[i]
		to := SupportedReleases[i+1]
		t.Run(from.Tag+"->"+to.Tag, func(t *testing.T) {
			f := Load(t, from)
			migrateFixtureTo(t, f, to.Version)
		})
	}

	// The final adjacent hop is the newest release -> current.
	last := SupportedReleases[len(SupportedReleases)-1]
	t.Run(last.Tag+"->current", func(t *testing.T) {
		f := Load(t, last)
		migrateFixtureTo(t, f, latest)
	})
}

// TestUpgradeLongestSkip is issue #529's headline upgrade shape: a v0.6.0
// fixture upgraded DIRECTLY to the current schema (the path a real operator
// takes after ignoring updates) — through the production entry point, not the
// stepwise migrator — with data intact.
func TestUpgradeLongestSkip(t *testing.T) {
	f := Load(t, SupportedReleases[0])

	before := tableCounts(t, f.DB)
	closeFixtureDB(t, f.DB)

	db, err := database.InitDB(f.Path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	version, dirty, ok, err := database.AppliedMigrationVersion(db)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, latest, version, "the longest supported skip must land on the current schema")
	assert.False(t, dirty)

	after := tableCounts(t, db)
	for table, want := range before {
		assert.Equalf(t, want, after[table],
			"the v0.6.0 -> current skip must preserve %s row counts", table)
	}
}

// TestUpgradeLeavesSearchConsistent mirrors DEPLOY-02's "search returns
// pre-upgrade contacts after the upgrade": after the longest skip, the FTS
// index must still resolve a contact that was loaded into the fixture.
func TestUpgradeLeavesSearchConsistent(t *testing.T) {
	f := Load(t, SupportedReleases[0])
	gina := f.Dataset.Contacts["gina"]

	closeFixtureDB(t, f.DB)
	db, err := database.InitDB(f.Path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	// gina is soft-deleted, so she must NOT be findable; her live counterpart
	// under the same vcard_uid (julie) must be.
	var julieHits, ginaHits int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM contacts_fts
		WHERE contacts_fts MATCH 'gina*' AND user_id = ?`, f.Dataset.User.ID).Scan(&ginaHits).Error)
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM contacts_fts
		WHERE contacts_fts MATCH 'julie*' AND user_id = ?`, f.Dataset.User.ID).Scan(&julieHits).Error)
	assert.Positive(t, julieHits, "a live contact must be searchable after the upgrade")
	assert.Zero(t, ginaHits, "a soft-deleted contact must not be searchable after the upgrade")

	var live string
	require.NoError(t, db.Raw("SELECT firstname FROM contacts WHERE vcard_uid = ? AND deleted_at IS NULL", gina.VCardUID).Scan(&live).Error)
	assert.NotEmpty(t, live, "julie's recreated contact must still exist after the upgrade")
}
