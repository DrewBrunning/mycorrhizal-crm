package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/i18n"
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
	db, err := database.InitDB(filepath.Join(t.TempDir(), "n9.db"))
	require.NoError(t, err)
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
// default, sized to len(payload)+webPushRecordOverhead, and an
// implausibly large payload must be capped at MaxRecordSize rather than
// growing unbounded.
func TestPushRecordSize(t *testing.T) {
	shortPayload := len(`{"title":"Test notification","body":"This is a test notification"}`)
	got := pushRecordSize(shortPayload)
	assert.Equal(t, uint32(shortPayload+webPushRecordOverhead), got)
	assert.Less(t, got, webpush.MaxRecordSize, "a short payload must not be padded out to the library's 4096-byte default")

	// A payload so large that payload+overhead would exceed MaxRecordSize
	// must be capped, not left to grow past what push services accept.
	assert.Equal(t, webpush.MaxRecordSize, pushRecordSize(int(webpush.MaxRecordSize)))
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

	recordNotificationDelivery(db, first.ID, models.ChannelNtfy, false, "boom")

	var d models.NotificationDelivery
	require.NoError(t, db.First(&d).Error)
	assert.Equal(t, "failed", d.Status)
	assert.Nil(t, d.SentAt)
	require.NotNil(t, d.Error)
	assert.Equal(t, "boom", *d.Error)

	recordNotificationDelivery(db, second.ID, models.ChannelGotify, true, "")
	var sent models.NotificationDelivery
	require.NoError(t, db.Where("reminder_id = ?", second.ID).First(&sent).Error)
	assert.Equal(t, "sent", sent.Status)
	require.NotNil(t, sent.SentAt)
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

	recordNotificationDelivery(db, first.ID, models.ChannelNtfy, true, "")
	recordNotificationDelivery(db, second.ID, models.ChannelGotify, true, "")

	require.NoError(t, DeleteNotificationDeliveries(db, []uint{first.ID}))

	var remaining int64
	require.NoError(t, db.Model(&models.NotificationDelivery{}).Where("reminder_id = ?", first.ID).Count(&remaining).Error)
	assert.Zero(t, remaining, "deliveries for the deleted reminder must be gone")
	require.NoError(t, db.Model(&models.NotificationDelivery{}).Where("reminder_id = ?", second.ID).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining, "other reminders' deliveries must be untouched")

	require.NoError(t, DeleteNotificationDeliveries(db, nil), "a nil id set must be a no-op")
}
