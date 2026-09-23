package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetContactScore returns the full, explainable relationship health score
// breakdown for one contact (issue #383, ADR-0023) — the contact-detail-page
// badge's popover data. Read-only, no writes, no cache. Same ownership-check
// shape as GetContactBriefing: the contact must belong to the caller, and
// (unlike the graph endpoints, which only ever show non-archived contacts)
// an archived contact still gets a real score here, matching
// GetContactBriefing's own no-archived-filter behavior.
func GetContactScore(c *gin.Context) {
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

	result, err := services.ComputeContactScore(db, userID, &contact, reminderNow(c))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute contact score").WithError(err))
		return
	}

	c.JSON(http.StatusOK, toContactScoreResponse(contact.ID, result))
}

// toContactScoreResponse maps the internal/scoring engine's Result onto the
// wire DTO, mirroring how buildContactBriefing wraps services.CadenceHealth
// in models.BriefingCadenceHealth rather than exposing the internal type
// directly.
func toContactScoreResponse(contactID uint, result scoring.Result) models.ContactScoreResponse {
	facet := func(f scoring.FacetResult) models.ContactScoreFacet {
		return models.ContactScoreFacet{Value: f.Value, Weight: f.Weight, Reason: f.Reason}
	}
	return models.ContactScoreResponse{
		ContactID:   contactID,
		Score:       result.Score,
		Band:        result.Band,
		Recency:     facet(result.Recency),
		Frequency:   facet(result.Frequency),
		Closeness:   facet(result.Closeness),
		ReachOut:    facet(result.ReachOut),
		LastUpdated: facet(result.LastUpdated),
	}
}
