package services

import (
	"errors"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Wedding anniversary <-> married LifeEvent synchronization.
//
// A wedding date has two legitimate homes that the user expects to stay in
// sync: the standards-facing `Card.Anniversaries[kind=wedding]` (vCard
// ANNIVERSARY, round-trips through CardDAV and the exporters) and the CRM
// narrative `LifeEvent{type: married}` (timeline, T5b reminders, gift links).
//
// The rule (deliberate, see the contact-field-ordering discussion): only the
// DATE is mirrored, and only in one direction per call — the side the user
// just edited is authoritative ("last edited wins"). The LifeEvent's
// description/related-entities/remind are never touched by the anniversary
// side, and the anniversary's Place/ID are preserved when the LifeEvent side
// writes. Clearing one side clears the other's date but keeps any narrative.
//
// Both sync functions converge: they compare before writing and only write the
// other entity when it actually differs, and they write through direct GORM
// model ops (not the HTTP controllers), so neither can recursively re-trigger
// the other.

const weddingAnniversaryKind = "wedding"

// WeddingDateFromCard extracts the wedding-anniversary PartialDate from a
// Card, or nil if the card has none. Timestamp-based anniversary dates are
// ignored (the UI writes Partial dates; a full timestamp on import simply
// isn't mirrored to the LifeEvent side).
func WeddingDateFromCard(card *contactmodel.Card) *contactmodel.PartialDate {
	if card == nil {
		return nil
	}
	for i := range card.Anniversaries {
		if card.Anniversaries[i].Kind == weddingAnniversaryKind {
			return card.Anniversaries[i].Date.Partial
		}
	}
	return nil
}

// SyncWeddingFromCard reconciles the contact's married LifeEvent with the
// card's wedding-anniversary date. Called after a contact record save; the
// just-saved card is authoritative. Creates a married LifeEvent when the
// anniversary appears with none existing; clears (or removes, when the event
// is date-only with no narrative) the LifeEvent date when the anniversary is
// removed.
func SyncWeddingFromCard(db *gorm.DB, userID uint, vcardUID string, cardDate *contactmodel.PartialDate) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var event models.LifeEvent
		err := tx.Where("entity_id = ? AND user_id = ? AND type = ?", vcardUID, userID, models.LifeEventTypeMarried).
			First(&event).Error
		switch {
		case err == nil:
			// existing event
		case errors.Is(err, gorm.ErrRecordNotFound):
			if cardDate == nil {
				return nil // nothing on either side
			}
			return tx.Create(&models.LifeEvent{
				UserID:   userID,
				EntityID: vcardUID,
				Type:     models.LifeEventTypeMarried,
				Date:     clonePartialDate(cardDate),
				Source:   models.LifeEventSourceUser,
			}).Error
		default:
			return err
		}

		if equalPartialDate(event.Date, cardDate) {
			return nil
		}
		if cardDate == nil {
			// Anniversary cleared: clear the LifeEvent date. A date-only event
			// with no narrative is an empty shell once its date is gone, so it
			// is removed rather than left as a blank row.
			event.Date = nil
			if event.Description == "" && !event.Remind && len(event.RelatedEntityIDs) == 0 {
				return tx.Delete(&event).Error
			}
			return tx.Save(&event).Error
		}
		event.Date = clonePartialDate(cardDate)
		return tx.Save(&event).Error
	})
}

// SyncWeddingFromLifeEvent sets the contact's wedding anniversary to match a
// married LifeEvent's date. Called after a married LifeEvent is created,
// updated, or deleted; the just-saved LifeEvent is authoritative. A nil date
// clears the card's wedding anniversary. The card is written through
// RecordForContact/ApplyRecordToContact so no other card data is lost
// (CLAUDE.md traps 2 and 3).
func SyncWeddingFromLifeEvent(db *gorm.DB, userID uint, vcardUID string, lifeEventDate *contactmodel.PartialDate) error {
	var contact models.Contact
	if err := db.Where("vcard_uid = ? AND user_id = ?", vcardUID, userID).First(&contact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // contact gone; nothing to update
		}
		return err
	}

	rec := models.RecordForContact(&contact, "", db)
	if equalPartialDate(WeddingDateFromCard(&rec.Card), lifeEventDate) {
		return nil
	}

	rec.Card.Anniversaries = withWeddingAnniversary(rec.Card.Anniversaries, lifeEventDate)
	models.ApplyRecordToContact(&contact, rec, "")
	return db.Save(&contact).Error
}

// withWeddingAnniversary sets or clears the wedding entry in an anniversary
// list. An existing wedding entry keeps its ID/Place; a nil date drops it.
func withWeddingAnniversary(anniversaries []contactmodel.Anniversary, date *contactmodel.PartialDate) []contactmodel.Anniversary {
	out := make([]contactmodel.Anniversary, 0, len(anniversaries)+1)
	replaced := false
	for _, a := range anniversaries {
		if a.Kind != weddingAnniversaryKind {
			out = append(out, a)
			continue
		}
		if date == nil {
			continue // drop the wedding entry
		}
		a.Date = contactmodel.AnniversaryDate{Partial: clonePartialDate(date)}
		out = append(out, a)
		replaced = true
	}
	if date != nil && !replaced {
		out = append(out, contactmodel.Anniversary{
			Kind: weddingAnniversaryKind,
			Date: contactmodel.AnniversaryDate{Partial: clonePartialDate(date)},
		})
	}
	return out
}

func clonePartialDate(d *contactmodel.PartialDate) *contactmodel.PartialDate {
	if d == nil {
		return nil
	}
	c := *d
	return &c
}

func equalPartialDate(a, b *contactmodel.PartialDate) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	eqInt := func(x, y *int) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		return *x == *y
	}
	return eqInt(a.Year, b.Year) && eqInt(a.Month, b.Month) && eqInt(a.Day, b.Day) && a.CalendarScale == b.CalendarScale
}
