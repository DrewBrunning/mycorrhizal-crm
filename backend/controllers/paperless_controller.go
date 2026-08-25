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

// abortPaperlessServiceError maps a services-level Paperless sentinel error to
// the right HTTP status: an unreachable instance or expired token is a 503
// (external service error) with a stable message; a missing/not-found document
// is a 404; a real (non-2xx) response from Paperless that isn't one of those
// is its own distinct 503; everything else is the same 503 fallback.
func abortPaperlessServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrPaperlessUnauthorized):
		// issue #524: a missing/never-configured connection or an
		// invalid/expired token is the caller's own credentials or setup, not
		// a server malfunction — 400, not 503.
		apperrors.AbortWithError(c, apperrors.ErrValidation("Paperless API token is invalid, expired, or not configured").WithError(err))
	case errors.Is(err, services.ErrPaperlessNotFound):
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Paperless document").WithError(err))
	case errors.Is(err, services.ErrPaperlessInvalidURL):
		apperrors.AbortWithError(c, apperrors.ErrValidation("Paperless base URL is invalid"))
	case errors.Is(err, services.ErrPaperlessRequestFailed):
		status := "an unexpected status"
		var reqErr *services.PaperlessRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		apperrors.AbortWithError(c, apperrors.ErrExternal("Paperless", fmt.Sprintf("Paperless returned an error (%s). The instance is reachable — this request itself failed.", status)).WithError(err))
	case errors.Is(err, services.ErrPaperlessInvalidData):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Paperless", "Paperless returned a response that could not be parsed. The API may have changed — check Paperless version compatibility.").WithError(err))
	default:
		apperrors.AbortWithError(c, apperrors.ErrExternal("Paperless", "Could not reach Paperless. Is the instance up?").WithError(err))
	}
}

// GetPaperlessConfig returns the current user's Paperless connection config,
// or an empty default when none is configured (200, not 404 — the frontend
// renders the "connect" form from the empty shape).
func GetPaperlessConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	pc, err := services.GetPaperlessConfigForUser(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Paperless config").WithError(err))
		return
	}
	if pc == nil {
		c.JSON(http.StatusOK, services.PaperlessConfigResponse{})
		return
	}

	c.JSON(http.StatusOK, services.PaperlessConfigResponse{
		BaseURL:     pc.BaseURL,
		HasAPIToken: pc.HasAPIToken(),
	})
}

// SavePaperlessConfig creates or updates the current user's Paperless
// connection. The API token is encrypted at rest and never returned.
func SavePaperlessConfig(c *gin.Context) {
	input, err := middleware.GetValidated[models.PaperlessConfigInput](c)
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
	existing, getErr := services.GetPaperlessConfigForUser(db, userID)
	if getErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Paperless config").WithError(getErr))
		return
	}
	// A create with no API token is a validation error ("API token required").
	if existing == nil && strings.TrimSpace(input.APIToken) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("api_token", "an API token is required when first connecting Paperless"))
		return
	}

	pc, saveErr := services.UpsertPaperlessConfig(db, cfg.JWTSecretKey, userID, *input)
	if saveErr != nil {
		if errors.Is(saveErr, services.ErrPaperlessInvalidURL) {
			abortPaperlessServiceError(c, saveErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save Paperless config").WithError(saveErr))
		return
	}

	c.JSON(http.StatusOK, services.PaperlessConfigResponse{
		BaseURL:     pc.BaseURL,
		HasAPIToken: pc.HasAPIToken(),
	})
}

// DeletePaperlessConfig removes the current user's Paperless connection.
// Linked ExternalIdentity rows are kept.
func DeletePaperlessConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if err := services.DeletePaperlessConfig(db, userID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Paperless config").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Paperless config deleted"})
}

// TestPaperlessConnection diagnoses the current user's saved Paperless
// connection (Settings' "Test connection" button): reachability, then API
// token validity, reported as a specific stage/message rather than a generic
// error. A service-level error (no connection configured, unparseable stored
// URL) still goes through abortPaperlessServiceError; otherwise this always
// responds 200 — a diagnosed failure (ok: false) is a successful response.
func TestPaperlessConnection(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	result, err := services.TestPaperlessConnection(db, cfg, userID)
	if err != nil {
		abortPaperlessServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListPaperlessDocuments browses documents in the user's Paperless instance so
// the L1 picker can search (optional ?query=). This is the user's own data, so
// no per-contact scope.
func ListPaperlessDocuments(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	documents, err := services.ListPaperlessDocumentsForUser(db, cfg, userID, c.Query("query"))
	if err != nil {
		abortPaperlessServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"documents": documents})
}

// LinkPaperlessContact links a Paperless document to a contact as an
// ExternalIdentity (system: "paperless"). Verifies the contact belongs to the
// user, then delegates the link to the service. A duplicate link is a 409.
func LinkPaperlessContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	if !verifyOwnedContact(c, db, userID, contactUID) {
		return
	}

	var input struct {
		DocumentID string `json:"document_id" validate:"required,min=1,max=64"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("request body", err.Error()))
		return
	}
	if verrs := middleware.ValidateStruct(&input); len(verrs) > 0 {
		appErr := apperrors.ErrValidation("Request validation failed")
		for _, ve := range verrs {
			appErr.WithDetails(ve.Field, ve.Message)
		}
		apperrors.AbortWithError(c, appErr)
		return
	}
	if strings.TrimSpace(input.DocumentID) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("document_id", "document_id is required"))
		return
	}

	// Pre-check the natural key so a duplicate link is a clear 409.
	var existing models.ExternalIdentity
	lookupErr := db.Where("system = ? AND external_id = ? AND user_id = ?", services.ExternalSystemPaperless, input.DocumentID, userID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Paperless link").WithDetails("document_id", input.DocumentID))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing Paperless link").WithError(lookupErr))
		return
	}

	cfg := currentConfig(c)
	identity, err := services.LinkPaperlessDocument(db, cfg, userID, contactUID, input.DocumentID)
	if err != nil {
		abortPaperlessServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"external_identity": identity})
}

// UnlinkPaperlessContact removes a specific Paperless ExternalIdentity (hard
// delete — a contact can link many documents, so the identity id identifies
// which one). Scoped to the contact in the path, so an identity can only be
// unlinked through its own contact.
func UnlinkPaperlessContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	identityID := c.Param("identity_id")
	var identity models.ExternalIdentity
	if err := db.Where("id = ? AND user_id = ? AND system = ? AND entity_id = ?", identityID, userID, services.ExternalSystemPaperless, contactUID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Paperless link"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Paperless link").WithError(err))
		}
		return
	}

	if err := db.Delete(&identity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Paperless link").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Paperless link removed"})
}
