package controllers

import (
	"errors"
	"fmt"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// BulkContactOperation runs one bulk action across many contacts with
// partial-success semantics (N5 — docs/fork-plan/tickets/27-N5-bulk-operations.md).
//
// Every target is resolved through Contact.VCardUID scoped to the
// authenticated user, which serves three purposes at once:
//   - membership rows (CircleMember/ContactTag) are keyed by VCardUID, so the
//     same identifier works for every action;
//   - ownership is enforced by construction — a VCardUID that is not the
//     user's own contact resolves to nothing and is reported as a failure,
//     never silently skipped;
//   - a non-existent or foreign circle/tag aborts the whole request (it is a
//     malformed action, not a per-contact outcome).
//
// Idempotency: "add" to something already present is success, and "remove"
// from something already absent is success — the operation is declarative
// (reach the end state), so the duplicate-add 409 the per-contact endpoints
// return becomes a no-op success here.
func BulkContactOperation(c *gin.Context) {
	input, err := middleware.GetValidated[models.BulkContactOperationInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// Dedupe the target list while preserving order, so a duplicated uid is
	// neither processed twice nor double-counted in the totals.
	uids := make([]string, 0, len(input.VCardUIDs))
	seen := make(map[string]bool, len(input.VCardUIDs))
	for _, uid := range input.VCardUIDs {
		if !seen[uid] {
			seen[uid] = true
			uids = append(uids, uid)
		}
	}

	// Resolve the target circle/tag once up-front. A missing or foreign one is
	// a malformed request, not a per-contact outcome.
	var circle *models.Circle
	var tag *models.Tag
	switch input.Action {
	case "add_circle", "remove_circle":
		if input.CircleID == "" {
			apperrors.AbortWithError(c, apperrors.ErrValidation("circle_id is required for this action"))
			return
		}
		var circleRow models.Circle
		if err := db.Where("id = ? AND user_id = ?", input.CircleID, userID).First(&circleRow).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				apperrors.AbortWithError(c, apperrors.ErrNotFound("Circle").WithDetails("id", input.CircleID))
			} else {
				apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve circle").WithError(err))
			}
			return
		}
		circle = &circleRow
	case "add_tag", "remove_tag":
		if input.TagID == "" {
			apperrors.AbortWithError(c, apperrors.ErrValidation("tag_id is required for this action"))
			return
		}
		var tagRow models.Tag
		if err := db.Where("id = ? AND user_id = ?", input.TagID, userID).First(&tagRow).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				apperrors.AbortWithError(c, apperrors.ErrNotFound("Tag").WithDetails("id", input.TagID))
			} else {
				apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve tag").WithError(err))
			}
			return
		}
		tag = &tagRow
	}

	// Load the user's own contacts for the requested uids. Any uid that does
	// not resolve here is a per-contact failure (ownership scoping).
	owned := make(map[string]models.Contact, len(uids))
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contacts").WithError(err))
		return
	}
	for i := range contacts {
		owned[contacts[i].VCardUID] = contacts[i]
	}

	result := models.BulkOperationResult{Action: input.Action, Total: len(uids)}
	// Contacts that were actually hard-deleted, so their profile photo files
	// can be removed after the batch (best-effort, like DeleteContact).
	var deletedContacts []models.Contact

	for _, uid := range uids {
		contact, ok := owned[uid]
		if !ok {
			result.Failed++
			result.Failures = append(result.Failures, models.BulkFailureEntry{VCardUID: uid, Reason: "contact not found or not owned"})
			continue
		}
		if err := runBulkContactAction(db, userID, contact, input.Action, circle, tag); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, models.BulkFailureEntry{VCardUID: uid, Reason: err.Error()})
			continue
		}
		if input.Action == "delete" {
			deletedContacts = append(deletedContacts, contact)
		}
		result.Succeeded++
	}

	// Clean up profile photo files for hard-deleted contacts. File deletion
	// cannot be rolled back, so it happens after the batch like DeleteContact.
	for _, contact := range deletedContacts {
		deleteContactPhotos(c, contact)
	}

	c.JSON(http.StatusOK, result)
}

// runBulkContactAction applies one bulk action to a single owned contact.
// Every action is idempotent: it returns nil when the desired end state
// already holds.
func runBulkContactAction(db *gorm.DB, userID uint, contact models.Contact, action string, circle *models.Circle, tag *models.Tag) error {
	switch action {
	case "add_circle":
		var existing models.CircleMember
		err := db.Where("circle_id = ? AND member_vcard_uid = ?", circle.ID, contact.VCardUID).First(&existing).Error
		if err == nil {
			return nil // already a member — success
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return db.Create(&models.CircleMember{CircleID: circle.ID, UserID: userID, MemberVCardUID: contact.VCardUID}).Error
	case "remove_circle":
		return db.Where("circle_id = ? AND user_id = ? AND member_vcard_uid = ?", circle.ID, userID, contact.VCardUID).
			Delete(&models.CircleMember{}).Error
	case "add_tag":
		var existing models.ContactTag
		err := db.Where("tag_id = ? AND contact_vcard_uid = ?", tag.ID, contact.VCardUID).First(&existing).Error
		if err == nil {
			return nil // already tagged — success
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return db.Create(&models.ContactTag{TagID: tag.ID, UserID: userID, ContactVCardUID: contact.VCardUID}).Error
	case "remove_tag":
		return db.Where("tag_id = ? AND user_id = ? AND contact_vcard_uid = ?", tag.ID, userID, contact.VCardUID).
			Delete(&models.ContactTag{}).Error
	case "archive":
		// Mirror the single-contact ArchiveContact: archiving also retires the
		// contact's reminders.
		return db.Transaction(func(tx *gorm.DB) error {
			// N9: clear delivery state for the contact's reminders before the
			// (soft) reminder delete leaves the rows dangling.
			if err := tx.Where("reminder_id IN (SELECT id FROM reminders WHERE contact_id = ? AND user_id = ?)", contact.ID, userID).Delete(&models.NotificationDelivery{}).Error; err != nil {
				return err
			}
			if err := tx.Where("contact_id = ? AND user_id = ?", contact.ID, userID).Delete(&models.Reminder{}).Error; err != nil {
				return err
			}
			return tx.Model(&contact).Update("archived", true).Error
		})
	case "unarchive":
		return db.Model(&contact).Update("archived", false).Error
	case "delete":
		// Per-contact transaction: the full DeleteContact cascade either
		// completes for this contact or rolls back cleanly, so a mid-batch
		// failure never leaves one contact half-cleaned (N5 trap).
		return db.Transaction(func(tx *gorm.DB) error {
			if err := deleteContactAssociations(tx, contact, userID); err != nil {
				return err
			}
			return tx.Delete(&contact).Error
		})
	}
	return fmt.Errorf("unknown bulk action %q", action)
}
