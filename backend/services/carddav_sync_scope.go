package services

import (
	"errors"
	"sort"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CardDAVSyncScopeSelection is a validated, normalized sync scope. CircleIDs
// is sorted and deduplicated so callers and tests see a stable order.
type CardDAVSyncScopeSelection struct {
	Mode            string
	CircleIDs       []string
	DefaultCircleID *string
}

// LoadCardDAVSyncScope returns the stored scope and selected circle ids. A
// user with no row is in mode "all" with no circles (ADR 0041 decision 2).
func LoadCardDAVSyncScope(db *gorm.DB, userID uint) (models.CardDAVSyncScope, []string, error) {
	var scope models.CardDAVSyncScope
	err := db.Where("user_id = ?", userID).First(&scope).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.CardDAVSyncScope{UserID: userID, Mode: models.CardDAVSyncScopeModeAll}, []string{}, nil
	}
	if err != nil {
		return scope, nil, err
	}

	ids := []string{}
	if err := db.Model(&models.CardDAVSyncScopeCircle{}).
		Where("user_id = ?", userID).
		Order("circle_id ASC").
		Pluck("circle_id", &ids).Error; err != nil {
		return scope, nil, err
	}
	return scope, ids, nil
}

// CardDAVSyncScopeSelectionFrom loads a selection from storage.
func CardDAVSyncScopeSelectionFrom(scope models.CardDAVSyncScope, circleIDs []string) CardDAVSyncScopeSelection {
	return CardDAVSyncScopeSelection{
		Mode:            scope.Mode,
		CircleIDs:       append([]string{}, circleIDs...),
		DefaultCircleID: scope.DefaultCircleID,
	}
}

// ValidateCardDAVSyncScope applies ADR 0041 decision 2's write-time rules and
// resolves every submitted circle id against the caller's own circles. An
// unknown id and another user's id are deliberately indistinguishable: both are
// a 400 with the same reason.
func ValidateCardDAVSyncScope(db *gorm.DB, userID uint, input models.CardDAVSyncScopeInput) (CardDAVSyncScopeSelection, *apperrors.AppError) {
	invalid := func(field, message string) (CardDAVSyncScopeSelection, *apperrors.AppError) {
		return CardDAVSyncScopeSelection{}, apperrors.ErrInvalidInput(field, message).
			WithReason(apperrors.ReasonIntegrationRequestInvalid, nil)
	}

	seen := make(map[string]bool, len(input.CircleIDs))
	ids := make([]string, 0, len(input.CircleIDs))
	for _, id := range input.CircleIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)

	switch input.Mode {
	case models.CardDAVSyncScopeModeAll:
		if len(ids) > 0 {
			return invalid("circle_ids", "circle_ids must be empty in all mode")
		}
		if input.DefaultCircleID != nil {
			return invalid("default_circle_id", "default_circle_id must be null in all mode")
		}
	case models.CardDAVSyncScopeModeInclude:
		if len(ids) == 0 {
			return invalid("circle_ids", "at least one circle is required in include mode")
		}
		if input.DefaultCircleID == nil {
			return invalid("default_circle_id", "default_circle_id is required in include mode")
		}
		if !seen[*input.DefaultCircleID] {
			return invalid("default_circle_id", "default_circle_id must be one of the included circles")
		}
	case models.CardDAVSyncScopeModeExclude:
		if len(ids) == 0 {
			return invalid("circle_ids", "at least one circle is required in exclude mode")
		}
		if input.DefaultCircleID != nil && seen[*input.DefaultCircleID] {
			return invalid("default_circle_id", "default_circle_id must not be an excluded circle")
		}
	default:
		return invalid("mode", "mode must be one of all, include or exclude")
	}

	// Ownership: every referenced circle, including the default, must exist
	// and belong to the caller (backend trap #5). Query once so unknown and
	// cross-user ids cannot be told apart by timing or error text.
	wanted := append([]string{}, ids...)
	if input.DefaultCircleID != nil && !seen[*input.DefaultCircleID] {
		wanted = append(wanted, *input.DefaultCircleID)
	}
	var owned int64
	if err := db.Model(&models.Circle{}).
		Where("user_id = ? AND id IN ?", userID, wanted).
		Count(&owned).Error; err != nil {
		return CardDAVSyncScopeSelection{}, apperrors.ErrDatabase("validate sync scope circles").WithError(err)
	}
	if int(owned) != len(wanted) {
		return invalid("circle_ids", "one or more circles were not found")
	}

	return CardDAVSyncScopeSelection{
		Mode:            input.Mode,
		CircleIDs:       ids,
		DefaultCircleID: input.DefaultCircleID,
	}, nil
}

// ReplaceCardDAVSyncScope persists the whole singleton and its circle set in
// one transaction (PUT is a full replace, ADR 0041 decision 2).
func ReplaceCardDAVSyncScope(db *gorm.DB, userID uint, sel CardDAVSyncScopeSelection) (models.CardDAVSyncScope, error) {
	var saved models.CardDAVSyncScope
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.CardDAVSyncScopeCircle{}).Error; err != nil {
			return err
		}
		for _, circleID := range sel.CircleIDs {
			if err := tx.Create(&models.CardDAVSyncScopeCircle{UserID: userID, CircleID: circleID}).Error; err != nil {
				return err
			}
		}
		saved = models.CardDAVSyncScope{
			UserID:          userID,
			Mode:            sel.Mode,
			DefaultCircleID: sel.DefaultCircleID,
		}
		// Upsert rather than Save: Save writes every column on an existing
		// row, which would reset created_at to the zero time. Preserve the
		// original creation time and only update the mutable fields.
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"mode", "default_circle_id", "updated_at"}),
		}).Create(&saved).Error; err != nil {
			return err
		}
		// Return the persisted row (not the insert attempt) so created_at is
		// populated on both the insert and the update path.
		return tx.Where("user_id = ?", userID).First(&saved).Error
	})
	return saved, err
}

// CardDAVSyncScopeCounts returns (in scope, total live contacts) for a
// selection. The preview endpoint uses this against an unpersisted proposal;
// GET uses it against the stored scope.
func CardDAVSyncScopeCounts(db *gorm.DB, userID uint, sel CardDAVSyncScopeSelection) (int, int, error) {
	var total int64
	if err := db.Model(&models.Contact{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return 0, 0, err
	}

	switch sel.Mode {
	case models.CardDAVSyncScopeModeInclude:
		if len(sel.CircleIDs) == 0 {
			return 0, int(total), nil
		}
		var in int64
		if err := db.Model(&models.Contact{}).
			Where("user_id = ?", userID).
			Where("EXISTS (SELECT 1 FROM circle_members cm WHERE cm.user_id = ? AND cm.member_vcard_uid = contacts.vcard_uid AND cm.circle_id IN ?)", userID, sel.CircleIDs).
			Count(&in).Error; err != nil {
			return 0, 0, err
		}
		return int(in), int(total), nil
	case models.CardDAVSyncScopeModeExclude:
		if len(sel.CircleIDs) == 0 {
			return int(total), int(total), nil
		}
		var excluded int64
		if err := db.Model(&models.Contact{}).
			Where("user_id = ?", userID).
			Where("EXISTS (SELECT 1 FROM circle_members cm WHERE cm.user_id = ? AND cm.member_vcard_uid = contacts.vcard_uid AND cm.circle_id IN ?)", userID, sel.CircleIDs).
			Count(&excluded).Error; err != nil {
			return 0, 0, err
		}
		return int(total - excluded), int(total), nil
	default:
		return int(total), int(total), nil
	}
}

// CardDAVSyncScopeWarnings derives the read-time warnings (ADR 0041 decision
// 3) from the current stored state. They are not stored, so a warning survives
// until the user fixes the scope and cannot be dismissed by accident.
func CardDAVSyncScopeWarnings(sel CardDAVSyncScopeSelection) []models.CardDAVSyncScopeWarning {
	warnings := []models.CardDAVSyncScopeWarning{}
	switch sel.Mode {
	case models.CardDAVSyncScopeModeInclude:
		if len(sel.CircleIDs) == 0 {
			warnings = append(warnings, models.CardDAVSyncScopeWarning{
				Code:    models.CardDAVSyncScopeWarningEmpty,
				Message: "No circles are selected, so no contacts sync to CardDAV clients.",
			})
		} else if sel.DefaultCircleID == nil {
			warnings = append(warnings, models.CardDAVSyncScopeWarning{
				Code:    models.CardDAVSyncScopeWarningDefaultMissing,
				Message: "No default circle is set, so contacts created on a phone cannot be saved.",
			})
		}
	case models.CardDAVSyncScopeModeExclude:
		if len(sel.CircleIDs) == 0 {
			warnings = append(warnings, models.CardDAVSyncScopeWarning{
				Code:    models.CardDAVSyncScopeWarningExclusionEmpty,
				Message: "No circles are excluded, so every contact syncs to CardDAV clients.",
			})
		}
	}
	return warnings
}

// CardDAVSyncScopeContactCondition is the in-query scope predicate (ADR 0041
// decision 4). It reads the scope tables as correlated subqueries, so a scope
// edit cannot race a contact read; there is no pre-fetched id list. It assumes
// the query's model is contacts and refers to contacts.user_id/vcard_uid. It
// re-joins circles on user_id so a row written by any future path cannot name
// another user's circle (backend trap #5).
const CardDAVSyncScopeContactCondition = `(
	COALESCE((SELECT css.mode FROM carddav_sync_scopes css WHERE css.user_id = contacts.user_id), 'all') = 'all'
	OR (COALESCE((SELECT css.mode FROM carddav_sync_scopes css WHERE css.user_id = contacts.user_id), 'all') = 'include'
	    AND EXISTS (
	        SELECT 1 FROM carddav_sync_scope_circles cscc
	        JOIN circle_members cm ON cm.user_id = cscc.user_id AND cm.circle_id = cscc.circle_id
	        JOIN circles ci ON ci.id = cscc.circle_id AND ci.user_id = cscc.user_id
	        WHERE cscc.user_id = contacts.user_id AND cm.member_vcard_uid = contacts.vcard_uid
	    ))
	OR (COALESCE((SELECT css.mode FROM carddav_sync_scopes css WHERE css.user_id = contacts.user_id), 'all') = 'exclude'
	    AND NOT EXISTS (
	        SELECT 1 FROM carddav_sync_scope_circles cscc
	        JOIN circle_members cm ON cm.user_id = cscc.user_id AND cm.circle_id = cscc.circle_id
	        JOIN circles ci ON ci.id = cscc.circle_id AND ci.user_id = cscc.user_id
	        WHERE cscc.user_id = contacts.user_id AND cm.member_vcard_uid = contacts.vcard_uid
	    ))
)`

// ApplyCardDAVSyncScope restricts a contacts query to the user's CardDAV sync
// scope. Every CardDAV read path must go through this helper (ADR 0041
// decision 4, test-plan item 10).
func ApplyCardDAVSyncScope(db *gorm.DB) *gorm.DB {
	return db.Where(CardDAVSyncScopeContactCondition)
}

// ApplyCardDAVSyncScopeOnCreate files a newly created CardDAV contact into the
// default circle inside the same transaction as the insert (ADR 0041 decision
// 5). In include mode a contact has no circle yet, so one is required; in
// exclude mode the contact is already in scope and a configured default is
// applied as a convenience. All mode adds nothing. A missing default circle
// refuses the create with 403 rather than committing an invisible contact.
func ApplyCardDAVSyncScopeOnCreate(tx *gorm.DB, userID uint, contact models.Contact) error {
	scope, ids, err := LoadCardDAVSyncScope(tx, userID)
	if err != nil {
		return err
	}
	sel := CardDAVSyncScopeSelectionFrom(scope, ids)
	switch sel.Mode {
	case models.CardDAVSyncScopeModeInclude:
		if sel.DefaultCircleID == nil {
			return apperrors.ErrForbidden("No default circle is configured for the CardDAV sync scope").
				WithReason(apperrors.ReasonCircleSyncScopeNoDefault, nil)
		}
	case models.CardDAVSyncScopeModeExclude:
		if sel.DefaultCircleID == nil {
			return nil
		}
	default:
		return nil
	}

	var owned int64
	if err := tx.Model(&models.Circle{}).
		Where("id = ? AND user_id = ?", *sel.DefaultCircleID, userID).
		Count(&owned).Error; err != nil {
		return err
	}
	if owned == 0 {
		return apperrors.ErrForbidden("No default circle is configured for the CardDAV sync scope").
			WithReason(apperrors.ReasonCircleSyncScopeNoDefault, nil)
	}
	return tx.Create(&models.CircleMember{
		CircleID:       *sel.DefaultCircleID,
		UserID:         userID,
		MemberVCardUID: contact.VCardUID,
	}).Error
}

// ContactInCardDAVSyncScope reports whether one live, owned contact is in the
// scope. CardDAV writes use it to distinguish an out-of-scope edit (403) from a
// missing resource (create) before the If-Match comparison.
func ContactInCardDAVSyncScope(db *gorm.DB, userID uint, contact models.Contact) (bool, error) {
	var count int64
	err := db.Model(&models.Contact{}).
		Where("user_id = ? AND id = ?", userID, contact.ID).
		Where(CardDAVSyncScopeContactCondition).
		Count(&count).Error
	return count > 0, err
}
