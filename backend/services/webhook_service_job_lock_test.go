package services

import (
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupWebhookRetryTestDB returns the real migrated schema with users 1 and 2
// seeded, so webhook fixtures (newTestWebhook's UserID 1, the fan-out test's
// userID+1) satisfy the real webhooks.user_id foreign key.
func setupWebhookRetryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	for i := 1; i <= 2; i++ {
		u := models.User{Username: fmt.Sprintf("webhook-user-%d", i), Email: fmt.Sprintf("webhook-user-%d@example.com", i), Password: "x"}
		require.NoError(t, db.Create(&u).Error)
		require.EqualValues(t, i, u.ID, "fresh schema assigns sequential user IDs")
	}
	return db
}

// seedTestWebhookID persists a webhook for user 1 and returns its ID, for
// tests that write deliveries directly (webhook_deliveries.webhook_id is a
// real foreign key).
func seedTestWebhookID(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	wh := newTestWebhook("https://receiver.example.com/hook", "secret")
	require.NoError(t, db.Create(&wh).Error)
	return wh.ID
}

// TestProcessWebhookRetriesSkipsWhenLocked is the regression test
// ProcessWebhookRetries
// previously had no job lock at all, unlike reminders/calendar sync, so
// multiple instances could double-process the same retry window.
func TestProcessWebhookRetriesSkipsWhenLocked(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	cfg := config.Config{}

	require.NoError(t, db.Create(&models.JobExecution{
		JobName:   models.JobNameWebhookRetries,
		LastRunAt: time.Now(),
	}).Error)

	require.NotPanics(t, func() {
		ProcessWebhookRetries(db, cfg)
	})

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameWebhookRetries).First(&job).Error)
	assert.Nil(t, job.LockedAt, "job must not have been re-locked while rate-limited")
}

// TestProcessWebhookRetriesAcquiresAndReleasesLock covers the opposite path:
// no lock row exists yet, so the job runs and the lock is released afterward
// (LastRunAt bumped, LockedAt cleared) rather than left held.
func TestProcessWebhookRetriesAcquiresAndReleasesLock(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	cfg := config.Config{}

	require.NotPanics(t, func() {
		ProcessWebhookRetries(db, cfg)
	})

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameWebhookRetries).First(&job).Error)
	assert.WithinDuration(t, time.Now(), job.LastRunAt, 5*time.Second)
	assert.Nil(t, job.LockedAt, "lock should be released after the job completes")
}
