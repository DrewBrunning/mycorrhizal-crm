package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestComputeNotificationChannelHealth exercises the per-channel delivery
// health fold (issue #422). One migrated DB is shared across the sub-cases,
// each clearing the tables it writes.
func TestComputeNotificationChannelHealth(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	cfg := config.Config{} // EmailEnabled false, no FCM service account

	clear := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.Exec("DELETE FROM notification_deliveries").Error)
		require.NoError(t, db.Exec("DELETE FROM notification_configs").Error)
		require.NoError(t, db.Exec("DELETE FROM push_subscriptions").Error)
		require.NoError(t, db.Exec("DELETE FROM device_registrations").Error)
		require.NoError(t, db.Exec("DELETE FROM server_settings").Error)
		require.NoError(t, db.Exec("DELETE FROM users").Error)
	}

	channels := func(t *testing.T) map[string]NotificationChannelHealth {
		t.Helper()
		rows, err := ComputeNotificationChannelHealth(ctx, db, cfg)
		require.NoError(t, err)
		require.Len(t, rows, 4)
		m := make(map[string]NotificationChannelHealth, len(rows))
		for _, r := range rows {
			m[r.Channel] = r
		}
		return m
	}

	t.Run("fresh DB: every channel unconfigured with zero counts, in order", func(t *testing.T) {
		clear(t)
		rows, err := ComputeNotificationChannelHealth(ctx, db, cfg)
		require.NoError(t, err)
		names := make([]string, len(rows))
		for i, r := range rows {
			names[i] = r.Channel
			assert.Equal(t, NotificationHealthUnconfigured, r.Status)
			assert.False(t, r.Configured)
			assert.False(t, r.Reachable)
			assert.Zero(t, r.EnabledUserCount)
			assert.Zero(t, r.AttemptedCount)
			assert.Zero(t, r.DeliveredCount)
			assert.Zero(t, r.ConsecutiveFailures)
			assert.Nil(t, r.LastSentAt)
			assert.Nil(t, r.LastFailedAt)
			assert.Empty(t, r.LastError)
		}
		assert.Equal(t, []string{"email", "ntfy", "gotify", "push"}, names)
	})

	t.Run("configured ntfy channel with no deliveries is healthy, and only enabled users are counted", func(t *testing.T) {
		clear(t)
		user := models.User{Username: "ntfy-user", Email: "ntfy@example.com", Password: "password123", NotifyNtfy: true}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.NotificationConfig{
			UserID:    user.ID,
			NtfyURL:   "https://ntfy.example.com",
			NtfyTopic: "my-topic",
		}).Error)
		// A second user with the config but the toggle off must not count.
		off := models.User{Username: "ntfy-off", Email: "off@example.com", Password: "password123"}
		require.NoError(t, db.Create(&off).Error)
		require.NoError(t, db.Create(&models.NotificationConfig{
			UserID:    off.ID,
			NtfyURL:   "https://ntfy.example.com",
			NtfyTopic: "someone-elses-topic",
		}).Error)

		h := channels(t)[string(models.ChannelNtfy)]
		assert.Equal(t, NotificationHealthHealthy, h.Status)
		assert.True(t, h.Configured)
		assert.True(t, h.Reachable)
		assert.Equal(t, int64(1), h.EnabledUserCount)
		assert.Zero(t, h.AttemptedCount)
	})

	t.Run("email is configured only when the server has a mailer", func(t *testing.T) {
		clear(t)
		require.NoError(t, db.Create(&models.User{Username: "email-user", Email: "email@example.com", Password: "password123"}).Error)

		rows := channels(t)
		assert.False(t, rows[string(models.ChannelEmail)].Configured)
		assert.Equal(t, NotificationHealthUnconfigured, rows[string(models.ChannelEmail)].Status)

		// With a mailer enabled the channel is configured and the user is
		// counted (email just needs the per-user address).
		cfgMail := config.Config{UseSMTP: true}
		h, err := ComputeNotificationChannelHealth(ctx, db, cfgMail)
		require.NoError(t, err)
		var email *NotificationChannelHealth
		for i := range h {
			if h[i].Channel == string(models.ChannelEmail) {
				email = &h[i]
			}
		}
		require.NotNil(t, email)
		assert.True(t, email.Configured)
		assert.Equal(t, NotificationHealthHealthy, email.Status)
		assert.Equal(t, int64(1), email.EnabledUserCount)
	})

	t.Run("delivery stats fold sent/failed rows and pending rows are not attempts", func(t *testing.T) {
		clear(t)
		user := models.User{Username: "delivery-user", Email: "delivery@example.com", Password: "password123", NotifyNtfy: true}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.NotificationConfig{
			UserID:    user.ID,
			NtfyURL:   "https://ntfy.example.com",
			NtfyTopic: "t",
		}).Error)
		reminders := healthTestReminders(t, db, user.ID, 4)

		require.NoError(t, db.Create(&models.NotificationDelivery{
			ReminderID: reminders[0], Channel: "ntfy", Status: "sent", SentAt: now(t, -4),
		}).Error)
		require.NoError(t, db.Create(&models.NotificationDelivery{
			ReminderID: reminders[1], Channel: "ntfy", Status: "failed", Error: str("HTTP 401"),
		}).Error)
		require.NoError(t, db.Create(&models.NotificationDelivery{
			ReminderID: reminders[2], Channel: "ntfy", Status: "failed", Error: str("HTTP 401"),
		}).Error)
		require.NoError(t, db.Create(&models.NotificationDelivery{
			ReminderID: reminders[3], Channel: "ntfy", Status: "pending",
		}).Error)

		h := channels(t)[string(models.ChannelNtfy)]
		assert.Equal(t, NotificationHealthFailing, h.Status)
		assert.False(t, h.Reachable)
		assert.Equal(t, int64(3), h.AttemptedCount) // pending row excluded
		assert.Equal(t, int64(1), h.DeliveredCount)
		assert.Equal(t, 2, h.ConsecutiveFailures)
		assert.Equal(t, "HTTP 401", h.LastError)
		require.NotNil(t, h.LastFailedAt)
		assert.WithinDuration(t, time.Now().UTC(), *h.LastFailedAt, 5*time.Minute)
	})

	t.Run("a success after failures returns the channel to healthy and resets the run", func(t *testing.T) {
		clear(t)
		user := models.User{Username: "rec-user", Email: "rec@example.com", Password: "password123", NotifyGotify: true}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.NotificationConfig{UserID: user.ID, GotifyURL: "https://gotify.example.com", GotifyTokenEncrypted: "encrypted"}).Error)
		reminders := healthTestReminders(t, db, user.ID, 2)

		require.NoError(t, db.Create(&models.NotificationDelivery{ReminderID: reminders[0], Channel: "gotify", Status: "failed", Error: str("boom")}).Error)

		h := channels(t)[string(models.ChannelGotify)]
		assert.Equal(t, NotificationHealthFailing, h.Status)
		assert.Equal(t, 1, h.ConsecutiveFailures)

		require.NoError(t, db.Create(&models.NotificationDelivery{ReminderID: reminders[1], Channel: "gotify", Status: "sent", SentAt: now(t, 0)}).Error)
		h = channels(t)[string(models.ChannelGotify)]
		assert.Equal(t, NotificationHealthHealthy, h.Status)
		assert.True(t, h.Reachable)
		assert.Zero(t, h.ConsecutiveFailures)
		assert.Empty(t, h.LastError)
		require.NotNil(t, h.LastSentAt)
		assert.WithinDuration(t, time.Now().UTC(), *h.LastSentAt, 5*time.Minute)
	})

	t.Run("push without any device is no_devices, not a failure", func(t *testing.T) {
		clear(t)
		// VAPID keys present (server has been provisioned for web push) but no
		// browser subscription and no FCM device: nobody can receive push.
		require.NoError(t, db.Create(&models.ServerSetting{Key: serverSettingVAPIDPublicKey, Value: "pub"}).Error)
		require.NoError(t, db.Create(&models.ServerSetting{Key: serverSettingVAPIDPrivateKey, Value: "priv"}).Error)

		h := channels(t)[string(models.ChannelPush)]
		assert.Equal(t, NotificationHealthNoDevices, h.Status)
		assert.True(t, h.Configured)
		assert.False(t, h.Reachable)
		assert.Zero(t, h.DeviceCount)
		assert.False(t, h.FCMConfigured)
	})

	t.Run("push with a web subscription is healthy and counts the device", func(t *testing.T) {
		clear(t)
		user := models.User{Username: "push-user", Email: "push@example.com", Password: "password123", NotifyPush: true}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.PushSubscription{
			UserID: user.ID, Endpoint: "https://push.example.com/ep1", P256dh: "k", Auth: "a",
		}).Error)

		h := channels(t)[string(models.ChannelPush)]
		assert.Equal(t, NotificationHealthHealthy, h.Status)
		assert.True(t, h.Reachable)
		assert.Equal(t, int64(1), h.DeviceCount)
		assert.Equal(t, int64(1), h.EnabledUserCount)
	})

	t.Run("push with only an FCM device is no_devices until the service account is configured", func(t *testing.T) {
		clear(t)
		user := models.User{Username: "fcm-user", Email: "fcm@example.com", Password: "password123", NotifyPush: true}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.DeviceRegistration{
			UserID: user.ID, Token: "tok", Client: string(models.PushClientFCM),
		}).Error)

		// No service account: the fcm registration is inert (M2) — the device
		// must not count as able to receive.
		h := channels(t)[string(models.ChannelPush)]
		assert.Equal(t, NotificationHealthNoDevices, h.Status)
		assert.True(t, h.Configured) // a device exists, so the channel is set up
		assert.Zero(t, h.DeviceCount)
		assert.False(t, h.FCMConfigured)

		// With a service account the same device becomes deliverable.
		cfgFCM := config.Config{FCMServiceAccountFile: writeFCMServiceAccount(t)}
		rows, err := ComputeNotificationChannelHealth(ctx, db, cfgFCM)
		require.NoError(t, err)
		var push *NotificationChannelHealth
		for i := range rows {
			if rows[i].Channel == string(models.ChannelPush) {
				push = &rows[i]
			}
		}
		require.NotNil(t, push)
		assert.Equal(t, NotificationHealthHealthy, push.Status)
		assert.True(t, push.FCMConfigured)
		assert.Equal(t, int64(1), push.DeviceCount)
	})

	t.Run("gotify is configured only when a token is stored", func(t *testing.T) {
		clear(t)
		user := models.User{Username: "gotify-config-user", Email: "gotify-config@example.com", Password: "password123"}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.NotificationConfig{UserID: user.ID, GotifyURL: "https://gotify.example.com"}).Error)

		h := channels(t)[string(models.ChannelGotify)]
		assert.False(t, h.Configured, "a URL without a stored token cannot dispatch")
		assert.Equal(t, NotificationHealthUnconfigured, h.Status)
	})
}

// healthTestReminders creates n reminders for the user (each on its own
// contact, so the notification_deliveries foreign key is satisfiable) and
// returns their ids.
func healthTestReminders(t *testing.T, db *gorm.DB, userID uint, n int) []uint {
	t.Helper()
	ids := make([]uint, 0, n)
	byMail := false
	for i := 0; i < n; i++ {
		contact := models.Contact{UserID: userID, Firstname: "Health", Lastname: "Test"}
		require.NoError(t, db.Create(&contact).Error)
		reminder := models.Reminder{
			UserID:     userID,
			ContactID:  &contact.ID,
			Message:    "health test reminder",
			ByMail:     &byMail,
			RemindAt:   time.Now().Add(time.Hour),
			Recurrence: "once",
		}
		require.NoError(t, db.Create(&reminder).Error)
		ids = append(ids, reminder.ID)
	}
	return ids
}

// str returns a pointer to s.
func str(s string) *string { return &s }

// now returns a time pointer n hours before now, for building SentAt values.
func now(t *testing.T, hoursAgo int) *time.Time {
	t.Helper()
	ts := time.Now().UTC().Add(-time.Duration(hoursAgo) * time.Hour)
	return &ts
}

// writeFCMServiceAccount writes a valid Firebase service-account JSON to a
// temp file and returns its path, so LoadFCMServiceAccount can load it via
// cfg.FCMServiceAccountFile (the same shape the existing FCM tests build).
func writeFCMServiceAccount(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	data, err := json.Marshal(map[string]string{
		"project_id":   "my-project",
		"client_email": "firebase-adminsdk@my-project.iam.gserviceaccount.com",
		"private_key":  string(pemBytes),
	})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "fcm-sa.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}
