package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestMigrationVersion_MatchesHighestFile(t *testing.T) {
	t.Parallel()
	v, err := LatestMigrationVersion()
	require.NoError(t, err)
	// There are at least 37 migrations as of issue #421; the helper must track
	// the highest, so it only ever grows.
	assert.GreaterOrEqual(t, v, uint(37))
}

func TestAppliedMigrationVersion_FreshDBIsAtLatest(t *testing.T) {
	t.Parallel()
	db, err := InitDB(filepath.Join(t.TempDir(), "applied.db"))
	require.NoError(t, err)

	applied, dirty, ok, err := AppliedMigrationVersion(db)
	require.NoError(t, err)
	require.True(t, ok, "a migrated DB has a schema_migrations row")
	assert.False(t, dirty)

	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	assert.Equal(t, latest, applied, "InitDB runs every pending migration")
}

func TestAppliedMigrationVersion_ReportsDirty(t *testing.T) {
	t.Parallel()
	db, err := InitDB(filepath.Join(t.TempDir(), "dirty.db"))
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE schema_migrations SET dirty = 1").Error)

	_, dirty, ok, err := AppliedMigrationVersion(db)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, dirty)
}
