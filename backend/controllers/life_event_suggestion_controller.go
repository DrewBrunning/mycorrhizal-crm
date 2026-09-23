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

// GetLifeEventSuggestions returns the inferred, not-yet-resolved life-event
// candidates for a contact (issue #354 follow-up, docs/adrs/0025-temporal-periods.md).
// Pure read: suggestions are computed from the contact's dated field periods
// and are not stored. Always an array, never null (frontend-trap 8).
func GetLifeEventSuggestions(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	suggestions, err := services.SuggestLifeEvents(db, &contact)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute life event suggestions").WithError(err))
		return
	}
	if suggestions == nil {
		suggestions = []models.LifeEventSuggestion{}
	}
	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// ResolveLifeEventSuggestion records the user's accept/dismiss decision for one
// inferred candidate so it is not offered again. It does not create a
// LifeEvent: accepting is done by the user through the normal life-event
// create endpoint (which also lets them change the inferred type), and this
// call just writes the resolution memory.
func ResolveLifeEventSuggestion(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	input, err := middleware.GetValidated[models.LifeEventSuggestionResolutionInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	// Ownership: the subject contact must belong to the caller.
	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	if err := services.ResolveLifeEventSuggestion(db, userID, *input); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to resolve life event suggestion").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Suggestion resolved"})
}
