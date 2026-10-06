package controllers

import (
	"errors"
	"fmt"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// abortGeoPulseServiceError maps a services-level GeoPulse sentinel error to the
// right HTTP status, the same shape as abortPaperlessServiceError: a
// missing/invalid token is the caller's own setup (400, issue #524), a real
// non-2xx from GeoPulse is its own 503, everything else is the unreachable 503.
func abortGeoPulseServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrGeoPulseUnauthorized):
		apperrors.AbortWithError(c, apperrors.ErrValidation("GeoPulse API token is invalid, expired, or not configured").WithError(err))
	case errors.Is(err, services.ErrGeoPulseConfigUnreadable):
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to load the GeoPulse connection").WithError(err))
	case errors.Is(err, services.ErrGeoPulseKeyUndecryptable):
		apperrors.AbortWithError(c, apperrors.ErrValidation("The stored GeoPulse API token can no longer be decrypted — re-enter it in Settings").WithError(err))
	case errors.Is(err, services.ErrGeoPulseNotFound):
		apperrors.AbortWithError(c, apperrors.ErrNotFound("GeoPulse resource").WithError(err))
	case errors.Is(err, services.ErrGeoPulseInvalidURL):
		apperrors.AbortWithError(c, apperrors.ErrValidation("GeoPulse base URL is invalid"))
	case errors.Is(err, services.ErrGeoPulseRedirect):
		apperrors.AbortWithError(c, apperrors.ErrExternal("GeoPulse", "GeoPulse answered with a redirect — check the base URL (http vs https, path).").WithError(err))
	case errors.Is(err, services.ErrGeoPulseInvalidDate):
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("date", "date must be YYYY-MM-DD and timezone a valid IANA name"))
	case errors.Is(err, services.ErrGeoPulseRequestFailed):
		status := "an unexpected status"
		var reqErr *services.GeoPulseRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		apperrors.AbortWithError(c, apperrors.ErrExternal("GeoPulse", fmt.Sprintf("GeoPulse returned an error (%s). The instance is reachable — this request itself failed.", status)).WithError(err))
	case errors.Is(err, services.ErrGeoPulseInvalidData):
		apperrors.AbortWithError(c, apperrors.ErrExternal("GeoPulse", "GeoPulse returned a response that could not be parsed. The API may have changed — check GeoPulse version compatibility.").WithError(err))
	default:
		apperrors.AbortWithError(c, apperrors.ErrExternal("GeoPulse", "Could not reach GeoPulse. Is the instance up?").WithError(err))
	}
}

// GetGeoPulseConfig returns the current user's GeoPulse connection config, or an
// empty default when none is configured (200, not 404).
func GetGeoPulseConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	gc, err := services.GetGeoPulseConfigForUser(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve GeoPulse config").WithError(err))
		return
	}
	if gc == nil {
		c.JSON(http.StatusOK, services.GeoPulseConfigResponse{})
		return
	}
	c.JSON(http.StatusOK, services.GeoPulseConfigResponse{BaseURL: gc.BaseURL, HasAPIKey: gc.HasAPIKey()})
}

// SaveGeoPulseConfig creates or updates the current user's GeoPulse connection.
// The API token is encrypted at rest and never returned.
func SaveGeoPulseConfig(c *gin.Context) {
	input, err := middleware.GetValidated[models.GeoPulseConfigInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	existing, getErr := services.GetGeoPulseConfigForUser(db, userID)
	if getErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve GeoPulse config").WithError(getErr))
		return
	}
	if existing == nil && strings.TrimSpace(input.APIKey) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("api_key", "an API token is required when first connecting GeoPulse"))
		return
	}

	gc, saveErr := services.UpsertGeoPulseConfig(db, cfg.JWTSecretKey, userID, *input)
	if saveErr != nil {
		if errors.Is(saveErr, services.ErrGeoPulseInvalidURL) {
			abortGeoPulseServiceError(c, saveErr)
			return
		}
		if errors.Is(saveErr, services.ErrGeoPulseTokenRequired) {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("api_key", saveErr.Error()))
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save GeoPulse config").WithError(saveErr))
		return
	}
	c.JSON(http.StatusOK, services.GeoPulseConfigResponse{BaseURL: gc.BaseURL, HasAPIKey: gc.HasAPIKey()})
}

// DeleteGeoPulseConfig removes the current user's GeoPulse connection.
// Activities already confirmed from GeoPulse stays are kept.
func DeleteGeoPulseConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if err := services.DeleteGeoPulseConfig(db, userID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete GeoPulse config").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "GeoPulse config deleted"})
}

// TestGeoPulseConnection diagnoses the saved GeoPulse connection (Settings'
// "Test connection" button). A service-level error (no connection, unparseable
// stored URL) goes through abortGeoPulseServiceError; otherwise it always
// responds 200 — a diagnosed failure (ok: false) is a successful response.
func TestGeoPulseConnection(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	result, err := services.TestGeoPulseConnection(c.Request.Context(), db, currentConfig(c), userID)
	if err != nil {
		abortGeoPulseServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetGeoPulseSuggestions returns the ephemeral "log activity from location
// history" suggestion list for ?date=YYYY-MM-DD (optional ?timezone= IANA name,
// default UTC). Read-only and unpersisted: confirming one is a normal
// POST /activities pre-filled from the suggestion (ADR 0033).
func GetGeoPulseSuggestions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	date := strings.TrimSpace(c.Query("date"))
	if date == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("date", "date is required (YYYY-MM-DD)"))
		return
	}

	result, err := services.GeoPulseSuggestionsForDate(c.Request.Context(), db, currentConfig(c), userID, date, strings.TrimSpace(c.Query("timezone")))
	if err != nil {
		abortGeoPulseServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
