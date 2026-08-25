package services

import (
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// RevokeAllAPITokens revokes every currently-active API token for a user and
// returns how many were revoked. Shared by every call site that must end a
// user's standing tokens as part of a bigger action: the self-service
// "revoke all my tokens" endpoint, an admin password reset (account-takeover
// response, issue #413), and the recovery-path password reset (issue #411,
// the original site this logic was extracted from). API tokens carry no
// TokenVersion of their own, so bumping a user's TokenVersion (which kills
// JWT sessions) never touches them -- this is the only way to end them in
// bulk.
func RevokeAllAPITokens(db *gorm.DB, userID uint) (int64, error) {
	result := db.Model(&models.ApiToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", time.Now())
	return result.RowsAffected, result.Error
}
