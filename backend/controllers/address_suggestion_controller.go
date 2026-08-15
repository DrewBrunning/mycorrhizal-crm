package controllers

import (
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SuggestContactAddresses is the trigger for contact-address suggestions (the
// inverse of T40): scan the caller's confirmed relationship edges and
// households and return every address a contact should probably have but
// doesn't yet. Read-only and idempotent — nothing is persisted here; the
// client renders the suggestions and lets the user apply or ignore each one.
func SuggestContactAddresses(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	suggestions, err := services.GenerateContactAddressSuggestions(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to scan address suggestions").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suggestions": suggestions,
		"total":       len(suggestions),
	})
}

// ApplyContactAddressSuggestion applies one address suggestion: the server
// re-derives the address from the current graph (the confirmed relationship
// or household membership still has to hold, and the source still has to
// carry that address) and appends it to the contact. Errors surface as
// specific statuses: 400 for an invalid suggestion identity, 404 for a
// missing contact/source, 409 when the link or address no longer holds (or
// the contact already has it).
func ApplyContactAddressSuggestion(c *gin.Context) {
	input, verr := middleware.GetValidated[models.ApplyContactAddressSuggestionInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contact, svcErr := services.ApplyContactAddressSuggestion(db, userID, input)
	if svcErr != nil {
		if appErr, ok := svcErr.(*apperrors.AppError); ok {
			apperrors.AbortWithError(c, appErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to apply address suggestion").WithError(svcErr))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "Address added to contact",
		"contact_vcard_uid": contact.VCardUID,
	})
}
