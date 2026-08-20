package controllers

import (
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListReachOutSuggestions returns every pending event-driven "reach out"
// suggestion for the user (issue #177) — mirrors GetOverdueCadences'
// read-only, no-pagination shape (cadence_controller.go).
func ListReachOutSuggestions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	suggestions, err := services.ListReachOutSuggestions(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to load reach-out suggestions").WithError(err))
		return
	}
	if suggestions == nil {
		suggestions = []models.ReachOutSuggestionResponse{}
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// DismissReachOutSuggestion marks a suggestion dismissed. Idempotent.
func DismissReachOutSuggestion(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	if err := services.DismissReachOutSuggestion(db, userID, id); err != nil {
		if appErr, ok := err.(*apperrors.AppError); ok {
			apperrors.AbortWithError(c, appErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to dismiss reach-out suggestion").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Suggestion dismissed"})
}
