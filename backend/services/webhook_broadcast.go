package services

import (
	"context"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// triggerWebhooksForAllUsers fans a system-wide event (not tied to any one
// user's data) out to every user who has at least one active webhook —
// TriggerWebhooks itself is per-user and filters by that user's event
// subscriptions, so this only needs the distinct set of candidate user IDs,
// mirroring how DetectReachOutSuggestions collects its per-user worklist
// (reach_out_trigger_service.go).
//
// Returns the total number of webhooks a delivery was enqueued for — a
// durable delivery (ProcessWebhookRetries replays it), which the alert
// evaluator treats as "handled" when deciding whether an undelivered raise
// still needs retrying (issue #973).
func triggerWebhooksForAllUsers(ctx context.Context, db *gorm.DB, cfg config.Config, eventType string, data interface{}) int {
	var userIDs []uint
	if err := db.Model(&models.Webhook{}).
		Where("is_active = ? AND deleted_at IS NULL", true).
		Distinct().Pluck("user_id", &userIDs).Error; err != nil {
		logger.Error().Err(err).Str("event", eventType).Msg("webhook broadcast: failed to load candidate users")
		return 0
	}
	queued := 0
	for _, userID := range userIDs {
		queued += TriggerWebhooks(ctx, db, cfg, userID, eventType, data)
	}
	return queued
}
