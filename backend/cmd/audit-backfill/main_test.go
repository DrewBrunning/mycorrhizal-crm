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

func TestRun_BackfillsLegacyRows(t *testing.T) {
	// Seed rows the way pre-000034 code wrote them: no hash. After run(), the
	// chain must be complete and verify clean.
	path := filepath.Join(t.TempDir(), "backfill.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	user := models.User{Username: "backfill", Password: "password123!A", Email: "backfill@example.com"}
	require.NoError(t, db.Create(&user).Error)
	legacy := []models.AuditEvent{
		{EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin, UserID: user.ID},
		{EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLoginFailed, UserID: user.ID},
	}
	for _, e := range legacy {
		require.NoError(t, db.Create(&e).Error)
	}

	gaps, err := models.VerifyAuditChain(db)
	require.NoError(t, err)
	require.NotEmpty(t, gaps, "the seeded rows must be backfill-pending before run()")

	require.NoError(t, run(path))

	gaps, err = models.VerifyAuditChain(db)
	require.NoError(t, err)
	require.Empty(t, gaps, "run() must complete the chain")
}

func TestRun_OpenFailure(t *testing.T) {
	err := run(filepath.Join(t.TempDir(), "no", "such", "dir", "x.db"))
	require.Error(t, err)
}
