package main

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/database"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCLILogger_DefaultAndEnv(t *testing.T) {
	oldLevel, hadLevel := os.LookupEnv("LOG_LEVEL")
	oldPretty, hadPretty := os.LookupEnv("LOG_PRETTY")
	t.Cleanup(func() {
		if hadLevel {
			os.Setenv("LOG_LEVEL", oldLevel)
		} else {
			os.Unsetenv("LOG_LEVEL")
		}
		if hadPretty {
			os.Setenv("LOG_PRETTY", oldPretty)
		} else {
			os.Unsetenv("LOG_PRETTY")
		}
	})

	// No env -> info level (the documented default), no panic.
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("LOG_PRETTY")
	initCLILogger()
	assert.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())

	// LOG_LEVEL=debug -> debug level.
	require.NoError(t, os.Setenv("LOG_LEVEL", "debug"))
	initCLILogger()
	assert.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())

	// LOG_PRETTY=1 is accepted (true/1).
	require.NoError(t, os.Setenv("LOG_PRETTY", "1"))
	initCLILogger()
	assert.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())
}

func TestRun_UpAppliesMigrations(t *testing.T) {
	t.Setenv("SQLITE_DB_PATH", filepath.Join(t.TempDir(), "up.db"))
	require.NoError(t, run("up"))

	version, dirty, ok, err := database.MigrationVersion(dbPath())
	require.NoError(t, err)
	require.True(t, ok)
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	assert.Equal(t, latest, version)
	assert.False(t, dirty)
}

func TestRun_DownRollsBackExactlyOne(t *testing.T) {
	t.Setenv("SQLITE_DB_PATH", filepath.Join(t.TempDir(), "down.db"))
	require.NoError(t, run("up"))

	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	require.Greater(t, latest, uint(0))

	require.NoError(t, run("down"))
	version, dirty, ok, err := database.MigrationVersion(dbPath())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, latest-1, version, "down rolls back exactly one migration")
	assert.False(t, dirty)
}

func TestRun_VersionOnNeverMigratedDatabase(t *testing.T) {
	t.Setenv("SQLITE_DB_PATH", filepath.Join(t.TempDir(), "version.db"))
	require.NoError(t, run("version"), "version on an unmigrated database is not an error")
}

func TestRun_UnknownCommandErrors(t *testing.T) {
	t.Setenv("SQLITE_DB_PATH", filepath.Join(t.TempDir(), "x.db"))
	err := run("frobnicate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

func TestDbPath_EnvThenDefault(t *testing.T) {
	t.Setenv("SQLITE_DB_PATH", "/custom/path.db")
	assert.Equal(t, "/custom/path.db", dbPath())

	oldEnv, hadEnv := os.LookupEnv("SQLITE_DB_PATH")
	t.Cleanup(func() {
		if hadEnv {
			os.Setenv("SQLITE_DB_PATH", oldEnv)
		} else {
			os.Unsetenv("SQLITE_DB_PATH")
		}
	})
	os.Unsetenv("SQLITE_DB_PATH")
	assert.Equal(t, defaultDBPath, dbPath())
}

// markDirty migrates dbPath fully, then marks the schema_migrations row dirty
// at the latest version — the crash signature `force` exists to recover, set
// at a version where the force can complete without re-applying DDL (the
// re-run behavior itself is pinned at the database layer).
func markDirty(t *testing.T, dbPath string) {
	t.Helper()
	require.NoError(t, database.MigrateUp(dbPath))
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	sqlDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer sqlDB.Close()
	_, err = sqlDB.Exec("UPDATE schema_migrations SET version = ?, dirty = 1", latest)
	require.NoError(t, err)
}

// runWithStdin runs the CLI with the given reader attached to the force
// command's confirmation prompt.
func runWithStdin(command string, in io.Reader) error {
	old := stdin
	stdin = in
	defer func() { stdin = old }()
	return run(command)
}

// TestRun_ForceRefusesWithoutExplicitConfirmation pins that the operator-only
// escape hatch cannot be triggered non-interactively by accident: anything
// other than "yes" aborts and leaves the dirty database untouched.
func TestRun_ForceRefusesWithoutExplicitConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "force-noconsent.db")
	markDirty(t, path)
	t.Setenv("SQLITE_DB_PATH", path)

	require.Error(t, runWithStdin("force", strings.NewReader("nope\n")))
	require.Error(t, runWithStdin("force", strings.NewReader("\n")))

	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	version, dirty, ok, err := database.MigrationVersion(path)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, latest, version, "an unconfirmed force must not touch the database")
	assert.True(t, dirty)
}

// TestRun_ForceRecoversDirtyDatabaseAfterConfirmation pins the full operator
// path: the prompt explains the state, an explicit "yes" runs the force, and
// the database lands clean at the latest schema.
func TestRun_ForceRecoversDirtyDatabaseAfterConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "force-consent.db")
	markDirty(t, path)
	t.Setenv("SQLITE_DB_PATH", path)

	require.NoError(t, runWithStdin("force", strings.NewReader("yes\n")))

	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	version, dirty, ok, err := database.MigrationVersion(path)
	require.NoError(t, err)
	require.True(t, ok)
	assert.EqualValues(t, latest, version, "a confirmed force must recover to the latest schema")
	assert.False(t, dirty)
}

// TestRun_ForceOnCleanDatabaseErrors pins that force refuses up front when
// there is nothing to force — a clean database is not a force candidate.
func TestRun_ForceOnCleanDatabaseErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "force-clean.db")
	t.Setenv("SQLITE_DB_PATH", path)
	require.NoError(t, run("up"))

	err := runWithStdin("force", strings.NewReader("yes\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not dirty")
}
