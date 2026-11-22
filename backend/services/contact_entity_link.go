package services

import (
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Contact-entity link writes (issue #2297, ADR 0043 decision 2). Both run
// inside the caller's transaction: the writer gate and _txlock=immediate
// serialise SQLite writers (backend trap #9), so the demote-then-set pair is
// atomic, and the partial unique index idx_contact_entity_links_primary is the
// backstop that makes a second live primary impossible even if that ever
// changed.

// demoteOtherPrimaries clears is_primary on every other live link of the same
// contact and kind. Each demoted link is saved through the revision path
// (not a bulk UPDATE) so its ETag moves and If-Match clients see the change.
func demoteOtherPrimaries(tx *gorm.DB, link *models.ContactEntityLink) error {
	var others []models.ContactEntityLink
	if err := tx.Where("user_id = ? AND contact_vcard_uid = ? AND kind = ? AND is_primary = ? AND id <> ?",
		link.UserID, link.ContactVCardUID, link.Kind, true, link.ID).Find(&others).Error; err != nil {
		return err
	}
	for i := range others {
		others[i].IsPrimary = false
		if err := tx.Save(&others[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// writeLink demotes the previous primary of the link's kind when link is
// primary, then inserts (create) or saves it.
func writeLink(tx *gorm.DB, link *models.ContactEntityLink, create bool) error {
	return trackLinkProjection(tx, link, func() error {
		if link.IsPrimary {
			if err := demoteOtherPrimaries(tx, link); err != nil {
				return err
			}
		}
		if create {
			return tx.Create(link).Error
		}
		return tx.Save(link).Error
	})
}

// trackLinkProjection runs write (a change to link, or to its siblings of the
// same contact) and bumps the link's contact when what it projects changed
// (issue #2565, ADR 0043 decision 3): one bump per write however many rows it
// touched, none for a pet link (pets never project) or a change the projection
// does not show (role, role label, a non-primary link).
func trackLinkProjection(tx *gorm.DB, link *models.ContactEntityLink, write func() error) error {
	if link.Kind == models.EntityKindPet {
		return write()
	}
	snap, err := models.SnapshotEntityProjections(tx, link.UserID, []string{link.ContactVCardUID})
	if err != nil {
		return err
	}
	if err := write(); err != nil {
		return err
	}
	_, err = snap.BumpChanged(tx)
	return err
}

// CreateContactEntityLink inserts link, first demoting the previous primary of
// its kind when link is itself primary.
func CreateContactEntityLink(tx *gorm.DB, link *models.ContactEntityLink) error {
	return writeLink(tx, link, true)
}

// SaveContactEntityLink saves an edited link, demoting the previous primary of
// its kind when link is (now) primary.
func SaveContactEntityLink(tx *gorm.DB, link *models.ContactEntityLink) error {
	return writeLink(tx, link, false)
}

// DeleteContactEntityLink soft-deletes link (ADR 0043 decision 6) and bumps its
// contact when the link was projecting.
func DeleteContactEntityLink(db *gorm.DB, link *models.ContactEntityLink) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return trackLinkProjection(tx, link, func() error { return tx.Delete(link).Error })
	})
}

// EntityLinkCounts returns, for each of entityIDs, how many distinct live
// contacts of userID are linked to it through a live link (issue #2743): the
// "Linked to N contacts" definition, where a contact holding several roles
// counts once and a link whose contact was deleted counts for nothing. An
// entity with no links is absent from the map (read as 0). It is one grouped
// query regardless of how many ids it is given, so a list or a suggestions
// page never issues one count per entity. Scoped by user_id on both tables.
func EntityLinkCounts(db *gorm.DB, userID uint, entityIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(entityIDs))
	if len(entityIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		EntityID string
		N        int
	}
	err := db.Table("contact_entity_links AS l").
		Select("l.entity_id AS entity_id, COUNT(DISTINCT l.contact_vcard_uid) AS n").
		Joins("JOIN contacts c ON c.user_id = l.user_id AND c.vcard_uid = l.contact_vcard_uid AND c.deleted_at IS NULL").
		Where("l.user_id = ? AND l.deleted_at IS NULL AND l.entity_id IN ?", userID, entityIDs).
		Group("l.entity_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.EntityID] = r.N
	}
	return counts, nil
}
