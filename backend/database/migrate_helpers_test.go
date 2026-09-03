package database

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenMigratedFileOpensWithoutMigrating covers OpenMigratedFile: a file
// that InitDB already migrated opens through the standard pragma DSN without
// re-running migrations, and a bogus path fails cleanly.
func TestOpenMigratedFileOpensWithoutMigrating(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "premigrated.db")
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	reopened, err := OpenMigratedFile(dbPath)
	require.NoError(t, err)

	var count int64
	require.NoError(t, reopened.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'",
	).Scan(&count).Error)
	assert.EqualValues(t, 1, count, "the reopened file must still carry the migrated schema")
	sqlDB2, err := reopened.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB2.Close())
}

func TestOpenMigratedFileRejectsBogusPath(t *testing.T) {
	t.Parallel()
	// A path inside a directory that does not exist: SQLite refuses to create
	// the file (unlike a missing file at an existing path, which it creates).
	_, err := OpenMigratedFile(filepath.Join(t.TempDir(), "no-such-dir", "x.db"))
	assert.Error(t, err, "opening a database in a nonexistent directory must fail")
}

// TestInitDBReportsMigrationFailure covers InitDB's RunMigrations error path:
// a database path whose parent directory does not exist cannot be migrated, so
// InitDB must fail rather than returning a half-migrated connection.
func TestInitDBReportsMigrationFailure(t *testing.T) {
	t.Parallel()
	_, err := InitDB(filepath.Join(t.TempDir(), "no-such-dir", "x.db"))
	assert.Error(t, err, "a migration failure must surface from InitDB")
}

// TestMigrationVersionOnUnmigratedDB pins the "no migration ever applied"
// signal: ok=false, not an error.
func TestMigrationVersionOnUnmigratedDB(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	assert.False(t, ok, "an unmigrated database must report ok=false")
	assert.Zero(t, version)
	assert.False(t, dirty)
}

func TestMigrationVersionOnMigratedDB(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "migrated-version.db")
	require.NoError(t, MigrateUp(dbPath))

	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, dirty)

	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	assert.Equal(t, latest, version)
}

func TestMigrationVersionReportsBrokenDB(t *testing.T) {
	t.Parallel()
	// A path whose parent does not exist makes sql.Open succeed lazily but
	// every query fail, so newMigrator's Ping surfaces the error.
	_, _, _, err := MigrationVersion(filepath.Join(t.TempDir(), "nope", "x.db"))
	assert.Error(t, err)
}

// TestAppliedMigrationVersionEmptyTableReturnsOkFalse covers the documented
// "no migration ever applied" read: with the schema_migrations table present
// but empty (the state a freshly-created version table is in), the helper
// reports ok=false with no error rather than failing.
func TestAppliedMigrationVersionEmptyTableReturnsOkFalse(t *testing.T) {
	t.Parallel()
	db, err := InitDB(filepath.Join(t.TempDir(), "empty-table.db"))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	// Wipe the version table's single row so it is present-but-empty, exactly
	// the state ensureVersionTable leaves it in before golang-migrate writes
	// the first version.
	require.NoError(t, db.Exec("DELETE FROM schema_migrations").Error)

	version, dirty, ok, err := AppliedMigrationVersion(db)
	require.NoError(t, err)
	assert.False(t, ok, "an empty schema_migrations table must read as never-migrated")
	assert.Zero(t, version)
	assert.False(t, dirty)
}

func TestAppliedMigrationVersionReportsQueryError(t *testing.T) {
	t.Parallel()
	db, err := InitDB(filepath.Join(t.TempDir(), "applied-err.db"))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close()) // subsequent queries fail

	_, _, _, err = AppliedMigrationVersion(db)
	assert.Error(t, err, "a failed query must surface from AppliedMigrationVersion")
}

// TestMigrationFileForVersion covers migrationFileForVersion's mapping between
// a version number and its NNNNNN_name.up.sql filename, plus its miss paths
// (unknown version, non-numeric / non-up entries silently skipped).
func TestMigrationFileForVersion(t *testing.T) {
	t.Parallel()
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)

	name := migrationFileForVersion(latest)
	require.NotEmpty(t, name, "the latest bundled migration must resolve to a filename")
	assert.True(t, strings.HasSuffix(name, ".up.sql"))

	assert.Empty(t, migrationFileForVersion(0), "version 0 does not exist in the chain")
	assert.Empty(t, migrationFileForVersion(999999), "an unknown version must resolve to empty")
}
