package services

import (
	"context"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Per-channel notification delivery health, issue #422.
//
// Reminder delivery happens over four channels (email, ntfy, Gotify, Web Push)
// and each attempt leaves a row in notification_deliveries
// (status 'sent'|'failed' + error). What the operators of an instance lacked
// was any aggregated view of whether each channel is actually working —
// "notifications are broken" is not the same as "no browser devices are
// registered". ComputeNotificationChannelHealth derives that view on read by
// folding the delivery rows and the per-user channel config, exactly like
// ComputeSubsystemHealth folds system_events (#427): nothing is stored, the
// state survives a restart and can never drift from the rows it summarizes.
//
// The statuses intentionally separate the three remedies the issue calls out:
//   - unconfigured  — the server (email) or no user (ntfy/Gotify/push) has the
//     channel set up; there is nothing to deliver on.
//   - no_devices    — push is provisioned (VAPID keys exist, FCM is configured
//     or has registered devices) but no browser subscription / mobile device
//     can currently receive; not a red failure, just nobody home.
//   - failing       — configured and reachable targets exist, but the most
//     recent delivery attempt failed (consecutive_failures since the last
//     success, with the reason).
//   - healthy       — configured and the most recent terminal delivery was a
//     success.
//
// The output is instance-wide (not user-scoped) and admin-only; it never
// carries a Gotify token or any per-user delivery content — only aggregate
// counts and the last failure's reason (which the admin system-event timeline
// already surfaces, #424).

// Notification channel health status values.
const (
	NotificationHealthUnconfigured = "unconfigured"
	NotificationHealthNoDevices    = "no_devices"
	NotificationHealthHealthy      = "healthy"
	NotificationHealthFailing      = "failing"
)

// maxNotificationHealthErrorLen bounds the failure reason surfaced in the
// admin health view. The stored delivery error is a diagnostic for the
// operator, so it is not collapsed the way NotificationTestErrorMessage
// collapses the SSRF-guard sentinels for a client-facing surface (#606) — but
// an arbitrary upstream body must still be bounded.
const maxNotificationHealthErrorLen = 512

// NotificationChannelHealth is the delivery-health state of one channel.
type NotificationChannelHealth struct {
	// Channel is the models.NotificationChannel wire name (email, ntfy,
	// gotify, push), in AllNotificationChannels dispatch order.
	Channel string `json:"channel"`
	// Status is one of NotificationHealthUnconfigured/NoDevices/Healthy/Failing.
	Status string `json:"status"`
	// Configured reports whether the channel can be dispatched at all: email
	// is server-configured (Resend/SMTP), ntfy/Gotify need at least one user's
	// connection settings, push needs VAPID keys / an FCM service account.
	Configured bool `json:"configured"`
	// Reachable reports whether the channel is believed able to deliver right
	// now — true exactly when status is healthy. The other statuses are each a
	// different reason it cannot.
	Reachable bool `json:"reachable"`
	// EnabledUserCount is the number of users for whom the channel is actually
	// on (per-user toggle + usable per-user config, per the senders' Enabled).
	EnabledUserCount int64 `json:"enabled_user_count"`
	// DeviceCount is the number of currently-receivable push endpoints (web
	// subscriptions + FCM devices when FCM is configured). Zero for non-push
	// channels.
	DeviceCount int64 `json:"device_count"`
	// FCMConfigured is push-only: whether the server has an FCM service
	// account file (M2). False for non-push channels.
	FCMConfigured bool `json:"fcm_configured"`

	// LastAttemptAt is the most recent terminal delivery (sent or failed).
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	// LastSentAt / LastFailedAt are the most recent of each outcome; nil when
	// the channel has no such delivery row yet.
	LastSentAt   *time.Time `json:"last_sent_at"`
	LastFailedAt *time.Time `json:"last_failed_at"`
	// ConsecutiveFailures is the unbroken run of failed deliveries since the
	// last success (all failures on record when there has been no success).
	// Non-zero exactly when status is failing.
	ConsecutiveFailures int `json:"consecutive_failures"`
	// LastError is the (length-capped) error of the most recent failed
	// delivery; empty unless status is failing.
	LastError string `json:"last_error"`

	// AttemptedCount / DeliveredCount are the terminal delivery totals for the
	// channel (pending rows — reminders still waiting to be delivered — are
	// not attempts).
	AttemptedCount int64 `json:"attempted_count"`
	DeliveredCount int64 `json:"delivered_count"`
}

// ComputeNotificationChannelHealth derives the delivery-health state of every
// channel from notification_deliveries + the per-user config, in
// models.AllNotificationChannels order.
func ComputeNotificationChannelHealth(ctx context.Context, db *gorm.DB, cfg config.Config) ([]NotificationChannelHealth, error) {
	out := make([]NotificationChannelHealth, 0, len(models.AllNotificationChannels))
	for _, ch := range models.AllNotificationChannels {
		h, err := computeNotificationChannelHealth(ctx, db, cfg, ch)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

func computeNotificationChannelHealth(ctx context.Context, db *gorm.DB, cfg config.Config, ch models.NotificationChannel) (NotificationChannelHealth, error) {
	h := NotificationChannelHealth{Channel: string(ch)}

	lastSent, err := lastDelivery(ctx, db, ch, "sent")
	if err != nil {
		return h, err
	}
	lastFailed, err := lastDelivery(ctx, db, ch, "failed")
	if err != nil {
		return h, err
	}
	if lastSent != nil {
		t := lastSent.CreatedAt
		h.LastSentAt = &t
	}
	if lastFailed != nil {
		t := lastFailed.CreatedAt
		h.LastFailedAt = &t
	}
	h.LastAttemptAt = laterTime(h.LastSentAt, h.LastFailedAt)

	if err := countDeliveryTotals(ctx, db, ch, &h); err != nil {
		return h, err
	}

	if err := notificationChannelSetup(ctx, db, cfg, ch, &h); err != nil {
		return h, err
	}

	switch {
	case !h.Configured:
		h.Status = NotificationHealthUnconfigured
	case ch == models.ChannelPush && h.DeviceCount == 0:
		h.Status = NotificationHealthNoDevices
	case lastFailed != nil && (lastSent == nil || afterDelivery(lastFailed, lastSent)):
		h.Status = NotificationHealthFailing
		count, err := failureRunSinceDelivery(ctx, db, ch, lastSent)
		if err != nil {
			return h, err
		}
		h.ConsecutiveFailures = count
		if lastFailed.Error != nil {
			h.LastError = capHealthError(*lastFailed.Error)
		}
	default:
		h.Status = NotificationHealthHealthy
	}
	h.Reachable = h.Status == NotificationHealthHealthy
	return h, nil
}

// notificationChannelSetup fills the config-derived half of the health view:
// whether the channel is configured instance-wide, how many users have it
// enabled, and (push only) the device/FCM counts.
func notificationChannelSetup(ctx context.Context, db *gorm.DB, cfg config.Config, ch models.NotificationChannel, h *NotificationChannelHealth) error {
	enabled, err := countNotificationEnabledUsers(ctx, db, cfg, ch)
	if err != nil {
		return err
	}
	h.EnabledUserCount = enabled

	switch ch {
	case models.ChannelEmail:
		h.Configured = cfg.EmailEnabled()
	case models.ChannelNtfy:
		var count int64
		if err := db.WithContext(ctx).Model(&models.NotificationConfig{}).
			Where("deleted_at IS NULL AND ntfy_url != '' AND ntfy_topic != ''").
			Count(&count).Error; err != nil {
			return err
		}
		h.Configured = count > 0
	case models.ChannelGotify:
		var count int64
		if err := db.WithContext(ctx).Model(&models.NotificationConfig{}).
			Where("deleted_at IS NULL AND gotify_url != '' AND gotify_token_encrypted != ''").
			Count(&count).Error; err != nil {
			return err
		}
		h.Configured = count > 0
	case models.ChannelPush:
		// FCM service account is what makes a mobile device deliverable (M2);
		// an apns registration alone is accepted but never dispatched, so only
		// fcm devices count toward DeviceCount, and only when the server can
		// actually reach them.
		sa, err := LoadFCMServiceAccount(cfg.FCMServiceAccountFile)
		if err != nil {
			// Set-but-invalid is rejected at boot (ErrFCMInvalidServiceAccount),
			// so this is defensive — treat it as unconfigured rather than lying.
			logger.Warn().Err(err).Msg("FCM service account invalid; reporting push health as unconfigured")
		}
		h.FCMConfigured = sa != nil

		var webSubs, fcmDevices int64
		if err := db.WithContext(ctx).Model(&models.PushSubscription{}).Count(&webSubs).Error; err != nil {
			return err
		}
		if err := db.WithContext(ctx).Model(&models.DeviceRegistration{}).
			Where("client = ?", models.PushClientFCM).
			Count(&fcmDevices).Error; err != nil {
			return err
		}
		h.DeviceCount = webSubs
		if h.FCMConfigured {
			h.DeviceCount += fcmDevices
		}

		// VAPID keys are generated server-wide on first use, so their
		// presence marks "the server can do web push" even when no browser
		// subscription is currently registered.
		var vapidCount int64
		if err := db.WithContext(ctx).Model(&models.ServerSetting{}).
			Where("key IN ?", []string{serverSettingVAPIDPublicKey, serverSettingVAPIDPrivateKey}).
			Count(&vapidCount).Error; err != nil {
			return err
		}
		vapidPresent := vapidCount == 2

		h.Configured = h.FCMConfigured || vapidPresent || webSubs > 0 || fcmDevices > 0
	}
	return nil
}

// countNotificationEnabledUsers sums, over all users, the senders' own
// Enabled() verdicts — the single source of truth for "this channel is on for
// this user" — rather than re-deriving the enablement rules here.
func countNotificationEnabledUsers(ctx context.Context, db *gorm.DB, cfg config.Config, ch models.NotificationChannel) (int64, error) {
	var users []models.User
	if err := db.WithContext(ctx).Find(&users).Error; err != nil {
		return 0, err
	}
	var count int64
	for _, u := range users {
		for _, s := range notificationSenders {
			if s.Channel() == ch && s.Enabled(db, cfg, u) {
				count++
				break
			}
		}
	}
	return count, nil
}

// lastDelivery returns the most recent notification_deliveries row for the
// channel with the given terminal status, or nil when there is none. Find (not
// Take/First) so an empty result is not logged as a record-not-found error
// every call (the same pattern as lastTerminalEvent).
func lastDelivery(ctx context.Context, db *gorm.DB, ch models.NotificationChannel, status string) (*models.NotificationDelivery, error) {
	var rows []models.NotificationDelivery
	err := db.WithContext(ctx).
		Where("channel = ? AND status = ?", ch, status).
		Order("created_at DESC, id DESC").
		Limit(1).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// failureRunSinceDelivery counts the unbroken run of failed deliveries for the
// channel that occurred after its last sent delivery (all failures on record
// when there has been no success).
func failureRunSinceDelivery(ctx context.Context, db *gorm.DB, ch models.NotificationChannel, lastSent *models.NotificationDelivery) (int, error) {
	q := db.WithContext(ctx).Model(&models.NotificationDelivery{}).
		Where("channel = ? AND status = ?", ch, "failed")
	if lastSent != nil {
		q = q.Where("created_at > ? OR (created_at = ? AND id > ?)",
			lastSent.CreatedAt, lastSent.CreatedAt, lastSent.ID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

// afterDelivery reports whether a occurred after b, breaking an exact-timestamp
// tie by autoincrement id (the later-inserted row is the later attempt).
func afterDelivery(a, b *models.NotificationDelivery) bool {
	if a.CreatedAt.Equal(b.CreatedAt) {
		return a.ID > b.ID
	}
	return a.CreatedAt.After(b.CreatedAt)
}

// countDeliveryTotals fills the terminal-attempt totals for the channel.
func countDeliveryTotals(ctx context.Context, db *gorm.DB, ch models.NotificationChannel, h *NotificationChannelHealth) error {
	if err := db.WithContext(ctx).Model(&models.NotificationDelivery{}).
		Where("channel = ? AND status IN ?", ch, []string{"sent", "failed"}).
		Count(&h.AttemptedCount).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Model(&models.NotificationDelivery{}).
		Where("channel = ? AND status = ?", ch, "sent").
		Count(&h.DeliveredCount).Error; err != nil {
		return err
	}
	return nil
}

func capHealthError(msg string) string {
	if len(msg) > maxNotificationHealthErrorLen {
		return msg[:maxNotificationHealthErrorLen]
	}
	return msg
}
