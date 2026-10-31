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

// buildCardDAVSyncScopeResponse loads the stored scope and computes the live
// counts and derived warnings. A user with no row gets the "all" defaults.
func buildCardDAVSyncScopeResponse(db *gorm.DB, userID uint) (models.CardDAVSyncScopeResponse, *apperrors.AppError) {
	scope, ids, err := services.LoadCardDAVSyncScope(db, userID)
	if err != nil {
		return models.CardDAVSyncScopeResponse{}, apperrors.ErrDatabase("load carddav sync scope").WithError(err)
	}
	sel := services.CardDAVSyncScopeSelectionFrom(scope, ids)
	inScope, total, err := services.CardDAVSyncScopeCounts(db, userID, sel)
	if err != nil {
		return models.CardDAVSyncScopeResponse{}, apperrors.ErrDatabase("count carddav sync scope contacts").WithError(err)
	}
	resp := models.CardDAVSyncScopeResponse{
		Mode:            sel.Mode,
		CircleIDs:       sel.CircleIDs,
		DefaultCircleID: sel.DefaultCircleID,
		InScopeCount:    inScope,
		TotalCount:      total,
		Warnings:        services.CardDAVSyncScopeWarnings(sel),
	}
	if !scope.UpdatedAt.IsZero() {
		updated := scope.UpdatedAt
		resp.UpdatedAt = &updated
	}
	return resp, nil
}

// GetCardDAVSyncScope returns the caller's CardDAV sync scope, its live
// contact counts, and any derived warnings (issue #1993, ADR 0041).
func GetCardDAVSyncScope(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	resp, appErr := buildCardDAVSyncScopeResponse(db, userID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// PutCardDAVSyncScope replaces the whole per-user sync scope (PUT is a full
// replace; there is no partial update). Every submitted circle must belong to
// the caller.
func PutCardDAVSyncScope(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	input, appErr := middleware.GetValidated[models.CardDAVSyncScopeInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	sel, appErr := services.ValidateCardDAVSyncScope(db, userID, *input)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	if _, err := services.ReplaceCardDAVSyncScope(db, userID, sel); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("save carddav sync scope").WithError(err))
		return
	}
	resp, appErr := buildCardDAVSyncScopeResponse(db, userID)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// PreviewCardDAVSyncScope validates a proposed scope and returns its live
// counts without persisting anything, so the UI can warn before saving.
func PreviewCardDAVSyncScope(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	input, appErr := middleware.GetValidated[models.CardDAVSyncScopeInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	sel, appErr := services.ValidateCardDAVSyncScope(db, userID, *input)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}
	inScope, total, err := services.CardDAVSyncScopeCounts(db, userID, sel)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("count preview carddav sync scope contacts").WithError(err))
		return
	}
	c.JSON(http.StatusOK, models.CardDAVSyncScopePreviewResponse{
		Mode:         sel.Mode,
		CircleIDs:    sel.CircleIDs,
		InScopeCount: inScope,
		TotalCount:   total,
		Warnings:     services.CardDAVSyncScopeWarnings(sel),
	})
}
