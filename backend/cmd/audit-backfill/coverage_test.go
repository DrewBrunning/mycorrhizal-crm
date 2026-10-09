package main

import (
	"path/filepath"
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDbPathEmptyEnvFallsBackToDefault pins the unset-or-empty branch: an
// explicitly empty SQLITE_DB_PATH must resolve to the server's default, not to
// an empty path (which would make the CLI open a database named "").
func TestDbPathEmptyEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("SQLITE_DB_PATH", "")
	assert.Equal(t, defaultDBPath, dbPath())
}

// TestRunOpenFailureNamesThePath strengthens the open-failure coverage: the
// error must be the InitDB wrap naming the offending path, so an operator can
// tell which database could not be opened.
func TestRunOpenFailureNamesThePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")
	err := run(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open database "+path)
}

// TestRunRecomputeFailureIsWrapped covers the run() failure wrap around the
// audit-chain recompute: when the chain cannot be read, run must return the
// distinct backfill error rather than a nil success. The audit_events table is
// dropped after InitDB so the database opens cleanly but the recompute read
// fails.
func TestRunRecomputeFailureIsWrapped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken-chain.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	require.NoError(t, db.Exec("DROP TABLE audit_events").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = run(path)
	require.Error(t, err, "a database without audit_events must fail the backfill")
	assert.Contains(t, err.Error(), "audit hash chain backfill failed")
	assert.Contains(t, err.Error(), "audit chain: read events")
}
