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
	if link.IsPrimary {
		if err := demoteOtherPrimaries(tx, link); err != nil {
			return err
		}
	}
	if create {
		return tx.Create(link).Error
	}
	return tx.Save(link).Error
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
