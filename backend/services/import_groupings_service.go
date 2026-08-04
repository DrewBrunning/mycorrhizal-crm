package services

import (
	"errors"
	"strings"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Grouping materialisation for imports (T3 — docs/fork-plan/95's Circle/Tag
// backend call-site rewiring).
//
// The problem this solves: CSV/vCard import parsed circle-ish columns into the
// flat `Contact.Circles` JSON column and stopped there, creating no Circle and
// no CircleMember rows. Every read surface in the app had already moved to the
// real Circle/Tag entities (ContactsPage's filter, NetworkGraph's grouping,
// ContactHeader's chips), so **imported circles were invisible in the UI** —
// the data landed in a column nothing displayed. CSV export then read that same
// flat column back, so it emitted the stale strings and omitted every real
// Circle membership the user had actually created. The round trip was broken in
// both directions.
//
// The flat `Contact.Circles` column survives as the *staging* value the parsers
// write and the import preview/merge-diff renders (it is what the T2 triage
// page migrates from, via GetCircles?legacy=true). Materialisation happens once
// the contact is persisted and its VCardUID exists, because membership rows are
// keyed by VCardUID, not by the numeric contact ID.
//
// Vocabulary split: `circles`/`groups` map to Circle (a group you belong to),
// `tags`/`labels`/`categories` map to Tag (a label). Before T3 all four
// collapsed onto the single flat `circles` field because Tag had nowhere to go.

// MaterializeImportedGroupings creates the Circle/Tag entities and membership
// rows implied by a just-imported contact's staged grouping values.
//
// Idempotent in both halves: an entity with that name is reused rather than
// duplicated, and an existing membership is left alone. Safe to call for both
// the "add" and "update" import actions, and safe to re-run over the same
// contact. Must be called inside the import transaction with `tx`, after the
// contact is persisted (it needs contact.VCardUID).
func MaterializeImportedGroupings(tx *gorm.DB, userID uint, contact *models.Contact) error {
	if contact.VCardUID == "" {
		// A contact with no VCardUID cannot own membership rows; this would be
		// a bug upstream (BeforeSave assigns one), so surface it rather than
		// silently dropping the groupings.
		return errors.New("cannot materialize groupings: contact has no VCardUID")
	}

	for _, name := range normalizeGroupingNames(contact.Circles) {
		circle, err := findOrCreateCircle(tx, userID, name)
		if err != nil {
			return err
		}
		if err := ensureCircleMember(tx, userID, circle.ID, contact.VCardUID); err != nil {
			return err
		}
	}

	for _, name := range normalizeGroupingNames(contact.ImportedTags) {
		tag, err := findOrCreateTag(tx, userID, name)
		if err != nil {
			return err
		}
		if err := ensureContactTag(tx, userID, tag.ID, contact.VCardUID); err != nil {
			return err
		}
	}

	return nil
}

// normalizeGroupingNames trims, drops empties, and de-duplicates
// case-insensitively while preserving the first spelling seen — so a CSV
// carrying "Family, family" yields one Circle named "Family" rather than two
// that render identically in the UI.
func normalizeGroupingNames(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, name := range raw {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	return out
}

// findOrCreateCircle returns the user's Circle with this name, creating it if
// absent. Matched case-insensitively so an import does not create "Family"
// alongside an existing "family".
func findOrCreateCircle(tx *gorm.DB, userID uint, name string) (*models.Circle, error) {
	var circle models.Circle
	err := tx.Where("user_id = ? AND name = ? COLLATE NOCASE", userID, name).First(&circle).Error
	if err == nil {
		return &circle, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	circle = models.Circle{UserID: userID, Name: name}
	if err := tx.Create(&circle).Error; err != nil {
		return nil, err
	}
	return &circle, nil
}

// findOrCreateTag mirrors findOrCreateCircle for Tag.
func findOrCreateTag(tx *gorm.DB, userID uint, name string) (*models.Tag, error) {
	var tag models.Tag
	err := tx.Where("user_id = ? AND name = ? COLLATE NOCASE", userID, name).First(&tag).Error
	if err == nil {
		return &tag, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	tag = models.Tag{UserID: userID, Name: name}
	if err := tx.Create(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

// ensureCircleMember adds the membership row unless it already exists. The
// (circle_id, member_vcard_uid) unique index would reject a duplicate anyway;
// checking first keeps this a no-op rather than a constraint error, matching
// the checked-409 precedent the membership endpoints set.
func ensureCircleMember(tx *gorm.DB, userID uint, circleID, vcardUID string) error {
	var existing models.CircleMember
	err := tx.Where("circle_id = ? AND member_vcard_uid = ?", circleID, vcardUID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return tx.Create(&models.CircleMember{
		CircleID:       circleID,
		UserID:         userID,
		MemberVCardUID: vcardUID,
	}).Error
}

// ensureContactTag mirrors ensureCircleMember for ContactTag.
func ensureContactTag(tx *gorm.DB, userID uint, tagID, vcardUID string) error {
	var existing models.ContactTag
	err := tx.Where("tag_id = ? AND contact_vcard_uid = ?", tagID, vcardUID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return tx.Create(&models.ContactTag{
		TagID:           tagID,
		UserID:          userID,
		ContactVCardUID: vcardUID,
	}).Error
}

// CircleNamesForContact returns the names of the Circles a contact belongs to,
// read from the real entities. Used by the CSV exporter, which previously read
// the flat Contact.Circles column and therefore exported stale legacy strings
// while omitting every real membership.
func CircleNamesForContact(db *gorm.DB, userID uint, vcardUID string) ([]string, error) {
	var names []string
	err := db.Model(&models.CircleMember{}).
		Joins("JOIN circles ON circles.id = circle_members.circle_id").
		Where("circle_members.user_id = ? AND circle_members.member_vcard_uid = ?", userID, vcardUID).
		Order("circles.name").
		Pluck("circles.name", &names).Error
	return names, err
}

// TagNamesForContact is CircleNamesForContact's Tag counterpart.
func TagNamesForContact(db *gorm.DB, userID uint, vcardUID string) ([]string, error) {
	var names []string
	err := db.Model(&models.ContactTag{}).
		Joins("JOIN tags ON tags.id = contact_tags.tag_id").
		Where("contact_tags.user_id = ? AND contact_tags.contact_vcard_uid = ?", userID, vcardUID).
		Order("tags.name").
		Pluck("tags.name", &names).Error
	return names, err
}
