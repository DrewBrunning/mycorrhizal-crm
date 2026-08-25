package main

import (
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

func TestDbPath_DefaultAndEnv(t *testing.T) {
	require.Equal(t, defaultDBPath, dbPath(), "dbPath must default to the server's SQLITE_DB_PATH default")
	t.Setenv("SQLITE_DB_PATH", "/tmp/custom.db")
	require.Equal(t, "/tmp/custom.db", dbPath())
}

// newVerifyDB builds a real migrated database at path with a seeded, intact
// audit chain (one login event backfilled via RecomputeAuditChain).
func newVerifyDB(t *testing.T, path string) {
	t.Helper()
	db, err := database.InitDB(path)
	require.NoError(t, err)
	user := models.User{Username: "verify", Password: "password123!A", Email: "verify@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.AuditEvent{
		EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin, UserID: user.ID,
	}).Error)
	require.NoError(t, models.RecomputeAuditChain(db))
}

func TestRun_IntactChainExitsZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intact.db")
	newVerifyDB(t, path)
	require.Equal(t, 0, run(path), "an intact chain must exit 0")
}

func TestRun_GapExitsOne(t *testing.T) {
	// A row with no hash (pre-backfill) is the first gap the verifier reports.
	path := filepath.Join(t.TempDir(), "gap.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	user := models.User{Username: "verifygap", Password: "password123!A", Email: "verifygap@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.AuditEvent{
		EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin, UserID: user.ID,
	}).Error)
	require.Equal(t, 1, run(path), "a backfill-pending row must exit 1")
}

func TestRun_OpenFailureExitsOne(t *testing.T) {
	require.Equal(t, 1, run(filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")),
		"an unopenable database must exit 1")
}

func TestRun_VerifyFailureExitsTwo(t *testing.T) {
	// A database whose audit_events read fails (table dropped) is a check
	// failure, not a gap: exit 2.
	path := filepath.Join(t.TempDir(), "verify-fail.db")
	newVerifyDB(t, path)
	db, err := database.InitDB(path)
	require.NoError(t, err)
	require.NoError(t, db.Exec("DROP TABLE audit_events").Error)
	require.Equal(t, 2, run(path), "a failed verification must exit 2")
}
