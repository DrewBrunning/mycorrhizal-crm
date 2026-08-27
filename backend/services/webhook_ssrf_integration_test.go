package services

import (
	"context"
	"fmt"
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

// TestTriggerWebhooksSSRFBlockedOnLivePath is the end-to-end SSRF guard test
// the unit-level dialer tests cannot cover: it registers real webhooks, fires
// TriggerWebhooks (the actual delivery-job entry point a webhook takes after
// creation), and asserts each delivery fails closed with the SSRF reason while
// the sink is never reached.
//
// With WEBHOOK_BLOCK_PRIVATE_URLS on, every target below is a private /
// loopback / link-local address, so the guarded delivery path must refuse all
// of them and surface the reason on the stored delivery record — a
// user-visible failure, never a silent reach into an internal address.
func TestTriggerWebhooksSSRFBlockedOnLivePath(t *testing.T) {
	db := dbtest.New(t)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	})

	user := models.User{Username: "ssrf-user", Password: "password123", Email: "ssrf@example.com"}
	require.NoError(t, db.Create(&user).Error)

	var sinkHits int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&sinkHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	targets := []string{
		"http://169.254.169.254/", // cloud metadata (link-local)
		"http://10.0.0.1/",        // private RFC1918
		"http://127.0.0.1/",       // loopback
		"http://[::1]/",           // IPv6 loopback
		sink.URL,                  // local sink: must never receive a request
	}

	for i, target := range targets {
		wh := models.Webhook{
			UserID:   user.ID,
			Name:     fmt.Sprintf("hook-%d", i),
			URL:      target,
			Events:   []string{"contact.created"},
			Secret:   "secret",
			IsActive: true,
		}
		require.NoError(t, db.Create(&wh).Error)
	}

	cfg := config.Config{WebhookBlockPrivateURLs: true}
	TriggerWebhooks(context.Background(), db, cfg, user.ID, "contact.created", map[string]string{"name": "Ada"})

	// Deliveries are written asynchronously from per-webhook goroutines, so
	// wait for all of them to land rather than racing the first one.
	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&count).Error)
		return count == int64(len(targets))
	}, 5*time.Second, 10*time.Millisecond, "every webhook must produce a delivery record")

	// Give any accidental delivery a moment to land, then confirm the sink was
	// never touched.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&sinkHits), "the guarded delivery must never reach the sink")

	var deliveries []models.WebhookDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, len(targets))
	for _, d := range deliveries {
		assert.Nil(t, d.StatusCode, "a refused delivery must carry no status code")
		require.NotNil(t, d.Error)
		assert.Equal(t, ErrWebhookPrivateAddress.Error(), *d.Error,
			"the delivery record must surface the SSRF reason verbatim, not a silent no-op")
	}
}
