package services

import (
	"errors"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ApplySelfContact moves the user's "Me" pointer to newUID ("" clears it) and
// keeps contacts.ambient consistent with it (issues #2193/#2192):
//
//   - the previous Me contact loses ambient only when
//     users.self_contact_ambient_set says marking turned it on; the flag is
//     then reset;
//   - the new Me contact gets ambient on, and the flag records whether marking
//     was what turned it on (false when it was already ambient);
//   - re-marking the current Me keeps the existing provenance.
//
// The caller has already verified that newUID names a contact the user owns.
// Contact writes go through Save so BeforeSave carries ambient into crm.ambient
// and the revision/ETag bump reaches sync. Run it inside a transaction.
func ApplySelfContact(tx *gorm.DB, userID uint, newUID string) error {
	var user models.User
	if err := tx.Select("id", "self_contact_vcard_uid", "self_contact_ambient_set").First(&user, userID).Error; err != nil {
		return err
	}
	prevUID := ""
	if user.SelfContactVCardUID != nil {
		prevUID = *user.SelfContactVCardUID
	}
	provenance := user.SelfContactAmbientSet

	if prevUID == newUID && newUID != "" {
		// Re-marking the current Me: make sure it is ambient, keep provenance.
		changed, err := setContactAmbient(tx, userID, newUID, true)
		if err != nil {
			return err
		}
		if changed {
			provenance = true
		}
		return writeSelfContact(tx, userID, newUID, provenance)
	}

	if prevUID != "" && provenance {
		if _, err := setContactAmbient(tx, userID, prevUID, false); err != nil {
			return err
		}
	}

	provenance = false
	if newUID != "" {
		changed, err := setContactAmbient(tx, userID, newUID, true)
		if err != nil {
			return err
		}
		provenance = changed
	}
	return writeSelfContact(tx, userID, newUID, provenance)
}

func writeSelfContact(tx *gorm.DB, userID uint, uid string, provenance bool) error {
	var ptr any
	if uid != "" {
		ptr = uid
	}
	return tx.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]any{"self_contact_vcard_uid": ptr, "self_contact_ambient_set": provenance}).Error
}

// setContactAmbient sets the owned contact's ambient flag and reports whether
// the stored value changed. A missing contact is not an error (it may have been
// deleted since the pointer was set).
func setContactAmbient(tx *gorm.DB, userID uint, uid string, value bool) (bool, error) {
	var contact models.Contact
	if err := tx.Where("user_id = ? AND vcard_uid = ?", userID, uid).First(&contact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if contact.Ambient == value {
		return false, nil
	}
	contact.Ambient = value
	if err := tx.Save(&contact).Error; err != nil {
		return false, err
	}
	return true, nil
}

// ClearSelfAmbientProvenanceIfMe resets users.self_contact_ambient_set when a
// manual edit changed the ambient flag of the user's Me contact (issue #2192:
// the user took ownership of the flag, so unmarking must not override it).
func ClearSelfAmbientProvenanceIfMe(tx *gorm.DB, userID uint, contactUID string) error {
	return tx.Model(&models.User{}).
		Where("id = ? AND self_contact_vcard_uid = ? AND self_contact_ambient_set = ?", userID, contactUID, true).
		Update("self_contact_ambient_set", false).Error
}
