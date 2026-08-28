package main

import (
	"os"
	"path/filepath"
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
