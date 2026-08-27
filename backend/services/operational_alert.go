package services

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Webhook event types for operational alerting (issue #428). One pair, with the
// condition carried in the payload, rather than an event type per condition —
// keeps the subscribable set small ("a small set of alert conditions"). These
// tokens are mirrored in models/dtos.go's WebhookInput `oneof` and in
// frontend/src/components/WebhooksSettings.tsx (frontend trap 4).
const (
	EventAlertRaised  = "alert.raised"
	EventAlertCleared = "alert.cleared"
)

// operationalAlert is one raised-or-cleared alert, ready to deliver.
type operationalAlert struct {
	conditionKey string
	title        string // human-facing subsystem name, e.g. "Backup"
	firing       bool
	detail       string
	// failureCount is the consecutive-failure count when firing, and the
	// number of failures the just-closed incident lasted when recovering.
	failureCount int
	since        time.Time // when the condition entered its current state
}

func (a operationalAlert) subject() string {
	if a.firing {
		return fmt.Sprintf("🔴 %s failed", a.title)
	}
	if a.failureCount > 0 {
		return fmt.Sprintf("🟢 %s recovered after %s", a.title, pluralFailures(a.failureCount))
	}
	return fmt.Sprintf("🟢 %s recovered", a.title)
}

func (a operationalAlert) body() string {
	var b strings.Builder
	b.WriteString(a.subject())
	if a.detail != "" {
		fmt.Fprintf(&b, "\n\n%s", a.detail)
	}
	if a.firing && a.failureCount > 0 {
		fmt.Fprintf(&b, "\n\nConsecutive failures: %d", a.failureCount)
	}
	fmt.Fprintf(&b, "\n\nCondition: %s\nSince: %s", a.conditionKey, a.since.UTC().Format(time.RFC3339))
	return b.String()
}

func pluralFailures(n int) string {
	if n == 1 {
		return "1 failure"
	}
	return fmt.Sprintf("%d failures", n)
}

// operationalAlertHTML wraps the plain-text body in the minimal HTML the mail
// transport expects (EmailMessage carries HTML only). No template file — an
// operator alert is a preformatted block, not a designed email.
func operationalAlertHTML(body string) string {
	return "<pre style=\"font-family:monospace;white-space:pre-wrap\">" + html.EscapeString(body) + "</pre>"
}

// alertDeliverer is the delivery implementation, indirected through a package
// var so alerting_service_test.go can record dispatches without real HTTP.
var alertDeliverer = deliverOperationalAlert

// dispatchOperationalAlert is called by transitionAlertState on every
// raise/clear transition.
func dispatchOperationalAlert(ctx context.Context, db *gorm.DB, cfg config.Config, a operationalAlert) {
	alertDeliverer(ctx, db, cfg, a)
}

// deliverOperationalAlert pushes one alert through every existing delivery
// path: webhooks (broadcast to all subscribers, like the current db.* operator
// events) and the personal notification channels (email / ntfy / Gotify /
// push) of ADMIN users only — infra health is an operator concern, not
// something to page every user of a shared instance about. Best-effort per
// channel: a failure on one never blocks the others.
func deliverOperationalAlert(ctx context.Context, db *gorm.DB, cfg config.Config, a operationalAlert) {
	eventType := EventAlertCleared
	state := models.AlertStateOK
	if a.firing {
		eventType = EventAlertRaised
		state = models.AlertStateAlerting
	}
	triggerWebhooksForAllUsers(ctx, db, cfg, eventType, map[string]interface{}{
		"condition":     a.conditionKey,
		"title":         a.title,
		"state":         state,
		"detail":        a.detail,
		"failure_count": a.failureCount,
		"since":         a.since.UTC().Format(time.RFC3339),
	})

	var admins []models.User
	if err := db.WithContext(ctx).Where("is_admin = ?", true).Find(&admins).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("operational alert: failed to load admin users")
		return
	}

	subject, body := a.subject(), a.body()
	for _, admin := range admins {
		deliverOperationalAlertToUser(ctx, db, cfg, admin, subject, body)
	}
}

// deliverOperationalAlertToUser fans one alert out to a single user's enabled
// personal channels, mirroring each channel's own enablement predicate from
// notification_service.go.
func deliverOperationalAlertToUser(ctx context.Context, db *gorm.DB, cfg config.Config, user models.User, subject, body string) {
	nc, err := GetNotificationConfigForUser(db, user.ID)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Uint("user_id", user.ID).Msg("operational alert: failed to load notification config")
	}

	if cfg.EmailEnabled() && user.Email != "" {
		if err := SendEmail(cfg, EmailMessage{To: user.Email, Subject: subject, HTML: operationalAlertHTML(body)}); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Uint("user_id", user.ID).Msg("operational alert: email delivery failed")
		}
	}
	if user.NotifyNtfy && nc != nil && nc.NtfyURL != "" && nc.NtfyTopic != "" {
		if err := sendNtfyMessage(cfg, nc, subject, body); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Uint("user_id", user.ID).Msg("operational alert: ntfy delivery failed")
		}
	}
	if user.NotifyGotify && nc != nil && nc.GotifyURL != "" && nc.HasGotifyToken() {
		if err := sendGotifyMessage(cfg, nc, subject, body); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Uint("user_id", user.ID).Msg("operational alert: gotify delivery failed")
		}
	}
	if user.NotifyPush {
		if err := deliverPushToUser(db, cfg, user, subject, body); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Uint("user_id", user.ID).Msg("operational alert: push delivery failed")
		}
	}
}
