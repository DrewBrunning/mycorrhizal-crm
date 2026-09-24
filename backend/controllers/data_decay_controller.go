package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// dataDecayPolicyWithHealth is the list/read response shape: the policy row
// with its DERIVED health embedded (services.DataDecayHealth) — never
// stored. Mirrors cadencePolicyWithHealth.
type dataDecayPolicyWithHealth struct {
	models.DataDecayPolicy
	Health services.DataDecayHealth `json:"health"`
}

// dataDecayHealthResponse embeds a policy's derived health, computed
// against `now` (the user's configured reminder timezone — the same "today"
// boundary cadenceNow uses, reused directly since both mean the same thing:
// "midnight in the user's own clock").
func dataDecayHealthResponse(c *gin.Context, policy *models.DataDecayPolicy) dataDecayPolicyWithHealth {
	health := services.ComputeDataDecayHealth(policy, reminderNow(c))
	return dataDecayPolicyWithHealth{DataDecayPolicy: *policy, Health: health}
}

// applyDataDecayInput copies the editable fields from a validated input onto
// a policy, defaulting Active to true when omitted (the JSON zero value for
// *bool is nil, not false, so this can't silently flip an existing policy to
// paused on a partial-looking update).
func applyDataDecayInput(policy *models.DataDecayPolicy, input *models.DataDecayPolicyInput) {
	policy.EntityID = input.EntityID
	policy.IntervalDays = input.IntervalDays
	if input.Active != nil {
		policy.Active = *input.Active
	} else {
		policy.Active = true
	}
}

// CreateDataDecayPolicy creates a new DataDecayPolicy (data_decay_policy.go,
// issue #352) for the authenticated user. One policy per contact: an
// existing non-deleted policy for the same entity is a checked 409
// ErrAlreadyExists (the partial unique index
// idx_data_decay_policies_user_entity is the backstop, not the first line of
// defense) — mirrors CreateCadencePolicy.
func CreateDataDecayPolicy(c *gin.Context) {
	input, err := middleware.GetValidated[models.DataDecayPolicyInput](c)
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

	var existing models.DataDecayPolicy
	lookupErr := db.Where("user_id = ? AND entity_id = ?", userID, input.EntityID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Data decay policy"))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing data decay policy").WithError(lookupErr))
		return
	}

	policy := models.DataDecayPolicy{UserID: userID}
	applyDataDecayInput(&policy, input)
	if err := db.Create(&policy).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save data decay policy").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Data decay policy created", "data_decay_policy": dataDecayHealthResponse(c, &policy)})
}

// GetDataDecayPolicy returns one DataDecayPolicy with its derived health.
func GetDataDecayPolicy(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var policy models.DataDecayPolicy
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Data decay policy").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve data decay policy").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, dataDecayHealthResponse(c, &policy))
}

// ListDataDecayPolicies returns the authenticated user's DataDecayPolicies,
// cursor-paginated (T17), optionally filtered by
// ?entity_id=<Contact.VCardUID> (the contact-page query). Health is embedded
// per policy. Mirrors ListCadencePolicies field-for-field, including its
// ?since= incremental-feed mode (data-decay policies soft-delete, T26, so
// tombstones are meaningful).
func ListDataDecayPolicies(c *gin.Context) {
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

	var policies []models.DataDecayPolicy

	if params.Since {
		query := db.Unscoped().Model(&models.DataDecayPolicy{}).Where("user_id = ?", userID)
		if params.Cursor != nil {
			pred, t, idv := cursorPredicate("data_decay_policies", params.Cursor, params.Cursor.ID, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "data_decay_policies", false).Limit(params.Limit + 1)
		if err := query.Find(&policies).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve data decay policies").WithError(err))
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
		c.JSON(http.StatusOK, gin.H{
			"data_decay_policies": policies,
			"next_cursor":         nextCursor,
			"limit":               params.Limit,
			"sync":                buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	entityID := c.Query("entity_id")

	var total int64
	baseQuery := db.Model(&models.DataDecayPolicy{}).Where("user_id = ?", userID)
	if entityID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", entityID)
	}
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count data decay policies").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("data_decay_policies", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "data_decay_policies", desc).
		Limit(params.Limit + 1).
		Find(&policies).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve data decay policies").WithError(err))
		return
	}
	nextCursor := ""
	if len(policies) > params.Limit {
		policies = policies[:params.Limit]
		nextCursor = EncodeCursor(policies[len(policies)-1].UpdatedAt, policies[len(policies)-1].ID)
	}

	response := make([]dataDecayPolicyWithHealth, 0, len(policies))
	for i := range policies {
		response = append(response, dataDecayHealthResponse(c, &policies[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data_decay_policies": response,
		"total":               total,
		"next_cursor":         nextCursor,
		"limit":               params.Limit,
		"sync":                buildSyncMeta(SyncModeIncremental),
	})
}

// UpdateDataDecayPolicy replaces a DataDecayPolicy's editable fields
// (EntityID, IntervalDays, Active) — full-replace via the same
// DataDecayPolicyInput as create, mirroring UpdateCadencePolicy. Does NOT
// touch LastVerifiedAt; that is exclusively VerifyDataDecayPolicy's job.
func UpdateDataDecayPolicy(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var policy models.DataDecayPolicy
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Data decay policy").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve data decay policy").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.DataDecayPolicyInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	// Updating to an entity that already has a policy would collide on the
	// partial unique index; surface it as a clear 409, matching create.
	if input.EntityID != policy.EntityID {
		var existing models.DataDecayPolicy
		lookupErr := db.Where("user_id = ? AND entity_id = ?", userID, input.EntityID).First(&existing).Error
		if lookupErr == nil {
			apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Data decay policy"))
			return
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing data decay policy").WithError(lookupErr))
			return
		}
	}

	applyDataDecayInput(&policy, input)
	if err := db.Save(&policy).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save data decay policy").WithError(err))
		return
	}

	c.JSON(http.StatusOK, dataDecayHealthResponse(c, &policy))
}

// VerifyDataDecayPolicy is the "confirm still current" action — the one
// write path a user actually clicks. Stamps LastVerifiedAt = now, which
// resets the derived health's baseline (services.ComputeDataDecayHealth).
func VerifyDataDecayPolicy(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var policy models.DataDecayPolicy
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Data decay policy").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve data decay policy").WithError(err))
		}
		return
	}

	now := reminderNow(c)
	if err := db.Model(&policy).Update("last_verified_at", now).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to verify data decay policy").WithError(err))
		return
	}
	policy.LastVerifiedAt = &now

	c.JSON(http.StatusOK, dataDecayHealthResponse(c, &policy))
}

// DeleteDataDecayPolicy soft-deletes a DataDecayPolicy (user-authored
// content, T26). A soft-deleted policy no longer participates in health
// derivation or overdue lists, and the partial unique index lets a new one
// be created for the same contact. Mirrors DeleteCadencePolicy.
func DeleteDataDecayPolicy(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var policy models.DataDecayPolicy
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Data decay policy").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve data decay policy").WithError(err))
		}
		return
	}

	if err := db.Delete(&policy).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete data decay policy").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Data decay policy deleted"})
}

// GetOverdueDataDecayPolicies returns the user's currently-overdue
// data-decay policies, most-overdue first, joined with each contact's
// numeric ID and display name so the frontend can link straight to
// /contacts/<id>. This is the data behind the "info needs a check" screen.
// Mirrors GetOverdueCadences.
func GetOverdueDataDecayPolicies(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	overdue, err := services.ListOverdueDataDecayPolicies(db, userID, reminderNow(c))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute overdue data decay policies").WithError(err))
		return
	}
	if overdue == nil {
		overdue = []services.OverdueDataDecayPolicy{}
	}

	c.JSON(http.StatusOK, gin.H{"overdue": overdue})
}
