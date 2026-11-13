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
