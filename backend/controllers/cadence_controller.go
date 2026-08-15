package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// cadencePolicyWithHealth is the list/read response shape: the policy row
// with its DERIVED health embedded (services.CadenceHealth) — never stored.
type cadencePolicyWithHealth struct {
	models.CadencePolicy
	Health services.CadenceHealth `json:"health"`
}

// cadenceNow returns the "today" boundary used for health derivation, in the
// user's configured reminder timezone so the day boundary matches their
// clock (the same location reminders run in).
func cadenceNow(c *gin.Context) time.Time {
	cfg := currentConfig(c)
	return time.Now().In(cfg.GetReminderLocation())
}

// policyHealth derives health for one policy, embedding it in the response
// shape. A DB failure is a hard error (abort); health itself is never an
// error even when undefined.
func policyHealth(c *gin.Context, db *gorm.DB, userID uint, policy *models.CadencePolicy) (cadencePolicyWithHealth, bool) {
	health, err := services.ComputeCadenceHealth(db, userID, policy, cadenceNow(c))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute cadence health").WithError(err))
		return cadencePolicyWithHealth{}, false
	}
	return cadencePolicyWithHealth{CadencePolicy: *policy, Health: health}, true
}

// CreateCadencePolicy creates a new CadencePolicy (cadence_policy.go)
// for the authenticated user. One policy per contact: an existing non-deleted
// policy for the same entity is a checked 409 ErrAlreadyExists (the partial
// unique index idx_cadence_policies_user_entity is the backstop, not the
// first line of defense).
func CreateCadencePolicy(c *gin.Context) {
	input, err := middleware.GetValidated[models.CadencePolicyInput](c)
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

	var existing models.CadencePolicy
	lookupErr := db.Where("user_id = ? AND entity_id = ?", userID, input.EntityID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Cadence policy"))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing cadence policy").WithError(lookupErr))
		return
	}

	policy := models.CadencePolicy{
		UserID:             userID,
		EntityID:           input.EntityID,
		TargetIntervalDays: input.TargetIntervalDays,
		QualifyingTypes:    input.QualifyingTypes,
	}
	if err := db.Create(&policy).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save cadence policy").WithError(err))
		return
	}

	resp, ok := policyHealth(c, db, userID, &policy)
	if !ok {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Cadence policy created", "cadence_policy": resp})
}

// GetCadencePolicy returns one CadencePolicy with its derived health.
func GetCadencePolicy(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var policy models.CadencePolicy
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Cadence policy").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve cadence policy").WithError(err))
		}
		return
	}

	resp, ok := policyHealth(c, db, userID, &policy)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, resp)
}

// ListCadencePolicies returns the authenticated user's CadencePolicies,
// cursor-paginated (T17), optionally filtered by ?entity_id=<Contact.VCardUID>
// (the contact-page query). Health is embedded per policy. Cadence policies
// soft-delete (user-authored content, T26), so the ?since= change feed
// returns tombstones and the collection is incremental — the same shape as
// Preferences, right down to keeping `total` in browse mode (small enough to
// be cheap here).
func ListCadencePolicies(c *gin.Context) {
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

	var policies []models.CadencePolicy

	if params.Since {
		query := db.Unscoped().Model(&models.CadencePolicy{}).Where("user_id = ?", userID)
		if params.Cursor != nil {
			pred, t, idv := cursorPredicate("cadence_policies", params.Cursor, params.Cursor.ID, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "cadence_policies", false).Limit(params.Limit + 1)
		if err := query.Find(&policies).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve cadence policies").WithError(err))
			return
		}
		nextCursor := ""
		if len(policies) > params.Limit {
			policies = policies[:params.Limit]
			nextCursor = EncodeCursor(policies[len(policies)-1].UpdatedAt, policies[len(policies)-1].ID)
		}
		for i := range policies {
			policies[i].Deleted = policies[i].DeletedAt.Valid
		}
		// Feed mode deliberately omits `total` (it would be the page count,
		// not the collection size — a feed is incremental). Health is
		// omitted too: a replica just needs the policy rows, and deriving
		// health per tombstone is pure waste.
		c.JSON(http.StatusOK, gin.H{
			"cadence_policies": policies,
			"next_cursor":      nextCursor,
			"limit":            params.Limit,
			"sync":             buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	entityID := c.Query("entity_id")

	var total int64
	baseQuery := db.Model(&models.CadencePolicy{}).Where("user_id = ?", userID)
	if entityID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", entityID)
	}
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count cadence policies").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("cadence_policies", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "cadence_policies", desc).
		Limit(params.Limit + 1).
		Find(&policies).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve cadence policies").WithError(err))
		return
	}
	nextCursor := ""
	if len(policies) > params.Limit {
		policies = policies[:params.Limit]
		nextCursor = EncodeCursor(policies[len(policies)-1].UpdatedAt, policies[len(policies)-1].ID)
	}

	response := make([]cadencePolicyWithHealth, 0, len(policies))
	for i := range policies {
		resp, ok := policyHealth(c, db, userID, &policies[i])
		if !ok {
			return
		}
		response = append(response, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"cadence_policies": response,
		"total":            total,
		"next_cursor":      nextCursor,
		"limit":            params.Limit,
		"sync":             buildSyncMeta(SyncModeIncremental),
	})
}

// UpdateCadencePolicy replaces a CadencePolicy's editable fields
// (TargetIntervalDays, QualifyingTypes, and EntityID) — full-replace via the
// same CadencePolicyInput as create, matching UpdateLifeEvent's precedent of
// treating every field in the shared input DTO as replaceable.
func UpdateCadencePolicy(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var policy models.CadencePolicy
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Cadence policy").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve cadence policy").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.CadencePolicyInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	// Updating an entity to one that already has a policy would collide on
	// the partial unique index; surface it as a clear 409, matching create.
	if input.EntityID != policy.EntityID {
		var existing models.CadencePolicy
		lookupErr := db.Where("user_id = ? AND entity_id = ?", userID, input.EntityID).First(&existing).Error
		if lookupErr == nil {
			apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Cadence policy"))
			return
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing cadence policy").WithError(lookupErr))
			return
		}
	}

	policy.EntityID = input.EntityID
	policy.TargetIntervalDays = input.TargetIntervalDays
	policy.QualifyingTypes = input.QualifyingTypes
	if err := db.Save(&policy).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save cadence policy").WithError(err))
		return
	}

	resp, ok := policyHealth(c, db, userID, &policy)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DeleteCadencePolicy soft-deletes a CadencePolicy (user-authored content,
// T26). A soft-deleted policy no longer participates in health derivation or
// overdue lists, and the partial unique index lets a new one be created for
// the same contact.
func DeleteCadencePolicy(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var policy models.CadencePolicy
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Cadence policy").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve cadence policy").WithError(err))
		}
		return
	}

	if err := db.Delete(&policy).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete cadence policy").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Cadence policy deleted"})
}

// GetOverdueCadences returns the user's currently-overdue cadence policies,
// most-overdue first, joined with each contact's numeric ID and display name
// so the frontend can link straight to /contacts/<id>. Health is derived per
// policy; a policy with no qualifying interaction ever can never be overdue.
// This is the data behind the overdue screen ("who have I not talked to in
// too long").
func GetOverdueCadences(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	overdue, err := services.ListOverdueCadences(db, userID, cadenceNow(c))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute overdue cadences").WithError(err))
		return
	}
	if overdue == nil {
		overdue = []services.OverdueCadence{}
	}

	c.JSON(http.StatusOK, gin.H{"overdue": overdue})
}
