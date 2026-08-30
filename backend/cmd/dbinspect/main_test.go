package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migratedDB builds a real migrated database at path via the app's own
// migration runner (database.MigrateUp), so dbinspect reads the same
// hand-written-schema file the server does.
func migratedDB(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, database.MigrateUp(path))
}

func TestDbPath_PositionalThenEnvThenDefault(t *testing.T) {
	oldArgs := append([]string(nil), os.Args...)
	t.Cleanup(func() { os.Args = oldArgs })
	oldEnv, hadEnv := os.LookupEnv("SQLITE_DB_PATH")
	t.Cleanup(func() {
		if hadEnv {
			os.Setenv("SQLITE_DB_PATH", oldEnv)
		} else {
			os.Unsetenv("SQLITE_DB_PATH")
		}
	})

	// No args, no env -> default.
	os.Unsetenv("SQLITE_DB_PATH")
	os.Args = []string{"dbinspect"}
	path, err := dbPath()
	require.NoError(t, err)
	assert.Equal(t, defaultDBPath, path)

	// No args, env set -> env wins over default.
	os.Setenv("SQLITE_DB_PATH", "/env/path.db")
	os.Args = []string{"dbinspect"}
	path, err = dbPath()
	require.NoError(t, err)
	assert.Equal(t, "/env/path.db", path)

	// One positional arg -> arg wins over env.
	os.Setenv("SQLITE_DB_PATH", "/env/path.db")
	os.Args = []string{"dbinspect", "/arg/path.db"}
	path, err = dbPath()
	require.NoError(t, err)
	assert.Equal(t, "/arg/path.db", path)

	// More than one arg -> usage error.
	os.Args = []string{"dbinspect", "/a.db", "/b.db"}
	_, err = dbPath()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "usage")
}

// TestRun_ReportsCleanLatest asserts the happy-path output the chaos harness
// greps for on a recovered database.
func TestRun_ReportsCleanLatest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clean.db")
	migratedDB(t, path)

	line, err := run(path)
	require.NoError(t, err)

	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	assert.Contains(t, line, "integrity_check=ok")
	assert.Contains(t, line, "dirty=false")
	assert.Contains(t, line, "version="+strconv.FormatUint(uint64(latest), 10))
}

// TestRun_ReportsDirty asserts a dirty database is reported, not failed — the
// state the chaos harness asserts before restart.
func TestRun_ReportsDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dirty.db")
	migratedDB(t, path)

	sqlDB, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = sqlDB.Exec("UPDATE schema_migrations SET dirty = 1")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	line, err := run(path)
	require.NoError(t, err)
	assert.Contains(t, line, "dirty=true", "a dirty flag is reported, not errored")
}

func TestRun_FailsOnMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.db")
	_, err := run(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open")
}

func TestRun_FailsOnCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.db")
	require.NoError(t, database.MigrateUp(path))

	// Truncate to the SQLite header only: a real file a partial write (disk
	// full, torn copy) would leave behind. It must be rejected — the corrupt-
	// database failure class the chaos harness must not mistake for clean.
	header := make([]byte, 512)
	src, err := os.Open(path)
	require.NoError(t, err)
	_, err = src.Read(header)
	require.NoError(t, src.Close())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, header, 0o600))

	_, err = run(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integrity", "a corrupt database must be rejected at the integrity check")
}

func TestRun_FailsOnNeverMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	// A 0-byte file opens as an empty (valid) SQLite database, so the
	// integrity check passes but there is no schema_migrations row.
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	_, err := run(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema_migrations")
}

func TestRun_DoesNotCloseTheDatabasePrematurely(t *testing.T) {
	// Guard against a regression where run closes its own handle and then
	// MigrationVersion re-opens it — both must work on the same file.
	path := filepath.Join(t.TempDir(), "reopen.db")
	migratedDB(t, path)

	line, err := run(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(line, "integrity_check=ok"))
}

// TestRunCLIExitCodes covers the three runCLI paths: success (0), a
// database error (1), and a usage error (2).
func TestRunCLIExitCodes(t *testing.T) {
	oldArgs := append([]string(nil), os.Args...)
	t.Cleanup(func() { os.Args = oldArgs })
	oldEnv, hadEnv := os.LookupEnv("SQLITE_DB_PATH")
	t.Cleanup(func() {
		if hadEnv {
			os.Setenv("SQLITE_DB_PATH", oldEnv)
		} else {
			os.Unsetenv("SQLITE_DB_PATH")
		}
	})

	// Usage error -> 2.
	os.Setenv("SQLITE_DB_PATH", "")
	os.Args = []string{"dbinspect", "/a.db", "/b.db"}
	assert.Equal(t, 2, runCLI(), "a usage error must exit 2")

	// Missing file -> 1.
	os.Args = []string{"dbinspect", filepath.Join(t.TempDir(), "nope.db")}
	assert.Equal(t, 1, runCLI(), "an uninspectable database must exit 1")

	// Clean migrated database -> 0.
	path := filepath.Join(t.TempDir(), "cli.db")
	migratedDB(t, path)
	os.Args = []string{"dbinspect", path}
	assert.Equal(t, 0, runCLI(), "a clean migrated database must exit 0")
}
