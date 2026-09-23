package services

import (
	"errors"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// lifeEventSuggestionRules maps a Card period kind to the life-event candidate
// it can imply. Deliberately narrow (docs/adrs/0025-temporal-periods.md): only
// cases where the inferred event is unambiguous ship. The candidate's date is
// the period's start; its EndDate is the period's end when present.
//
// Why not more: an address could be a second home, a dorm, or a mailing
// address, so only the presence of a start date at the address an entry
// represents is treated as a move; an address END has no event type of its
// own. Extending this is a rule addition, not a schema change.
var lifeEventSuggestionRules = map[string]struct{ Type, Category string }{
	contactmodel.PeriodKindAddress: {
		Type:     models.LifeEventTypeMoved,
		Category: models.LifeEventCategoryHomeLiving,
	},
	contactmodel.PeriodKindOrganization: {
		Type:     models.LifeEventTypeJobChange,
		Category: models.LifeEventCategoryWorkEducation,
	},
}

// SuggestLifeEvents infers candidate life events from a contact's dated field
// periods. It never writes anything: a candidate is offered, and only the
// user's accept (a normal LifeEvent create) or dismiss (a resolution row)
// persists a decision. A candidate is suppressed when a matching event already
// exists (the accepted case, robust even if the resolution row was lost) or
// the (kind, entry, type) tuple has been resolved before.
//
// Nil-safe: a nil db or contact yields no suggestions. Computation happens on
// read, so a bulk import does not manufacture candidates for every contact.
func SuggestLifeEvents(db *gorm.DB, contact *models.Contact) ([]models.LifeEventSuggestion, error) {
	if db == nil || contact == nil {
		return nil, nil
	}

	var resolutions []models.LifeEventSuggestionResolution
	if err := db.Where("user_id = ? AND entity_id = ?", contact.UserID, contact.VCardUID).
		Find(&resolutions).Error; err != nil {
		return nil, err
	}
	resolved := make(map[[2]string]bool, len(resolutions))
	for _, r := range resolutions {
		resolved[[2]string{r.SourceKind + "\x00" + r.SourceEntryID, r.EventType}] = true
	}

	var events []models.LifeEvent
	if err := db.Where("user_id = ? AND entity_id = ?", contact.UserID, contact.VCardUID).
		Find(&events).Error; err != nil {
		return nil, err
	}

	var out []models.LifeEventSuggestion
	for _, p := range contact.CRM.Periods {
		rule, ok := lifeEventSuggestionRules[p.Kind]
		if !ok || p.Range.Start == nil || p.EntryID == "" {
			continue
		}
		if resolved[[2]string{p.Kind + "\x00" + p.EntryID, rule.Type}] {
			continue
		}
		if lifeEventAlreadyCovered(events, rule.Type, p.Range.Start) {
			continue
		}
		out = append(out, models.LifeEventSuggestion{
			EntityID:      contact.VCardUID,
			Type:          rule.Type,
			Category:      rule.Category,
			Date:          p.Range.Start,
			EndDate:       p.Range.End,
			SourceKind:    p.Kind,
			SourceEntryID: p.EntryID,
		})
	}
	return out, nil
}

// lifeEventAlreadyCovered reports whether an event of the given type already
// anchors on the same date — the "the user already recorded this" guard that
// keeps an accepted suggestion from being re-offered even without a resolution
// row.
func lifeEventAlreadyCovered(events []models.LifeEvent, eventType string, date *contactmodel.PartialDate) bool {
	for i := range events {
		if events[i].Type == eventType && equalPartialDate(events[i].Date, date) {
			return true
		}
	}
	return false
}

// ResolveLifeEventSuggestion records the user's terminal decision for one
// inferred candidate. Idempotent: resolving the same candidate twice is a
// no-op, so a retried accept/dismiss cannot create a duplicate. It never
// creates a LifeEvent — accepting is done by the normal create endpoint; this
// only writes the "do not offer again" memory.
func ResolveLifeEventSuggestion(db *gorm.DB, userID uint, in models.LifeEventSuggestionResolutionInput) error {
	if db == nil {
		return nil
	}
	var existing models.LifeEventSuggestionResolution
	err := db.Where(
		"user_id = ? AND entity_id = ? AND source_kind = ? AND source_entry_id = ? AND event_type = ?",
		userID, in.EntityID, in.SourceKind, in.SourceEntryID, in.EventType,
	).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(&models.LifeEventSuggestionResolution{
		UserID:        userID,
		EntityID:      in.EntityID,
		SourceKind:    in.SourceKind,
		SourceEntryID: in.SourceEntryID,
		EventType:     in.EventType,
		Resolution:    in.Resolution,
	}).Error
}
