package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// INT-03 (issue #466): retry safety of the one outbound webhook operation.
// A transient failure is retried with bounded backoff; a permanent HTTP status
// is not retried at all; a retry produces exactly one additional remote effect.

func countIntegrationFailedEvents(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.SystemEvent{}).
		Where("event_type = ?", models.SysEventIntegrationFailed).Count(&n).Error)
	return n
}

func TestWebhookDelivery_PermanentStatusIsTerminalNotRetried(t *testing.T) {
	cases := []struct {
		status     int
		wantReason string
	}{
		{http.StatusUnauthorized, "auth-expiry"},
		{http.StatusForbidden, "authz-revoked"},
		{http.StatusNotFound, "remote-resource-deleted"},
		{http.StatusGone, "remote-resource-deleted"},
		{http.StatusBadRequest, "client-error"},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			db := setupWebhookRetryTestDB(t)
			var posts int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&posts, 1)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			wh := newTestWebhook(srv.URL, "secret")
			require.NoError(t, db.Create(&wh).Error)

			d := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", []byte(`{}`), 1)

			assert.True(t, d.FailedPermanently, "a %d must be recorded as a permanent failure", tc.status)
			assert.Equal(t, tc.wantReason, d.TerminalReason)
			assert.Nil(t, d.NextRetryAt, "a permanent failure must not schedule a retry")
			assert.Equal(t, 1, d.Attempts, "it is terminal on the first attempt, not after burning the budget")
			assert.EqualValues(t, 1, countIntegrationFailedEvents(t, db),
				"integration_failed must be raised on the failing attempt, once")

			// ProcessWebhookRetries must never pick a permanent row back up.
			ProcessWebhookRetries(db, config.Config{})
			WaitForWebhookGoroutines()
			assert.EqualValues(t, 1, atomic.LoadInt32(&posts), "no retry POST for a permanent failure")

			var count int64
			require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&count).Error)
			assert.EqualValues(t, 1, count, "no second delivery row")
		})
	}
}

func TestWebhookDelivery_RateLimitedHonorsRetryAfter(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	wh := newTestWebhook(srv.URL, "secret")
	require.NoError(t, db.Create(&wh).Error)

	before := time.Now()
	d := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", []byte(`{}`), 1)

	assert.False(t, d.FailedPermanently, "429 is transient")
	require.NotNil(t, d.NextRetryAt, "429 schedules a retry")
	assert.False(t, d.NextRetryAt.Before(before.Add(120*time.Second)),
		"Retry-After: 120 must not be shortened by the backoff (got %v)", d.NextRetryAt)
}

func TestWebhookDelivery_RateLimitedRetryAfterHTTPDate(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	when := time.Now().UTC().Add(90 * time.Second).Format(http.TimeFormat)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", when)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	wh := newTestWebhook(srv.URL, "secret")
	require.NoError(t, db.Create(&wh).Error)

	before := time.Now()
	d := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", []byte(`{}`), 1)
	require.NotNil(t, d.NextRetryAt)
	assert.False(t, d.NextRetryAt.Before(before.Add(80*time.Second)),
		"an HTTP-date Retry-After must be parsed and honored (got %v)", d.NextRetryAt)
}

func TestWebhookDelivery_TransientStatusBacksOffAndIsBounded(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wh := newTestWebhook(srv.URL, "secret")
	require.NoError(t, db.Create(&wh).Error)

	before := time.Now()
	d1 := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", []byte(`{}`), 1)
	after := time.Now()
	require.NotNil(t, d1.NextRetryAt, "500 on attempt 1 schedules a retry")
	assert.False(t, d1.FailedPermanently)
	base := webhookRetryPolicy.BaseDelay
	assert.False(t, d1.NextRetryAt.Before(before.Add(time.Duration(float64(base)*0.8))))
	assert.False(t, d1.NextRetryAt.After(after.Add(time.Duration(float64(base)*1.2))))
	assert.Zero(t, countIntegrationFailedEvents(t, db), "not terminal mid-budget")

	// The final attempt spends the budget: no retry, one integration_failed.
	d3 := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", []byte(`{}`), maxDeliveryAttempts)
	assert.Nil(t, d3.NextRetryAt, "budget spent -> no further retry")
	assert.False(t, d3.FailedPermanently, "budget-exhaustion is not the same flag as a permanent status")
	assert.EqualValues(t, 1, countIntegrationFailedEvents(t, db))
}

func TestWebhookDelivery_IdempotencyKeyIsStableAcrossRetries(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusInternalServerError) // force the retry path
	}))
	defer srv.Close()

	wh := newTestWebhook(srv.URL, "secret")
	require.NoError(t, db.Create(&wh).Error)

	body, err := buildPayloadBody("contact.created", map[string]any{"id": 7})
	require.NoError(t, err)
	wantID := webhookPayloadID(body)
	require.NotEmpty(t, wantID)

	// Two deliveries replaying the same body — as ProcessWebhookRetries does.
	deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", body, 1)
	deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", body, 2)

	require.Len(t, keys, 2)
	assert.Equal(t, wantID, keys[0], "the envelope id is sent as Idempotency-Key")
	assert.Equal(t, keys[0], keys[1], "a retry carries the same key so a receiver can de-duplicate")
}

func TestWebhookPayloadID(t *testing.T) {
	body, err := buildPayloadBody("contact.created", map[string]any{"x": 1})
	require.NoError(t, err)
	assert.NotEmpty(t, webhookPayloadID(body), "a real envelope has an id")

	assert.Empty(t, webhookPayloadID([]byte("not json at all")), "a non-envelope body yields no key")
	assert.Empty(t, webhookPayloadID([]byte(`{"event":"x"}`)), "an envelope with no id yields no key")
}

func TestWebhookDelivery_RetryProducesExactlyOneAdditionalPost(t *testing.T) {
	db := setupWebhookRetryTestDB(t)
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&posts, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError) // first attempt fails
			return
		}
		w.WriteHeader(http.StatusOK) // the retry lands
	}))
	defer srv.Close()

	wh := newTestWebhook(srv.URL, "secret")
	require.NoError(t, db.Create(&wh).Error)

	body, err := buildPayloadBody("contact.created", map[string]any{"id": 1})
	require.NoError(t, err)

	first := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", body, 1)
	require.NotNil(t, first.NextRetryAt)

	// Make the retry due and run the loop.
	past := time.Now().Add(-time.Minute)
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Where("id = ?", first.ID).Update("next_retry_at", past).Error)
	ProcessWebhookRetries(db, config.Config{})
	WaitForWebhookGoroutines()

	assert.EqualValues(t, 2, atomic.LoadInt32(&posts), "exactly one retry POST — no duplicate side effect")

	var successes int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).
		Where("status_code >= ? AND status_code < ?", 200, 300).Count(&successes).Error)
	assert.EqualValues(t, 1, successes, "the retry recorded exactly one successful delivery")
}
