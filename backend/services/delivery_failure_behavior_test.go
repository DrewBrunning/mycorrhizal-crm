package services

import (
	"context"
	"errors"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/faults"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// INT-02 (issue #465), action 4: for the delivery integrations (outbound
// webhooks, ntfy/Gotify/Web Push notifications) a send failure must be
// *recorded* — a failed delivery row with the reason — and must not corrupt
// state or silently drop the work. Retries stay bounded (webhooks:
// maxDeliveryAttempts; notifications: the reminder stays due for the next run).

// TestWebhookDelivery_InjectedFaultRecordsAndSchedulesRetry arms the webhook
// delivery seam and asserts the failure is persisted with the next retry
// scheduled, exactly like a real transport error.
func TestWebhookDelivery_InjectedFaultRecordsAndSchedulesRetry(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupWebhookRetryTestDB(t)
	wh := newTestWebhook("https://receiver.example.com/hook", "secret")
	require.NoError(t, db.Create(&wh).Error)

	faults.ArmError(faultWebhookDelivery, errors.New("receiver refused the connection"))
	t.Cleanup(func() { faults.Disarm(faultWebhookDelivery) })

	body := []byte(`{"id":"evt_x","event":"contact.created","timestamp":"2026-01-01T00:00:00Z","data":{}}`)
	delivery := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", body, 1)

	var loaded models.WebhookDelivery
	require.NoError(t, db.First(&loaded, delivery.ID).Error)
	require.NotNil(t, loaded.Error)
	assert.Contains(t, *loaded.Error, "receiver refused the connection", "the injected cause must be recorded")
	assert.Equal(t, 1, loaded.Attempts)
	require.NotNil(t, loaded.NextRetryAt, "a failed delivery within budget must be scheduled for retry")
}

// TestWebhookDelivery_TerminalAtMaxAttempts pins the bound: at the final
// attempt there is no further retry scheduled — the failure hands off to #467's
// terminal state rather than retrying forever.
func TestWebhookDelivery_TerminalAtMaxAttempts(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupWebhookRetryTestDB(t)
	wh := newTestWebhook("https://receiver.example.com/hook", "secret")
	require.NoError(t, db.Create(&wh).Error)

	faults.ArmError(faultWebhookDelivery, errors.New("still refusing"))
	t.Cleanup(func() { faults.Disarm(faultWebhookDelivery) })

	delivery := deliverWebhook(context.Background(), db, config.Config{}, wh, "contact.created", []byte(`{}`), maxDeliveryAttempts)

	var loaded models.WebhookDelivery
	require.NoError(t, db.First(&loaded, delivery.ID).Error)
	assert.Equal(t, maxDeliveryAttempts, loaded.Attempts)
	assert.Nil(t, loaded.NextRetryAt, "the final attempt must not schedule another retry")
}

// TestNotificationDelivery_InjectedFaultRecordsFailureAndKeepsReminderDue pins
// the notification contract: a failed send writes a 'failed' NotificationDelivery
// row and does not mark the reminder sent for that channel, so the next run
// retries it — no silent drop, no double-send.
func TestNotificationDelivery_InjectedFaultRecordsFailureAndKeepsReminderDue(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, true /*ntfy*/, false, false, "https://ntfy.example.com", "")
	reminder := newDueReminder(t, db, user, "Call Jane")

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	t.Cleanup(func() { sendReminderEmailFn = originalSender })

	faults.ArmError(faultNotificationDelivery, errors.New("ntfy endpoint unreachable"))
	t.Cleanup(func() { faults.Disarm(faultNotificationDelivery) })

	sendRemindersExpectErrT(t, db, config.Config{ReminderTime: "12:00"})

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", reminder.ID).Find(&deliveries).Error)
	require.NotEmpty(t, deliveries)
	var ntfy *models.NotificationDelivery
	for i := range deliveries {
		if deliveries[i].Channel == string(models.ChannelNtfy) {
			ntfy = &deliveries[i]
		}
	}
	require.NotNil(t, ntfy, "an ntfy delivery attempt must be recorded")
	assert.Equal(t, "failed", ntfy.Status)
	require.NotNil(t, ntfy.Error)
	assert.Contains(t, *ntfy.Error, "unreachable")
	assert.Nil(t, ntfy.SentAt, "a failed delivery must not be marked sent")

	// No 'sent' row for the ntfy channel → the reminder is still due there and
	// the next run will retry it.
	var sentCount int64
	require.NoError(t, db.Model(&models.NotificationDelivery{}).
		Where("reminder_id = ? AND channel = ? AND status = ?", reminder.ID, models.ChannelNtfy, "sent").
		Count(&sentCount).Error)
	assert.Zero(t, sentCount)
}

// TestNotificationDelivery_InjectedFaultOnWebPushKeepsSubscription pins that
// the same seam covers the Web Push path (sendPushMessage): the send is
// recorded failed, and the subscription is *kept* (a fault is not a 404/410,
// so it is not a reason to drop the device).
func TestNotificationDelivery_InjectedFaultOnWebPushKeepsSubscription(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, true /*push*/, "", "")
	reminder := newDueReminder(t, db, user, "Push me")
	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: "https://push.example.com/x",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
	require.NoError(t, db.Create(&sub).Error)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	t.Cleanup(func() { sendReminderEmailFn = originalSender })

	faults.ArmError(faultNotificationDelivery, errors.New("push service unreachable"))
	t.Cleanup(func() { faults.Disarm(faultNotificationDelivery) })

	sendRemindersExpectErrT(t, db, config.Config{ReminderTime: "12:00"})

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ? AND channel = ?", reminder.ID, "push").Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "failed", deliveries[0].Status)

	var remaining int64
	require.NoError(t, db.Model(&models.PushSubscription{}).Where("id = ?", sub.ID).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining, "an injected transport fault must not drop the subscription (only 404/410 does)")
}
