package controllers

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"gorm.io/gorm"
)

// bulkFieldUpdate is the validated, normalised form of the "update_fields"
// bulk action (issue #1996). It is built once per request by
// buildBulkFieldUpdate, so every rejection (unknown field, over-long value,
// foreign or mistyped custom-field definition) happens before any contact is
// touched: a malformed action is a 400, never a per-contact outcome.
type bulkFieldUpdate struct {
	set         map[string]string
	clear       map[string]bool
	customSet   []models.FieldValueInput
	customClear []string
	photoDir    string
}

// buildBulkFieldUpdate validates in against the explicit allowlist
// (models.BulkEditableFields) and the user's own FieldDefinitions.
func buildBulkFieldUpdate(db *gorm.DB, userID uint, photoDir string, in models.BulkContactOperationInput) (*bulkFieldUpdate, *apperrors.AppError) {
	if len(in.Set) == 0 && len(in.Clear) == 0 && len(in.CustomSet) == 0 && len(in.CustomClear) == 0 {
		return nil, apperrors.ErrValidation("update_fields requires at least one of set, clear, custom_set or custom_clear")
	}
	u := &bulkFieldUpdate{set: map[string]string{}, clear: map[string]bool{}, photoDir: photoDir}

	for key, value := range in.Set {
		limit, ok := models.BulkEditableFields[key]
		if !ok {
			return nil, apperrors.ErrInvalidInput("set."+key, "field is not bulk-editable")
		}
		if strings.TrimSpace(value) == "" {
			return nil, apperrors.ErrInvalidInput("set."+key, "value must not be empty; list the field in clear to empty it")
		}
		if utf8.RuneCountInString(value) > limit {
			return nil, apperrors.ErrInvalidInput("set."+key, fmt.Sprintf("must be at most %d characters", limit))
		}
		u.set[key] = value
	}
	for _, key := range in.Clear {
		if _, ok := models.BulkEditableFields[key]; !ok {
			return nil, apperrors.ErrInvalidInput("clear", fmt.Sprintf("%q is not bulk-editable", key))
		}
		if _, dup := u.set[key]; dup {
			return nil, apperrors.ErrInvalidInput("clear", fmt.Sprintf("%q is in both set and clear", key))
		}
		u.clear[key] = true
	}

	if len(in.CustomSet) == 0 && len(in.CustomClear) == 0 {
		return u, nil
	}
	var defs []models.FieldDefinition
	if err := db.Where("user_id = ?", userID).Find(&defs).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to retrieve field definitions").WithError(err)
	}
	defByID := make(map[string]models.FieldDefinition, len(defs))
	for _, d := range defs {
		defByID[d.ID] = d
	}
	setIDs := map[string]bool{}
	for _, fv := range in.CustomSet {
		def, ok := defByID[fv.FieldDefinitionID]
		if !ok {
			return nil, apperrors.ErrInvalidInput("custom_set", "unknown or not-owned field definition")
		}
		if setIDs[fv.FieldDefinitionID] {
			return nil, apperrors.ErrInvalidInput("custom_set", "duplicate field definition")
		}
		setIDs[fv.FieldDefinitionID] = true
		if err := services.ValidateFieldValue(def, fv.Value); err != nil {
			return nil, apperrors.ErrInvalidInput("custom_set", err.Error())
		}
		u.customSet = append(u.customSet, fv)
	}
	for _, id := range in.CustomClear {
		if _, ok := defByID[id]; !ok {
			return nil, apperrors.ErrInvalidInput("custom_clear", "unknown or not-owned field definition")
		}
		if setIDs[id] {
			return nil, apperrors.ErrInvalidInput("custom_clear", "field definition is in both custom_set and custom_clear")
		}
		u.customClear = append(u.customClear, id)
	}
	return u, nil
}

// apply writes the update to one owned contact atomically: the core-field
// save and every custom-field write share one transaction, so a failure part
// way through leaves that contact exactly as it was (no partial write).
//
// Core fields go through the same Record round trip as PUT /contacts/:id
// (RecordForContact -> edit -> ApplyRecordToContact -> Save), never a direct
// Card/CRM mutation (backend traps 2 and 3), so the neutral Card, flat
// columns, revision/ETag and audit trail stay consistent.
func (u *bulkFieldUpdate) apply(db *gorm.DB, userID uint, contact models.Contact) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if len(u.set) > 0 || len(u.clear) > 0 {
			record := models.RecordForContact(&contact, u.photoDir, tx)
			u.editRecord(record)
			models.ApplyRecordToContact(&contact, record, u.photoDir)
			// ApplyRecordToContact only copies a non-empty envelope gender
			// (the legacy sibling field wins otherwise), so a clear must
			// be applied to the flat field explicitly.
			if u.clear["gender"] {
				contact.Gender = ""
			}
			if err := tx.Save(&contact).Error; err != nil {
				return err
			}
		}
		for _, id := range u.customClear {
			if err := tx.Where("field_definition_id = ? AND entity_id = ? AND user_id = ?", id, contact.VCardUID, userID).
				Delete(&models.FieldValue{}).Error; err != nil {
				return err
			}
		}
		for _, fv := range u.customSet {
			var existing models.FieldValue
			err := tx.Where("field_definition_id = ? AND entity_id = ? AND user_id = ?", fv.FieldDefinitionID, contact.VCardUID, userID).
				First(&existing).Error
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
}

// editRecord applies the allowlisted set/clear edits to the neutral Record.
func (u *bulkFieldUpdate) editRecord(r *contactmodel.Record) {
	// value returns the new value for key and whether the key is touched.
	value := func(key string) (string, bool) {
		if v, ok := u.set[key]; ok {
			return v, true
		}
		return "", u.clear[key]
	}

	if v, ok := value("organization"); ok {
		if len(r.Card.Organizations) == 0 {
			r.Card.Organizations = []contactmodel.Organization{{}}
		}
		r.Card.Organizations[0].Name = v
	}
	if v, ok := value("department"); ok {
		if len(r.Card.Organizations) == 0 {
			r.Card.Organizations = []contactmodel.Organization{{}}
		}
		org := &r.Card.Organizations[0]
		switch {
		case v != "" && len(org.Units) > 0:
			org.Units[0].Name = v
		case v != "":
			org.Units = []contactmodel.OrgUnit{{Name: v}}
		case len(org.Units) > 0:
			org.Units = org.Units[1:]
		}
	}
	if len(r.Card.Organizations) > 0 && r.Card.Organizations[0].Name == "" && len(r.Card.Organizations[0].Units) == 0 {
		r.Card.Organizations = r.Card.Organizations[1:]
	}
	if v, ok := value("job_title"); ok {
		r.Card.Titles = setTitle(r.Card.Titles, "title", v)
	}
	if v, ok := value("role"); ok {
		r.Card.Titles = setTitle(r.Card.Titles, "role", v)
	}
	if v, ok := value("gender"); ok {
		r.Envelope.Gender = v
	}
	if v, ok := value("how_we_met"); ok {
		r.Envelope.HowWeMet = v
	}
	if v, ok := value("work_information"); ok {
		r.Envelope.WorkInformation = v
	}
	if v, ok := value("contact_information"); ok {
		r.Envelope.ContactInformation = v
	}
}

// setTitle sets (name != "") or removes (name == "") the title of the given
// kind. ApplyRecordToContact reads the last entry of a kind, so that is the
// one overwritten; a clear removes every entry of the kind, otherwise an
// earlier one would resurface in the flat column.
func setTitle(titles []contactmodel.Title, kind, name string) []contactmodel.Title {
	if name == "" {
		kept := titles[:0:0]
		for _, t := range titles {
			if t.Kind != kind {
				kept = append(kept, t)
			}
		}
		return kept
	}
	for i := len(titles) - 1; i >= 0; i-- {
		if titles[i].Kind == kind {
			titles[i].Name = name
			return titles
		}
	}
	return append(titles, contactmodel.Title{Name: name, Kind: kind})
}
