package controllers

import (
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetUpcomingOccasions is the "upcoming occasions" dashboard-widget aggregate
// (ADR 0024, issue #387, ticket #1224): every birthday/anniversary/life-event/
// active-obligation occurrence within the next `days` (30 or 90, default 30),
// sorted ascending by days-until.
//
// "Today" is decided via reminderNow(c) — the single reminder-zone clock
// (ADR 0015 Rule 4), not the server's own local zone.
func GetUpcomingOccasions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 30 && parsed != 90) {
			apperrors.AbortWithError(c, apperrors.ErrValidation("days must be 30 or 90"))
			return
		}
		days = parsed
	}
	includeSensitive := c.Query("include_sensitive") == "true"

	now := reminderNow(c)
	occasions, err := services.GetUpcomingOccasions(db, userID, now, days, includeSensitive)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve upcoming occasions").WithError(err))
		return
	}
	// CLAUDE.md frontend trap #8: never let an empty result serialize as an
	// absent key — a nil slice with `omitempty` disappears from the JSON
	// entirely, which a required TS array type cannot guard against.
	if occasions == nil {
		occasions = []models.UpcomingOccasion{}
	}

	c.JSON(http.StatusOK, gin.H{"occasions": occasions, "days": days})
}
