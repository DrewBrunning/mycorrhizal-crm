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

// abortSeafileServiceError maps a services-level Seafile sentinel error to the
// right HTTP status: an unreachable instance or expired token is a 503; a
// missing/not-found library or file is a 404; everything else is the same 503
// fallback.
func abortSeafileServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrSeafileUnauthorized):
		// issue #524: a missing/never-configured connection or an
		// invalid/expired token is the caller's own credentials or setup, not
		// a server malfunction — 400, not 503.
		apperrors.AbortWithError(c, apperrors.ErrValidation("Seafile API token is invalid, expired, or not configured").WithError(err))
	case errors.Is(err, services.ErrSeafileNotFound):
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Seafile library or file").WithError(err))
	case errors.Is(err, services.ErrSeafileInvalidURL):
		apperrors.AbortWithError(c, apperrors.ErrValidation("Seafile base URL is invalid"))
	case errors.Is(err, services.ErrSeafileRequestFailed):
		status := "an unexpected status"
		var reqErr *services.SeafileRequestError
		if errors.As(err, &reqErr) {
			status = reqErr.Status
		}
		apperrors.AbortWithError(c, apperrors.ErrExternal("Seafile", fmt.Sprintf("Seafile returned an error (%s). The instance is reachable — this request itself failed.", status)).WithError(err))
	case errors.Is(err, services.ErrSeafileInvalidData):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Seafile", "Seafile returned a response that could not be parsed. The API may have changed — check Seafile version compatibility.").WithError(err))
	default:
		apperrors.AbortWithError(c, apperrors.ErrExternal("Seafile", "Could not reach Seafile. Is the server up?").WithError(err))
	}
}

// GetSeafileConfig returns the current user's Seafile connection config, or an
// empty default when none is configured (200, not 404).
func GetSeafileConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	sc, err := services.GetSeafileConfigForUser(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Seafile config").WithError(err))
		return
	}
	if sc == nil {
		c.JSON(http.StatusOK, services.SeafileConfigResponse{})
		return
	}

	c.JSON(http.StatusOK, services.SeafileConfigResponse{
		BaseURL:     sc.BaseURL,
		HasAPIToken: sc.HasAPIToken(),
	})
}

// SaveSeafileConfig creates or updates the current user's Seafile connection.
// The API token is encrypted at rest and never returned.
func SaveSeafileConfig(c *gin.Context) {
	input, err := middleware.GetValidated[models.SeafileConfigInput](c)
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
	existing, getErr := services.GetSeafileConfigForUser(db, userID)
	if getErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Seafile config").WithError(getErr))
		return
	}
	// A create with no API token is a validation error ("API token required").
	if existing == nil && strings.TrimSpace(input.APIToken) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("api_token", "an API token is required when first connecting Seafile"))
		return
	}

	sc, saveErr := services.UpsertSeafileConfig(db, cfg.JWTSecretKey, userID, *input)
	if saveErr != nil {
		if errors.Is(saveErr, services.ErrSeafileInvalidURL) {
			abortSeafileServiceError(c, saveErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save Seafile config").WithError(saveErr))
		return
	}

	c.JSON(http.StatusOK, services.SeafileConfigResponse{
		BaseURL:     sc.BaseURL,
		HasAPIToken: sc.HasAPIToken(),
	})
}

// DeleteSeafileConfig removes the current user's Seafile connection. Linked
// ExternalIdentity rows are kept.
func DeleteSeafileConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if err := services.DeleteSeafileConfig(db, userID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Seafile config").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Seafile config deleted"})
}

// TestSeafileConnection diagnoses the current user's saved Seafile connection
// (Settings' "Test connection" button): reachability, then token validity. A
// service-level error still goes through abortSeafileServiceError; otherwise
// this always responds 200 — a diagnosed failure (ok: false) is a successful
// response.
func TestSeafileConnection(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	result, err := services.TestSeafileConnection(db, cfg, userID)
	if err != nil {
		abortSeafileServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListSeafileLibraries browses every library the user's token can access (the
// L1 picker's first level). This is the user's own data, so no per-contact
// scope.
func ListSeafileLibraries(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	libraries, err := services.ListSeafileLibrariesForUser(db, cfg, userID)
	if err != nil {
		abortSeafileServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"libraries": libraries})
}

// ListSeafileDir lists a library folder's contents (the L1 picker's
// navigation). ?path= defaults to "/".
func ListSeafileDir(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	repoID := c.Param("repo_id")
	path := c.DefaultQuery("path", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	cfg := currentConfig(c)
	items, err := services.ListSeafileDirForUser(db, cfg, userID, repoID, path)
	if err != nil {
		abortSeafileServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// LinkSeafileContact links a Seafile file/folder to a contact as an
// ExternalIdentity (system: "seafile"). Verifies the contact belongs to the
// user, then delegates the link to the service. A duplicate link is a 409.
func LinkSeafileContact(c *gin.Context) {
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
		RepoID string `json:"repo_id" validate:"required,min=1,max=64"`
		Path   string `json:"path" validate:"required,min=1,max=512"`
		Name   string `json:"name" validate:"required,min=1,max=255"`
		Type   string `json:"type" validate:"required,oneof=file dir"`
		Size   int64  `json:"size,omitempty"`
		MTime  int64  `json:"mtime,omitempty"`
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
	if strings.TrimSpace(input.RepoID) == "" || strings.TrimSpace(input.Path) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("repo_id/path", "repo_id and path are required"))
		return
	}

	// Pre-check the natural key so a duplicate link is a clear 409.
	externalID := input.RepoID + ":" + input.Path
	var existing models.ExternalIdentity
	lookupErr := db.Where("system = ? AND external_id = ? AND user_id = ?", services.ExternalSystemSeafile, externalID, userID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Seafile link").WithDetails("repo_id", input.RepoID).WithDetails("path", input.Path))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing Seafile link").WithError(lookupErr))
		return
	}

	cfg := currentConfig(c)
	identity, err := services.LinkSeafileItem(db, cfg, userID, contactUID, services.SeafileLinkMetadata{
		RepoID: input.RepoID,
		Path:   input.Path,
		Name:   input.Name,
		Type:   input.Type,
		Size:   input.Size,
		MTime:  input.MTime,
	})
	if err != nil {
		abortSeafileServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"external_identity": identity})
}

// UnlinkSeafileContact removes a specific Seafile ExternalIdentity (hard
// delete — a contact can link many files/folders, so the identity id
// identifies which one). Scoped to the contact in the path.
func UnlinkSeafileContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	identityID := c.Param("identity_id")
	var identity models.ExternalIdentity
	if err := db.Where("id = ? AND user_id = ? AND system = ? AND entity_id = ?", identityID, userID, services.ExternalSystemSeafile, contactUID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Seafile link"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Seafile link").WithError(err))
		}
		return
	}

	if err := db.Delete(&identity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Seafile link").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Seafile link removed"})
}
