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

// ListLinkFieldTypes returns the authenticated user's LinkFieldTypes (T34),
// lazily seeding LinkFieldTypeDefaults on the user's first call. No
// pagination — like ListWebhooks, this is a small Settings-owned registry
// (tens of rows at most), not a change-feed graph entity like Circle.
func ListLinkFieldTypes(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if err := seedLinkFieldTypesIfEmpty(db, userID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to seed default link field types").WithError(err))
		return
	}

	var types []models.LinkFieldType
	if err := db.Where("user_id = ?", userID).Order("category, position, name").Find(&types).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve link field types").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"link_field_types": types})
}

// seedLinkFieldTypesIfEmpty creates the user's default LinkFieldTypes the
// first time they have none (there is no existing "seed per-user defaults
// on first fetch" pattern elsewhere in this repo to copy — this is a fresh
// design for T34). Best-effort under a concurrent race: two simultaneous
// first-fetches could both see count==0 and both try to insert; GORM wraps
// the batch Create in an implicit transaction (CLAUDE.md trap #9), so the
// loser's insert fails atomically on the partial unique index rather than
// leaving a half-seeded set. That failure is treated as "someone else
// already seeded this user," not a hard error — recheck the count and
// proceed if it's now non-zero.
func seedLinkFieldTypesIfEmpty(db *gorm.DB, userID uint) error {
	var count int64
	if err := db.Model(&models.LinkFieldType{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	seed := make([]models.LinkFieldType, len(models.LinkFieldTypeDefaults))
	for i, d := range models.LinkFieldTypeDefaults {
		d.UserID = userID
		d.IsDefault = true
		seed[i] = d
	}
	if err := db.Create(&seed).Error; err != nil {
		var recount int64
		if countErr := db.Model(&models.LinkFieldType{}).Where("user_id = ?", userID).Count(&recount).Error; countErr == nil && recount > 0 {
			return nil
		}
		return err
	}
	return nil
}

// nextLinkFieldTypePosition returns one past the user's current highest
// Position, for a create call that doesn't specify one explicitly.
func nextLinkFieldTypePosition(db *gorm.DB, userID uint) (int, error) {
	var result struct {
		MaxPosition *int
	}
	if err := db.Model(&models.LinkFieldType{}).Where("user_id = ?", userID).
		Select("MAX(position) as max_position").Scan(&result).Error; err != nil {
		return 0, err
	}
	if result.MaxPosition == nil {
		return 0, nil
	}
	return *result.MaxPosition + 1, nil
}

// GetLinkFieldType returns one LinkFieldType.
func GetLinkFieldType(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var linkType models.LinkFieldType
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&linkType).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Link field type").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve link field type").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, linkType)
}

// CreateLinkFieldType creates a new LinkFieldType for the authenticated
// user. A duplicate active Name is a checked 409 ErrAlreadyExists (not a
// sniffed constraint error, matching Circle/CadencePolicy's convention) —
// the partial unique index is the backstop, not the first line of defense.
// IsDefault is always forced false here; only seedLinkFieldTypesIfEmpty
// creates IsDefault:true rows.
func CreateLinkFieldType(c *gin.Context) {
	input, err := middleware.GetValidated[models.LinkFieldTypeInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var existing models.LinkFieldType
	lookupErr := db.Where("user_id = ? AND name = ?", userID, input.Name).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Link field type"))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing link field type").WithError(lookupErr))
		return
	}

	position := 0
	if input.Position != nil {
		position = *input.Position
	} else {
		next, err := nextLinkFieldTypePosition(db, userID)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute link field type position").WithError(err))
			return
		}
		position = next
	}

	linkType := models.LinkFieldType{
		UserID:    userID,
		Name:      input.Name,
		Protocol:  input.Protocol,
		Category:  input.Category,
		Icon:      input.Icon,
		IsDefault: false,
		Position:  position,
	}
	if err := db.Create(&linkType).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save link field type").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Link field type created", "link_field_type": linkType})
}

// UpdateLinkFieldType replaces a LinkFieldType's editable fields
// (Name/Protocol/Category/Icon/Position — full replace, matching
// CadencePolicyInput's precedent). IsDefault is never touched here: editing
// a seeded type's protocol keeps it flagged as a default, since IsDefault
// is provenance ("came from the seed set"), not "unmodified since seeding."
func UpdateLinkFieldType(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var linkType models.LinkFieldType
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&linkType).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Link field type").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve link field type").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.LinkFieldTypeInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if input.Name != linkType.Name {
		var existing models.LinkFieldType
		lookupErr := db.Where("user_id = ? AND name = ?", userID, input.Name).First(&existing).Error
		if lookupErr == nil {
			apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Link field type"))
			return
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing link field type").WithError(lookupErr))
			return
		}
	}

	linkType.Name = input.Name
	linkType.Protocol = input.Protocol
	linkType.Category = input.Category
	linkType.Icon = input.Icon
	if input.Position != nil {
		linkType.Position = *input.Position
	}

	if err := db.Save(&linkType).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save link field type").WithError(err))
		return
	}

	c.JSON(http.StatusOK, linkType)
}

// DeleteLinkFieldType soft-deletes a LinkFieldType (user-authored
// configuration, T26). The partial unique index lets a new type be created
// with the same Name afterward.
func DeleteLinkFieldType(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var linkType models.LinkFieldType
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&linkType).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Link field type").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve link field type").WithError(err))
		}
		return
	}

	if err := db.Delete(&linkType).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete link field type").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Link field type deleted"})
}

// ReorderLinkFieldTypes persists a new display order for the authenticated
// user's LinkFieldTypes. The full set of IDs must be supplied and every ID
// must belong to the caller — a partial or foreign-owned ID list is
// rejected outright rather than silently reordering a subset (COUNT of
// matching owned rows must equal the list length, which also catches
// duplicate IDs in the payload).
func ReorderLinkFieldTypes(c *gin.Context) {
	input, err := middleware.GetValidated[models.LinkFieldTypeReorderInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var count int64
	if err := db.Model(&models.LinkFieldType{}).Where("user_id = ? AND id IN ?", userID, input.Order).Count(&count).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to verify link field types").WithError(err))
		return
	}
	if int(count) != len(input.Order) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("order", "every ID must belong to the current user, with no duplicates"))
		return
	}

	txErr := db.Transaction(func(tx *gorm.DB) error {
		for i, id := range input.Order {
			if err := tx.Model(&models.LinkFieldType{}).Where("id = ? AND user_id = ?", id, userID).
				Update("position", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to reorder link field types").WithError(txErr))
		return
	}

	var types []models.LinkFieldType
	if err := db.Where("user_id = ?", userID).Order("category, position, name").Find(&types).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve link field types").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"link_field_types": types})
}
