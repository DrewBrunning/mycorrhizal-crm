package schemafixture

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// populatedSourceDB builds a current-schema scratch database populated with
// the canonical manifest plus one distinctive probe user, and closes its GORM
// handle on cleanup. It is the source side of TransplantDataToVersion: the
// same population Load uses, but handed to the caller as a *gorm.DB so the
// transplant primitive (not just the loader) can be driven directly.
func populatedSourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	srcPath := filepath.Join(t.TempDir(), "transplant-src.db")
	srcDB, err := database.InitDB(srcPath)
	require.NoError(t, err, "a fresh current-schema scratch database must migrate")
	t.Cleanup(func() { closeFixtureDB(t, srcDB) })

	m, err := canonicalfixture.Read()
	require.NoError(t, err, "the committed canonical manifest must load")
	_, err = canonicalfixture.Populate(srcDB, m)
	require.NoError(t, err, "the manifest must populate the current-schema source")

	probe := models.User{Username: "transplant-probe", Password: "password123!A", Email: "transplant-probe@example.com"}
	require.NoError(t, srcDB.Create(&probe).Error)
	return srcDB
}

// TestTransplantDataToVersionCopiesDataIntoHistoricalSchema is the happy path
// of the upgrade-matrix fixture builder (issue #436/#495): data populated at
// the CURRENT schema must transplant into a fresh database built to exactly
// the floor release's version, preserving every row that table exists at that
// version and dropping only the columns added by later migrations.
func TestTransplantDataToVersionCopiesDataIntoHistoricalSchema(t *testing.T) {
	src := populatedSourceDB(t)
	floor := SupportedReleases[0]
	outPath := filepath.Join(t.TempDir(), "transplanted.db")

	require.NoError(t, TransplantDataToVersion(src, floor.Version, outPath))

	// The output presents exactly the requested version, clean.
	version, dirty, ok, err := database.MigrationVersion(outPath)
	require.NoError(t, err)
	require.True(t, ok, "the transplanted database must carry a schema_migrations row")
	assert.EqualValues(t, floor.Version, version, "the transplant must land on the requested version")
	assert.False(t, dirty, "the transplanted database must not be dirty")
	assertIntegrity(t, outPath, "ok")

	out := databaseMustOpen(t, outPath)
	defer closeFixtureDB(t, out)

	// Every table present in both schemas keeps its row count — the copy is
	// lossless for the version-appropriate subset.
	before := tableCounts(t, src)
	after := tableCounts(t, out)
	compared := 0
	for table, want := range before {
		got, exists := after[table]
		if !exists {
			// Table added after the release: no home in this fixture.
			continue
		}
		assert.EqualValuesf(t, want, got, "table %s must carry the same rows after the transplant", table)
		compared++
	}
	assert.Positive(t, compared, "the transplant must have copied at least one table")

	// A specific authored row round-trips: the probe user is present with the
	// exact username and email it was created with.
	var count int64
	require.NoError(t, out.Raw("SELECT count(*) FROM users WHERE username = ?", "transplant-probe").Scan(&count).Error)
	assert.EqualValues(t, 1, count, "the probe user must survive the transplant")
	var email string
	require.NoError(t, out.Raw("SELECT email FROM users WHERE username = ?", "transplant-probe").Scan(&email).Error)
	assert.Equal(t, "transplant-probe@example.com", email, "the probe user's email must round-trip")

	// Version-appropriate subsetting: a column added AFTER the floor version
	// is absent, while a column that predates it is present. life_events.end_date
	// is migration 000058 and preferences.level is 000063, both above the
	// v1.0.0 floor (migration 57).
	lifeEventCols := tableColumnsOf(t, out, "life_events")
	assert.Contains(t, lifeEventCols, "date", "life_events.date predates the floor and must be copied")
	assert.NotContains(t, lifeEventCols, "end_date", "life_events.end_date was added after the floor and must be absent")
	assert.NotContains(t, tableColumnsOf(t, out, "preferences"), "level", "preferences.level was added after the floor and must be absent")
}

// TestTransplantDataToVersionOutputIsSelfContained pins the checkpoint contract
// the large-dataset harness depends on: after the call the WAL is folded into
// the main file, so copying ONLY the main file yields a complete, readable
// database with the copied data intact.
func TestTransplantDataToVersionOutputIsSelfContained(t *testing.T) {
	src := populatedSourceDB(t)
	outPath := filepath.Join(t.TempDir(), "transplanted.db")
	require.NoError(t, TransplantDataToVersion(src, SupportedReleases[0].Version, outPath))

	contents, err := os.ReadFile(outPath)
	require.NoError(t, err)
	forkPath := filepath.Join(t.TempDir(), "fork.db")
	require.NoError(t, os.WriteFile(forkPath, contents, 0o600))

	db := databaseMustOpen(t, forkPath)
	defer closeFixtureDB(t, db)

	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM users WHERE username = ?", "transplant-probe").Scan(&count).Error)
	assert.EqualValues(t, 1, count, "the main file alone must carry the copied data")
}

// TestTransplantDataToVersionExtractDataFailure covers the first error wrap:
// when the source cannot be read, the function must fail before it creates or
// touches the output file. Closing the source's underlying pool makes its
// sqlite_master read fail without the source being a valid database.
func TestTransplantDataToVersionExtractDataFailure(t *testing.T) {
	srcPath := filepath.Join(t.TempDir(), "broken-src.db")
	srcDB, err := database.InitDB(srcPath)
	require.NoError(t, err)
	sqlDB, err := srcDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close(), "closing the source pool must succeed")

	outPath := filepath.Join(t.TempDir(), "never-created.db")
	err = TransplantDataToVersion(srcDB, SupportedReleases[0].Version, outPath)
	require.Error(t, err, "an unreadable source must fail the transplant")
	assert.NoFileExists(t, outPath, "the failure must land before the output schema is built")
}

// TestTransplantDataToVersionBuildSchemaFailure covers the second error wrap:
// when the output path cannot host the schema, the returned error names the
// version being built. A path whose parent directory does not exist cannot be
// opened by the migration chain.
func TestTransplantDataToVersionBuildSchemaFailure(t *testing.T) {
	srcPath := filepath.Join(t.TempDir(), "ok-src.db")
	srcDB, err := database.InitDB(srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { closeFixtureDB(t, srcDB) })

	floor := SupportedReleases[0]
	outPath := filepath.Join(t.TempDir(), "missing-parent", "fixture.db")
	err = TransplantDataToVersion(srcDB, floor.Version, outPath)
	require.Error(t, err, "an unbuildable output path must fail the transplant")
	assert.Contains(t, err.Error(), fmt.Sprintf("building version %d schema", floor.Version),
		"the wrap must name the version whose schema failed to build")
	assert.NoFileExists(t, outPath)
}
