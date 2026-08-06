package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mycorrhizal/config"
	"mycorrhizal/i18n"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
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

func (pushNotificationSender) Enabled(db *gorm.DB, _ config.Config, user models.User) bool {
	if !user.NotifyPush {
		return false
	}
	var count int64
	if err := db.Model(&models.PushSubscription{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		logger.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to count push subscriptions for enablement")
		return false
	}
	return count > 0
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
	if len(subs) == 0 {
		return fmt.Errorf("no push devices registered")
	}

	vapidPublic, vapidPrivate, err := GetVAPIDKeys(db)
	if err != nil {
		return fmt.Errorf("failed to load VAPID keys: %w", err)
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
	return sendErr
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
		var subs []models.PushSubscription
		if err := db.Where("user_id = ?", user.ID).Find(&subs).Error; err != nil {
			return err
		}
		if len(subs) == 0 {
			return fmt.Errorf("no push devices registered — enable browser notifications in a supported browser first")
		}
		vapidPublic, vapidPrivate, err := GetVAPIDKeys(db)
		if err != nil {
			return err
		}
		var sendErr error
		for _, sub := range subs {
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
