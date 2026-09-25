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

// TestProcessWebhookRetriesOrphanedDeliveryClearsNextRetryAt covers a
// pending delivery whose webhook has since been deactivated: the webhook
// lookup misses (First filters on is_active = true), and the delivery's
// next_retry_at must be cleared so it isn't picked up again forever, rather
// than left set on a delivery that can never be retried.
func TestProcessWebhookRetriesOrphanedDeliveryClearsNextRetryAt(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	whID := seedTestWebhookID(t, db)
	require.NoError(t, db.Model(&models.Webhook{}).Where("id = ?", whID).Update("is_active", false).Error)

	past := time.Now().Add(-time.Minute)
	delivery := models.WebhookDelivery{WebhookID: whID, EventType: "contact.created", Payload: "{}", Attempts: 1, NextRetryAt: &past}
	require.NoError(t, db.Create(&delivery).Error)

	ProcessWebhookRetries(db, config.Config{})

	var got models.WebhookDelivery
	require.NoError(t, db.First(&got, delivery.ID).Error)
	assert.Nil(t, got.NextRetryAt, "an orphaned delivery's next_retry_at must be cleared")
}

// TestProcessWebhookRetriesOrphanedDeliveryUpdateFailureIsLogged pins the
// CLAUDE.md trap #4 fix for this branch: the Update clearing an orphaned
// delivery's next_retry_at can itself fail, and that must be logged rather
// than silently swallowed. There is no return value to assert on here, so
// this pins that ProcessWebhookRetries does not panic and still finishes
// (releases the job lock) even when that Update fails.
func TestProcessWebhookRetriesOrphanedDeliveryUpdateFailureIsLogged(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	whID := seedTestWebhookID(t, db)
	require.NoError(t, db.Model(&models.Webhook{}).Where("id = ?", whID).Update("is_active", false).Error)

	past := time.Now().Add(-time.Minute)
	delivery := models.WebhookDelivery{WebhookID: whID, EventType: "contact.created", Payload: "{}", Attempts: 1, NextRetryAt: &past}
	require.NoError(t, db.Create(&delivery).Error)

	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fail_webhook_deliveries_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "webhook_deliveries" {
			tx.AddError(assert.AnError)
		}
	}))

	require.NotPanics(t, func() {
		ProcessWebhookRetries(db, config.Config{})
	})

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameWebhookRetries).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock should still be released even though the delivery update failed")
}
