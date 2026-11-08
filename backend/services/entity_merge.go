package services

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Entity merge (issue #2299, ADR 0043 decision 8). One transaction: the
// loser's links are re-pointed to the survivor, its custom values are re-keyed
// where the survivor has none, explicit resolutions fill empty survivor core
// fields, and the loser is soft-deleted through the ordinary entity delete (so
// it gets a batch id, a snapshot and a trash entry, and its dismissals go with
// it via EntityCascadeRegistry). Nothing is merged implicitly: the survivor
// keeps its own core, name and values unless a resolution says otherwise.

// Sentinel errors the controller maps to reason codes.
var (
	ErrEntityMergeSame         = errors.New("an entity cannot be merged into itself")
	ErrEntityMergeKindMismatch = errors.New("only entities of the same kind can be merged")
)

// EntityMergeResolutionError is a rejected resolutions map (unknown field, a
// field the survivor already has, or an unsupported choice).
type EntityMergeResolutionError struct{ Reason string }

func (e *EntityMergeResolutionError) Error() string { return e.Reason }

// EntityMergeResult reports what a merge did.
type EntityMergeResult struct {
	Survivor           models.Entity
	LinksRepointed     int
	LinksCollapsed     int
	PrimariesDemoted   []models.EntityMergeDemotion
	FieldValuesMoved   int
	FieldValuesDropped []string
	ResolvedFields     []string
}

// entityFillableFields lists, per kind, the core fields a resolution may fill.
func entityFillableFields(kind string) []string {
	switch kind {
	case models.EntityKindOrganisation:
		return []string{"organization.sortAs", "organization.units", "url", "notes"}
	case models.EntityKindPlace:
		return []string{"address", "notes"}
	case models.EntityKindPet:
		return []string{"species", "breed", "birth", "death", "notes"}
	}
	return nil
}

// fillCoreField copies field from loser into survivor when the survivor's is
// empty. It returns (filled, known): known is false for a field name the kind
// does not have; filled is false when the survivor already had a value.
func fillCoreField(field string, survivor, loser any) (filled, known bool) {
	switch s := survivor.(type) {
	case *models.OrganisationCore:
		l := loser.(*models.OrganisationCore)
		switch field {
		case "organization.sortAs":
			known = true
			if s.Organization.SortAs == "" {
				s.Organization.SortAs, filled = l.Organization.SortAs, l.Organization.SortAs != ""
			}
		case "organization.units":
			known = true
			if len(s.Organization.Units) == 0 {
				s.Organization.Units, filled = l.Organization.Units, len(l.Organization.Units) > 0
			}
		case "url":
			known = true
			if s.URL == "" {
				s.URL, filled = l.URL, l.URL != ""
			}
		case "notes":
			known = true
			if s.Notes == "" {
				s.Notes, filled = l.Notes, l.Notes != ""
			}
		}
	case *models.PlaceCore:
		l := loser.(*models.PlaceCore)
		switch field {
		case "address":
			known = true
			if len(s.Address.Components) == 0 && strings.TrimSpace(s.Address.Full) == "" {
				s.Address = l.Address
				filled = len(l.Address.Components) > 0 || strings.TrimSpace(l.Address.Full) != ""
			}
		case "notes":
			known = true
			if s.Notes == "" {
				s.Notes, filled = l.Notes, l.Notes != ""
			}
		}
	case *models.PetCore:
		l := loser.(*models.PetCore)
		switch field {
		case "species":
			known = true
			if s.Species == "" {
				s.Species, filled = l.Species, l.Species != ""
			}
		case "breed":
			known = true
			if s.Breed == "" {
				s.Breed, filled = l.Breed, l.Breed != ""
			}
		case "birth":
			known = true
			if s.Birth == nil {
				s.Birth, filled = l.Birth, l.Birth != nil
			}
		case "death":
			known = true
			if s.Death == nil {
				s.Death, filled = l.Death, l.Death != nil
			}
		case "notes":
			known = true
			if s.Notes == "" {
				s.Notes, filled = l.Notes, l.Notes != ""
			}
		}
	}
	return filled, known
}

// applyEntityResolutions validates and applies resolutions to the survivor's
// core, returning the sorted list of fields filled. Reject, never merge: any
// invalid entry fails the whole merge before anything is written.
func applyEntityResolutions(survivor, loser *models.Entity, resolutions map[string]string) ([]string, error) {
	fields := make([]string, 0, len(resolutions))
	for f := range resolutions {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	sc, err := survivor.DecodeCore()
	if err != nil { // # pragma: no cover — a stored core is validated JSON on every write
		return nil, err
	}
	lc, err := loser.DecodeCore()
	if err != nil { // # pragma: no cover — a stored core is validated JSON on every write
		return nil, err
	}
	filledFields := []string{}
	for _, f := range fields {
		choice := resolutions[f]
		known := false
		for _, k := range entityFillableFields(survivor.Kind) {
			if k == f {
				known = true
			}
		}
		if !known {
			return nil, &EntityMergeResolutionError{Reason: fmt.Sprintf("%q is not a resolvable field of a %s", f, survivor.Kind)}
		}
		switch choice {
		case "survivor":
			continue
		case "loser":
		default:
			return nil, &EntityMergeResolutionError{Reason: fmt.Sprintf("the choice for %q must be \"loser\" or \"survivor\"", f)}
		}
		filled, _ := fillCoreField(f, sc, lc)
		if !filled {
			// Either the survivor already has a value (never overwritten) or the
			// loser has nothing to give: both are a rejected, not a silent, no-op.
			return nil, &EntityMergeResolutionError{Reason: fmt.Sprintf("%q cannot be filled from the loser: the survivor already has a value or the loser has none", f)}
		}
		filledFields = append(filledFields, f)
	}
	if len(filledFields) > 0 {
		if err := survivor.SetCore(sc); err != nil {
			return nil, &EntityMergeResolutionError{Reason: err.Error()}
		}
		// Content copied from a more sensitive loser must not land in a less
		// sensitive entity (the copy would then leave the instance in exports
		// the loser's own sensitivity excluded): the survivor takes the stricter.
		if sensRank(loser.Sensitivity) > sensRank(survivor.Sensitivity) {
			survivor.Sensitivity = loser.Sensitivity
		}
	}
	return filledFields, nil
}

// linkPeriodEqual compares two link periods by value (nil equals an empty one).
func linkPeriodEqual(a, b *models.ContactEntityLink) bool {
	return reflect.DeepEqual(a.Period, b.Period)
}

// MergeEntities merges loserID into survivorID for userID in one transaction.
// Either id that is not a live entity of userID returns gorm.ErrRecordNotFound.
func MergeEntities(db *gorm.DB, userID uint, loserID, survivorID string, resolutions map[string]string, now time.Time) (*EntityMergeResult, error) {
	if loserID == survivorID {
		return nil, ErrEntityMergeSame
	}
	res := &EntityMergeResult{PrimariesDemoted: []models.EntityMergeDemotion{}, FieldValuesDropped: []string{}, ResolvedFields: []string{}}
	err := db.Transaction(func(tx *gorm.DB) error {
		var loser, survivor models.Entity
		if err := tx.Where("id = ? AND user_id = ?", loserID, userID).First(&loser).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", survivorID, userID).First(&survivor).Error; err != nil {
			return err
		}
		if loser.Kind != survivor.Kind {
			return ErrEntityMergeKindMismatch
		}

		filled, err := applyEntityResolutions(&survivor, &loser, resolutions)
		if err != nil {
			return err
		}
		res.ResolvedFields = filled

		// Links: re-point or collapse, in creation order so the outcome is stable.
		var loserLinks []models.ContactEntityLink
		if err := tx.Where("entity_id = ? AND user_id = ?", loser.ID, userID).Order("created_at, id").Find(&loserLinks).Error; err != nil {
			return err
		}
		var kept []models.ContactEntityLink // the survivor's live links, plus loser links re-pointed so far
		if err := tx.Where("entity_id = ? AND user_id = ?", survivor.ID, userID).Order("created_at, id").Find(&kept).Error; err != nil {
			return err
		}
		for i := range loserLinks {
			l := loserLinks[i]
			dup := -1
			for j := range kept {
				if kept[j].ContactVCardUID == l.ContactVCardUID && kept[j].Role == l.Role && linkPeriodEqual(&kept[j], &l) {
					dup = j
					break
				}
			}
			if dup >= 0 {
				// The loser's link asserts nothing the survivor's does not: collapse
				// it. A primary flag is not lost with it.
				if err := tx.Delete(&l).Error; err != nil {
					return err
				}
				if l.IsPrimary && !kept[dup].IsPrimary {
					kept[dup].IsPrimary = true
					if err := SaveContactEntityLink(tx, &kept[dup]); err != nil {
						return err
					}
				}
				res.LinksCollapsed++
				continue
			}
			l.EntityID = survivor.ID
			if l.IsPrimary {
				var other int64
				if err := tx.Model(&models.ContactEntityLink{}).
					Where("user_id = ? AND contact_vcard_uid = ? AND kind = ? AND is_primary = ? AND id <> ?", userID, l.ContactVCardUID, l.Kind, true, l.ID).
					Count(&other).Error; err != nil {
					return err
				}
				if other > 0 { // the survivor's existing primary keeps the flag
					l.IsPrimary = false
					res.PrimariesDemoted = append(res.PrimariesDemoted, models.EntityMergeDemotion{LinkID: l.ID, ContactVCardUID: l.ContactVCardUID})
				}
			}
			if err := tx.Save(&l).Error; err != nil {
				return err
			}
			kept = append(kept, l)
			res.LinksRepointed++
		}

		// Custom values: re-key where the survivor has none; otherwise the
		// survivor's wins and the loser's stays in the loser's deletion snapshot.
		var loserValues []models.FieldValue
		if err := tx.Where("entity_id = ? AND user_id = ?", loser.ID, userID).Order("id").Find(&loserValues).Error; err != nil {
			return err
		}
		for _, v := range loserValues {
			var have int64
			if err := tx.Model(&models.FieldValue{}).
				Where("entity_id = ? AND user_id = ? AND field_definition_id = ?", survivor.ID, userID, v.FieldDefinitionID).Count(&have).Error; err != nil {
				return err
			}
			if have > 0 {
				res.FieldValuesDropped = append(res.FieldValuesDropped, v.FieldDefinitionID)
				continue
			}
			if err := tx.Model(&models.FieldValue{}).Where("id = ? AND user_id = ?", v.ID, userID).
				UpdateColumn("entity_id", survivor.ID).Error; err != nil {
				return err
			}
			res.FieldValuesMoved++
		}

		// Save through the model so the survivor's revision and ETag advance
		// (ADR 0006/0008) even when nothing in its core changed.
		if err := tx.Save(&survivor).Error; err != nil {
			return err
		}
		res.Survivor = survivor

		// The loser goes through the ordinary entity delete: batch id, snapshot,
		// trash entry. Its remaining links (none) and dismissals are cascaded by
		// EntityCascadeRegistry; the snapshot remembers the survivor.
		return deleteEntity(tx, userID, loser.ID, now, survivor.ID)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
