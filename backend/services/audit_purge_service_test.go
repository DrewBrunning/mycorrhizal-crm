package services

import (
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPurgeExpiredAuditEvents pins the retention job: rows older than
// AUDIT_RETENTION_DAYS are removed, newer rows survive, and a non-positive
// retention disables purging entirely (never deletes everything).
func TestPurgeExpiredAuditEvents(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit-purge.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)

	user := models.User{Username: "auditpurge", Password: "password123!A", Email: "auditpurge@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Two events with controlled ages, inserted directly (the recorder writes
	// "now", which makes the retention window hard to pin).
	old := models.AuditEvent{EntityType: "contact", EntityID: "old-1", Operation: "create", UserID: user.ID, CreatedAt: time.Now().AddDate(0, 0, -40), UpdatedAt: time.Now()}
	new := models.AuditEvent{EntityType: "contact", EntityID: "new-1", Operation: "create", UserID: user.ID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Create(&new).Error)

	// Retention 30 days: the 40-day-old row goes, the fresh one stays.
	PurgeExpiredAuditEvents(db, config.Config{AuditRetentionDays: 30})

	var remaining []models.AuditEvent
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, "new-1", remaining[0].EntityID)

	// Retention <= 0 disables purging entirely.
	require.NoError(t, db.Create(&models.AuditEvent{EntityType: "contact", EntityID: "old-2", Operation: "create", UserID: user.ID, CreatedAt: time.Now().AddDate(0, 0, -100), UpdatedAt: time.Now()}).Error)
	PurgeExpiredAuditEvents(db, config.Config{AuditRetentionDays: 0})
	var all []models.AuditEvent
	require.NoError(t, db.Find(&all).Error)
	assert.Len(t, all, 2, "a non-positive retention must disable purging, not delete everything")
}

// TestPurgeExpiredAuditEvents_RelinksHashChain pins issue #381's purge
// contract: the retention job is the sanctioned DELETE that breaks the
// tamper-evident hash chain at its head, so after purging the survivors must
// verify as a clean chain again (RecomputeAuditChain re-links them).
func TestPurgeExpiredAuditEvents_RelinksHashChain(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit-purge-chain.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() {
		models.AuditFlush()
		models.RegisterAuditDB(nil)
	})

	user := models.User{Username: "auditpurgechain", Password: "password123!A", Email: "auditpurgechain@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Build a chain from directly-inserted rows with controlled ages (the
	// recorder writes "now", which makes the retention window hard to pin).
	// The immutability trigger rejects UPDATE, so age via the insert, not a
	// backdate.
	events := []models.AuditEvent{
		{EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin, UserID: user.ID, CreatedAt: time.Now().AddDate(0, 0, -40).UTC()},
		{EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin, UserID: user.ID, CreatedAt: time.Now().AddDate(0, 0, -40).UTC()},
		{EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin, UserID: user.ID, CreatedAt: time.Now().UTC()},
		{EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin, UserID: user.ID, CreatedAt: time.Now().UTC()},
	}
	for _, e := range events {
		require.NoError(t, db.Create(&e).Error)
	}
	require.NoError(t, models.RecomputeAuditChain(db))
	gaps, err := models.VerifyAuditChain(db)
	require.NoError(t, err)
	assert.Empty(t, gaps, "the seeded chain must verify clean before the purge")

	PurgeExpiredAuditEvents(db, config.Config{AuditRetentionDays: 30})

	var remaining []models.AuditEvent
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 2, "the two aged-out head rows must be purged")

	gaps, err = models.VerifyAuditChain(db)
	require.NoError(t, err)
	assert.Empty(t, gaps, "the purge must re-link the survivors into a clean hash chain")

	var head models.AuditEvent
	require.NoError(t, db.Order("id asc").First(&head).Error)
	assert.Empty(t, head.PrevHash, "the new head must restart the chain from genesis")
}

// TestPurgeExpiredAuditEvents_RecomputeFailureIsSwallowed pins that a failure
// to re-link the hash chain after a purge is logged and swallowed, never a
// panic: the retention delete already happened, so losing the re-link must not
// take the whole purge down.
func TestPurgeExpiredAuditEvents_RecomputeFailureIsSwallowed(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit-purge-fail.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)
	t.Cleanup(func() {
		models.AuditFlush()
		models.RegisterAuditDB(nil)
	})

	user := models.User{Username: "purgefail", Password: "password123!A", Email: "purgefail@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.AuditEvent{
		EntityType: models.AuditEntityAuth, EntityID: "alice", Operation: models.AuditOpLogin,
		UserID: user.ID, CreatedAt: time.Now().AddDate(0, 0, -40).UTC(),
	}).Error)
	require.NoError(t, db.Create(&models.AuditEvent{
		EntityType: models.AuditEntityAuth, EntityID: "bob", Operation: models.AuditOpLogin,
		UserID: user.ID, CreatedAt: time.Now().UTC(),
	}).Error)

	// A second BEFORE UPDATE trigger makes the re-link fail: RecomputeAuditChain
	// only drops its own audit_events_no_update trigger, so this one survives
	// and rejects the hash/prev_hash UPDATE. The purge must still delete the
	// aged-out row and continue without panicking.
	require.NoError(t, db.Exec("CREATE TRIGGER audit_events_block_update "+
		"BEFORE UPDATE ON audit_events BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error)

	PurgeExpiredAuditEvents(db, config.Config{AuditRetentionDays: 30})

	var remaining []models.AuditEvent
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1, "the aged-out row must still be purged even though re-linking failed")
	assert.Equal(t, "bob", remaining[0].EntityID)
}
