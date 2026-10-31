package services

import (
	"context"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// SaveContact is the single write path for a contact's create/update shared by
// the REST controllers and the CardDAV PUT (issue #2255), so a contact edited
// on a phone ends in the same state as the same edit made on the web.
//
// The caller has already applied the incoming record onto contact (and, for an
// update, loaded it scoped by user_id and checked its If-Match / ETag
// precondition). SaveContact then:
//
//  1. refuses a create with the opt-in per-user contact quota (issue #2261),
//     before anything is written, so REST and CardDAV cannot diverge;
//  2. writes the row in one transaction: Create when contact.ID is zero, else
//     Save (revision CAS, ADR 0018; the audit events come from the model hooks
//     inside it), plus ClearSelfAmbientProvenanceIfMe when the ambient flag
//     changed from ambientBefore (issues #2192/#2193);
//  3. after the commit, mirrors the card's wedding anniversary into the
//     married LifeEvent (SyncWeddingFromCard; a failure is logged, not
//     returned, because the contact write has already committed);
//  4. fires the contact.created / contact.updated webhook, so a failed write
//     delivers nothing.
//
// A write error is returned unwrapped so callers can classify it (revision
// conflict, unique violation) and map it to their own protocol's status.
func SaveContact(ctx context.Context, db *gorm.DB, cfg config.Config, userID uint, contact *models.Contact, ambientBefore bool) error {
	isNew := contact.ID == 0
	if isNew {
		if err := UserQuotaFromConfig(cfg).CheckContactCreate(db, userID); err != nil {
			return err
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if isNew {
			return tx.Create(contact).Error
		}
		if err := tx.Save(contact).Error; err != nil {
			return err
		}
		// A manual ambient change on the "Me" contact is the user taking
		// ownership of the flag: unmarking "Me" must no longer override it.
		if contact.Ambient != ambientBefore {
			return ClearSelfAmbientProvenanceIfMe(tx, userID, contact.VCardUID)
		}
		return nil
	}); err != nil {
		return err
	}

	// See wedding_sync.go: the just-saved card is authoritative.
	if err := SyncWeddingFromCard(db, userID, contact.VCardUID, WeddingDateFromCard(&contact.Card)); err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("Error syncing wedding anniversary to life event")
	}

	event := "contact.updated"
	if isNew {
		event = "contact.created"
	}
	TriggerWebhooksAsync(ctx, db, cfg, userID, event, *contact)
	return nil
}
