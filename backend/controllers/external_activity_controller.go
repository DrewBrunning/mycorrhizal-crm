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

// CreateExternalActivity creates a new ExternalActivity (external_activity.go)
// for the authenticated user — the L2 "enrichment" write of the generic
// integration substrate: an event that happened in an
// external system, linkable into the contact's timeline. Same ownership check
// + natural-key 409 as CreateExternalIdentity.
func CreateExternalActivity(c *gin.Context) {
	input, err := middleware.GetValidated[models.ExternalActivityInput](c)
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

	var existing models.ExternalActivity
	lookupErr := db.Where("source_system = ? AND external_id = ? AND user_id = ?", input.SourceSystem, input.ExternalID, userID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("External activity").WithDetails("source_system", input.SourceSystem).WithDetails("external_id", input.ExternalID))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing external activity").WithError(lookupErr))
		return
	}

	provenance := input.Provenance
	if provenance == "" {
		provenance = models.ExternalActivityProvenanceExternal
	}
	syncState := input.SyncState
	if syncState == "" {
		syncState = models.ExternalActivitySyncStateSynced
	}

	activity := models.ExternalActivity{
		UserID:       userID,
		EntityID:     input.EntityID,
		SourceSystem: input.SourceSystem,
		ExternalID:   input.ExternalID,
		Type:         input.Type,
		OccurredAt:   input.OccurredAt,
		Payload:      input.Payload,
		Provenance:   provenance,
		SyncState:    syncState,
	}
	if err := db.Create(&activity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save external activity").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "External activity created successfully", "external_activity": activity})
}

// GetExternalActivity returns one ExternalActivity.
func GetExternalActivity(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var activity models.ExternalActivity
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&activity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("External activity").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external activity").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, activity)
}

// ListExternalActivities returns the authenticated user's ExternalActivities,
// cursor-paginated, optionally filtered by ?contact_id=<Contact.VCardUID> and
// ?system=<integration>. The contact_id filter is what feeds an
// ExternalActivity into a contact's timeline (T14's "attach into the
// timeline" capability): the timeline read-model is composed in the frontend
// from this response plus notes/activities/life-events. Hard-deleted +
// bounded, so this is a full_resync collection (`total` kept).
func ListExternalActivities(c *gin.Context) {
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

	baseQuery := db.Model(&models.ExternalActivity{}).Where("user_id = ?", userID)
	if contactID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", contactID)
	}
	if system != "" {
		baseQuery = baseQuery.Where("source_system = ?", system)
	}

	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count external activities").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("external_activities", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	var activities []models.ExternalActivity
	if err := cursorOrderBy(baseQuery, "external_activities", desc).
		Limit(params.Limit + 1).
		Find(&activities).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external activities").WithError(err))
		return
	}
	nextCursor := ""
	if len(activities) > params.Limit {
		activities = activities[:params.Limit]
		nextCursor = EncodeCursor(activities[len(activities)-1].UpdatedAt, activities[len(activities)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"external_activities": activities,
		"total":               total,
		"next_cursor":         nextCursor,
		"limit":               params.Limit,
		"sync":                buildSyncMeta(SyncModeFullResync),
	})
}

// UpdateExternalActivity replaces an ExternalActivity's editable fields
// (EntityID/SourceSystem/ExternalID/Type/OccurredAt/Payload/Provenance/
// SyncState) — full-replace semantics, natural-key collision-checked.
func UpdateExternalActivity(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var activity models.ExternalActivity
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&activity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("External activity").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external activity").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.ExternalActivityInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	if input.SourceSystem != activity.SourceSystem || input.ExternalID != activity.ExternalID {
		var existing models.ExternalActivity
		lookupErr := db.Where("source_system = ? AND external_id = ? AND user_id = ? AND id != ?", input.SourceSystem, input.ExternalID, userID, activity.ID).First(&existing).Error
		if lookupErr == nil {
			apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("External activity").WithDetails("source_system", input.SourceSystem).WithDetails("external_id", input.ExternalID))
			return
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing external activity").WithError(lookupErr))
			return
		}
	}

	provenance := input.Provenance
	if provenance == "" {
		provenance = activity.Provenance
	}
	syncState := input.SyncState
	if syncState == "" {
		syncState = activity.SyncState
	}

	activity.EntityID = input.EntityID
	activity.SourceSystem = input.SourceSystem
	activity.ExternalID = input.ExternalID
	activity.Type = input.Type
	activity.OccurredAt = input.OccurredAt
	activity.Payload = input.Payload
	activity.Provenance = provenance
	activity.SyncState = syncState

	if err := db.Save(&activity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save external activity").WithError(err))
		return
	}

	c.JSON(http.StatusOK, activity)
}

// DeleteExternalActivity deletes an ExternalActivity (hard delete — the event
// is edge/join-shaped, so a re-sync can recreate it without a unique-index
// ghost).
func DeleteExternalActivity(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var activity models.ExternalActivity
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&activity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("External activity").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve external activity").WithError(err))
		}
		return
	}

	if err := db.Delete(&activity).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete external activity").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "External activity deleted"})
}
