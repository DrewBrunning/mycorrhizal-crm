package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAlertUser(t *testing.T, db *gorm.DB, name string, isAdmin bool, ntfyURL string) models.User {
	t.Helper()
	user := models.User{
		Username:   name,
		Password:   "password123",
		Email:      name + "@example.com",
		Language:   "en",
		IsAdmin:    isAdmin,
		NotifyNtfy: ntfyURL != "",
	}
	require.NoError(t, db.Create(&user).Error)
	if ntfyURL != "" {
		require.NoError(t, db.Create(&models.NotificationConfig{
			UserID:    user.ID,
			NtfyURL:   ntfyURL,
			NtfyTopic: "ops",
		}).Error)
	}
	return user
}

// TestDeliverOperationalAlert_AdminOnly pins that personal-channel alerts reach
// admin users only — infra health is an operator concern, not something to page
// every user of a shared instance about (issue #428).
func TestDeliverOperationalAlert_AdminOnly(t *testing.T) {
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	srv := newFakeChannelServer(t, nil)

	newAlertUser(t, db, "ops-admin", true, srv.URL())
	newAlertUser(t, db, "ops-regular", false, srv.URL())

	cfg := config.Config{} // no mail, no webhook private-URL block

	deliverOperationalAlert(context.Background(), db, cfg, operationalAlert{
		conditionKey: alertConditionKeyBackup,
		title:        "Backup",
		firing:       true,
		detail:       "snapshot failed",
		failureCount: 2,
		since:        time.Now(),
	})

	require.Equal(t, 1, srv.count(), "only the admin's ntfy endpoint should have been hit")
	assert.Contains(t, srv.lastBody(), "Backup failed")
}

// TestOperationalAlertDeliveryOutcome pins the signal the evaluator retries a
// raise on (issue #973): a dispatch is "handled" when a channel accepts it or
// there is nothing to attempt, and "undelivered" only when a channel was
// attempted and rejected. A regression that always reports handled would
// silently reinstate the lost-raise bug.
func TestOperationalAlertDeliveryOutcome(t *testing.T) {
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	okSrv := newFakeChannelServer(t, nil)
	failSrv := newFakeChannelServer(t, map[string]int{"/": http.StatusInternalServerError})

	admin := newAlertUser(t, db, "outcome-admin", true, okSrv.URL())
	cfg := config.Config{}
	alert := operationalAlert{
		conditionKey: alertConditionKeyBackup,
		title:        "Backup",
		firing:       true,
		detail:       "snapshot failed",
		failureCount: 1,
		since:        time.Now(),
	}

	setNtfy := func(t *testing.T, url string, enabled bool) {
		t.Helper()
		require.NoError(t, db.Model(&models.User{}).Where("id = ?", admin.ID).
			Update("notify_ntfy", enabled).Error)
		if url != "" {
			require.NoError(t, db.Model(&models.NotificationConfig{}).Where("user_id = ?", admin.ID).
				Update("ntfy_url", url).Error)
		}
	}

	t.Run("a channel that accepts the alert is handled", func(t *testing.T) {
		setNtfy(t, okSrv.URL(), true)
		assert.True(t, deliverOperationalAlert(context.Background(), db, cfg, alert))
	})

	t.Run("every attempted channel failing is undelivered", func(t *testing.T) {
		setNtfy(t, failSrv.URL(), true)
		assert.False(t, deliverOperationalAlert(context.Background(), db, cfg, alert),
			"a raise no channel accepted must be reported as undelivered so it is retried")
	})

	t.Run("no enabled channel is handled", func(t *testing.T) {
		setNtfy(t, "", false)
		assert.True(t, deliverOperationalAlert(context.Background(), db, cfg, alert),
			"nothing to attempt is not a delivery failure")
	})

	t.Run("no admin is handled", func(t *testing.T) {
		require.NoError(t, db.Model(&models.User{}).Where("id = ?", admin.ID).
			Update("is_admin", false).Error)
		assert.True(t, deliverOperationalAlert(context.Background(), db, cfg, alert),
			"with no operator to page there is nothing to retry")
	})
}

// TestOperationalAlertDeliveryPerChannel drives the per-channel outcome
// accounting the retry gate depends on (issue #973): each of the four personal
// channels must report both a sent and a failed outcome. A regression that
// pinned the counters to one value would either lose the retry (always handled)
// or re-page an already-delivered alert (always undelivered).
func TestOperationalAlertDeliveryPerChannel(t *testing.T) {
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	okSrv := newFakeChannelServer(t, nil)
	failSrv := newFakeChannelServer(t, map[string]int{"/": http.StatusInternalServerError})

	emailOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "email_ok"})
	}))
	t.Cleanup(emailOK.Close)
	emailFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"down"}`))
	}))
	t.Cleanup(emailFail.Close)

	origResend := resendBaseURLOverride
	t.Cleanup(func() { resendBaseURLOverride = origResend })

	user := newNotificationUser(t, db, true, true, true, okSrv.URL(), okSrv.URL())
	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: okSrv.URL() + "/push",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
	require.NoError(t, db.Create(&sub).Error)

	ctx := context.Background()
	cfg := config.Config{
		UseResend:       true,
		ResendAPIKey:    "test",
		ResendFromEmail: "noreply@example.com",
		JWTSecretKey:    "test-jwt-secret-that-is-long-enough",
	}

	resendBaseURLOverride = emailOK.URL + "/"
	sent, failed := deliverOperationalAlertToUser(ctx, db, cfg, user, "subject", "body")
	assert.Equal(t, 4, sent, "email, ntfy, gotify and push must each report a successful send")
	assert.Equal(t, 0, failed)

	// Point every channel at a failing endpoint.
	require.NoError(t, db.Model(&models.NotificationConfig{}).Where("user_id = ?", user.ID).
		Updates(map[string]interface{}{"ntfy_url": failSrv.URL(), "gotify_url": failSrv.URL()}).Error)
	require.NoError(t, db.Model(&models.PushSubscription{}).Where("id = ?", sub.ID).
		Update("endpoint", failSrv.URL()+"/push").Error)
	resendBaseURLOverride = emailFail.URL + "/"

	sent, failed = deliverOperationalAlertToUser(ctx, db, cfg, user, "subject", "body")
	assert.Equal(t, 0, sent)
	assert.Equal(t, 4, failed, "each failing channel must be counted so the raise is retried")
}

// TestOperationalAlertSubject covers the raise/recover subject phrasing the
// issue calls out ("🟢 Backup recovered after 3 failures" must follow
// "🔴 Backup failed").
func TestOperationalAlertSubject(t *testing.T) {
	raised := operationalAlert{title: "Backup", firing: true, failureCount: 3}
	assert.Equal(t, "🔴 Backup failed", raised.subject())

	recovered := operationalAlert{title: "Backup", firing: false, failureCount: 3}
	assert.Equal(t, "🟢 Backup recovered after 3 failures", recovered.subject())

	recoveredOne := operationalAlert{title: "Disk space", firing: false, failureCount: 1}
	assert.Equal(t, "🟢 Disk space recovered after 1 failure", recoveredOne.subject())

	recoveredZero := operationalAlert{title: "Disk space", firing: false, failureCount: 0}
	assert.Equal(t, "🟢 Disk space recovered", recoveredZero.subject())
}

func TestOperationalAlertHTML_Escapes(t *testing.T) {
	got := operationalAlertHTML("a <script> & \"quote\"")
	assert.NotContains(t, got, "<script>")
	assert.True(t, strings.HasPrefix(got, "<pre"))
}
