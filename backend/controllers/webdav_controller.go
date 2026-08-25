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

// abortWebDAVServiceError maps a services-level WebDAV sentinel error to the
// right HTTP status: a missing/never-configured connection or an
// invalid/expired app password is a 400 (issue #524 — the caller's own
// credentials or setup, not a server malfunction); a missing/not-found file
// or folder is a 404; everything else is the same 503 fallback (Nextcloud
// genuinely unreachable — that IS a server-side/upstream failure).
func abortWebDAVServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrWebDAVUnauthorized):
		apperrors.AbortWithError(c, apperrors.ErrValidation("Nextcloud app password is invalid, expired, or not configured").WithError(err))
	case errors.Is(err, services.ErrWebDAVNotFound):
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Nextcloud file or folder").WithError(err))
	case errors.Is(err, services.ErrWebDAVInvalidURL):
		apperrors.AbortWithError(c, apperrors.ErrValidation("Nextcloud base URL is invalid"))
	case errors.Is(err, services.ErrWebDAVRequestFailed):
		status := "an unexpected status"
		var reqErr *services.WebDAVRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		apperrors.AbortWithError(c, apperrors.ErrExternal("Nextcloud", fmt.Sprintf("Nextcloud returned an error (%s). The instance is reachable — this request itself failed.", status)).WithError(err))
	case errors.Is(err, services.ErrWebDAVInvalidData):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Nextcloud", "Nextcloud returned a response that could not be parsed. The WebDAV surface may have changed — check the instance version.").WithError(err))
	default:
		apperrors.AbortWithError(c, apperrors.ErrExternal("Nextcloud", "Could not reach Nextcloud. Is the instance up?").WithError(err))
	}
}

// GetWebDAVConfig returns the current user's Nextcloud connection config, or
// an empty default when none is configured (200, not 404).
func GetWebDAVConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	wc, err := services.GetWebDAVConfigForUser(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Nextcloud config").WithError(err))
		return
	}
	if wc == nil {
		c.JSON(http.StatusOK, services.WebDAVConfigResponse{})
		return
	}

	c.JSON(http.StatusOK, services.WebDAVConfigResponse{
		BaseURL:        wc.BaseURL,
		Username:       wc.Username,
		HasAppPassword: wc.HasAppPassword(),
	})
}

// SaveWebDAVConfig creates or updates the current user's Nextcloud connection.
// The app password is encrypted at rest and never returned.
func SaveWebDAVConfig(c *gin.Context) {
	input, err := middleware.GetValidated[models.WebDAVConfigInput](c)
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
	existing, getErr := services.GetWebDAVConfigForUser(db, userID)
	if getErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Nextcloud config").WithError(getErr))
		return
	}
	// A create with no app password is a validation error.
	if existing == nil && strings.TrimSpace(input.AppPassword) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("app_password", "an app password is required when first connecting Nextcloud"))
		return
	}

	wc, saveErr := services.UpsertWebDAVConfig(db, cfg.JWTSecretKey, userID, *input)
	if saveErr != nil {
		if errors.Is(saveErr, services.ErrWebDAVInvalidURL) {
			abortWebDAVServiceError(c, saveErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save Nextcloud config").WithError(saveErr))
		return
	}

	c.JSON(http.StatusOK, services.WebDAVConfigResponse{
		BaseURL:        wc.BaseURL,
		Username:       wc.Username,
		HasAppPassword: wc.HasAppPassword(),
	})
}

// DeleteWebDAVConfig removes the current user's Nextcloud connection. Linked
// ExternalIdentity rows are kept.
func DeleteWebDAVConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if err := services.DeleteWebDAVConfig(db, userID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Nextcloud config").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Nextcloud config deleted"})
}

// TestWebDAVConnection diagnoses the current user's saved Nextcloud connection
// (Settings' "Test connection" button). A service-level error still goes
// through abortWebDAVServiceError; otherwise this always responds 200 — a
// diagnosed failure (ok: false) is a successful response.
func TestWebDAVConnection(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	result, err := services.TestWebDAVConnection(db, cfg, userID)
	if err != nil {
		abortWebDAVServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListWebDAVDir lists a directory's children (the L1 picker's navigation).
// ?path= defaults to "/" (the dav root). This is the user's own data, so no
// per-contact scope.
func ListWebDAVDir(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	path := c.DefaultQuery("path", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	cfg := currentConfig(c)
	items, err := services.ListWebDAVDirForUser(db, cfg, userID, path)
	if err != nil {
		abortWebDAVServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// LinkWebDAVContact links a Nextcloud file/folder to a contact as an
// ExternalIdentity (system: "nextcloud"). Verifies the contact belongs to the
// user, then delegates the link to the service. A duplicate link is a 409.
func LinkWebDAVContact(c *gin.Context) {
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
		Path       string `json:"path" validate:"required,min=1,max=512"`
		Name       string `json:"name" validate:"required,min=1,max=255"`
		Type       string `json:"type" validate:"required,oneof=file dir"`
		Size       int64  `json:"size,omitempty"`
		ModifiedAt string `json:"modified_at,omitempty" validate:"omitempty,max=64"`
		FileID     string `json:"file_id,omitempty" validate:"omitempty,max=64"`
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
	if strings.TrimSpace(input.Path) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("path", "path is required"))
		return
	}

	// Pre-check the natural key so a duplicate link is a clear 409.
	externalID := services.NormalizeWebDAVPathForKey(input.Path)
	var existing models.ExternalIdentity
	lookupErr := db.Where("system = ? AND external_id = ? AND user_id = ?", services.ExternalSystemWebDAV, externalID, userID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Nextcloud link").WithDetails("path", input.Path))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing Nextcloud link").WithError(lookupErr))
		return
	}

	cfg := currentConfig(c)
	identity, err := services.LinkWebDAVItem(db, cfg, userID, contactUID, services.WebDAVLinkMetadata{
		Path:       input.Path,
		Name:       input.Name,
		Type:       input.Type,
		Size:       input.Size,
		ModifiedAt: input.ModifiedAt,
		FileID:     input.FileID,
	})
	if err != nil {
		abortWebDAVServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"external_identity": identity})
}

// UnlinkWebDAVContact removes a specific Nextcloud ExternalIdentity (hard
// delete — a contact can link many files/folders, so the identity id
// identifies which one). Scoped to the contact in the path.
func UnlinkWebDAVContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	identityID := c.Param("identity_id")
	var identity models.ExternalIdentity
	if err := db.Where("id = ? AND user_id = ? AND system = ? AND entity_id = ?", identityID, userID, services.ExternalSystemWebDAV, contactUID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Nextcloud link"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Nextcloud link").WithError(err))
		}
		return
	}

	if err := db.Delete(&identity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Nextcloud link").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Nextcloud link removed"})
}
