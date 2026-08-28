package services

import (
	"encoding/json"
	"io"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCadenceJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// ProcessOverdueCadences dispatches deliveries through goroutines, so a
	// single connection must serve them all (the:memory: sqlite gotcha).
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(
		&models.User{}, &models.Contact{}, &models.Activity{}, &models.CadencePolicy{},
		&models.Webhook{}, &models.WebhookDelivery{}, &models.JobExecution{},
	))
	return db
}

func seedOverduePolicy(t *testing.T, db *gorm.DB, user models.User, contact models.Contact) models.CadencePolicy {
	t.Helper()
	// Last qualifying interaction 40 days ago, 30-day interval -> ~10 days overdue.
	activity := models.Activity{
		UserID: user.ID, Title: "Last call", Date: time.Now().AddDate(0, 0, -40),
		Type: models.InteractionTypeCall, Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&activity).Error)
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&policy).Error)
	return policy
}

func TestProcessOverdueCadencesSkipsWhenLocked(t *testing.T) {
	db := setupCadenceJobTestDB(t)
	cfg := config.Config{}

	user := models.User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	seedOverduePolicy(t, db, user, contact)

	// A webhook that WOULD receive the emission if the job ran.
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "vikunja", URL: server.URL, Events: []string{"cadence.overdue"}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)

	// Lock row with a recent run -> acquireJobLock must refuse.
	require.NoError(t, db.Create(&models.JobExecution{
		JobName: models.JobNameCadenceOverdue, LastRunAt: time.Now(),
	}).Error)

	require.NotPanics(t, func() { ProcessOverdueCadences(db, cfg) })

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "the locked job must not emit any webhook")
	var deliveries int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&deliveries).Error)
	assert.Zero(t, deliveries)

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameCadenceOverdue).First(&job).Error)
	assert.Nil(t, job.LockedAt, "job must not have been re-locked while rate-limited")
}

func TestProcessOverdueCadencesEmitsWebhookForOverduePolicy(t *testing.T) {
	db := setupCadenceJobTestDB(t)
	cfg := config.Config{}

	user := models.User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	overdueContact := models.Contact{UserID: user.ID, Firstname: "Neglected"}
	require.NoError(t, db.Create(&overdueContact).Error)
	policy := seedOverduePolicy(t, db, user, overdueContact)

	// Not overdue: recent interaction, so no emission for it.
	freshContact := models.Contact{UserID: user.ID, Firstname: "Fresh"}
	require.NoError(t, db.Create(&freshContact).Error)
	fresh := models.Activity{
		UserID: user.ID, Title: "Recent chat", Date: time.Now().AddDate(0, 0, -5),
		Type: models.InteractionTypeVisit, Contacts: []models.Contact{freshContact},
	}
	require.NoError(t, db.Create(&fresh).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: freshContact.VCardUID, TargetIntervalDays: 30}).Error)

	var hits int32
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "vikunja", URL: server.URL, Events: []string{"cadence.overdue"}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)

	require.NotPanics(t, func() { ProcessOverdueCadences(db, cfg) })

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&hits) >= 1
	}, 3*time.Second, 10*time.Millisecond, "the overdue policy must trigger a cadence.overdue webhook")

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "exactly one emission (only the overdue policy)")

	var deliveries []models.WebhookDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, wh.ID, deliveries[0].WebhookID)
	assert.Equal(t, "cadence.overdue", deliveries[0].EventType)

	var envelope struct {
		Event string `json:"event"`
		Data  struct {
			CadencePolicyID string `json:"cadence_policy_id"`
			EntityID        string `json:"entity_id"`
			OverdueBy       int    `json:"overdue_by"`
			ContactID       uint   `json:"contact_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(receivedBody, &envelope))
	assert.Equal(t, "cadence.overdue", envelope.Event)
	assert.Equal(t, policy.ID, envelope.Data.CadencePolicyID)
	assert.Equal(t, overdueContact.VCardUID, envelope.Data.EntityID)
	assert.True(t, envelope.Data.OverdueBy > 0)
	assert.Equal(t, overdueContact.ID, envelope.Data.ContactID)

	// The delivered body above carries the full payload; the stored receipt
	// is the trimmed envelope (issue #622: a successful delivery never needs
	// the entity body it carried).
	var storedEnvelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &storedEnvelope))
	assert.Equal(t, "cadence.overdue", storedEnvelope.Event)
	assert.Nil(t, storedEnvelope.Data, "the stored receipt must not retain the entity body")

	// Lock lifecycle: acquired and released.
	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameCadenceOverdue).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock should be released after the job completes")
}
