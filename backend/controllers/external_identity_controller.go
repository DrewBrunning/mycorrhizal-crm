package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateExternalIdentity creates a new ExternalIdentity (external_identity.go)
// for the authenticated user — the L1 "linking" write of the generic
// integration substrate (§91.12, ticket T14). Verifies the target contact
// belongs to the user first, then checks the (system, external_id, user_id)
// natural key so a duplicate link is a clear 409, not a sniffed constraint
// error (the CircleMember precedent). Hard delete + unique key = a re-sync
// upserts on this pair, never duplicates.
func CreateExternalIdentity(c *gin.Context) {
	input, err := middleware.GetValidated[models.ExternalIdentityInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	var existing models.ExternalIdentity
	lookupErr := db.Where("system = ? AND external_id = ? AND user_id = ?", input.System, input.ExternalID, userID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("External identity").WithDetails("system", input.System).WithDetails("external_id", input.ExternalID))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing external identity").WithError(lookupErr))
		return
	}

	syncStatus := input.SyncStatus
	if syncStatus == "" {
		syncStatus = models.ExternalIdentitySyncStatusIdle
	}

	identity := models.ExternalIdentity{
		UserID:     userID,
		EntityID:   input.EntityID,
		System:     input.System,
		ExternalID: input.ExternalID,
		URL:        input.URL,
		Metadata:   input.Metadata,
		SyncStatus: syncStatus,
	}
	if err := db.Create(&identity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save external identity").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "External identity created successfully", "external_identity": identity})
}

// GetExternalIdentity returns one ExternalIdentity.
func GetExternalIdentity(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var identity models.ExternalIdentity
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("External identity").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external identity").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, identity)
}

// ListExternalIdentities returns the authenticated user's ExternalIdentities,
// cursor-paginated, optionally filtered by ?contact_id=<Contact.VCardUID> and
// ?system=<integration>. Hard-deleted + bounded, so this is a full_resync
// collection (`total` is kept) — a client re-pulls them wholesale.
func ListExternalIdentities(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	params, err := GetCursorParams(c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	contactID := c.Query("contact_id")
	system := c.Query("system")

	baseQuery := db.Model(&models.ExternalIdentity{}).Where("user_id = ?", userID)
	if contactID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", contactID)
	}
	if system != "" {
		baseQuery = baseQuery.Where("system = ?", system)
	}

	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count external identities").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("external_identities", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	var identities []models.ExternalIdentity
	if err := cursorOrderBy(baseQuery, "external_identities", desc).
		Limit(params.Limit + 1).
		Find(&identities).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external identities").WithError(err))
		return
	}
	nextCursor := ""
	if len(identities) > params.Limit {
		identities = identities[:params.Limit]
		nextCursor = EncodeCursor(identities[len(identities)-1].UpdatedAt, identities[len(identities)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"external_identities": identities,
		"total":               total,
		"next_cursor":         nextCursor,
		"limit":               params.Limit,
		"sync":                buildSyncMeta(SyncModeFullResync),
	})
}

// UpdateExternalIdentity replaces an ExternalIdentity's editable fields
// (EntityID/System/ExternalID/URL/Metadata/SyncStatus) — full-replace
// semantics, matching UpdateLifeEvent's precedent. The natural key change is
// allowed (it is what a re-link after an upstream ID change needs) but is
// still uniqueness-checked so it cannot collide with another link.
func UpdateExternalIdentity(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var identity models.ExternalIdentity
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("External identity").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external identity").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.ExternalIdentityInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	if input.System != identity.System || input.ExternalID != identity.ExternalID {
		var existing models.ExternalIdentity
		lookupErr := db.Where("system = ? AND external_id = ? AND user_id = ? AND id != ?", input.System, input.ExternalID, userID, identity.ID).First(&existing).Error
		if lookupErr == nil {
			apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("External identity").WithDetails("system", input.System).WithDetails("external_id", input.ExternalID))
			return
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing external identity").WithError(lookupErr))
			return
		}
	}

	syncStatus := input.SyncStatus
	if syncStatus == "" {
		syncStatus = identity.SyncStatus
	}

	identity.EntityID = input.EntityID
	identity.System = input.System
	identity.ExternalID = input.ExternalID
	identity.URL = input.URL
	identity.Metadata = input.Metadata
	identity.SyncStatus = syncStatus

	if err := db.Save(&identity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save external identity").WithError(err))
		return
	}

	c.JSON(http.StatusOK, identity)
}

// DeleteExternalIdentity deletes an ExternalIdentity (hard delete — the link
// is edge/join-shaped, so a re-link must be possible without a unique-index
// ghost).
func DeleteExternalIdentity(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var identity models.ExternalIdentity
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("External identity").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external identity").WithError(err))
		}
		return
	}

	if err := db.Delete(&identity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete external identity").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "External identity deleted"})
}
