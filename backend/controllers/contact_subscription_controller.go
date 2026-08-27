package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// maxContactSubscriptionsPerUser mirrors maxCalendarSubscriptionsPerUser
// (calendar_controller.go)'s cap for the same reason: bound how many
// external servers one user's account can fan out sync requests to.
const maxContactSubscriptionsPerUser = 10

func ListContactSubscriptions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var subscriptions []models.ContactSubscription
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&subscriptions).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query"))
		return
	}

	counts, err := pendingConflictCounts(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"contact_subscriptions": toContactSubscriptionResponses(subscriptions, counts)})
}

// pendingConflictCounts returns the per-subscription tally of unreviewed
// CardDAV sync conflicts (issue #395), for the sync-health surface (issue
// #390). One grouped query, not a per-subscription COUNT.
func pendingConflictCounts(db *gorm.DB, userID uint) (map[uint]int64, error) {
	type row struct {
		SubscriptionID uint
		N              int64
	}
	var rows []row
	if err := db.Model(&models.ContactSyncConflict{}).
		Select("subscription_id, COUNT(*) AS n").
		Where("user_id = ? AND status = ?", userID, models.SyncConflictStatusPending).
		Group("subscription_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]int64, len(rows))
	for _, r := range rows {
		out[r.SubscriptionID] = r.N
	}
	return out, nil
}

func CreateContactSubscription(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var count int64
	if err := db.Model(&models.ContactSubscription{}).Where("user_id = ? AND deleted_at IS NULL", userID).Count(&count).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("count"))
		return
	}
	if count >= maxContactSubscriptionsPerUser {
		apperrors.AbortWithError(c, apperrors.ErrConflict("maximum of 10 contact subscriptions per user reached"))
		return
	}

	input, appErr := middleware.GetValidated[models.ContactSubscriptionInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	if _, err := services.NormalizeContactSubscriptionURL(input.URL); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("url", "must be an http or https URL"))
		return
	}

	passwordEncrypted, err := services.EncryptCredential(currentConfig(c).JWTSecretKey, input.Password)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("failed to store credentials"))
		return
	}

	subscription := models.ContactSubscription{
		UserID:            userID,
		Name:              input.Name,
		URL:               input.URL,
		Username:          input.Username,
		PasswordEncrypted: passwordEncrypted,
		SyncEnabled:       input.SyncEnabled == nil || *input.SyncEnabled,
	}

	if err := db.Create(&subscription).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("insert"))
		return
	}

	// A just-created subscription has no sync history and no conflicts yet.
	c.JSON(http.StatusCreated, toContactSubscriptionResponse(subscription, 0))
}

func UpdateContactSubscription(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	subscription, found := findContactSubscription(c, db, userID)
	if !found {
		return
	}

	input, appErr := middleware.GetValidated[models.ContactSubscriptionInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	if _, err := services.NormalizeContactSubscriptionURL(input.URL); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("url", "must be an http or https URL"))
		return
	}

	// The subscription's URL changed identity in CardDAV terms, so a
	// previously stored sync-token (scoped to the old collection) would no
	// longer be valid; reset it so the next sync performs a fresh full sync
	// via sync-collection's own initial-sync behavior (empty token).
	if subscription.URL != input.URL {
		subscription.SyncToken = ""
	}

	subscription.Name = input.Name
	subscription.URL = input.URL
	subscription.Username = input.Username
	if input.SyncEnabled != nil {
		subscription.SyncEnabled = *input.SyncEnabled
	}

	if input.ClearPassword {
		subscription.PasswordEncrypted = ""
	} else if input.Password != "" {
		passwordEncrypted, err := services.EncryptCredential(currentConfig(c).JWTSecretKey, input.Password)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInternal("failed to store credentials"))
			return
		}
		subscription.PasswordEncrypted = passwordEncrypted
	}

	if err := db.Save(&subscription).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update"))
		return
	}

	var pendingConflicts int64
	if err := db.Model(&models.ContactSyncConflict{}).
		Where("user_id = ? AND subscription_id = ? AND status = ?", userID, subscription.ID, models.SyncConflictStatusPending).
		Count(&pendingConflicts).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query"))
		return
	}

	c.JSON(http.StatusOK, toContactSubscriptionResponse(subscription, pendingConflicts))
}

func DeleteContactSubscription(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	subscription, found := findContactSubscription(c, db, userID)
	if !found {
		return
	}

	// Synced contacts stay (they are real, user-owned Contact rows per
	// decision); only the subscription, its sync links, and its pending sync
	// conflicts go.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", subscription.ID).Delete(&models.ContactSyncLink{}).Error; err != nil {
			return err
		}
		// Issue #395: nothing left to review for a removed subscription.
		if err := tx.Where("subscription_id = ?", subscription.ID).Delete(&models.ContactSyncConflict{}).Error; err != nil {
			return err
		}
		return tx.Delete(&subscription).Error
	}); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("delete"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contact subscription deleted"})
}

func SyncContactSubscription(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	subscription, found := findContactSubscription(c, db, userID)
	if !found {
		return
	}

	cfg := currentConfig(c)
	service := services.NewContactSyncService(cfg.CalDAVBlockPrivateURLs)
	stats, err := service.SyncSubscription(c.Request.Context(), db, cfg, &subscription)
	if err != nil {
		logger.FromContext(c).Warn().Err(err).Uint("subscription_id", subscription.ID).Msg("Manual contact sync failed")
		apperrors.AbortWithError(c, contactSyncError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Contact sync completed",
		"created":  stats.Created,
		"updated":  stats.Updated,
		"archived": stats.Archived,
		"skipped":  stats.Skipped,
	})
}

// contactSyncError maps the sync service's sentinel errors to API errors.
func contactSyncError(err error) *apperrors.AppError {
	switch {
	case errors.Is(err, services.ErrContactSyncInvalidURL):
		return apperrors.ErrInvalidInput("url", "contact subscription URL is invalid")
	case errors.Is(err, services.ErrContactSyncUnauthorized):
		return apperrors.ErrExternal("CardDAV", "authentication failed - check username and password")
	case errors.Is(err, services.ErrContactSyncNotFound):
		return apperrors.ErrExternal("CardDAV", "address book not found at the given URL")
	case errors.Is(err, services.ErrContactSyncPrivateAddress):
		return apperrors.ErrExternal("CardDAV", "URL resolves to a blocked private address")
	case errors.Is(err, services.ErrContactSyncTooLarge):
		return apperrors.ErrExternal("CardDAV", "server response is too large")
	case errors.Is(err, services.ErrContactSyncInvalidData):
		return apperrors.ErrExternal("CardDAV", "server returned data that could not be parsed")
	case errors.Is(err, services.ErrContactSyncUnreachable):
		// issue #524: the subscription's own URL is what can't be reached —
		// the caller's configuration, not a server malfunction — so this is a
		// 400, not a 503 (Schemathesis's not_a_server_error check correctly
		// refuses to accept a 5xx here).
		return apperrors.ErrValidation("server could not be reached — check the subscription URL")
	default:
		return apperrors.ErrOperationFailed("Contact sync", err.Error())
	}
}

func findContactSubscription(c *gin.Context, db *gorm.DB, userID uint) (models.ContactSubscription, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "must be a positive integer"))
		return models.ContactSubscription{}, false
	}

	var subscription models.ContactSubscription
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&subscription).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact subscription"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("query"))
		}
		return models.ContactSubscription{}, false
	}
	return subscription, true
}

func toContactSubscriptionResponse(sub models.ContactSubscription, pendingConflicts int64) models.ContactSubscriptionResponse {
	return models.ContactSubscriptionResponse{
		ID:             sub.ID,
		Name:           sub.Name,
		URL:            sub.URL,
		Username:       sub.Username,
		HasPassword:    sub.PasswordEncrypted != "",
		SyncEnabled:    sub.SyncEnabled,
		LastSyncedAt:   sub.LastSyncedAt,
		LastSyncStatus: sub.LastSyncStatus,
		LastSyncError:  sub.LastSyncError,
		CreatedAt:      sub.CreatedAt,

		SyncHealthResponse: models.NewSyncHealthResponse(sub.SyncHealthFields),
		PendingConflicts:   pendingConflicts,
	}
}

func toContactSubscriptionResponses(subs []models.ContactSubscription, pendingConflicts map[uint]int64) []models.ContactSubscriptionResponse {
	out := make([]models.ContactSubscriptionResponse, len(subs))
	for i, sub := range subs {
		out[i] = toContactSubscriptionResponse(sub, pendingConflicts[sub.ID])
	}
	return out
}
