package controllers

import (
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListContactSyncConflicts returns every pending CardDAV sync conflict for
// the user (issue #395) — a remote change overwrote a local edit and the
// notice is the only record of it. Mirrors ListReachOutSuggestions'
// read-only, no-pagination shape.
func ListContactSyncConflicts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	conflicts, err := services.ListContactSyncConflicts(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to load sync conflicts").WithError(err))
		return
	}
	if conflicts == nil {
		conflicts = []models.ContactSyncConflictResponse{}
	}

	c.JSON(http.StatusOK, gin.H{"sync_conflicts": conflicts})
}

// RestoreContactSyncConflict re-applies a conflict's overwritten local value
// onto its contact and dismisses the conflict.
func RestoreContactSyncConflict(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	if err := services.RestoreContactSyncConflict(db, userID, id); err != nil {
		if appErr, ok := err.(*apperrors.AppError); ok {
			apperrors.AbortWithError(c, appErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to restore sync conflict").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Local value restored"})
}

// DismissContactSyncConflict acknowledges a conflict without restoring its
// local value. Idempotent.
func DismissContactSyncConflict(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	id := c.Param("id")
	if err := services.DismissContactSyncConflict(db, userID, id); err != nil {
		if appErr, ok := err.(*apperrors.AppError); ok {
			apperrors.AbortWithError(c, appErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to dismiss sync conflict").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sync conflict dismissed"})
}
