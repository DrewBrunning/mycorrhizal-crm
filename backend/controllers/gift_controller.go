package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// validateGiftValueCurrency enforces T20b's explicit-currency rule: a stored
// gift value must never be ambiguous about what it is denominated in. A
// non-zero value requires a currency, and a currency without a value is
// meaningless — so the pair must be set together.
func validateGiftValueCurrency(input *models.GiftInput) *apperrors.AppError {
	if (input.ValueCents > 0) != (input.Currency != "") {
		return apperrors.ErrValidation("A gift value requires an explicit ISO 4217 currency, and a currency is only meaningful alongside a value")
	}
	return nil
}

// verifyOwnedLifeEvent confirms a soft-referenced LifeEvent belongs to the
// user (no FK, so ownership is verified in the query — CLAUDE.md trap 5).
func verifyOwnedLifeEvent(c *gin.Context, db *gorm.DB, userID uint, id string) bool {
	if id == "" {
		return true
	}
	var count int64
	if err := db.Model(&models.LifeEvent{}).Where("id = ? AND user_id = ?", id, userID).Count(&count).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to verify life event").WithError(err))
		return false
	}
	if count == 0 {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Life event").WithDetails("id", id))
		return false
	}
	return true
}

// verifyOwnedActivity confirms a soft-referenced Activity belongs to the user,
// mirroring DiscussConversationAgenda's own ownership check.
func verifyOwnedActivity(c *gin.Context, db *gorm.DB, userID uint, id *uint) bool {
	if id == nil {
		return true
	}
	var count int64
	if err := db.Model(&models.Activity{}).Where("id = ? AND user_id = ?", *id, userID).Count(&count).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to verify activity").WithError(err))
		return false
	}
	if count == 0 {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Activity").WithDetails("id", *id))
		return false
	}
	return true
}

// CreateGift creates a new Gift (gift.go) for the authenticated
// user, scoped to a Contact they own via EntityID. Status defaults to "idea" —
// a gift idea is captured opportunistically, mid conversation, without
// choosing a state. The optional LifeEvent/Activity references must belong to
// the user, and any value must carry an explicit currency.
func CreateGift(c *gin.Context) {
	input, err := middleware.GetValidated[models.GiftInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}
	if vErr := validateGiftValueCurrency(input); vErr != nil {
		apperrors.AbortWithError(c, vErr)
		return
	}
	if !verifyOwnedLifeEvent(c, db, userID, input.LifeEventID) {
		return
	}
	if !verifyOwnedActivity(c, db, userID, input.ActivityID) {
		return
	}

	status := input.Status
	if status == "" {
		status = models.GiftStatusIdea
	}

	gift := models.Gift{
		UserID:      userID,
		EntityID:    input.EntityID,
		Status:      status,
		Occasion:    input.Occasion,
		Description: input.Description,
		URL:         input.URL,
		Notes:       input.Notes,
		Date:        input.Date,
		ValueCents:  input.ValueCents,
		Currency:    input.Currency,
		LifeEventID: input.LifeEventID,
		ActivityID:  input.ActivityID,
	}

	if err := db.Create(&gift).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save gift").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Gift created successfully", "gift": gift})
}

// GetGift returns one Gift.
func GetGift(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var gift models.Gift
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&gift).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Gift").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve gift").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, gift)
}

// ListGifts returns the authenticated user's Gifts, cursor-paginated (T17),
// optionally filtered by ?entity_id=<Contact.VCardUID> in browse mode. The
// ?since= change feed ignores entity_id and returns every changed gift —
// including soft-deleted ones (tombstones) — so a replica converges. `total`
// is deliberately gone: gift records accumulate without bound and exact counts
// are the thing cursor pagination gives up.
func ListGifts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	params, err := GetCursorParams(c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	if err := CheckFeedCursorAge(c, params); err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	var gifts []models.Gift

	if params.Since {
		query := db.Unscoped().Model(&models.Gift{}).Where("user_id = ?", userID)
		if params.Cursor != nil {
			pred, t, idv := cursorPredicate("gifts", params.Cursor, params.Cursor.ID, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "gifts", false).Limit(params.Limit + 1)
		if err := query.Find(&gifts).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve gifts").WithError(err))
			return
		}
		nextCursor := ""
		if len(gifts) > params.Limit {
			gifts = gifts[:params.Limit]
			nextCursor = EncodeCursor(gifts[len(gifts)-1].UpdatedAt, gifts[len(gifts)-1].ID)
		}
		for i := range gifts {
			gifts[i].Deleted = gifts[i].DeletedAt.Valid
		}
		c.JSON(http.StatusOK, gin.H{
			"gifts":       gifts,
			"next_cursor": nextCursor,
			"limit":       params.Limit,
			"sync":        buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	entityID := c.Query("entity_id")

	baseQuery := db.Model(&models.Gift{}).Where("user_id = ?", userID)
	if entityID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", entityID)
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("gifts", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "gifts", desc).
		Limit(params.Limit + 1).
		Find(&gifts).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve gifts").WithError(err))
		return
	}
	nextCursor := ""
	if len(gifts) > params.Limit {
		gifts = gifts[:params.Limit]
		nextCursor = EncodeCursor(gifts[len(gifts)-1].UpdatedAt, gifts[len(gifts)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"gifts":       gifts,
		"next_cursor": nextCursor,
		"limit":       params.Limit,
		"sync":        buildSyncMeta(SyncModeIncremental),
	})
}

// UpdateGift updates a Gift — full-replace semantics via the same GiftInput as
// create, matching UpdateLifeEvent/UpdateConversationAgenda's precedent. The
// same ownership and value/currency/reference checks as create apply.
func UpdateGift(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var gift models.Gift
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&gift).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Gift").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve gift").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.GiftInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}
	if vErr := validateGiftValueCurrency(input); vErr != nil {
		apperrors.AbortWithError(c, vErr)
		return
	}
	if !verifyOwnedLifeEvent(c, db, userID, input.LifeEventID) {
		return
	}
	if !verifyOwnedActivity(c, db, userID, input.ActivityID) {
		return
	}

	status := input.Status
	if status == "" {
		status = models.GiftStatusIdea
	}

	gift.EntityID = input.EntityID
	gift.Status = status
	gift.Occasion = input.Occasion
	gift.Description = input.Description
	gift.URL = input.URL
	gift.Notes = input.Notes
	gift.Date = input.Date
	gift.ValueCents = input.ValueCents
	gift.Currency = input.Currency
	gift.LifeEventID = input.LifeEventID
	gift.ActivityID = input.ActivityID

	if err := db.Save(&gift).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save gift").WithError(err))
		return
	}

	services.TriggerWebhooksAsync(c.Request.Context(), db, currentConfig(c), userID, "gift.updated", gift)

	c.JSON(http.StatusOK, gift)
}

// DeleteGift soft-deletes a Gift (user-authored content). The row stays in the
// change feed as a tombstone but leaves the browse list.
func DeleteGift(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var gift models.Gift
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&gift).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Gift").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve gift").WithError(err))
		}
		return
	}

	if err := db.Delete(&gift).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete gift").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Gift deleted"})
}
