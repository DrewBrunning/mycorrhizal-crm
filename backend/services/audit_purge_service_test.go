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
