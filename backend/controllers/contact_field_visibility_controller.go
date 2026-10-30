package controllers

import (
	"errors"
	"net/http"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Per-contact field visibility (issue #1997). Cosmetic display preferences kept
// outside the neutral Card: these handlers never touch the contact row, so they
// do not bump its revision/ETag (and so need no If-Match, the same exemption as
// PUT /contacts/:id/field-values), and the data never reaches vCard/JSContact,
// CardDAV/CalDAV, shares or MCP. Ownership is resolved first
// (resolveOwnedContactByID -> 404 for another user's or a missing contact) and
// every query below is scoped by user_id as well.

// fieldOverrideErrorToAppError maps a service validation failure to a 400
// carrying the matching reason code (ADR 0038).
func fieldOverrideErrorToAppError(err error) (*apperrors.AppError, bool) {
	var oe *services.FieldOverrideError
	if !errors.As(err, &oe) {
		return nil, false
	}
	params := map[string]any{"field": oe.Key}
	if oe.Kind == services.FieldOverrideUnknownKey {
		return apperrors.ErrInvalidInput("overrides", oe.Error()).
			WithReason(apperrors.ReasonContactFieldOverrideUnknownKey, params), true
	}
	return apperrors.ErrInvalidInput("overrides", oe.Error()).
		WithReason(apperrors.ReasonContactFieldOverrideStateInvalid, params), true
}

// GetContactFieldVisibility returns the raw overrides and the resolved
// visibility (per-contact override > account default > built-in default) for
// one of the caller's contacts.
func GetContactFieldVisibility(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := resolveOwnedContactByID(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	respondContactFieldVisibility(c, db, userID, contact.VCardUID)
}

// PutContactFieldVisibility replaces the contact's complete override set.
func PutContactFieldVisibility(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := resolveOwnedContactByID(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	input, err := middleware.GetValidated[models.ContactFieldVisibilityInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	if err := services.ReplaceContactFieldOverrides(db, userID, contact.VCardUID, *input); err != nil {
		if appErr, ok := fieldOverrideErrorToAppError(err); ok {
			apperrors.AbortWithError(c, appErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update field visibility").WithError(err))
		return
	}
	respondContactFieldVisibility(c, db, userID, contact.VCardUID)
}

// ResetContactFieldVisibility removes every override of the contact, so it
// inherits the account defaults again, and returns the resulting visibility.
func ResetContactFieldVisibility(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := resolveOwnedContactByID(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	if err := services.ClearContactFieldOverrides(db, userID, contact.VCardUID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("reset field visibility").WithError(err))
		return
	}
	respondContactFieldVisibility(c, db, userID, contact.VCardUID)
}

func respondContactFieldVisibility(c *gin.Context, db *gorm.DB, userID uint, contactUID string) {
	resp, err := services.GetContactFieldVisibility(db, userID, contactUID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query field visibility").WithError(err))
		return
	}
	c.JSON(http.StatusOK, resp)
}

// UpdateHideEmptyFields sets the account-wide hide-empty default.
func UpdateHideEmptyFields(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	input, err := middleware.GetValidated[models.HideEmptyFieldsInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	if input.HideEmptyFields == nil { // defence in depth: the `required` tag normally rejects this first
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("hide_empty_fields", "required").
			WithReason(apperrors.ReasonValidationMissingField, map[string]any{"field": "hide_empty_fields"}))
		return
	}
	// Column-scoped write so the password hash / OIDC fields are never rewritten.
	if err := db.Model(&models.User{}).Where("id = ?", userID).
		Update("hide_empty_fields", *input.HideEmptyFields).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update user").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":           "Hide-empty default updated successfully",
		"hide_empty_fields": *input.HideEmptyFields,
	})
}
