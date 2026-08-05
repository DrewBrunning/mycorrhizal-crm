package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// abortImmichServiceError maps a services-level Immich sentinel error to the
// right HTTP status: an unreachable instance or expired key is a 503
// (external service error) with a stable message; a missing/not-found person
// is a 404; everything else is a 503 fallback.
func abortImmichServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrImmichUnauthorized):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Immich", "Immich API key is invalid, expired, or not configured").WithError(err))
	case errors.Is(err, services.ErrImmichNotFound):
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Immich person").WithError(err))
	case errors.Is(err, services.ErrImmichInvalidURL):
		apperrors.AbortWithError(c, apperrors.ErrValidation("Immich base URL is invalid"))
	default:
		apperrors.AbortWithError(c, apperrors.ErrExternal("Immich", "Could not reach Immich. Is the instance up?").WithError(err))
	}
}

// GetImmichConfig returns the current user's Immich connection config, or an
// empty default when none is configured (200, not 404 — the frontend renders
// the "connect" form from the empty shape).
func GetImmichConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	ic, err := services.GetImmichConfigForUser(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Immich config").WithError(err))
		return
	}
	if ic == nil {
		c.JSON(http.StatusOK, services.ImmichConfigResponse{})
		return
	}

	c.JSON(http.StatusOK, services.ImmichConfigResponse{
		BaseURL:        ic.BaseURL,
		HasAPIKey:      ic.HasAPIKey(),
		SyncEnabled:    ic.SyncEnabled,
		LastSyncedAt:   ic.LastSyncedAt,
		LastSyncStatus: ic.LastSyncStatus,
		LastSyncError:  ic.LastSyncError,
	})
}

// SaveImmichConfig creates or updates the current user's Immich connection.
// The API key is encrypted at rest and never returned.
func SaveImmichConfig(c *gin.Context) {
	input, err := middleware.GetValidated[models.ImmichConfigInput](c)
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
	existing, getErr := services.GetImmichConfigForUser(db, userID)
	if getErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Immich config").WithError(getErr))
		return
	}
	// A create with no API key is a validation error ("API key required").
	if existing == nil && strings.TrimSpace(input.APIKey) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("api_key", "an API key is required when first connecting Immich"))
		return
	}

	ic, saveErr := services.UpsertImmichConfig(db, cfg.JWTSecretKey, userID, *input)
	if saveErr != nil {
		// A malformed base URL is the user's own input, not a database
		// failure — reject it as a 400 with the specific reason
		// (NormalizeImmichBaseURL/abortImmichServiceError), immediately at
		// save time rather than only on first use.
		if errors.Is(saveErr, services.ErrImmichInvalidURL) {
			abortImmichServiceError(c, saveErr)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save Immich config").WithError(saveErr))
		return
	}

	c.JSON(http.StatusOK, services.ImmichConfigResponse{
		BaseURL:        ic.BaseURL,
		HasAPIKey:      ic.HasAPIKey(),
		SyncEnabled:    ic.SyncEnabled,
		LastSyncedAt:   ic.LastSyncedAt,
		LastSyncStatus: ic.LastSyncStatus,
		LastSyncError:  ic.LastSyncError,
	})
}

// DeleteImmichConfig removes the current user's Immich connection. Linked
// ExternalIdentity/ExternalActivity rows are kept.
func DeleteImmichConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if err := services.DeleteImmichConfig(db, userID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Immich config").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Immich config deleted"})
}

// TestImmichConnection diagnoses the current user's saved Immich connection
// (Settings' "Test connection" button): reachability, then API key validity,
// reported as a specific stage/message rather than a generic error. A
// service-level error (no connection configured, unparseable stored URL)
// still goes through abortImmichServiceError; otherwise this always responds
// 200 — a diagnosed failure (ok: false) is a successful response.
func TestImmichConnection(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	result, err := services.TestImmichConnection(db, cfg, userID)
	if err != nil {
		abortImmichServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListImmichPeople browses every person in the user's Immich instance so the
// L1 picker can search by name (immich.go's client does the stable-API
// pagination; this endpoint is the user's own data, so no per-contact scope).
func ListImmichPeople(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	people, err := services.ListImmichPeopleForUser(db, cfg, userID)
	if err != nil {
		abortImmichServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"people": people})
}

// LinkImmichContact links an Immich person to a contact as an
// ExternalIdentity (system: "immich"). Verifies the contact belongs to the
// user, then delegates the link to the service. A duplicate link is a 409.
func LinkImmichContact(c *gin.Context) {
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
		PersonID   string `json:"person_id" validate:"required,min=1,max=255"`
		PersonName string `json:"person_name,omitempty" validate:"max=200"`
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
	if strings.TrimSpace(input.PersonID) == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("person_id", "person_id is required"))
		return
	}

	// Pre-check the natural key so a duplicate link is a clear 409.
	var existing models.ExternalIdentity
	lookupErr := db.Where("system = ? AND external_id = ? AND user_id = ?", services.ExternalSystemImmich, input.PersonID, userID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Immich link").WithDetails("person_id", input.PersonID))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing Immich link").WithError(lookupErr))
		return
	}

	cfg := currentConfig(c)
	identity, err := services.LinkImmichPerson(db, cfg, userID, contactUID, input.PersonID, input.PersonName)
	if err != nil {
		abortImmichServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"external_identity": identity})
}

// UnlinkImmichContact removes a contact's Immich ExternalIdentity (hard
// delete). Its ExternalActivity timeline entries are kept — unlinking the
// person stops new appearances but preserves the history.
func UnlinkImmichContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	var identity models.ExternalIdentity
	if err := db.Where("user_id = ? AND system = ? AND entity_id = ?", userID, services.ExternalSystemImmich, contactUID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Immich link"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Immich link").WithError(err))
		}
		return
	}

	if err := db.Delete(&identity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete Immich link").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Immich link removed"})
}

// GetImmichContactSummary returns a contact's Immich link plus live person
// info (photo count, latest appearance). No link → 200 with null (the contact
// page renders the "link a person" affordance). Upstream failure degrades to
// the cached metadata rather than erroring the page (T16 trap).
func GetImmichContactSummary(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	var identity models.ExternalIdentity
	if err := db.Where("user_id = ? AND system = ? AND entity_id = ?", userID, services.ExternalSystemImmich, contactUID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{"summary": nil})
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve Immich link").WithError(err))
		return
	}

	cfg := currentConfig(c)
	summary := services.FetchImmichPersonSummary(db, cfg, userID, &identity)
	c.JSON(http.StatusOK, gin.H{"summary": summary})
}

// SyncImmichNow triggers the enrichment sync for the current user (the same
// code the scheduler runs, minus the job-lock — a manual trigger is always
// allowed). Failure to reach Immich surfaces as a 503.
func SyncImmichNow(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	if err := services.SyncImmichForUser(db, cfg, userID); err != nil {
		abortImmichServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Immich sync completed"})
}

// GetImmichThumbnail proxies a linked person's thumbnail for the contact
// page, applying the same hardening as ProxyImage: SVG is rejected (an image
// served from the API origin is an XSS vector) and Content-Disposition is set
// so the response is only ever interpreted as an image resource. The fetch
// itself runs with the user's own connection and SSRF policy.
func GetImmichThumbnail(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	cfg := currentConfig(c)
	body, contentType, err := services.FetchImmichThumbnail(db, cfg, userID, contactUID)
	if err != nil {
		abortImmichServiceError(c, err)
		return
	}
	writeImmichImage(c, body, contentType)
}

// ListImmichContactAssets returns a linked contact's recent Immich photos
// (id + occurred_at) for the profile-photo picker's browse-then-pick step
// (Part 3). No link → the same 503 "not configured"-shaped error as every
// other per-contact Immich endpoint.
func ListImmichContactAssets(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	cfg := currentConfig(c)
	assets, err := services.ListImmichRecentAssetsForContact(db, cfg, userID, contactUID)
	if err != nil {
		abortImmichServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"assets": assets})
}

// GetImmichAssetImage proxies one of a linked contact's recent Immich photos
// for the profile-photo picker, applying the same SVG-rejection +
// Content-Disposition hardening as GetImmichThumbnail.
func GetImmichAssetImage(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contactUID := c.Param("vcard_uid")
	assetID := c.Param("asset_id")
	cfg := currentConfig(c)
	body, contentType, err := services.FetchImmichAssetImage(db, cfg, userID, contactUID, assetID)
	if err != nil {
		abortImmichServiceError(c, err)
		return
	}
	writeImmichImage(c, body, contentType)
}

// writeImmichImage is the shared response tail for GetImmichThumbnail and
// GetImmichAssetImage: reject SVG (an image served from the API origin is an
// XSS vector), then write the body with Content-Disposition: inline so it is
// only ever interpreted as an image resource.
func writeImmichImage(c *gin.Context, body []byte, contentType string) {
	lowerContentType := strings.ToLower(contentType)
	if strings.HasPrefix(lowerContentType, "image/svg") {
		logger.FromContext(c).Warn().Str("content_type", contentType).Msg("Rejected SVG Immich image (XSS risk)")
		apperrors.AbortWithError(c, apperrors.ErrValidation("SVG images are not supported by the image proxy"))
		return
	}

	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, contentType, body)
}
