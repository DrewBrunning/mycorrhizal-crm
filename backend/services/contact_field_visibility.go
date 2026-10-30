package services

import (
	"fmt"
	"sort"

	"mycorrhizal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Per-contact field visibility (issue #1997). Cosmetic display state kept in
// contact_field_overrides, outside the neutral Card: nothing here reads or
// writes the contact row, so it never bumps the contact's revision/ETag and
// can never reach vCard/JSContact, CardDAV/CalDAV, shares or MCP.

// FieldOverrideError is a validation failure on an override write. Key is the
// offending map key (or "hide_empty"); the controller maps Kind to a reason
// code.
type FieldOverrideError struct {
	Kind  FieldOverrideErrorKind
	Key   string
	Value string
}

// FieldOverrideErrorKind classifies a FieldOverrideError.
type FieldOverrideErrorKind int

const (
	// FieldOverrideUnknownKey: the key is not on models.ContactFieldKeys.
	FieldOverrideUnknownKey FieldOverrideErrorKind = iota
	// FieldOverrideInvalidState: the state is not inherit / show / hide.
	FieldOverrideInvalidState
)

func (e *FieldOverrideError) Error() string {
	if e.Kind == FieldOverrideUnknownKey {
		return fmt.Sprintf("unknown contact field key %q", e.Key)
	}
	return fmt.Sprintf("invalid field state %q for %q", e.Value, e.Key)
}

func validFieldState(s string) bool {
	return s == models.FieldStateInherit || s == models.FieldStateShow || s == models.FieldStateHide
}

// ValidateFieldOverrideInput checks an input against the allowlist and the
// tri-state values. Keys are checked in sorted order so the first reported
// error is deterministic.
func ValidateFieldOverrideInput(in models.ContactFieldVisibilityInput) error {
	keys := make([]string, 0, len(in.Overrides))
	for k := range in.Overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !models.IsContactFieldKey(k) {
			return &FieldOverrideError{Kind: FieldOverrideUnknownKey, Key: k}
		}
		if !validFieldState(in.Overrides[k]) {
			return &FieldOverrideError{Kind: FieldOverrideInvalidState, Key: k, Value: in.Overrides[k]}
		}
	}
	if in.HideEmpty != "" && !validFieldState(in.HideEmpty) {
		return &FieldOverrideError{Kind: FieldOverrideInvalidState, Key: "hide_empty", Value: in.HideEmpty}
	}
	return nil
}

// GetContactFieldVisibility returns the raw overrides and the resolved
// visibility for one of the user's contacts. The caller has already resolved
// ownership of contactUID (resolveOwnedContactByID); every query here is
// scoped by user_id as well.
func GetContactFieldVisibility(db *gorm.DB, userID uint, contactUID string) (*models.ContactFieldVisibilityResponse, error) {
	var user models.User
	if err := db.Select("id", "enabled_contact_fields", "hide_empty_fields").First(&user, userID).Error; err != nil {
		return nil, err
	}
	var rows []models.ContactFieldOverride
	if err := db.Where("user_id = ? AND contact_vcard_uid = ?", userID, contactUID).Find(&rows).Error; err != nil {
		return nil, err
	}
	stored := make(map[string]string, len(rows))
	overrides := map[string]string{}
	hideEmpty := models.FieldStateInherit
	for _, r := range rows {
		stored[r.FieldKey] = r.State
		switch {
		case r.FieldKey == models.HideEmptyOverrideKey:
			hideEmpty = r.State
		case models.IsContactFieldKey(r.FieldKey):
			overrides[r.FieldKey] = r.State
		}
	}
	defaults, resolved := models.ResolveFieldVisibility(user.EnabledContactFields, user.HideEmptyFields, stored)
	return &models.ContactFieldVisibilityResponse{
		Overrides: overrides,
		HideEmpty: hideEmpty,
		Defaults:  defaults,
		Resolved:  resolved,
	}, nil
}

// ReplaceContactFieldOverrides makes the stored override set for the contact
// exactly the one in the input: explicit show/hide entries are upserted,
// everything else (absent or "inherit") is deleted. Validation runs first, so
// a bad request writes nothing. One transaction.
func ReplaceContactFieldOverrides(db *gorm.DB, userID uint, contactUID string, in models.ContactFieldVisibilityInput) error {
	if err := ValidateFieldOverrideInput(in); err != nil {
		return err
	}
	want := map[string]string{}
	for k, s := range in.Overrides {
		if s == models.FieldStateShow || s == models.FieldStateHide {
			want[k] = s
		}
	}
	if in.HideEmpty == models.FieldStateShow || in.HideEmpty == models.FieldStateHide {
		want[models.HideEmptyOverrideKey] = in.HideEmpty
	}
	return db.Transaction(func(tx *gorm.DB) error {
		del := tx.Where("user_id = ? AND contact_vcard_uid = ?", userID, contactUID)
		if len(want) > 0 {
			keep := make([]string, 0, len(want))
			for k := range want {
				keep = append(keep, k)
			}
			del = del.Where("field_key NOT IN ?", keep)
		}
		if err := del.Delete(&models.ContactFieldOverride{}).Error; err != nil {
			return err
		}
		for k, s := range want {
			row := models.ContactFieldOverride{UserID: userID, ContactVCardUID: contactUID, FieldKey: k, State: s}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "contact_vcard_uid"}, {Name: "field_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ClearContactFieldOverrides removes every override of the contact ("reset to
// default").
func ClearContactFieldOverrides(db *gorm.DB, userID uint, contactUID string) error {
	return db.Where("user_id = ? AND contact_vcard_uid = ?", userID, contactUID).
		Delete(&models.ContactFieldOverride{}).Error
}
