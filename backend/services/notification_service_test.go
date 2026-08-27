package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/i18n"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupNotificationTestDB opens a REAL migrated database (database.InitDB, not
// AutoMigrate — the N9 tables carry CHECK constraints and a partial unique
// index that AutoMigrate cannot see, and the senders rely on the exact columns
// the migration creates).
func setupNotificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// fakeChannelServer is a configurable stand-in for a ntfy/Gotify/push-service
// endpoint: it records every request it receives and can be told to answer
// specific path prefixes with specific status codes.
type fakeChannelServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	hits     []string
	bodyLens []int
	bodies   []string
	statuses map[string]int
}

func newFakeChannelServer(t *testing.T, statuses map[string]int) *fakeChannelServer {
	t.Helper()
	f := &fakeChannelServer{statuses: statuses}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) //nolint:errcheck
		f.mu.Lock()
		f.hits = append(f.hits, r.URL.Path)
		f.bodyLens = append(f.bodyLens, len(body))
		f.bodies = append(f.bodies, string(body))
		status := http.StatusOK
		for prefix, s := range statuses {
			if strings.HasPrefix(r.URL.Path, prefix) {
				status = s
				break
			}
		}
		f.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeChannelServer) URL() string { return f.server.URL }

func (f *fakeChannelServer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.hits)
}

// lastBody returns the raw body of the most recently received request, or ""
// if no request has landed yet.
func (f *fakeChannelServer) lastBody() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) == 0 {
		return ""
	}
	return f.bodies[len(f.bodies)-1]
}

// lastBodyLen returns the byte length of the most recently received request
// body, or -1 if no request has landed yet.
func (f *fakeChannelServer) lastBodyLen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodyLens) == 0 {
		return -1
	}
	return f.bodyLens[len(f.bodyLens)-1]
}

// newNotificationUser creates a user with the given channel toggles enabled
// and a saved channel config pointing at the fake servers.
func newNotificationUser(t *testing.T, db *gorm.DB, notifyNtfy, notifyGotify, notifyPush bool, ntfyURL, gotifyURL string) models.User {
	t.Helper()
	user := models.User{
		Username:     "n9-" + strings.ReplaceAll(t.Name(), "/", "-"),
		Password:     "password123",
		Email:        "n9@example.com",
		Language:     "en",
		NotifyNtfy:   notifyNtfy,
		NotifyGotify: notifyGotify,
		NotifyPush:   notifyPush,
	}
	require.NoError(t, db.Create(&user).Error)

	nc := &models.NotificationConfig{
		UserID:    user.ID,
		NtfyURL:   ntfyURL,
		NtfyTopic: "my-topic",
		GotifyURL: gotifyURL,
	}
	if gotifyURL != "" {
		enc, err := EncryptCredential("test-jwt-secret-that-is-long-enough", "gotify-token")
		require.NoError(t, err)
		nc.GotifyTokenEncrypted = enc
	}
	if ntfyURL != "" || gotifyURL != "" {
		require.NoError(t, db.Create(nc).Error)
	}
	return user
}

func newDueReminder(t *testing.T, db *gorm.DB, user models.User, message string) models.Reminder {
	t.Helper()
	contact := models.Contact{UserID: user.ID, Firstname: "Jane", Lastname: "Doe"}
	require.NoError(t, db.Create(&contact).Error)

	byMailTrue := true
	r := models.Reminder{
		UserID:     user.ID,
		ContactID:  &contact.ID,
		Message:    message,
		ByMail:     &byMailTrue,
		RemindAt:   time.Now().Add(-1 * time.Hour),
		Recurrence: "once",
	}
	require.NoError(t, db.Create(&r).Error)
	return r
}

// TestNotificationSendersCoverAllChannels pins the channel registry against
// the model's AllNotificationChannels list: every declared channel must have a
// sender registered, so adding a channel to the enum without wiring a sender
// (or vice versa) is caught here rather than silently never dispatching.
func TestNotificationSendersCoverAllChannels(t *testing.T) {
	registered := map[string]bool{}
	for _, s := range notificationSenders {
		registered[string(s.Channel())] = true
	}
	for _, ch := range models.AllNotificationChannels {
		assert.True(t, registered[string(ch)], "channel %q must have a registered sender", ch)
	}
	assert.Len(t, notificationSenders, len(models.AllNotificationChannels))
}

// TestSendReminders_DispatchesToNtfy is the primary per-channel dispatch test:
// a due reminder plus an enabled, configured ntfy channel produces exactly one
// POST to the user's topic, a 'sent' delivery row, and no re-send on a second
// run (the 'sent' delivery is what makes the reminder due-for-that-channel no
// longer).
func TestSendReminders_DispatchesToNtfy(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, true, false, false, fake.URL(), "")
	reminder := newDueReminder(t, db, user, "Call Jane")

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{UseResend: true, ResendAPIKey: "k", ResendFromEmail: "noreply@example.com", ReminderTime: "12:00"}
	require.NoError(t, SendReminders(db, cfg))

	require.Equal(t, 1, fake.count(), "the ntfy topic must receive exactly one POST")
	assert.Equal(t, "/my-topic", fake.hits[0], "the POST must go to /{topic}")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ? AND channel = ?", reminder.ID, "ntfy").Find(&deliveries).Error)
	require.Len(t, deliveries, 1, "one delivery row for the ntfy channel")
	assert.Equal(t, "ntfy", deliveries[0].Channel)
	assert.Equal(t, "sent", deliveries[0].Status)
	require.NotNil(t, deliveries[0].SentAt)
	assert.Nil(t, deliveries[0].Error)

	// Second run: the sent delivery makes the reminder not-due for ntfy.
	require.NoError(t, SendReminders(db, cfg))
	assert.Equal(t, 1, fake.count(), "the sent delivery must prevent a second ntfy POST")
}

// TestSendReminders_ChannelFailureIsolation pins the ticket's core invariant:
// a failure in one channel must not mark the reminder as sent and must not
// block another channel from dispatching.
func TestSendReminders_ChannelFailureIsolation(t *testing.T) {
	db := setupNotificationTestDB(t)
	// The ntfy target is /{topic} = /my-topic; the gotify target is {url}/message.
	fake := newFakeChannelServer(t, map[string]int{"/message": 500})
	user := newNotificationUser(t, db, true, true, false, fake.URL(), fake.URL())
	reminder := newDueReminder(t, db, user, "Isolate failures")

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{UseResend: true, ResendAPIKey: "k", ResendFromEmail: "noreply@example.com", ReminderTime: "12:00", JWTSecretKey: "test-jwt-secret-that-is-long-enough"}
	require.NoError(t, SendReminders(db, cfg))

	// ntfy succeeded, gotify failed — both delivered independently.
	var ntfyHits, gotifyHits int
	for _, p := range fake.hits {
		if strings.HasPrefix(p, "/my-topic") {
			ntfyHits++
		}
		if strings.HasPrefix(p, "/message") {
			gotifyHits++
		}
	}
	assert.Equal(t, 1, ntfyHits, "ntfy must dispatch despite gotify failing")
	assert.Equal(t, 1, gotifyHits, "gotify must still be attempted")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ? AND channel IN ?", reminder.ID, []string{"ntfy", "gotify"}).Find(&deliveries).Error)
	require.Len(t, deliveries, 2, "one delivery row per push-style channel")

	statusByChannel := map[string]string{}
	for _, d := range deliveries {
		statusByChannel[d.Channel] = d.Status
	}
	assert.Equal(t, "sent", statusByChannel["ntfy"])
	assert.Equal(t, "failed", statusByChannel["gotify"])

	// A second run retries only the failed gotify channel.
	require.NoError(t, SendReminders(db, cfg))

	ntfyHits, gotifyHits = 0, 0
	for _, p := range fake.hits {
		if strings.HasPrefix(p, "/my-topic") {
			ntfyHits++
		}
		if strings.HasPrefix(p, "/message") {
			gotifyHits++
		}
	}
	assert.Equal(t, 1, ntfyHits, "ntfy must NOT re-send (it has a sent delivery)")
	assert.Equal(t, 2, gotifyHits, "gotify must retry (its failed delivery leaves the reminder due)")
}

// TestSendReminders_PrivateAddressPerPolicy pins the SSRF policy reuse: with
// WEBHOOK_BLOCK_PRIVATE_URLS on, a private-address ntfy target is rejected
// before any network call and recorded as failed; with it off (the default,
// for self-hosters), the same target is reached.
func TestSendReminders_PrivateAddressPerPolicy(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, true, false, false, fake.URL(), "")
	reminder := newDueReminder(t, db, user, "SSRF policy")

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{ReminderTime: "12:00"}

	// Guard on: the loopback fake server must never be reached.
	cfg.WebhookBlockPrivateURLs = true
	require.NoError(t, SendReminders(db, cfg))
	assert.Equal(t, 0, fake.count(), "the guarded dispatch must never reach a private-address target")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", reminder.ID).Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "failed", deliveries[0].Status)
	require.NotNil(t, deliveries[0].Error)
	assert.Contains(t, *deliveries[0].Error, "private or loopback", "the delivery record must carry the SSRF reason")

	// Guard off (default): the same private target is reached.
	cfg.WebhookBlockPrivateURLs = false
	require.NoError(t, SendReminders(db, cfg))
	require.Equal(t, 1, fake.count(), "the default (unfiltered) policy must reach a private ntfy server")
}

// TestSendReminders_PushPrivateAddressBlocked pins that the push channel's
// reuse of clientFor is governed by the same SSRF policy as webhooks. Unlike
// ntfy/gotify (whose postNotificationJSON does its own pre-flight),
// sendPushMessage hands clientFor straight to webpush-go — the guarded dialer
// is the only thing between a user-controlled push endpoint and the network,
// so a loopback endpoint must be refused on the real send path and the
// reminder stays due (a failed delivery, not a silent drop).
func TestSendReminders_PushPrivateAddressBlocked(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, false, false, true, "", "")
	reminder := newDueReminder(t, db, user, "SSRF push")

	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: fake.URL() + "/push",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
	require.NoError(t, db.Create(&sub).Error)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{ReminderTime: "12:00", WebhookBlockPrivateURLs: true}
	require.NoError(t, SendReminders(db, cfg))

	assert.Equal(t, 0, fake.count(), "the guarded push client must never reach a loopback endpoint")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ? AND channel = ?", reminder.ID, "push").Find(&deliveries).Error)
	require.Len(t, deliveries, 1, "the push failure must be recorded, not silently dropped")
	assert.Equal(t, "failed", deliveries[0].Status)
	require.NotNil(t, deliveries[0].Error)
	assert.Contains(t, *deliveries[0].Error, ErrWebhookPrivateAddress.Error(),
		"the push delivery record must carry the SSRF reason surfaced by the guarded dialer")
}

// TestSendReminders_PushDeliversAndRecords verifies Web Push end to end
// against a fake push service: a registered device gets one encrypted push per
// due reminder, recorded as 'sent'.
func TestSendReminders_PushDeliversAndRecords(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, false, false, true, "", "")
	reminder := newDueReminder(t, db, user, "Push me")

	// The subscription endpoint is the fake push service; keys are the known
	// valid pair webpush-go's own tests use.
	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: fake.URL() + "/push",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
	require.NoError(t, db.Create(&sub).Error)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{ReminderTime: "12:00"}
	require.NoError(t, SendReminders(db, cfg))

	require.Equal(t, 1, fake.count(), "the push service must receive one encrypted push")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", reminder.ID).Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "push", deliveries[0].Channel)
	assert.Equal(t, "sent", deliveries[0].Status)

	// The subscription survives a successful push.
	var remaining int64
	require.NoError(t, db.Model(&models.PushSubscription{}).Where("id = ?", sub.ID).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining)
}

// TestPushSender_RemovesStaleSubscription verifies the 404/410 path: a push
// service that no longer knows a subscription must have that subscription
// dropped, and the reminder must NOT be marked sent (it stays due for push so
// a re-registered device gets it).
func TestPushSender_RemovesStaleSubscription(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, map[string]int{"/push": 404})
	user := newNotificationUser(t, db, false, false, true, "", "")
	reminder := newDueReminder(t, db, user, "Stale device")

	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: fake.URL() + "/push",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
	require.NoError(t, db.Create(&sub).Error)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{ReminderTime: "12:00"}
	require.NoError(t, SendReminders(db, cfg))

	var remaining int64
	require.NoError(t, db.Model(&models.PushSubscription{}).Where("id = ?", sub.ID).Count(&remaining).Error)
	assert.Zero(t, remaining, "a 404/410 subscription must be removed")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", reminder.ID).Find(&deliveries).Error)
	assert.Empty(t, deliveries, "a stale-subscription push must not be recorded as sent — it stays due")
}

// TestSendReminders_EmailAndNtfyBothDispatch verifies that the legacy email
// digest and a push-style channel can dispatch for the same reminder in the
// same run, each with its own delivery record.
func TestSendReminders_EmailAndNtfyBothDispatch(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, true, false, false, fake.URL(), "")
	reminder := newDueReminder(t, db, user, "Both channels")

	var emailCalls int
	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error {
		emailCalls++
		return nil
	}
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{UseResend: true, ResendAPIKey: "k", ResendFromEmail: "noreply@example.com", ReminderTime: "12:00"}
	require.NoError(t, SendReminders(db, cfg))

	assert.Equal(t, 1, emailCalls, "the email digest must dispatch")
	assert.Equal(t, 1, fake.count(), "ntfy must dispatch")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", reminder.ID).Find(&deliveries).Error)
	require.Len(t, deliveries, 2, "one delivery per channel")

	statusByChannel := map[string]string{}
	for _, d := range deliveries {
		statusByChannel[d.Channel] = d.Status
	}
	assert.Equal(t, "sent", statusByChannel["email"])
	assert.Equal(t, "sent", statusByChannel["ntfy"])

	// Legacy mirror stays in step for email consumers.
	var reloaded models.Reminder
	require.NoError(t, db.First(&reloaded, reminder.ID).Error)
	assert.True(t, reloaded.EmailSent)
	assert.NotNil(t, reloaded.LastSent)
}

// TestSaveNotificationConfig_RoundTrip covers config save/reload: URLs and
// toggles persist, the Gotify token is stored encrypted (never plaintext) and
// an empty token on update keeps the stored one.
func TestSaveNotificationConfig_RoundTrip(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, false, "", "")

	trueVal := true
	saved, err := SaveNotificationConfig(db, "test-jwt-secret-that-is-long-enough", user.ID, models.NotificationConfigInput{
		NtfyURL:     "https://ntfy.example.com",
		NtfyTopic:   "alerts",
		GotifyURL:   "https://gotify.example.com",
		GotifyToken: "secret-token",
		NotifyNtfy:  &trueVal,
	})
	require.NoError(t, err)
	assert.True(t, saved.HasGotifyToken())

	loaded, err := GetNotificationConfigForUser(db, user.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "https://ntfy.example.com", loaded.NtfyURL)
	assert.Equal(t, "alerts", loaded.NtfyTopic)
	assert.Equal(t, "https://gotify.example.com", loaded.GotifyURL)

	// Token is encrypted at rest and round-trips.
	assert.NotContains(t, loaded.GotifyTokenEncrypted, "secret-token", "the raw token must never be stored")
	decrypted, err := DecryptCredential("test-jwt-secret-that-is-long-enough", loaded.GotifyTokenEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "secret-token", decrypted)

	var userReloaded models.User
	require.NoError(t, db.First(&userReloaded, user.ID).Error)
	assert.True(t, userReloaded.NotifyNtfy, "the per-user toggle must be applied by SaveNotificationConfig")

	// An empty token on update keeps the stored one.
	_, err = SaveNotificationConfig(db, "test-jwt-secret-that-is-long-enough", user.ID, models.NotificationConfigInput{
		NtfyURL:   "https://ntfy.example.com",
		NtfyTopic: "alerts",
	})
	require.NoError(t, err)
	loaded, err = GetNotificationConfigForUser(db, user.ID)
	require.NoError(t, err)
	decrypted, err = DecryptCredential("test-jwt-secret-that-is-long-enough", loaded.GotifyTokenEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "secret-token", decrypted, "an empty token must leave the stored token unchanged")
}

// TestCreatePushSubscription_DedupeEndpoint verifies that re-registering the
// same endpoint updates the existing row instead of creating a duplicate
// device.
func TestCreatePushSubscription_DedupeEndpoint(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, false, "", "")

	first, err := CreatePushSubscription(db, user.ID, models.PushSubscriptionInput{
		Endpoint: "https://push.example.com/abc",
		P256dh:   "key1",
		Auth:     "auth1",
	})
	require.NoError(t, err)

	second, err := CreatePushSubscription(db, user.ID, models.PushSubscriptionInput{
		Endpoint: "https://push.example.com/abc",
		P256dh:   "key2",
		Auth:     "auth2",
	})
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "re-registering the same endpoint must update the same row")

	var count int64
	require.NoError(t, db.Model(&models.PushSubscription{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// TestTestNotificationChannel covers the Settings test button: configured ntfy
// sends a test notification; an unconfigured channel returns a descriptive
// error instead of failing silently.
func TestTestNotificationChannel(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, true, false, false, fake.URL(), "")

	cfg := config.Config{ReminderTime: "12:00"}
	require.NoError(t, TestNotificationChannel(db, cfg, user, models.ChannelNtfy))
	assert.Equal(t, 1, fake.count(), "the test button must send one ntfy notification")

	// No delivery rows are written for a test — it is not reminder-scoped.
	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	assert.Empty(t, deliveries)

	// Unconfigured channel: descriptive error, no panic.
	other := models.User{Username: "n9-unconfigured", Password: "password123", Email: "u@example.com"}
	require.NoError(t, db.Create(&other).Error)
	err := TestNotificationChannel(db, cfg, other, models.ChannelNtfy)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

// TestPushRecordSize pins the RecordSize computation (T51): a short payload
// must produce a record far below webpush-go's 4096-byte MaxRecordSize
// default, and a payload too large to fit must be capped at MaxRecordSize
// rather than growing unbounded.
//
// The expectations are deliberately written as literals rather than as
// len(payload)+webPushRecordOverhead: an assertion built from the same
// constant the implementation uses holds for *any* value of that constant,
// including a wrong one, so it would pin nothing. The constant itself is
// pinned against the real library in TestPushRecordSizeMatchesWebpushFraming.
func TestPushRecordSize(t *testing.T) {
	assert.Equal(t, 103, webPushRecordOverhead,
		"webpush-go aes128gcm framing: 16B salt + 4B rs + 1B keylen + 65B pubkey + 1B delimiter + 16B GCM tag")

	assert.Equal(t, uint32(103), pushRecordSize(0))
	assert.Equal(t, uint32(123), pushRecordSize(20))

	shortPayload := len(`{"title":"Test notification","body":"This is a test notification"}`)
	assert.Equal(t, uint32(169), pushRecordSize(shortPayload))
	assert.Less(t, pushRecordSize(shortPayload), webpush.MaxRecordSize,
		"a short payload must not be padded out to the library's 4096-byte default")

	// Cap boundary: 3993 is the largest payload that still fits exactly, so
	// it lands on MaxRecordSize uncapped; everything above it is capped
	// there rather than left to grow past what push services accept.
	fits := int(webpush.MaxRecordSize) - webPushRecordOverhead
	assert.Equal(t, 3993, fits)
	assert.Equal(t, webpush.MaxRecordSize, pushRecordSize(fits))
	assert.Equal(t, webpush.MaxRecordSize, pushRecordSize(fits+1))
	assert.Equal(t, webpush.MaxRecordSize, pushRecordSize(int(webpush.MaxRecordSize)))
}

// TestPushRecordSizeMatchesWebpushFraming pins webPushRecordOverhead against
// the actual library rather than against arithmetic that reuses it (T51).
// For each payload size, the RecordSize pushRecordSize computes must encode
// successfully and put exactly that many bytes on the wire, and one byte
// less must be rejected with ErrMaxPadExceeded — which is what makes 103 the
// exact minimum rather than merely "big enough". An overhead that is too
// small breaks every push send outright; this is the assertion that catches
// it.
func TestPushRecordSizeMatchesWebpushFraming(t *testing.T) {
	fake := newFakeChannelServer(t, nil)
	vapidPrivate, vapidPublic, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)

	sub := &webpush.Subscription{
		Endpoint: fake.URL() + "/push",
		Keys: webpush.Keys{
			P256dh: "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
			Auth:   "zqbxT6JKstKSY9JKibZLSQ",
		},
	}
	opts := func(rs uint32) *webpush.Options {
		return &webpush.Options{
			Subscriber:      "mailto:test@example.com",
			VAPIDPublicKey:  vapidPublic,
			VAPIDPrivateKey: vapidPrivate,
			TTL:             86400,
			RecordSize:      rs,
		}
	}

	for _, payloadLen := range []int{0, 1, 20, 200, int(webpush.MaxRecordSize) - webPushRecordOverhead} {
		payload := bytes.Repeat([]byte("a"), payloadLen)
		rs := pushRecordSize(payloadLen)

		resp, err := webpush.SendNotification(payload, sub, opts(rs))
		require.NoErrorf(t, err, "a %d-byte payload must encode at RecordSize %d", payloadLen, rs)
		resp.Body.Close()
		assert.Equalf(t, int(rs), fake.lastBodyLen(),
			"a %d-byte payload must put exactly RecordSize (%d) bytes on the wire", payloadLen, rs)

		_, err = webpush.SendNotification(payload, sub, opts(rs-1))
		assert.ErrorIsf(t, err, webpush.ErrMaxPadExceeded,
			"RecordSize %d (one under) must not fit a %d-byte payload — overhead %d is not minimal",
			rs-1, payloadLen, webPushRecordOverhead)
	}
}

// TestTestNotificationChannel_PushBodySizeMatchesPayload verifies the fix
// end to end at the wire level (T51): the actual encrypted request body the
// push service receives for a short test notification is sized to that
// payload, not padded out to the ~4180-byte body the library's 4096-byte
// default would have produced regardless of content.
func TestTestNotificationChannel_PushBodySizeMatchesPayload(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, false, false, true, "", "")

	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: fake.URL() + "/push",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
	require.NoError(t, db.Create(&sub).Error)

	cfg := config.Config{ReminderTime: "12:00"}
	require.NoError(t, TestNotificationChannel(db, cfg, user, models.ChannelPush))
	require.Equal(t, 1, fake.count(), "the test button must send one push notification")

	lang := notificationLanguage(user)
	payload, err := json.Marshal(map[string]string{
		"title": i18n.T(lang, "notifications.testTitle"),
		"body":  i18n.T(lang, "notifications.testBody"),
	})
	require.NoError(t, err)

	wantBodyLen := int(pushRecordSize(len(payload)))
	assert.Equal(t, wantBodyLen, fake.lastBodyLen(), "the wire body must be sized to the actual payload's RecordSize")
	assert.Less(t, fake.lastBodyLen(), 300, "a short test notification must not be padded out toward the 4096-byte default")
}

// TestSendReminders_JobLockStillPreventsDoubleSend verifies the N9 rewrite did
// not weaken the distributed lock: a second SendRemindersWithRateLimit call
// inside the min interval sends nothing on any channel.
func TestSendReminders_JobLockStillPreventsDoubleSend(t *testing.T) {
	db := setupNotificationTestDB(t)
	fake := newFakeChannelServer(t, nil)
	user := newNotificationUser(t, db, true, false, false, fake.URL(), "")
	newDueReminder(t, db, user, "Locked reminder")

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	originalInterval := ReminderMinInterval
	ReminderMinInterval = time.Hour
	defer func() { ReminderMinInterval = originalInterval }()

	cfg := config.Config{ReminderTime: "12:00"}
	require.NoError(t, SendRemindersWithRateLimit(db, cfg))
	require.NoError(t, SendRemindersWithRateLimit(db, cfg))

	assert.Equal(t, 1, fake.count(), "the job lock must prevent a second dispatch run")
}

// TestNotificationShortBody pins the short-form body builder: contact name is
// prefixed when present, raw message otherwise.
func TestNotificationShortBody(t *testing.T) {
	contactID := uint(7)
	withContact := map[uint]string{7: "Jane Doe"}
	noContact := map[uint]string{}

	assert.Equal(t, "Jane Doe: Call me", notificationShortBody(models.Reminder{ContactID: &contactID, Message: "Call me"}, withContact))
	assert.Equal(t, "Call me", notificationShortBody(models.Reminder{ContactID: &contactID, Message: "Call me"}, noContact))
	assert.Equal(t, "Just the message", notificationShortBody(models.Reminder{Message: "Just the message"}, withContact))
}

// TestRecordNotificationDeliveryFailureCarriesError pins the failed-delivery
// bookkeeping used by every channel sender.
func TestRecordNotificationDeliveryFailureCarriesError(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, false, "", "")
	first := newDueReminder(t, db, user, "first")
	second := newDueReminder(t, db, user, "second")

	recordNotificationDelivery(context.Background(), db, first.ID, models.ChannelNtfy, false, "boom")

	var d models.NotificationDelivery
	require.NoError(t, db.First(&d).Error)
	assert.Equal(t, "failed", d.Status)
	assert.Nil(t, d.SentAt)
	require.NotNil(t, d.Error)
	assert.Equal(t, "boom", *d.Error)

	recordNotificationDelivery(context.Background(), db, second.ID, models.ChannelGotify, true, "")
	var sent models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", second.ID).First(&sent).Error)
	assert.Equal(t, "sent", sent.Status)
	require.NotNil(t, sent.SentAt)

	// Each attempt also emits an operational event (issue #424): the failed
	// ntfy attempt -> notification_failed (severity error), the successful
	// gotify one -> notification_sent. detail carries the channel name only.
	var failedEv models.SystemEvent
	require.NoError(t, db.Where("event_type = ?", models.SysEventNotificationFailed).First(&failedEv).Error)
	assert.Equal(t, logger.ComponentNotify, failedEv.Component)
	assert.Equal(t, logger.SeverityError, failedEv.Severity)
	assert.Equal(t, "channel=ntfy", failedEv.Detail)
	assert.Equal(t, "boom", failedEv.Error)

	var sentEv models.SystemEvent
	require.NoError(t, db.Where("event_type = ?", models.SysEventNotificationSent).First(&sentEv).Error)
	assert.Equal(t, "channel=gotify", sentEv.Detail)
	require.NotNil(t, sentEv.Result)
	assert.Equal(t, logger.ResultSuccess, *sentEv.Result)
	assert.Nil(t, sent.Error)
}

// TestVAPIDKeysGeneratedOnce pins VAPID key management: the pair is generated
// once and stable across subsequent calls (changing keys would orphan existing
// subscriptions).
func TestVAPIDKeysGeneratedOnce(t *testing.T) {
	db := setupNotificationTestDB(t)

	pub1, priv1, err := GetVAPIDKeys(db)
	require.NoError(t, err)
	require.NotEmpty(t, pub1)
	require.NotEmpty(t, priv1)

	pub2, priv2, err := GetVAPIDKeys(db)
	require.NoError(t, err)
	assert.Equal(t, pub1, pub2, "the VAPID public key must be stable once generated")
	assert.Equal(t, priv1, priv2, "the VAPID private key must be stable once generated")
}

// TestDeleteNotificationDeliveries covers the cascade helper used at every
// reminder-deletion site.
func TestDeleteNotificationDeliveries(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, false, "", "")
	first := newDueReminder(t, db, user, "first")
	second := newDueReminder(t, db, user, "second")

	recordNotificationDelivery(context.Background(), db, first.ID, models.ChannelNtfy, true, "")
	recordNotificationDelivery(context.Background(), db, second.ID, models.ChannelGotify, true, "")

	require.NoError(t, DeleteNotificationDeliveries(db, []uint{first.ID}))

	var remaining int64
	require.NoError(t, db.Model(&models.NotificationDelivery{}).Where("reminder_id = ?", first.ID).Count(&remaining).Error)
	assert.Zero(t, remaining, "deliveries for the deleted reminder must be gone")
	require.NoError(t, db.Model(&models.NotificationDelivery{}).Where("reminder_id = ?", second.ID).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining, "other reminders' deliveries must be untouched")

	require.NoError(t, DeleteNotificationDeliveries(db, nil), "a nil id set must be a no-op")
}

// ---------------------------------------------------------------------------
// M2 — mobile device registrations + FCM delivery
// ---------------------------------------------------------------------------

// newFCMServiceAccount builds a valid in-memory service-account struct with a
// real generated RSA key, so the JWT-minting path actually signs.
func newFCMServiceAccount(t *testing.T) *fcmServiceAccount {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return &fcmServiceAccount{
		ProjectID:   "my-project",
		ClientEmail: "firebase-adminsdk@my-project.iam.gserviceaccount.com",
		PrivateKey:  string(pemBytes),
	}
}

// TestCreateDeviceRegistration_DedupeToken verifies that re-registering the
// same (client, token) for the SAME user updates the existing row instead of
// duplicating it — a device re-registers on every app launch.
func TestCreateDeviceRegistration_DedupeToken(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, false, "", "")

	first, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{
		Token:       "fcm-token-123",
		Client:      "fcm",
		DeviceLabel: "Pixel 8",
	})
	require.NoError(t, err)

	second, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{
		Token:       "fcm-token-123",
		Client:      "fcm",
		DeviceLabel: "Pixel 8 (renamed)",
	})
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "re-registering the same token must update the same row")
	assert.Equal(t, "Pixel 8 (renamed)", second.DeviceLabel)

	var count int64
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// A different client with the same token string is a distinct device
	// (an iOS app is a different registration than an Android app).
	_, err = CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{
		Token:  "fcm-token-123",
		Client: "apns",
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

// TestCreateDeviceRegistration_ReassignsAcrossUsers verifies the fix for the
// cross-user token leak: the same physical device logging out of user A and
// into user B must move the single (client, token) row to B, not accumulate a
// second live row that keeps pushing A's reminders to B's device. See
// migration 000019's comment.
func TestCreateDeviceRegistration_ReassignsAcrossUsers(t *testing.T) {
	db := setupNotificationTestDB(t)
	userA := newNotificationUser(t, db, false, false, false, "", "")
	userB := models.User{Username: "n9-user-b", Password: "password123", Email: "n9-b@example.com"}
	require.NoError(t, db.Create(&userB).Error)

	first, err := CreateDeviceRegistration(db, userA.ID, models.DeviceRegistrationInput{
		Token:       "shared-device-token",
		Client:      "fcm",
		DeviceLabel: "Family tablet (A)",
	})
	require.NoError(t, err)

	second, err := CreateDeviceRegistration(db, userB.ID, models.DeviceRegistrationInput{
		Token:       "shared-device-token",
		Client:      "fcm",
		DeviceLabel: "Family tablet (B)",
	})
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "the same token must reassign the existing row, not create a second one")
	assert.Equal(t, userB.ID, second.UserID)
	assert.Equal(t, "Family tablet (B)", second.DeviceLabel)

	var count int64
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Where("token = ? AND client = ?", "shared-device-token", "fcm").Count(&count).Error)
	assert.Equal(t, int64(1), count, "only one row must exist for the token — user A must no longer own it")

	aDevices, err := ListDeviceRegistrations(db, userA.ID)
	require.NoError(t, err)
	assert.Empty(t, aDevices, "user A's reminders must not keep targeting a device now owned by user B")

	bDevices, err := ListDeviceRegistrations(db, userB.ID)
	require.NoError(t, err)
	require.Len(t, bDevices, 1)
	assert.Equal(t, "shared-device-token", bDevices[0].Token)
}

// TestDeleteDeviceRegistration_OwnershipScoped verifies the service-level
// ownership scoping: another user's device cannot be deleted.
func TestDeleteDeviceRegistration_OwnershipScoped(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, false, "", "")
	other := models.User{Username: "n9-other-owner", Password: "password123", Email: "n9-other@example.com"}
	require.NoError(t, db.Create(&other).Error)

	others, err := CreateDeviceRegistration(db, other.ID, models.DeviceRegistrationInput{Token: "t", Client: "fcm"})
	require.NoError(t, err)

	err = DeleteDeviceRegistration(db, user.ID, others.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "another user's device must be untouchable")

	var remaining int64
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Where("id = ?", others.ID).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining, "the other user's device must survive the attempt")
}

// TestLoadFCMServiceAccount covers config-file loading: empty path is a clean
// nil, a valid file loads, a set-but-invalid file is rejected.
func TestLoadFCMServiceAccount(t *testing.T) {
	sa, err := LoadFCMServiceAccount("")
	require.NoError(t, err)
	assert.Nil(t, sa, "empty path must mean FCM is not configured")

	path := filepath.Join(t.TempDir(), "service-account.json")
	valid := `{"project_id":"p","client_email":"e@example.com","private_key":"-----BEGIN PRIVATE KEY-----\nZm9v\n-----END PRIVATE KEY-----\n"}`
	require.NoError(t, os.WriteFile(path, []byte(valid), 0o600))

	sa, err = LoadFCMServiceAccount(path)
	require.NoError(t, err)
	require.NotNil(t, sa)
	assert.Equal(t, "p", sa.ProjectID)
	assert.Equal(t, "e@example.com", sa.ClientEmail)

	// Missing a required field.
	badPath := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(badPath, []byte(`{"project_id":"p"}`), 0o600))
	_, err = LoadFCMServiceAccount(badPath)
	assert.ErrorIs(t, err, ErrFCMInvalidServiceAccount)

	// Unreadable path.
	_, err = LoadFCMServiceAccount(filepath.Join(t.TempDir(), "nope.json"))
	assert.ErrorIs(t, err, ErrFCMInvalidServiceAccount)
}

// TestFcmReminderData pins the FCM `data` payload shape the Android client
// keys on (issue #152): a canonical second-truncated UTC `due_at`, the reminder
// id, and a contact deep link only when the reminder has a contact. The
// canonical `due_at` matters because the client normalizes both this and the
// polling worker's `remind_at` to epoch seconds to form one idempotence key.
func TestFcmReminderData(t *testing.T) {
	remindAt := time.Date(2026, 8, 18, 14, 14, 28, 638126406, time.FixedZone("EST", -5*60*60))

	t.Run("with contact", func(t *testing.T) {
		contactID := uint(7)
		data := fcmReminderData(models.Reminder{Model: gorm.Model{ID: 42}, RemindAt: remindAt, ContactID: &contactID})
		assert.Equal(t, "reminder", data["type"])
		assert.Equal(t, "42", data["reminder_id"])
		// Second-truncated UTC — nanoseconds and the local offset are gone.
		assert.Equal(t, "2026-08-18T19:14:28Z", data["due_at"])
		assert.Equal(t, "mycorrhizal://contacts/7", data["deep_link"])
	})

	t.Run("without contact", func(t *testing.T) {
		data := fcmReminderData(models.Reminder{Model: gorm.Model{ID: 42}, RemindAt: remindAt})
		assert.Equal(t, "42", data["reminder_id"])
		assert.Equal(t, "2026-08-18T19:14:28Z", data["due_at"])
		_, ok := data["deep_link"]
		assert.False(t, ok, "a contact-less reminder must not carry a deep_link")
	})
}

// TestSendFCMMessage_HappyPath delivers a test message through a fake
// Google token + FCM endpoint, asserting the full request flow (token
// exchange, then messages:send) and the 404-stale path.
func TestSendFCMMessage_HappyPath(t *testing.T) {
	sa := newFCMServiceAccount(t)

	// The token exchange needs an access_token in the response — a dedicated
	// fake Google token server.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token","expires_in":3600}`))
	}))
	defer tokenServer.Close()
	originalTokenURL := fcmTokenEndpoint
	fcmTokenEndpoint = tokenServer.URL
	defer func() { fcmTokenEndpoint = originalTokenURL }()

	// The messages:send call goes to the recorded fake channel server.
	fake := newFakeChannelServer(t, nil)
	originalSendURL := fcmSendEndpoint
	fcmSendEndpoint = fake.URL()
	defer func() { fcmSendEndpoint = originalSendURL }()

	stale, err := sendFCMMessage(config.Config{ReminderTime: "12:00"}, sa, "device-token", "Title", "Body", map[string]string{"type": "reminder"})
	require.NoError(t, err)
	assert.False(t, stale)
	require.Equal(t, 1, fake.count())
	assert.Equal(t, "/projects/my-project/messages:send", fake.hits[0])
}

// TestSendFCMMessage_StaleToken verifies the 404 → stale path: a token FCM no
// longer knows must be reported as stale so the caller drops the registration.
func TestSendFCMMessage_StaleToken(t *testing.T) {
	sa := newFCMServiceAccount(t)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token"}`))
	}))
	defer tokenServer.Close()
	fcmTokenEndpoint = tokenServer.URL

	fake := newFakeChannelServer(t, map[string]int{"/projects": 404})
	originalSendURL := fcmSendEndpoint
	fcmSendEndpoint = fake.URL()
	defer func() { fcmSendEndpoint = originalSendURL }()

	stale, err := sendFCMMessage(config.Config{ReminderTime: "12:00"}, sa, "dead-token", "T", "B", nil)
	require.NoError(t, err)
	assert.True(t, stale, "a 404 from FCM must be reported as stale")
}

// TestSendReminders_FCMDelivers verifies the push channel dispatches to an FCM
// device registration end to end: one message:send per due reminder, a 'sent'
// delivery row, and the registration surviving.
func TestSendReminders_FCMDelivers(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, true, "", "")
	reminder := newDueReminder(t, db, user, "Push via FCM")
	sa := newFCMServiceAccount(t)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token"}`))
	}))
	defer tokenServer.Close()
	fcmTokenEndpoint = tokenServer.URL

	fake := newFakeChannelServer(t, nil)
	fcmSendEndpoint = fake.URL()

	device, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{Token: "fcm-token", Client: "fcm", DeviceLabel: "Pixel"})
	require.NoError(t, err)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	// Write the service account to a temp file so SendReminders can load it
	// via cfg.FCMServiceAccountFile.
	saJSON := map[string]string{
		"project_id":   sa.ProjectID,
		"client_email": sa.ClientEmail,
		"private_key":  sa.PrivateKey,
	}
	data, err := json.Marshal(saJSON)
	require.NoError(t, err)
	saPath := filepath.Join(t.TempDir(), "sa.json")
	require.NoError(t, os.WriteFile(saPath, data, 0o600))

	cfg := config.Config{ReminderTime: "12:00", FCMServiceAccountFile: saPath}
	require.NoError(t, SendReminders(db, cfg))

	require.Equal(t, 1, fake.count(), "the FCM endpoint must receive one messages:send")
	assert.Equal(t, "/projects/my-project/messages:send", fake.hits[0])

	// The `data` payload is what lets the Android client deep-link into the
	// contact and dedupe against the polling worker (M5 §6.6/§6.8): assert the
	// deep_link, reminder_id and due_at all survive the round trip.
	var sent struct {
		Message struct {
			Data map[string]string `json:"data"`
		} `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(fake.lastBody()), &sent))
	assert.Equal(t, "reminder", sent.Message.Data["type"])
	assert.Equal(t, strconv.FormatUint(uint64(reminder.ID), 10), sent.Message.Data["reminder_id"])
	assert.Equal(t, reminder.RemindAt.UTC().Truncate(time.Second).Format(time.RFC3339), sent.Message.Data["due_at"])
	assert.Equal(t, fmt.Sprintf("mycorrhizal://contacts/%d", *reminder.ContactID), sent.Message.Data["deep_link"])

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", reminder.ID).Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "push", deliveries[0].Channel)
	assert.Equal(t, "sent", deliveries[0].Status)

	var remaining int64
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Where("id = ?", device.ID).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining, "a successful send must keep the registration")
}

// TestSendReminders_FCMStaleRegistrationDropped verifies the dead-token path:
// a 404 from FCM drops the registration and the reminder stays due for push.
func TestSendReminders_FCMStaleRegistrationDropped(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, true, "", "")
	newDueReminder(t, db, user, "Dead device")
	sa := newFCMServiceAccount(t)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token"}`))
	}))
	defer tokenServer.Close()
	fcmTokenEndpoint = tokenServer.URL

	fake := newFakeChannelServer(t, map[string]int{"/projects": 404})
	fcmSendEndpoint = fake.URL()

	device, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{Token: "dead-token", Client: "fcm"})
	require.NoError(t, err)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	saJSON := map[string]string{
		"project_id":   sa.ProjectID,
		"client_email": sa.ClientEmail,
		"private_key":  sa.PrivateKey,
	}
	data, _ := json.Marshal(saJSON)
	saPath := filepath.Join(t.TempDir(), "sa.json")
	require.NoError(t, os.WriteFile(saPath, data, 0o600))

	cfg := config.Config{ReminderTime: "12:00", FCMServiceAccountFile: saPath}
	require.NoError(t, SendReminders(db, cfg))

	var remaining int64
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Where("id = ?", device.ID).Count(&remaining).Error)
	assert.Zero(t, remaining, "a 404 registration must be dropped")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	assert.Empty(t, deliveries, "a stale-token send must not be recorded as sent — it stays due")
}

// TestSendReminders_APNSNeverMarkedSent verifies M2 design decision 2: an apns
// registration is accepted but its delivery is a logged skip, never a marked-
// sent delivery row.
func TestSendReminders_APNSNeverMarkedSent(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, true, "", "")
	newDueReminder(t, db, user, "iOS device")

	_, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{Token: "apns-token", Client: "apns"})
	require.NoError(t, err)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	cfg := config.Config{ReminderTime: "12:00"}
	require.NoError(t, SendReminders(db, cfg))

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	assert.Empty(t, deliveries, "an apns-only device must never produce a sent delivery row")
}

// TestSendReminders_PushChannelDispatchToBoth verifies the push channel
// dispatches to both a browser web subscription and an FCM device in one run.
func TestSendReminders_PushChannelDispatchToBoth(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, true, "", "")
	reminder := newDueReminder(t, db, user, "Both devices")
	sa := newFCMServiceAccount(t)

	// Browser web subscription against the fake push service.
	fake := newFakeChannelServer(t, nil)
	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: fake.URL() + "/push",
		P256dh:   "BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk",
		Auth:     "zqbxT6JKstKSY9JKibZLSQ",
	}
	require.NoError(t, db.Create(&sub).Error)

	// FCM device against the same fake server's /projects path.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token"}`))
	}))
	defer tokenServer.Close()
	fcmTokenEndpoint = tokenServer.URL
	fcmSendEndpoint = fake.URL()

	_, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{Token: "fcm-token", Client: "fcm"})
	require.NoError(t, err)

	originalSender := sendReminderEmailFn
	sendReminderEmailFn = func(u models.User, reminders []models.Reminder, cfg config.Config, db *gorm.DB) error { return nil }
	defer func() { sendReminderEmailFn = originalSender }()

	saJSON := map[string]string{
		"project_id":   sa.ProjectID,
		"client_email": sa.ClientEmail,
		"private_key":  sa.PrivateKey,
	}
	data, _ := json.Marshal(saJSON)
	saPath := filepath.Join(t.TempDir(), "sa.json")
	require.NoError(t, os.WriteFile(saPath, data, 0o600))

	cfg := config.Config{ReminderTime: "12:00", FCMServiceAccountFile: saPath}
	require.NoError(t, SendReminders(db, cfg))

	// The web push POST goes to /push, the FCM send to /projects/...
	webHits, fcmHits := 0, 0
	for _, p := range fake.hits {
		if strings.HasPrefix(p, "/push") {
			webHits++
		}
		if strings.HasPrefix(p, "/projects") {
			fcmHits++
		}
	}
	assert.Equal(t, 1, webHits, "the web subscription must receive one push")
	assert.Equal(t, 1, fcmHits, "the FCM device must receive one messages:send")

	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", reminder.ID).Find(&deliveries).Error)
	assert.Equal(t, 2, len(deliveries), "one delivery row per push endpoint type (web + fcm)")
}

// TestTestNotificationChannel_ProbesFCMDevice pins M2 design decision 5: the
// Settings "test" button probes an FCM device, not just browser Web Push
// subscriptions — the gap that shipped without a test the first time around.
func TestTestNotificationChannel_ProbesFCMDevice(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, true, "", "")
	sa := newFCMServiceAccount(t)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token"}`))
	}))
	defer tokenServer.Close()
	originalTokenURL := fcmTokenEndpoint
	fcmTokenEndpoint = tokenServer.URL
	defer func() { fcmTokenEndpoint = originalTokenURL }()

	fake := newFakeChannelServer(t, nil)
	originalSendURL := fcmSendEndpoint
	fcmSendEndpoint = fake.URL()
	defer func() { fcmSendEndpoint = originalSendURL }()

	_, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{Token: "fcm-token", Client: "fcm", DeviceLabel: "Pixel"})
	require.NoError(t, err)

	saJSON := map[string]string{"project_id": sa.ProjectID, "client_email": sa.ClientEmail, "private_key": sa.PrivateKey}
	data, marshalErr := json.Marshal(saJSON)
	require.NoError(t, marshalErr)
	saPath := filepath.Join(t.TempDir(), "sa.json")
	require.NoError(t, os.WriteFile(saPath, data, 0o600))

	cfg := config.Config{ReminderTime: "12:00", FCMServiceAccountFile: saPath}
	require.NoError(t, TestNotificationChannel(db, cfg, user, models.ChannelPush))
	require.Equal(t, 1, fake.count(), "the test button must reach the FCM endpoint for a registered device")
	assert.Equal(t, "/projects/my-project/messages:send", fake.hits[0])

	// No reminder-scoped delivery rows are written by a test notification,
	// same invariant as every other channel's test path.
	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	assert.Empty(t, deliveries)
}

// TestTestNotificationChannel_FCMDeviceWithoutServerConfig verifies that an
// FCM-only user testing the push channel on a server with no
// FCM_SERVICE_ACCOUNT_FILE gets a descriptive "not configured" error instead
// of a silent false-positive "ok: true".
func TestTestNotificationChannel_FCMDeviceWithoutServerConfig(t *testing.T) {
	db := setupNotificationTestDB(t)
	user := newNotificationUser(t, db, false, false, true, "", "")
	_, err := CreateDeviceRegistration(db, user.ID, models.DeviceRegistrationInput{Token: "fcm-token", Client: "fcm"})
	require.NoError(t, err)

	cfg := config.Config{ReminderTime: "12:00"} // FCMServiceAccountFile unset
	err = TestNotificationChannel(db, cfg, user, models.ChannelPush)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}
