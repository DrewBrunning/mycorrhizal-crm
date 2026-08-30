package controllers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// closeTestDBAtTeardown registers a cleanup that drains the in-flight
// fire-and-forget goroutines (webhook deliveries, audit writes) that hold this
// *gorm.DB, then closes the per-test file DB before t.TempDir() removes it.
//
// The close alone is not enough (issue #703, follow-up to #264): DB.Close()
// closes idle connections but does NOT wait for a connection a straggler has
// already checked out. If t.TempDir()'s RemoveAll deletes the DB file from
// under that open modernc/sqlite connection, the driver recreates a transient
// "001" temp directory and the cleanup fails with "directory not empty" — the
// TestDeleteContact_NullsSelfContactPointer flake. Draining first (waiting for
// every tracked webhook/audit goroutine) guarantees no connection is in use
// when the file is removed. The drain runs inside the same cleanup, so it is
// guaranteed to complete before this close and before TempDir's RemoveAll
// (cleanups run LIFO).
func closeTestDBAtTeardown(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		services.WaitForWebhookGoroutines()
		models.AuditFlush()
		_ = sqlDB.Close()
	})
}

// Issue #703 regression: the webhook-triggering handlers must fire through the
// tracked entry point (services.TriggerWebhooksAsync), so that the drain in
// closeTestDBAtTeardown / dbtest actually waits for the straggler goroutine
// that holds the test DB. A raw `go services.TriggerWebhooks(...)` is invisible
// to WaitForWebhookGoroutines, and the "directory not empty" TempDir flake
// comes back. The assertion is deterministic: the slow delivery target only
// responds after a delay, so if the goroutine were untracked,
// WaitForWebhookGoroutines would return while the delivery was still in flight
// and the counter would be 0.
func TestDeleteContact_WebhookGoroutineIsTracked(t *testing.T) {
	db := selfContactTestDB(t)
	user := models.User{Username: "tracked", Password: "password123!A", Email: "tracked@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Webhook Fodder"}
	require.NoError(t, db.Create(&contact).Error)

	var received atomic.Int64
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	webhook := models.Webhook{
		UserID: user.ID, Name: "delete-hook", URL: slow.URL,
		Events: []string{"contact.deleted"}, IsActive: true,
	}
	require.NoError(t, db.Create(&webhook).Error)

	router := selfContactTestRouter(t, db, user.ID)
	router.DELETE("/contacts/:id", DeleteContact)

	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.FormatUint(uint64(contact.ID), 10), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	services.WaitForWebhookGoroutines()

	require.Equal(t, int64(1), received.Load(),
		"the delete path must fire its webhook through a tracked goroutine so teardown can drain it before closing the test DB")

	var deliveries int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Where("webhook_id = ?", webhook.ID).Count(&deliveries).Error)
	require.Equal(t, int64(1), deliveries,
		"the delivery must have persisted while the test DB was still open, proving the drain waited before the pool closed")
}

// Issue #703 regression for the OTHER half of the straggler: the delete path's
// webhook goroutine does an initial webhooks-table read BEFORE any (tracked)
// per-delivery goroutine is spawned. With no webhook subscription that read is
// the whole goroutine, and if the handler fired it with a raw
// `go services.TriggerWebhooks(...)` the read would be invisible to
// WaitForWebhookGoroutines — the exact untracked window that produced the
// "directory not empty" TempDir flake. This test slows that read and asserts
// WaitForWebhookGoroutines blocks until it completes, which it can only do if
// the handler goes through the tracked services.TriggerWebhooksAsync entry
// point.
func TestDeleteContact_WebhookOuterGoroutineIsTracked(t *testing.T) {
	// Flush any in-flight tracked goroutines from earlier tests so the elapsed
	// assertion below reflects only this test's goroutine.
	services.WaitForWebhookGoroutines()

	db := selfContactTestDB(t)
	user := models.User{Username: "outer", Password: "password123!A", Email: "outer@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Outer Goroutine Fodder"}
	require.NoError(t, db.Create(&contact).Error)

	// The user has NO webhook subscription, so TriggerWebhooks only performs its
	// initial SELECT on the webhooks table. Slow that read so an untracked
	// goroutine (raw `go`) is observable: WaitForWebhookGoroutines would return
	// while the read was still sleeping.
	var reads atomic.Int64
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:slow-webhooks-find", func(tx *gorm.DB) {
		if tx.Statement.Table == "webhooks" {
			reads.Add(1)
			time.Sleep(300 * time.Millisecond)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Query().Remove("test:slow-webhooks-find"))
	})

	router := selfContactTestRouter(t, db, user.ID)
	router.DELETE("/contacts/:id", DeleteContact)

	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.FormatUint(uint64(contact.ID), 10), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	start := time.Now()
	services.WaitForWebhookGoroutines()
	elapsed := time.Since(start)

	require.Equal(t, int64(1), reads.Load(), "the delete path must query webhooks exactly once")
	require.GreaterOrEqual(t, elapsed, 250*time.Millisecond,
		"WaitForWebhookGoroutines must have waited for the delete-path webhook goroutine's DB read — the handler must fire through the tracked services.TriggerWebhooksAsync entry point, not a raw `go`")
}
