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

// CreatePreference creates a new Preference (preference.go) for the
// authenticated user, scoped to a Contact they own via EntityID.
func CreatePreference(c *gin.Context) {
	input, err := middleware.GetValidated[models.PreferenceInput](c)
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

	pref := models.Preference{
		UserID:        userID,
		EntityID:      input.EntityID,
		Category:      input.Category,
		Key:           input.Key,
		Value:         input.Value,
		Notes:         input.Notes,
		Source:        input.Source,
		Confidence:    input.Confidence,
		LastConfirmed: input.LastConfirmed,
	}
	if input.Sensitivity == "" {
		pref.Sensitivity = models.RelationshipSensitivityNormal
	} else {
		pref.Sensitivity = input.Sensitivity
	}

	if err := db.Create(&pref).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save preference").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Preference created successfully", "preference": pref})
}

// GetPreference returns one Preference.
func GetPreference(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var pref models.Preference
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&pref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Preference").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve preference").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, pref)
}

// ListPreferences returns the authenticated user's Preferences, cursor-
// paginated (T17), optionally filtered by ?entity_id=<Contact.VCardUID> in
// browse mode. Preferences soft-delete (user-authored content, T20a), so the
// ?since= change feed returns tombstones and the collection is incremental.
// Unlike the unbounded timeline collections, preferences are small enough
// that `total` stays — it is cheap here and the Preferences UI reads it.
func ListPreferences(c *gin.Context) {
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
	if err := CheckFeedCursorAge(c, params); err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	var prefs []models.Preference

	if params.Since {
		query := db.Unscoped().Model(&models.Preference{}).Where("user_id = ?", userID)
		if params.Cursor != nil {
			pred, t, idv := cursorPredicate("preferences", params.Cursor, params.Cursor.ID, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "preferences", false).Limit(params.Limit + 1)
		if err := query.Find(&prefs).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve preferences").WithError(err))
			return
		}
		nextCursor := ""
		if len(prefs) > params.Limit {
			prefs = prefs[:params.Limit]
			nextCursor = EncodeCursor(prefs[len(prefs)-1].UpdatedAt, prefs[len(prefs)-1].ID)
		}
		for i := range prefs {
			prefs[i].Deleted = prefs[i].DeletedAt.Valid
		}
		// Feed mode deliberately omits `total` (it would be the page count,
		// not the collection size — a feed is incremental).
		c.JSON(http.StatusOK, gin.H{
			"preferences": prefs,
			"next_cursor": nextCursor,
			"limit":       params.Limit,
			"sync":        buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	entityID := c.Query("entity_id")

	var total int64

	baseQuery := db.Model(&models.Preference{}).Where("user_id = ?", userID)
	if entityID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", entityID)
	}

	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count preferences").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("preferences", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "preferences", desc).
		Limit(params.Limit + 1).
		Find(&prefs).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve preferences").WithError(err))
		return
	}
	nextCursor := ""
	if len(prefs) > params.Limit {
		prefs = prefs[:params.Limit]
		nextCursor = EncodeCursor(prefs[len(prefs)-1].UpdatedAt, prefs[len(prefs)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"preferences": prefs,
		"next_cursor": nextCursor,
		"limit":       params.Limit,
		"total":       total,
		"sync":        buildSyncMeta(SyncModeIncremental),
	})
}

// UpdatePreference updates a Preference — full-replace semantics via the same
// PreferenceInput as create, matching UpdateLifeEvent's own precedent.
func UpdatePreference(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var pref models.Preference
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&pref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Preference").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve preference").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.PreferenceInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	pref.EntityID = input.EntityID
	pref.Category = input.Category
	pref.Key = input.Key
	pref.Value = input.Value
	pref.Notes = input.Notes
	pref.Source = input.Source
	pref.Confidence = input.Confidence
	pref.LastConfirmed = input.LastConfirmed
	if input.Sensitivity == "" {
		pref.Sensitivity = models.RelationshipSensitivityNormal
	} else {
		pref.Sensitivity = input.Sensitivity
	}

	if err := db.Save(&pref).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save preference").WithError(err))
		return
	}

	c.JSON(http.StatusOK, pref)
}

// DeletePreference soft-deletes a Preference (user-authored content).
func DeletePreference(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var pref models.Preference
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&pref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Preference").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve preference").WithError(err))
		}
		return
	}

	if err := db.Delete(&pref).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete preference").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Preference deleted"})
}
