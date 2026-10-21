package controllers

import (
	"errors"
	"net/http"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/scoring"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// loadOwnedContactForTarget loads the caller's contact for the
// target-closeness endpoints, writing the 404/500 itself on failure.
func loadOwnedContactForTarget(c *gin.Context) (*gorm.DB, uint, *models.Contact, bool) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return nil, 0, nil, false
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return nil, 0, nil, false
	}
	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return nil, 0, nil, false
	}
	return db, userID, &contact, true
}

// SetContactTargetCloseness sets the contact's target closeness (issue #1796,
// ADR 0023 amendment 2026-10-16). The write is the same hook-firing
// `Update` FavoriteContact uses, so the revision bumps. When the contact has no
// live cadence policy the response carries a cadence_policy_offer that a
// client can accept with a plain POST /cadence-policies.
func SetContactTargetCloseness(c *gin.Context) {
	db, userID, contact, ok := loadOwnedContactForTarget(c)
	if !ok {
		return
	}
	input, err := middleware.GetValidated[models.ContactTargetClosenessInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	cfg, cfgErr := scoring.LoadConfig()
	if cfgErr != nil { // # pragma: no cover — the embedded tunables file is validated by the scoring package tests, so it cannot fail to load
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to load scoring config").WithError(cfgErr))
		return
	}
	days := cfg.TargetTierIntervalDays[input.Tier]

	if err := db.Model(contact).Update("target_closeness", input.Tier).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to set target closeness").WithError(err))
		return
	}

	var policies int64
	if err := db.Model(&models.CadencePolicy{}).
		Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).
		Count(&policies).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check cadence policy").WithError(err))
		return
	}

	tier := input.Tier
	resp := models.ContactTargetClosenessResponse{
		ContactID:           contact.ID,
		TargetCloseness:     &tier,
		DefaultIntervalDays: &days,
	}
	if policies == 0 {
		resp.CadencePolicyOffer = &models.CadencePolicyInput{EntityID: contact.VCardUID, TargetIntervalDays: days}
	}
	c.JSON(http.StatusOK, resp)
}

// ClearContactTargetCloseness clears the target and dismisses the contact's
// pending target_behind suggestions. Idempotent: clearing an unset target is a
// 200.
func ClearContactTargetCloseness(c *gin.Context) {
	db, userID, contact, ok := loadOwnedContactForTarget(c)
	if !ok {
		return
	}

	if err := db.Model(contact).Update("target_closeness", nil).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to clear target closeness").WithError(err))
		return
	}
	if err := services.DismissPendingTargetBehindSuggestions(db, userID, contact.VCardUID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to dismiss target suggestions").WithError(err))
		return
	}

	c.JSON(http.StatusOK, models.ContactTargetClosenessResponse{ContactID: contact.ID})
}
