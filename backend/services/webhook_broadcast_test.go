package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTriggerWebhooksForAllUsers pins the fan-out helper the two new
// scheduled jobs (#273, #275) share: every user with an active webhook
// subscribed to the given event type gets a delivery; an inactive webhook
// and a webhook subscribed to a different event do not.
func TestTriggerWebhooksForAllUsers(t *testing.T) {
	db := dbtest.New(t)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	const eventType = "db.integrity_check_failed"

	var subscribedHits, otherEventHits, inactiveHits int32
	subscribedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&subscribedHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer subscribedServer.Close()
	otherEventServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&otherEventHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer otherEventServer.Close()
	inactiveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&inactiveHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer inactiveServer.Close()

	subscribedUser := models.User{Username: "subscribed", Password: "password123!A", Email: "subscribed@example.com"}
	require.NoError(t, db.Create(&subscribedUser).Error)
	require.NoError(t, db.Create(&models.Webhook{
		UserID: subscribedUser.ID, Name: "alerts", URL: subscribedServer.URL,
		Events: []string{eventType}, Secret: "s", IsActive: true,
	}).Error)

	otherEventUser := models.User{Username: "otherevent", Password: "password123!A", Email: "otherevent@example.com"}
	require.NoError(t, db.Create(&otherEventUser).Error)
	require.NoError(t, db.Create(&models.Webhook{
		UserID: otherEventUser.ID, Name: "unrelated", URL: otherEventServer.URL,
		Events: []string{"contact.created"}, Secret: "s", IsActive: true,
	}).Error)

	inactiveUser := models.User{Username: "inactiveuser", Password: "password123!A", Email: "inactiveuser@example.com"}
	require.NoError(t, db.Create(&inactiveUser).Error)
	inactiveWebhook := models.Webhook{
		UserID: inactiveUser.ID, Name: "disabled", URL: inactiveServer.URL,
		Events: []string{eventType}, Secret: "s", IsActive: true,
	}
	require.NoError(t, db.Create(&inactiveWebhook).Error)
	// Webhook.IsActive is `gorm:"default:true"`, so Create() with
	// IsActive:false silently persists as active (Go's bool zero value is
	// indistinguishable from "unset" to GORM's default-tag handling — a real
	// bug, flagged separately, not something to work around by asserting the
	// wrong thing here). Toggle it off via Update instead, which isn't
	// affected.
	require.NoError(t, db.Model(&inactiveWebhook).Update("is_active", false).Error)

	triggerWebhooksForAllUsers(context.Background(), db, config.Config{}, eventType, map[string]interface{}{"detail": "corruption found"})

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&subscribedHits) >= 1
	}, 3*time.Second, 10*time.Millisecond, "the subscribed active webhook must receive a delivery")

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&subscribedHits))
	assert.Zero(t, atomic.LoadInt32(&otherEventHits), "a webhook not subscribed to this event must not fire")
	assert.Zero(t, atomic.LoadInt32(&inactiveHits), "an inactive webhook must not fire")
}
