package services

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"mycorrhizal/config"
	"mycorrhizal/i18n"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/golang-jwt/jwt/v4"
	"gorm.io/gorm"
)

// ErrNotificationPrivateAddress is the sentinel surfaced when a user-supplied
// ntfy/Gotify URL resolves to a private/loopback address while
// WEBHOOK_BLOCK_PRIVATE_URLS is on. It ends up in the stored delivery record's
// Error field, exactly like the webhook dialer sentinels — and the Settings UI
// documents the flag next to the URL field so a private-address URL fails with
// an explanation, not a silent no-op.
var ErrNotificationPrivateAddress = errors.New("notification URL resolves to a private or loopback address")

// ErrInvalidNotificationURL is returned when a user-supplied ntfy/Gotify URL is
// not a well-formed http(s) URL — caught at save time, like
// NormalizeImmichBaseURL, rather than the first time the channel is used.
var ErrInvalidNotificationURL = errors.New("notification URL must be a valid http(s) URL")

// normalizeNotificationURL trims and validates a user-supplied ntfy/Gotify base
// URL. Empty is allowed (a user may use only one push-style channel); non-empty
// must parse as an explicit http/https URL with a host. Mirrors the
// NormalizeImmichBaseURL precedent: reject malformed input rather than guessing
// what the user meant (there is no way to tell whether a schemeless input means
// http or https).
func normalizeNotificationURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrInvalidNotificationURL
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// NotificationSender is one notification channel's delivery implementation
// (email, ntfy, gotify, push). Register an implementation in
// notificationSenders and SendReminders dispatches it automatically — adding a
// channel is a new implementation, never a new branch inside the dispatcher.
type NotificationSender interface {
	// Channel identifies the channel this sender delivers on.
	Channel() models.NotificationChannel

	// Enabled reports whether the channel can currently deliver for the user:
	// the channel's per-user toggle plus a usable per-channel config. When
	// false, SendReminders skips the channel entirely and the reminders are
	// preserved (not marked sent) so they are picked up once it is configured.
	Enabled(db *gorm.DB, cfg config.Config, user models.User) bool

	// Send delivers each of the given reminders to the user and records one
	// NotificationDelivery row per reminder (status 'sent'|'failed'). A failure
	// for one reminder must never prevent the others from being delivered, and
	// must never mark any reminder as sent. Returns nil when every reminder was
	// handled (sent or recorded failed).
	Send(db *gorm.DB, cfg config.Config, user models.User, reminders []models.Reminder) error
}

// notificationSenders is the channel registry, in dispatch order.
var notificationSenders = []NotificationSender{
	emailNotificationSender{},
	ntfyNotificationSender{},
	gotifyNotificationSender{},
	pushNotificationSender{},
}

// recordNotificationDelivery writes one delivery row for a reminder/channel
// attempt. sent=false stores status 'failed' plus the error message; the
// reminder stays "due" for that channel (no 'sent' row), so the next run
// retries it.
func recordNotificationDelivery(db *gorm.DB, reminderID uint, channel models.NotificationChannel, sent bool, errMsg string) {
	var (
		sentAt *time.Time
		status = "failed"
		errPtr *string
	)
	if sent {
		status = "sent"
		now := time.Now()
		sentAt = &now
	} else if errMsg != "" {
		msg := errMsg
		errPtr = &msg
	}

	d := models.NotificationDelivery{
		ReminderID: reminderID,
		Channel:    string(channel),
		Status:     status,
		SentAt:     sentAt,
		Error:      errPtr,
	}
	if err := db.Create(&d).Error; err != nil {
		logger.Error().Err(err).Uint("reminder_id", reminderID).Str("channel", string(channel)).Msg("Failed to record notification delivery")
	}
}

// DeleteNotificationDeliveries hard-deletes every delivery row for the given
// reminders. Called by the controllers' manual cascade wherever reminders are
// deleted (soft deletes leave the reminder row in place, so the delivery rows
// would otherwise dangle), and when a recurring reminder is rescheduled to its
// next occurrence (the old occurrence's deliveries must not block the new one).
func DeleteNotificationDeliveries(db *gorm.DB, reminderIDs []uint) error {
	if len(reminderIDs) == 0 {
		return nil
	}
	return db.Where("reminder_id IN ?", reminderIDs).Delete(&models.NotificationDelivery{}).Error
}

// ---------------------------------------------------------------------------
// Email
// ---------------------------------------------------------------------------

type emailNotificationSender struct{}

func (emailNotificationSender) Channel() models.NotificationChannel { return models.ChannelEmail }

func (emailNotificationSender) Enabled(_ *gorm.DB, cfg config.Config, user models.User) bool {
	// Email is configured server-side (Resend/SMTP env vars) and gated
	// per-reminder by Reminder.ByMail; the per-user email address is required.
	return cfg.EmailEnabled() && user.Email != ""
}

// Send delivers the daily email digest for the user's eligible reminders
// (birthdays are included by sendReminderEmail). On success each eligible
// reminder gets a 'sent' delivery row AND its legacy email_sent mirror flipped
// (pre-N9 consumers still read it); on failure each gets a 'failed' row and
// email_sent stays untouched so the next run retries.
func (emailNotificationSender) Send(db *gorm.DB, cfg config.Config, user models.User, reminders []models.Reminder) error {
	eligible := make([]models.Reminder, 0, len(reminders))
	for _, r := range reminders {
		if r.ByMail != nil && *r.ByMail {
			eligible = append(eligible, r)
		}
	}

	if err := sendReminderEmailFn(user, eligible, cfg, db); err != nil {
		for _, r := range eligible {
			recordNotificationDelivery(db, r.ID, models.ChannelEmail, false, err.Error())
		}
		return err
	}

	for _, r := range eligible {
		now := time.Now()
		r.EmailSent = true
		r.LastSent = &now
		if err := db.Save(&r).Error; err != nil {
			logger.Error().Err(err).Uint("reminder_id", r.ID).Msg("Failed to update reminder after sending email")
		}
		recordNotificationDelivery(db, r.ID, models.ChannelEmail, true, "")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ntfy
// ---------------------------------------------------------------------------

type ntfyNotificationSender struct{}

func (ntfyNotificationSender) Channel() models.NotificationChannel { return models.ChannelNtfy }

func (ntfyNotificationSender) Enabled(db *gorm.DB, _ config.Config, user models.User) bool {
	if !user.NotifyNtfy {
		return false
	}
	nc, err := GetNotificationConfigForUser(db, user.ID)
	if err != nil {
		logger.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to load notification config for ntfy enablement")
		return false
	}
	return nc != nil && nc.NtfyURL != "" && nc.NtfyTopic != ""
}

// sendNtfyMessage posts a single ntfy message to the user's configured topic.
func sendNtfyMessage(cfg config.Config, nc *models.NotificationConfig, title, message string) error {
	target := strings.TrimRight(nc.NtfyURL, "/") + "/" + url.PathEscape(nc.NtfyTopic)
	payload, err := json.Marshal(map[string]string{"title": title, "message": message})
	if err != nil {
		return err
	}
	return postNotificationJSON(cfg, target, payload, "", "")
}

func (ntfyNotificationSender) Send(db *gorm.DB, cfg config.Config, user models.User, reminders []models.Reminder) error {
	nc, err := GetNotificationConfigForUser(db, user.ID)
	if err != nil {
		return fmt.Errorf("failed to load notification config: %w", err)
	}
	if nc == nil || nc.NtfyURL == "" || nc.NtfyTopic == "" {
		return fmt.Errorf("ntfy is not configured")
	}

	lang := notificationLanguage(user)
	contactMap := loadReminderContactNames(db, user.ID, reminders)
	title := i18n.T(lang, "notifications.reminderTitle")

	var sendErr error
	for _, r := range reminders {
		body := notificationShortBody(r, contactMap)
		if err := sendNtfyMessage(cfg, nc, title, body); err != nil {
			recordNotificationDelivery(db, r.ID, models.ChannelNtfy, false, err.Error())
			if sendErr == nil {
				sendErr = err
			}
			continue
		}
		recordNotificationDelivery(db, r.ID, models.ChannelNtfy, true, "")
	}
	return sendErr
}

// ---------------------------------------------------------------------------
// Gotify
// ---------------------------------------------------------------------------

type gotifyNotificationSender struct{}

func (gotifyNotificationSender) Channel() models.NotificationChannel { return models.ChannelGotify }

func (gotifyNotificationSender) Enabled(db *gorm.DB, _ config.Config, user models.User) bool {
	if !user.NotifyGotify {
		return false
	}
	nc, err := GetNotificationConfigForUser(db, user.ID)
	if err != nil {
		logger.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to load notification config for Gotify enablement")
		return false
	}
	return nc != nil && nc.GotifyURL != "" && nc.HasGotifyToken()
}

// sendGotifyMessage posts a single message to the user's Gotify instance.
func sendGotifyMessage(cfg config.Config, nc *models.NotificationConfig, title, message string) error {
	token, err := DecryptCredential(cfg.JWTSecretKey, nc.GotifyTokenEncrypted)
	if err != nil {
		return fmt.Errorf("failed to decrypt Gotify token: %w", err)
	}
	target := strings.TrimRight(nc.GotifyURL, "/") + "/message"
	payload, err := json.Marshal(map[string]interface{}{
		"title":    title,
		"message":  message,
		"priority": 5,
	})
	if err != nil {
		return err
	}
	return postNotificationJSON(cfg, target, payload, "X-Gotify-Key", token)
}

func (gotifyNotificationSender) Send(db *gorm.DB, cfg config.Config, user models.User, reminders []models.Reminder) error {
	nc, err := GetNotificationConfigForUser(db, user.ID)
	if err != nil {
		return fmt.Errorf("failed to load notification config: %w", err)
	}
	if nc == nil || nc.GotifyURL == "" || !nc.HasGotifyToken() {
		return fmt.Errorf("gotify is not configured")
	}

	lang := notificationLanguage(user)
	contactMap := loadReminderContactNames(db, user.ID, reminders)
	title := i18n.T(lang, "notifications.reminderTitle")

	var sendErr error
	for _, r := range reminders {
		body := notificationShortBody(r, contactMap)
		if err := sendGotifyMessage(cfg, nc, title, body); err != nil {
			recordNotificationDelivery(db, r.ID, models.ChannelGotify, false, err.Error())
			if sendErr == nil {
				sendErr = err
			}
			continue
		}
		recordNotificationDelivery(db, r.ID, models.ChannelGotify, true, "")
	}
	return sendErr
}

// ---------------------------------------------------------------------------
// Web Push
// ---------------------------------------------------------------------------

const (
	serverSettingVAPIDPublicKey  = "vapid_public_key"
	serverSettingVAPIDPrivateKey = "vapid_private_key"
)

// GetVAPIDKeys returns the server-wide Web Push VAPID keypair, generating and
// persisting one on first use. VAPID identifies the application (like the email
// From address), so it is shared by all users — per-user keys would orphan
// every existing subscription when a user re-saved their config.
func GetVAPIDKeys(db *gorm.DB) (publicKey, privateKey string, err error) {
	var public, private models.ServerSetting
	pubErr := db.Where("key = ?", serverSettingVAPIDPublicKey).First(&public).Error
	privErr := db.Where("key = ?", serverSettingVAPIDPrivateKey).First(&private).Error
	if pubErr == nil && privErr == nil {
		return public.Value, private.Value, nil
	}

	privateKey, publicKey, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate VAPID keys: %w", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, kv := range []models.ServerSetting{
			{Key: serverSettingVAPIDPublicKey, Value: publicKey},
			{Key: serverSettingVAPIDPrivateKey, Value: privateKey},
		} {
			var existing models.ServerSetting
			if err := tx.Where("key = ?", kv.Key).First(&existing).Error; err == nil {
				existing.Value = kv.Value
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&kv).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		return nil
	}); err != nil {
		return "", "", fmt.Errorf("failed to persist VAPID keys: %w", err)
	}

	return publicKey, privateKey, nil
}

type pushNotificationSender struct{}

func (pushNotificationSender) Channel() models.NotificationChannel { return models.ChannelPush }

func (pushNotificationSender) Enabled(db *gorm.DB, cfg config.Config, user models.User) bool {
	if !user.NotifyPush {
		return false
	}
	var webCount int64
	if err := db.Model(&models.PushSubscription{}).Where("user_id = ?", user.ID).Count(&webCount).Error; err != nil {
		logger.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to count push subscriptions for enablement")
		return false
	}
	if webCount > 0 {
		return true
	}
	// No browser subscriptions: the channel is enabled only if the user has a
	// mobile FCM device AND the server is configured to deliver to it (M2 —
	// an fcm registration alone is inert until FCM_SERVICE_ACCOUNT_FILE is set).
	var deviceCount int64
	if err := db.Model(&models.DeviceRegistration{}).
		Where("user_id = ? AND client = ?", user.ID, string(models.PushClientFCM)).
		Count(&deviceCount).Error; err != nil {
		logger.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to count FCM device registrations for enablement")
		return false
	}
	if deviceCount == 0 {
		return false
	}
	sa, err := LoadFCMServiceAccount(cfg.FCMServiceAccountFile)
	if err != nil {
		logger.Warn().Err(err).Uint("user_id", user.ID).Msg("FCM service account misconfigured")
		return false
	}
	return sa != nil
}

// webPushRecordOverhead is the fixed non-payload overhead webpush-go's
// aes128gcm encoding wraps around the plaintext for every record: 16-byte
// salt + 4-byte record-size header + 1-byte public-key-length + 65-byte
// uncompressed P-256 public key + 1-byte padding delimiter + 16-byte
// AES-GCM auth tag (RFC 8291 §4; webpush-go's SendNotificationWithContext).
// A webpush.Options.RecordSize smaller than len(payload)+webPushRecordOverhead
// makes webpush-go's internal pad() fail with ErrMaxPadExceeded.
const webPushRecordOverhead = 103

// pushRecordSize sizes webpush.Options.RecordSize to the payload actually
// being sent instead of relying on webpush-go's default (MaxRecordSize,
// 4096), which pads every message — including a two-word test notification
// — out to a request body of exactly RecordSize bytes regardless of content
// (the framing is self-balancing: header 86 + ciphertext RecordSize-86), and
// gets rejected with 413 by push services enforcing a smaller per-message
// limit (T51). Capped at webpush.MaxRecordSize, the largest size push
// services are generally known to accept; a payload too large to fit even
// then still fails, just with webpush-go's own "payload has exceeded the
// maximum length" error instead of a 413 from the service — the same
// threshold (payload > 3993 bytes) the library's own default already had.
func pushRecordSize(payloadLen int) uint32 {
	size := payloadLen + webPushRecordOverhead
	if size > int(webpush.MaxRecordSize) {
		return webpush.MaxRecordSize
	}
	return uint32(size) // #nosec G115 -- size is clamped to webpush.MaxRecordSize above, so it cannot overflow
}

// sendPushMessage delivers one Web Push message to a subscription. Returns
// stale=true when the push service no longer knows the subscription
// (404/410) — the caller should drop it. Reuses clientFor so the webhook SSRF
// policy governs the push endpoint too.
func sendPushMessage(db *gorm.DB, cfg config.Config, user models.User, sub models.PushSubscription, vapidPublic, vapidPrivate, title, message string) (stale bool, err error) {
	payload, err := json.Marshal(map[string]string{"title": title, "body": message})
	if err != nil {
		return false, err
	}

	resp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, &webpush.Options{
		Subscriber:      "mailto:" + user.Email,
		VAPIDPublicKey:  vapidPublic,
		VAPIDPrivateKey: vapidPrivate,
		TTL:             86400,
		RecordSize:      pushRecordSize(len(payload)),
		HTTPClient:      clientFor(cfg),
	})
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return false, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return true, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status %d from push service", resp.StatusCode)
}

func (pushNotificationSender) Send(db *gorm.DB, cfg config.Config, user models.User, reminders []models.Reminder) error {
	var subs []models.PushSubscription
	if err := db.Where("user_id = ?", user.ID).Find(&subs).Error; err != nil {
		return fmt.Errorf("failed to load push subscriptions: %w", err)
	}
	var devices []models.DeviceRegistration
	if err := db.Where("user_id = ?", user.ID).Find(&devices).Error; err != nil {
		return fmt.Errorf("failed to load device registrations: %w", err)
	}
	if len(subs) == 0 && len(devices) == 0 {
		return fmt.Errorf("no push devices registered")
	}

	vapidPublic, vapidPrivate, err := GetVAPIDKeys(db)
	if err != nil {
		return fmt.Errorf("failed to load VAPID keys: %w", err)
	}

	// FCM service account is loaded once per run; nil when the server isn't
	// configured for mobile push delivery (M2). An fcm device is only
	// deliverable when it's non-nil; otherwise the device is skipped and the
	// reminder stays due for push (so enabling FCM later picks it up).
	var fcmAccount *fcmServiceAccount
	if cfg.FCMServiceAccountFile != "" {
		fcmAccount, err = LoadFCMServiceAccount(cfg.FCMServiceAccountFile)
		if err != nil {
			logger.Warn().Err(err).Msg("FCM service account misconfigured; mobile push delivery skipped")
		}
	}

	lang := notificationLanguage(user)
	contactMap := loadReminderContactNames(db, user.ID, reminders)
	title := i18n.T(lang, "notifications.reminderTitle")

	var sendErr error
	for _, sub := range subs {
		for _, r := range reminders {
			body := notificationShortBody(r, contactMap)
			stale, err := sendPushMessage(db, cfg, user, sub, vapidPublic, vapidPrivate, title, body)
			if err != nil {
				recordNotificationDelivery(db, r.ID, models.ChannelPush, false, err.Error())
				if sendErr == nil {
					sendErr = err
				}
				continue
			}
			if stale {
				if delErr := db.Delete(&sub).Error; delErr != nil {
					logger.Warn().Err(delErr).Uint("subscription_id", sub.ID).Msg("Failed to delete stale push subscription")
				} else {
					logger.Info().Uint("subscription_id", sub.ID).Msg("Removed stale push subscription (404/410 from push service)")
				}
				continue
			}
			recordNotificationDelivery(db, r.ID, models.ChannelPush, true, "")
		}
	}

	for _, device := range devices {
		switch models.PushClient(device.Client) {
		case models.PushClientAPNS:
			// Accepted at registration but delivery is not implemented (M2
			// design decision 2). Logged, never marked sent — so the reminder
			// stays due for push and an eventual APNS sender picks it up
			// without a contract change.
			logger.Warn().Uint("device_id", device.ID).Msg("APNS delivery is not implemented; skipping mobile push device")
			continue
		case models.PushClientFCM:
			if fcmAccount == nil {
				continue
			}
		}
		for _, r := range reminders {
			body := notificationShortBody(r, contactMap)
			stale, err := sendFCMMessage(cfg, fcmAccount, device.Token, title, body, fcmReminderData(r))
			if err != nil {
				recordNotificationDelivery(db, r.ID, models.ChannelPush, false, err.Error())
				if sendErr == nil {
					sendErr = err
				}
				continue
			}
			if stale {
				if delErr := db.Delete(&device).Error; delErr != nil {
					logger.Warn().Err(delErr).Uint("device_id", device.ID).Msg("Failed to delete stale FCM device registration")
				} else {
					logger.Info().Uint("device_id", device.ID).Msg("Removed stale FCM device registration (404 from FCM)")
				}
				continue
			}
			recordNotificationDelivery(db, r.ID, models.ChannelPush, true, "")
		}
	}

	return sendErr
}

// ---------------------------------------------------------------------------
// Mobile push (FCM) — M2
// ---------------------------------------------------------------------------

// fcmMessagingScope is the OAuth2 scope required to call the FCM HTTP v1 API.
const fcmMessagingScope = "https://www.googleapis.com/auth/firebase.messaging"

// fcmTokenEndpoint and fcmSendEndpoint are package vars so tests can redirect
// them at a fake Google server (the fakeChannelServer pattern) — the production
// values are Google's real endpoints.
var (
	fcmTokenEndpoint = "https://oauth2.googleapis.com/token" // #nosec G101 -- public OAuth2 endpoint URL, not a credential
	fcmSendEndpoint  = "https://fcm.googleapis.com/v1"
)

// fcmServiceAccount is the subset of a Firebase service-account JSON that FCM
// delivery needs: the project ID (to address the v1 API), and the client
// email + private key (to mint the OAuth2 JWT that exchanges for an access
// token). Everything else in the JSON (client_id, token_uri, ...) is ignored.
type fcmServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// ErrFCMInvalidServiceAccount is returned when FCM_SERVICE_ACCOUNT_FILE points
// at a file that is set but unreadable or missing a required field — rejected
// at boot (config load) rather than failing the first send, per M2's design
// decision 4.
var ErrFCMInvalidServiceAccount = errors.New("FCM service account file is invalid")

// LoadFCMServiceAccount reads and validates the Firebase service-account JSON
// at path. Empty path returns (nil, nil) — FCM delivery is simply not
// configured. A set-but-invalid path returns ErrFCMInvalidServiceAccount so
// the server refuses to boot with a misconfiguration instead of discovering it
// on the first reminder.
func LoadFCMServiceAccount(path string) (*fcmServiceAccount, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is an operator-supplied config path, not request input
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFCMInvalidServiceAccount, err)
	}
	var sa fcmServiceAccount
	if err := json.Unmarshal(data, &sa); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFCMInvalidServiceAccount, err)
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("%w: missing project_id, client_email, or private_key", ErrFCMInvalidServiceAccount)
	}
	return &sa, nil
}

// sendFCMMessage delivers one notification to one device token via the FCM
// HTTP v1 API. Returns stale=true when FCM no longer knows the token
// (404 NOT_FOUND) — the caller should drop the registration, mirroring
// sendPushMessage's 404/410 handling for web push subscriptions.
//
// [data] is the FCM `data` payload, carried alongside the visible
// `notification` block so the Android client can deep-link into the app and
// dedupe against the WorkManager polling worker (M5 §6.6/§6.8 — the
// reminder_id + due_at idempotence key). Empty/nil emits no `data` key (the
// Settings "test push" path sends a bare notification).
//
// Auth: mints an RS256 JWT signed with the service account's private key and
// exchanges it for a short-lived OAuth2 access token at Google's token
// endpoint (github.com/golang-jwt/jwt/v4 — already a dependency). The access
// token is minted fresh per call; reminder dispatch is a low-frequency daily
// job, so caching adds complexity for no measurable win.
func sendFCMMessage(cfg config.Config, sa *fcmServiceAccount, token, title, message string, data map[string]string) (stale bool, err error) {
	accessToken, err := fcmAccessToken(cfg, sa)
	if err != nil {
		return false, err
	}

	messagePayload := map[string]interface{}{
		"token": token,
		"notification": map[string]string{
			"title": title,
			"body":  message,
		},
	}
	if len(data) > 0 {
		messagePayload["data"] = data
	}
	payload, err := json.Marshal(map[string]interface{}{
		"message": messagePayload,
	})
	if err != nil {
		return false, err
	}

	target := fcmSendEndpoint + "/projects/" + sa.ProjectID + "/messages:send"
	req, err := http.NewRequest("POST", target, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := clientFor(cfg).Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return true, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, nil
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("unexpected status %d from FCM: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
}

// fcmReminderData builds the FCM `data` payload for a reminder push. The
// Android client keys on it for two things (M5 §6.6/§6.8): `deep_link` routes
// the notification tap into the contact's page, and the `reminder_id` +
// `due_at` pair is the idempotence key that suppresses the double-notify when
// the WorkManager polling worker posts the same reminder around the same time.
// Kept on the reminder so a future APNS sender can reuse the same payload.
func fcmReminderData(reminder models.Reminder) map[string]string {
	data := map[string]string{
		"type":        "reminder",
		"reminder_id": strconv.FormatUint(uint64(reminder.ID), 10),
		// Canonical UTC, truncated to the second. The Android client normalizes
		// the polling worker's `remind_at` (Go's RFC3339Nano, which carries
		// nanoseconds and the server's local offset) to epoch seconds when it
		// derives the same idempotence key — so the two sides must agree on a
		// second-granularity, offset-free instant, or the poll-boundary
		// double-notify would survive (two different `due_at` strings → two
		// different notification ids).
		"due_at": reminder.RemindAt.UTC().Truncate(time.Second).Format(time.RFC3339),
	}
	if reminder.ContactID != nil {
		data["deep_link"] = fmt.Sprintf("mycorrhizal://contacts/%d", *reminder.ContactID)
	}
	return data
}

// fcmAccessToken obtains a short-lived OAuth2 access token for the FCM API by
// minting a signed JWT from the service account's private key and exchanging
// it at Google's token endpoint (the two-legged OAuth2 JWT flow, RFC 7523).
// Uses clientFor(cfg) so the webhook SSRF dialer policy governs the call.
func fcmAccessToken(cfg config.Config, sa *fcmServiceAccount) (string, error) {
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode FCM service account private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse FCM service account private key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("FCM service account private key is not an RSA key")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   sa.ClientEmail,
		"scope": fcmMessagingScope,
		"aud":   "https://oauth2.googleapis.com/token",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(rsaKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign FCM access token JWT: %w", err)
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", signed)

	req, err := http.NewRequest("POST", fcmTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := clientFor(cfg).Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d from Google token endpoint: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode Google token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("Google token response contained no access_token")
	}
	return tokenResp.AccessToken, nil
}

// ---------------------------------------------------------------------------
// Device registrations (M2)
// ---------------------------------------------------------------------------

// ListDeviceRegistrations returns the user's registered mobile push devices.
func ListDeviceRegistrations(db *gorm.DB, userID uint) ([]models.DeviceRegistration, error) {
	var devices []models.DeviceRegistration
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

// CreateDeviceRegistration registers a mobile device push token. Re-registering
// the same (client, token) reassigns the existing row's UserID (and label)
// instead of duplicating it: a device re-registers on every app launch, and
// the token identifies a physical app install, not a person — if a device
// logs out of one user and into another, the second registration must move
// the row rather than leaving a stale row that still pushes the first user's
// reminders to it. See migration 000019's comment.
func CreateDeviceRegistration(db *gorm.DB, userID uint, input models.DeviceRegistrationInput) (*models.DeviceRegistration, error) {
	var existing models.DeviceRegistration
	if err := db.Where("client = ? AND token = ?", input.Client, input.Token).First(&existing).Error; err == nil {
		existing.UserID = userID
		existing.DeviceLabel = input.DeviceLabel
		if err := db.Save(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	device := models.DeviceRegistration{
		UserID:      userID,
		Token:       input.Token,
		Client:      input.Client,
		DeviceLabel: input.DeviceLabel,
	}
	if err := db.Create(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// DeleteDeviceRegistration removes one of the user's mobile push devices.
func DeleteDeviceRegistration(db *gorm.DB, userID uint, id uint) error {
	result := db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.DeviceRegistration{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// postNotificationJSON performs the SSRF-guarded HTTP POST every push-style
// channel uses (ntfy and Gotify are both trivial POSTs; push goes through
// webpush-go's own HTTP client, which reuses clientFor too). When
// WEBHOOK_BLOCK_PRIVATE_URLS is on, a private-address target is rejected with
// ErrNotificationPrivateAddress before any network call.
func postNotificationJSON(cfg config.Config, rawURL string, body []byte, headerKey, headerValue string) error {
	if cfg.WebhookBlockPrivateURLs && isPrivateURL(rawURL) {
		return ErrNotificationPrivateAddress
	}

	req, err := http.NewRequest("POST", rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if headerKey != "" {
		req.Header.Set(headerKey, headerValue)
	}

	resp, err := clientFor(cfg).Do(req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("unexpected status %d from notification endpoint", resp.StatusCode)
}

// notificationLanguage returns the user's language, defaulting to the app
// default when unset — the same rule sendReminderEmail uses.
func notificationLanguage(user models.User) string {
	if user.Language == "" {
		return i18n.DefaultLanguage
	}
	return user.Language
}

// loadReminderContactNames batch-fetches contact display names for a set of
// reminders so the push-style channels can prefix "<Contact>: " without one
// query per reminder.
func loadReminderContactNames(db *gorm.DB, userID uint, reminders []models.Reminder) map[uint]string {
	contactIDs := make([]uint, 0, len(reminders))
	for _, r := range reminders {
		if r.ContactID != nil {
			contactIDs = append(contactIDs, *r.ContactID)
		}
	}
	contactMap := make(map[uint]string, len(contactIDs))
	if len(contactIDs) == 0 {
		return contactMap
	}

	var contacts []models.Contact
	if err := db.Where("user_id = ? AND id IN ?", userID, contactIDs).Find(&contacts).Error; err != nil {
		logger.Warn().Err(err).Uint("user_id", userID).Msg("Failed to batch-fetch contacts for notification")
		return contactMap
	}
	for _, c := range contacts {
		contactMap[c.ID] = strings.TrimSpace(c.Firstname + " " + c.Lastname)
	}
	return contactMap
}

// notificationShortBody builds the short-form body for the push-style
// channels: "<Contact>: <Message>" when a contact name resolves, otherwise the
// raw message. The format is locale-agnostic (every locale uses the same
// `Name: message` pattern for push notifications); if a future locale needs a
// different template, add an i18n key with a {{contact}} placeholder.
func notificationShortBody(reminder models.Reminder, contactMap map[uint]string) string {
	if reminder.ContactID != nil {
		if name, ok := contactMap[*reminder.ContactID]; ok && name != "" {
			return name + ": " + reminder.Message
		}
	}
	return reminder.Message
}

// ---------------------------------------------------------------------------
// Config + subscriptions (Settings UI backing)
// ---------------------------------------------------------------------------

// GetNotificationConfigForUser returns the user's NotificationConfig, or nil
// when they have not set one up.
func GetNotificationConfigForUser(db *gorm.DB, userID uint) (*models.NotificationConfig, error) {
	var nc models.NotificationConfig
	if err := db.Where("user_id = ?", userID).First(&nc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &nc, nil
}

// SaveNotificationConfig upserts the user's channel config and applies the
// per-user channel toggles (which live on users, so they are updated in the
// same call the Settings form makes). A non-empty GotifyToken is encrypted at
// rest (credential_crypto.go); an empty one keeps the stored token unchanged.
func SaveNotificationConfig(db *gorm.DB, jwtSecret string, userID uint, input models.NotificationConfigInput) (*models.NotificationConfig, error) {
	ntfyURL, err := normalizeNotificationURL(input.NtfyURL)
	if err != nil {
		return nil, err
	}
	gotifyURL, err := normalizeNotificationURL(input.GotifyURL)
	if err != nil {
		return nil, err
	}

	nc, err := GetNotificationConfigForUser(db, userID)
	if err != nil {
		return nil, err
	}
	if nc == nil {
		nc = &models.NotificationConfig{UserID: userID}
	}
	nc.NtfyURL = ntfyURL
	nc.NtfyTopic = input.NtfyTopic
	nc.GotifyURL = gotifyURL
	if input.GotifyToken != "" {
		enc, err := EncryptCredential(jwtSecret, input.GotifyToken)
		if err != nil {
			return nil, err
		}
		nc.GotifyTokenEncrypted = enc
	}
	if err := db.Save(nc).Error; err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.NotifyNtfy != nil {
		updates["notify_ntfy"] = *input.NotifyNtfy
	}
	if input.NotifyGotify != nil {
		updates["notify_gotify"] = *input.NotifyGotify
	}
	if input.NotifyPush != nil {
		updates["notify_push"] = *input.NotifyPush
	}
	if len(updates) > 0 {
		if err := db.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return nc, nil
}

// TestNotificationChannel sends a single test notification through the given
// channel using the user's saved config, without touching reminder delivery
// records. Returns a descriptive error when the channel is unconfigured,
// misconfigured, or the delivery fails — the Settings "test" button surfaces
// it verbatim.
func TestNotificationChannel(db *gorm.DB, cfg config.Config, user models.User, channel models.NotificationChannel) error {
	lang := notificationLanguage(user)
	title := i18n.T(lang, "notifications.testTitle")
	message := i18n.T(lang, "notifications.testBody")

	switch channel {
	case models.ChannelNtfy:
		nc, err := GetNotificationConfigForUser(db, user.ID)
		if err != nil {
			return err
		}
		if nc == nil || nc.NtfyURL == "" || nc.NtfyTopic == "" {
			return fmt.Errorf("ntfy is not configured — set a URL and topic and save first")
		}
		return sendNtfyMessage(cfg, nc, title, message)

	case models.ChannelGotify:
		nc, err := GetNotificationConfigForUser(db, user.ID)
		if err != nil {
			return err
		}
		if nc == nil || nc.GotifyURL == "" || !nc.HasGotifyToken() {
			return fmt.Errorf("gotify is not configured — set a URL and token and save first")
		}
		return sendGotifyMessage(cfg, nc, title, message)

	case models.ChannelPush:
		// M2: the test button probes every registered push endpoint — browser
		// Web Push subscriptions AND FCM mobile devices — mirroring
		// pushNotificationSender.Send's dispatch (design decision 5: one
		// "push" channel, tested as one unit).
		var subs []models.PushSubscription
		if err := db.Where("user_id = ?", user.ID).Find(&subs).Error; err != nil {
			return err
		}
		var devices []models.DeviceRegistration
		if err := db.Where("user_id = ? AND client = ?", user.ID, string(models.PushClientFCM)).Find(&devices).Error; err != nil {
			return err
		}
		if len(subs) == 0 && len(devices) == 0 {
			return fmt.Errorf("no push devices registered — enable browser notifications or register a mobile device first")
		}

		var sendErr error
		var attempted bool

		if len(subs) > 0 {
			vapidPublic, vapidPrivate, err := GetVAPIDKeys(db)
			if err != nil {
				return err
			}
			for _, sub := range subs {
				attempted = true
				stale, err := sendPushMessage(db, cfg, user, sub, vapidPublic, vapidPrivate, title, message)
				if err != nil {
					if sendErr == nil {
						sendErr = err
					}
					continue
				}
				if stale {
					if delErr := db.Delete(&sub).Error; delErr != nil {
						logger.Warn().Err(delErr).Uint("subscription_id", sub.ID).Msg("Failed to delete stale push subscription")
					}
				}
			}
		}

		if len(devices) > 0 {
			fcmAccount, err := LoadFCMServiceAccount(cfg.FCMServiceAccountFile)
			if err != nil || fcmAccount == nil {
				if len(subs) == 0 {
					// Nothing else was even attempted — a silent no-op would
					// look like success. Surface the missing config instead.
					return fmt.Errorf("mobile push delivery is not configured on this server (FCM_SERVICE_ACCOUNT_FILE unset)")
				}
				// A web subscription was already probed above; an
				// unconfigured FCM device is inert-by-design (M2 design
				// decision 4), not a failure of the channel as a whole.
			} else {
				for _, device := range devices {
					attempted = true
					stale, err := sendFCMMessage(cfg, fcmAccount, device.Token, title, message, nil)
					if err != nil {
						if sendErr == nil {
							sendErr = err
						}
						continue
					}
					if stale {
						if delErr := db.Delete(&device).Error; delErr != nil {
							logger.Warn().Err(delErr).Uint("device_id", device.ID).Msg("Failed to delete stale FCM device registration")
						}
					}
				}
			}
		}

		if !attempted {
			return fmt.Errorf("no push devices registered — enable browser notifications or register a mobile device first")
		}
		return sendErr

	default:
		return fmt.Errorf("unsupported notification channel %q", channel)
	}
}

// ListPushSubscriptions returns the user's registered push devices.
func ListPushSubscriptions(db *gorm.DB, userID uint) ([]models.PushSubscription, error) {
	var subs []models.PushSubscription
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

// CreatePushSubscription registers a browser push subscription. Re-registering
// the same endpoint updates the existing row (keys/label) instead of
// duplicating it — a browser re-subscribe is a refresh, not a second device.
func CreatePushSubscription(db *gorm.DB, userID uint, input models.PushSubscriptionInput) (*models.PushSubscription, error) {
	var existing models.PushSubscription
	if err := db.Where("user_id = ? AND endpoint = ?", userID, input.Endpoint).First(&existing).Error; err == nil {
		existing.P256dh = input.P256dh
		existing.Auth = input.Auth
		existing.DeviceLabel = input.DeviceLabel
		if err := db.Save(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	sub := models.PushSubscription{
		UserID:      userID,
		Endpoint:    input.Endpoint,
		P256dh:      input.P256dh,
		Auth:        input.Auth,
		DeviceLabel: input.DeviceLabel,
	}
	if err := db.Create(&sub).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

// DeletePushSubscription removes one of the user's push devices.
func DeletePushSubscription(db *gorm.DB, userID uint, id uint) error {
	result := db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.PushSubscription{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
