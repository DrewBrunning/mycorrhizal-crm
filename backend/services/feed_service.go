package services

import (
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// feedTouchInterval is how stale a feed's last_accessed_at must be before a
// serve rewrites it. Readers poll often, so this throttles the write load on
// SQLite while still making a stale feed visible in the management UI.
const feedTouchInterval = time.Hour

// TouchFeed asynchronously records that a feed was served, at most once per
// feedTouchInterval. It mirrors middleware.TouchAPIToken's fire-and-forget
// shape: failure is logged, never fatal to the request.
func TouchFeed(db *gorm.DB, feed *models.Feed) {
	if feed.LastAccessedAt != nil && Now().Sub(*feed.LastAccessedAt) < feedTouchInterval {
		return
	}
	feedID := feed.ID
	go func(id string) {
		if err := db.Model(&models.Feed{}).Where("id = ?", id).
			Update("last_accessed_at", Now()).Error; err != nil {
			logger.Logger.Warn().Err(err).Str("feed_id", id).Msg("Failed to update feed last_accessed_at") // # pragma: no cover — background best-effort write; only a failing store trips this
		}
	}(feedID)
}

// RevokeAllFeeds revokes every currently-active private feed credential for a
// user and returns how many were revoked. It mirrors RevokeAllAPITokens: a
// feed URL grants read access to the same data a leaked API token would, so
// every account-compromise response that ends standing API tokens must end the
// user's feeds too (the recovery-path password reset in
// controllers/user_controller.go and the admin password reset in
// controllers/admin_user_controller.go). ADR 0030 decision 6.
//
// Failures are the caller's to log and swallow: by the time this runs the
// password change has already succeeded, so surfacing an error would be
// misleading. Nothing is re-armed on undo -- a revoked feed stays revoked.
func RevokeAllFeeds(db *gorm.DB, userID uint) (int64, error) {
	result := db.Model(&models.Feed{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", Now())
	return result.RowsAffected, result.Error
}
