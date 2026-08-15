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

// This file is T6: the
// CRUD routes that make the already-built FieldDefinition/FieldValue model
// reachable. Endpoint shape follows circle_controller.go's idiom (the newer
// controllers use), ownership is by user_id everywhere, and every FieldValue
// write is validated against its definition by services.ValidateFieldValue.
//
// Endpoint-shape decision (documented per the ticket's instruction to pick
// one and say why): FieldValues live NESTED under the contact --
// GET/PUT /contacts/:id/field-values -- rather than flat
// /field-values?entity_id=. Two reasons: (1) values are inherently
// per-contact, and /contacts/:id/<subresource> is the established idiom this
// codebase already uses for notes/activities/reminders, so a client editing
// one contact needs exactly one consistent address; (2) the frontend's
// add-contact flow creates the contact first and then sets values, which a
// contact-scoped resource expresses naturally. The flat /field-values list
// is not needed today -- there is no cross-contact field-values screen, and
// adding it later is trivial (life_event_controller.go's ?entity_id= filter
// is the template).
//
// Sensitivity on the internal API: ALL sensitivities are returned (and
// accepted) here, because this is the owner's own CRM surface -- the same
// stance export_controller.go documents for the full-data CSV backup. The
// "exclude non-normal from anything that leaves the instance" rule is
// enforced where it actually matters, in the projection path: projectCustomFields
// (models/contact_record.go) filters sensitivity='normal' in the query for
// vCard/JSContact export. Filtering here too would make private/secret
// custom fields impossible to view or edit at all, defeating their purpose.

// CreateFieldDefinition creates a new FieldDefinition (field_definition.go)
// for the authenticated user.
func CreateFieldDefinition(c *gin.Context) {
	input, err := middleware.GetValidated[models.FieldDefinitionInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// Check-then-create, matching AddCircleMember's idiom (circle_controller.go):
	// a duplicate (user_id, key) is a clear checked 409, not a sniffed
	// constraint error, so the client gets the right status and a precise
	// detail instead of a database-error string.
	var existing models.FieldDefinition
	lookupErr := db.Where("user_id = ? AND key = ?", userID, input.Key).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Field definition").WithDetails("key", input.Key))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing field definition").WithError(lookupErr))
		return
	}

	def := models.FieldDefinition{
		UserID:      userID,
		Label:       input.Label,
		Key:         input.Key,
		Target:      orDefault(input.Target, models.FieldDefinitionTargetContact),
		Type:        input.Type,
		Constraints: input.Constraints,
		Projection:  orDefault(input.Projection, "internal-only"),
		Sensitivity: orDefault(input.Sensitivity, models.RelationshipSensitivityNormal),
	}
	if err := db.Create(&def).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save field definition").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Field definition created successfully", "field_definition": def})
}

// GetFieldDefinition returns one FieldDefinition owned by the authenticated user.
func GetFieldDefinition(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var def models.FieldDefinition
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&def).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Field definition").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field definition").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, def)
}

// ListFieldDefinitions returns the authenticated user's FieldDefinitions,
// paginated, ordered by label.
func ListFieldDefinitions(c *gin.Context) {
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

	var defs []models.FieldDefinition
	var total int64

	baseQuery := db.Model(&models.FieldDefinition{}).Where("user_id = ?", userID)
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count field definitions").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("field_definitions", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "field_definitions", desc).
		Limit(params.Limit + 1).
		Find(&defs).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field definitions").WithError(err))
		return
	}
	nextCursor := ""
	if len(defs) > params.Limit {
		defs = defs[:params.Limit]
		nextCursor = EncodeCursor(defs[len(defs)-1].UpdatedAt, defs[len(defs)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"field_definitions": defs,
		"total":             total,
		"next_cursor":       nextCursor,
		"limit":             params.Limit,
		"sync":              buildSyncMeta(SyncModeFullResync),
	})
}

// UpdateFieldDefinition replaces a FieldDefinition's editable fields. Key is
// deliberately immutable: it is the stable machine name ("the map key /
// API field name"), so changing it would silently orphan every FieldValue's
// key-based identity and break the "upgrade type/constraints in place, never
// rename" migration story. Label is editable (it is display), as are
// Type/Constraints/Projection/Sensitivity -- a user can upgrade a migrated
// definition's type/constraints/projection in place exactly promises.
func UpdateFieldDefinition(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var def models.FieldDefinition
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&def).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Field definition").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field definition").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.FieldDefinitionInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	def.Label = input.Label
	def.Target = orDefault(input.Target, def.Target)
	def.Type = input.Type
	def.Constraints = input.Constraints
	if input.Projection != "" {
		def.Projection = input.Projection
	}
	if input.Sensitivity != "" {
		def.Sensitivity = input.Sensitivity
	}
	// Key is intentionally not assigned -- see the doc comment above.

	if err := db.Save(&def).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save field definition").WithError(err))
		return
	}

	c.JSON(http.StatusOK, def)
}

// DeleteFieldDefinition deletes a FieldDefinition. Its FieldValues cascade at
// the DB level (migrations/000033's field_values.field_definition_id ON
// DELETE CASCADE) -- values are join-shaped rows keyed by their definition,
// so deleting the schema half removes the data half with it, rather than
// blocking the delete or orphaning values.
func DeleteFieldDefinition(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var def models.FieldDefinition
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&def).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Field definition").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field definition").WithError(err))
		}
		return
	}

	if err := db.Delete(&def).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete field definition").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Field definition deleted"})
}

// ListContactFieldValues returns every FieldValue on the contact referenced
// by:id, owned by the authenticated user. The:id is the contact's numeric
// ID (the same path parameter every /contacts/:id/<subresource> route uses);
// values are stored against the contact's VCardUID (the graph invariant, see
// FieldValue.EntityID's doc comment), resolved here. All sensitivities are
// returned -- this is the owner's own CRM surface, see the file doc comment.
func ListContactFieldValues(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contact, ok := resolveOwnedContactByID(c, db, userID, c.Param("id"))
	if !ok {
		return
	}

	var values []models.FieldValue
	if err := db.Where("entity_id = ? AND user_id = ?", contact.VCardUID, userID).Find(&values).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field values").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"field_values": values})
}

// ReplaceContactFieldValues is the write side of the nested field-values
// surface: full-replace of the contact's FieldValues for the authenticated
// user's definitions. The request body carries the complete desired set; any
// existing value on this contact for a definition NOT in the payload is
// deleted, matching the "CardDAV/REST writes are full-overwrite by design"
// convention this codebase already pins for contact writes (and the merge
// service's FieldValue handling, contact_merge_service.go). Runs in a
// transaction so the delete/upsert set commits atomically.
//
// Every value is validated against its definition via
// services.ValidateFieldValue before any write: a type mismatch (e.g. a
// number value for a string field, an element outside an enum's allowed
// values) is a 400 with the definition's key and the specific failure, never
// a 500. A FieldDefinitionID that is not one of the user's own definitions is
// rejected as invalid input (never silently accepted -- ownership scoping,
// the same rule every handler in this codebase follows).
func ReplaceContactFieldValues(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contact, ok := resolveOwnedContactByID(c, db, userID, c.Param("id"))
	if !ok {
		return
	}

	input, err := middleware.GetValidated[models.ContactFieldValuesInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	// Load the user's definitions once, keyed by ID, so ownership checking
	// and value validation share a single lookup instead of N+1 queries.
	var defs []models.FieldDefinition
	if err := db.Where("user_id = ?", userID).Find(&defs).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field definitions").WithError(err))
		return
	}
	defByID := make(map[string]models.FieldDefinition, len(defs))
	for _, d := range defs {
		defByID[d.ID] = d
	}

	incoming := make([]models.FieldValueInput, len(input.FieldValues))
	for i, fv := range input.FieldValues {
		def, ok := defByID[fv.FieldDefinitionID]
		if !ok {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("field_definition_id", "unknown or not-owned field definition"))
			return
		}
		if err := services.ValidateFieldValue(def, fv.Value); err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("value", err.Error()))
			return
		}
		incoming[i] = fv
	}

	txErr := db.Transaction(func(tx *gorm.DB) error {
		// Remove values whose definition is no longer in the payload. When
		// the payload is empty, every existing value is out-of-set, so
		// delete all values for the contact — the explicit WHERE catches
		// this case rather than relying on NOT IN () semantics, which GORM
		// does not support (it replaces empty slices with NULL).
		if len(incoming) == 0 {
			if err := tx.Where(
				"entity_id = ? AND user_id = ?",
				contact.VCardUID, userID,
			).Delete(&models.FieldValue{}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Where(
				"entity_id = ? AND user_id = ? AND field_definition_id NOT IN ?",
				contact.VCardUID, userID,
				incomingDefIDs(incoming),
			).Delete(&models.FieldValue{}).Error; err != nil {
				return err
			}
		}

		for _, fv := range incoming {
			var existing models.FieldValue
			err := tx.Where(
				"field_definition_id = ? AND entity_id = ?",
				fv.FieldDefinitionID, contact.VCardUID,
			).First(&existing).Error
			switch {
			case err == nil:
				existing.Value = fv.Value
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				if err := tx.Create(&models.FieldValue{
					FieldDefinitionID: fv.FieldDefinitionID,
					UserID:            userID,
					EntityID:          contact.VCardUID,
					Value:             fv.Value,
				}).Error; err != nil {
					return err
				}
			default:
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save field values").WithError(txErr))
		return
	}

	var values []models.FieldValue
	if err := db.Where("entity_id = ? AND user_id = ?", contact.VCardUID, userID).Find(&values).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field values").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Field values saved successfully", "field_values": values})
}

// incomingDefIDs returns the definition IDs in a payload set; used by
// ReplaceContactFieldValues' delete-absent clause. Returns a non-nil empty
// slice so the NOT IN (...) query still parses when the payload is empty.
func incomingDefIDs(incoming []models.FieldValueInput) []string {
	ids := make([]string, len(incoming))
	for i, fv := range incoming {
		ids[i] = fv.FieldDefinitionID
	}
	return ids
}

// resolveOwnedContactByID loads the Contact with the given numeric ID owned
// by userID, aborting with 404/500 as appropriate. Returns false when it
// aborted. This is the same contact-resolution idiom CreateNote uses
// (note_controller.go) for /contacts/:id/<subresource> routes.
func resolveOwnedContactByID(c *gin.Context, db *gorm.DB, userID uint, id string) (*models.Contact, bool) {
	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return nil, false
	}
	return &contact, true
}

// orDefault returns def when s is empty, else s -- the tiny helper the
// defaults-on-omitted DTO fields above use. Kept local to this controller
// until a second caller appears.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
