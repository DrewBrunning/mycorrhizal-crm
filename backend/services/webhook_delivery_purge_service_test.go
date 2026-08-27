package services

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #622: webhook_deliveries carries a plaintext copy of the full
// serialized entity that triggered each event (a `contact.created` delivery
// holds the whole contact record), and before this ticket the only thing that
// ever removed a row was cascade-on-parent (account deletion / the webhook
// row's FK). These tests pin the WEBHOOK_DELIVERY_RETENTION_DAYS window
// (default 30), anchored on created_at — the moment of the delivery attempt.
//
// Real migrated schema (database.InitDB via dbtest), not AutoMigrate, per
// CLAUDE.md trap #1 — the raw SQL names the real webhook_deliveries columns
// and FKs (foreign_keys(1) is on, so a delivery needs a real parent webhook).

func newWebhookDeliveryPurgeDB(t *testing.T) (*gorm.DB, uint) {
	t.Helper()

	db := dbtest.New(t)

	user := models.User{Username: "wh-purge", Email: "wh-purge@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	wh := models.Webhook{
		UserID:   user.ID,
		Name:     "purge-hook",
		URL:      "https://example.com/hook",
		Events:   []string{"contact.created"},
		Secret:   "s",
		IsActive: true,
	}
	require.NoError(t, db.Create(&wh).Error)
	return db, wh.ID
}

func webhookDeliveryPurgeConfig() config.Config {
	return config.Config{WebhookDeliveryRetentionDays: 30}
}

// createDelivery inserts a delivery and backdates its created_at so it falls
// on the chosen side of the retention cutoff — GORM stamps "now" on Create,
// which is always inside the window.
func createDelivery(t *testing.T, db *gorm.DB, webhookID uint, when time.Time) models.WebhookDelivery {
	t.Helper()
	d := models.WebhookDelivery{
		WebhookID: webhookID,
		EventType: "contact.created",
		Payload:   `{"id":"x","event":"contact.created","timestamp":"2026-01-01T00:00:00Z","data":{"fullName":"Alice","email":"alice@example.com"}}`,
	}
	require.NoError(t, db.Create(&d).Error)
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Where("id = ?", d.ID).
		Update("created_at", when).Error)
	return d
}

func countDeliveries(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&count).Error)
	return count
}

// A delivery older than the window must be purged: this is exactly the
// unbounded plaintext PII copy this ticket exists to bound.
func TestPurgeExpiredWebhookDeliveries_OldPastRetention(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())

	assert.Zero(t, countDeliveries(t, db), "a delivery created 60 days ago must be purged at a 30-day window")
}

func TestPurgeExpiredWebhookDeliveries_FreshInsideRetention(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -5))

	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())

	assert.Equal(t, int64(1), countDeliveries(t, db), "a delivery created 5 days ago is still inside a 30-day window")
}

// A failed delivery that is still within its retry window must not be touched:
// the purge is a retention window, not a "remove failures" job.
func TestPurgeExpiredWebhookDeliveries_FreshFailedRowUntouched(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	when := time.Now().AddDate(0, 0, -5)
	d := createDelivery(t, db, webhookID, when)
	errMsg := "unexpected status 500"
	next := when.Add(5 * time.Minute)
	require.NoError(t, db.Model(&d).
		Updates(map[string]interface{}{"status_code": 500, "error": &errMsg, "next_retry_at": &next}).Error)

	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())

	assert.Equal(t, int64(1), countDeliveries(t, db), "a failed delivery 5 days old must survive the window, not be deleted for failing")
}

// A failed row past the window is purged like any other delivery. This is the
// deliberate decision (see the service's doc comment): a delivery older than
// 30 days is far past every retry a webhook could take (max 3 attempts across
// a ~20-minute backoff), so the retention window being the last word is not a
// lost delivery — re-sending a month-old webhook is the same stale-PII hazard
// the window exists to bound.
func TestPurgeExpiredWebhookDeliveries_OldFailedRowPurged(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	when := time.Now().AddDate(0, 0, -60)
	d := createDelivery(t, db, webhookID, when)
	errMsg := "unexpected status 500"
	require.NoError(t, db.Model(&d).
		Updates(map[string]interface{}{"status_code": 500, "error": &errMsg}).Error)

	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())

	assert.Zero(t, countDeliveries(t, db), "a failed delivery 60 days old must still be purged at a 30-day window")
}

func TestPurgeExpiredWebhookDeliveries_MixedAges(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -60))
	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -45))
	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -5))
	createDelivery(t, db, webhookID, time.Now())

	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())

	assert.Equal(t, int64(2), countDeliveries(t, db), "only the deliveries past the 30-day window must be purged")
}

// WEBHOOK_DELIVERY_RETENTION_DAYS <= 0 means "disabled", never "delete
// everything" — mirrors audit_purge_service.go's stance on a misconfigured
// window.
func TestPurgeExpiredWebhookDeliveries_DisabledWhenRetentionZero(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredWebhookDeliveries(db, config.Config{WebhookDeliveryRetentionDays: 0})

	assert.Equal(t, int64(1), countDeliveries(t, db), "a zero retention window must disable the purge, not delete every delivery")
}

func TestPurgeExpiredWebhookDeliveries_IsIdempotent(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -60))

	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())
	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())

	assert.Zero(t, countDeliveries(t, db))
}

// The purge only ever touches webhook_deliveries: a webhook row older than
// the window stays, since the parent webhook is a live configuration object,
// not a PII copy. (FK ON DELETE CASCADE means the reverse — deleting the
// webhook removes its deliveries — which is the existing account-cascade
// behavior, unrelated to retention.)
func TestPurgeExpiredWebhookDeliveries_DoesNotTouchWebhooks(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	require.NoError(t, db.Model(&models.Webhook{}).Where("id = ?", webhookID).
		Update("created_at", time.Now().AddDate(0, 0, -60)).Error)

	PurgeExpiredWebhookDeliveries(db, webhookDeliveryPurgeConfig())

	var wh models.Webhook
	require.NoError(t, db.First(&wh, webhookID).Error, "an old webhook row must survive its deliveries' purge")
}

// TestPurgeExpiredWebhookDeliveriesScheduled_JobLock proves the scheduled
// entry point takes the job lock and does not panic on repeated invocation.
func TestPurgeExpiredWebhookDeliveriesScheduled_JobLock(t *testing.T) {
	db, webhookID := newWebhookDeliveryPurgeDB(t)

	createDelivery(t, db, webhookID, time.Now().AddDate(0, 0, -60))
	createDelivery(t, db, webhookID, time.Now())

	cfg := webhookDeliveryPurgeConfig()
	require.NotPanics(t, func() {
		PurgeExpiredWebhookDeliveriesScheduled(db, cfg)
		PurgeExpiredWebhookDeliveriesScheduled(db, cfg) // second call: lock rate-limits it
	})

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameWebhookDeliveryPurge).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock should be released after the job completes")
	assert.Equal(t, int64(1), countDeliveries(t, db), "the scheduled run must still purge through the lock")
}
