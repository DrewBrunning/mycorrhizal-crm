package services

import (
	"errors"
	"sort"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// lifeEventSuggestionRules maps a Card period kind to the life-event candidate
// its *start* date can imply. Deliberately narrow (docs/adrs/0025-temporal-periods.md):
// only cases where the inferred event is unambiguous ship. The candidate's date
// is the period's start; its EndDate is the period's end when present.
//
// Why not more: an address could be a second home, a dorm, or a mailing
// address, so only the presence of a start date at the address an entry
// represents is treated as a move. An address END is a departure and is handled
// separately (see addressDepartureSuggestion) because "moved out of X" is a
// distinct event from "moved to X" — `moved` has no direction of its own.
// Titles and organizations both read as a job/affiliation change; when both
// carry the same anchor date only one candidate is offered (see
// suggestionAlreadyOffered), preferring the organization entry.
//
// Extending this table is a rule addition, not a schema change (issue #1233).
var lifeEventSuggestionRules = map[string]struct{ Type, Category string }{
	contactmodel.PeriodKindAddress: {
		Type:     models.LifeEventTypeMoved,
		Category: models.LifeEventCategoryHomeLiving,
	},
	contactmodel.PeriodKindOrganization: {
		Type:     models.LifeEventTypeJobChange,
		Category: models.LifeEventCategoryWorkEducation,
	},
	contactmodel.PeriodKindTitle: {
		Type:     models.LifeEventTypeJobChange,
		Category: models.LifeEventCategoryWorkEducation,
	},
}

// suggestionKindPriority orders period kinds when more than one can imply the
// same event, so a candidate is attributed to the most specific entry
// deterministically: an organization before a title (both imply job_change).
// Unknown kinds sort last and are skipped by the rule lookup anyway.
var suggestionKindPriority = map[string]int{
	contactmodel.PeriodKindAddress:      0,
	contactmodel.PeriodKindOrganization: 1,
	contactmodel.PeriodKindTitle:        2,
}

func suggestionKindRank(kind string) int {
	if rank, ok := suggestionKindPriority[kind]; ok {
		return rank
	}
	return len(suggestionKindPriority)
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

	// Deterministic, kind-prioritized order so a shared anchor date is
	// attributed to the organization rather than the title.
	periods := make([]contactmodel.EntryPeriod, len(contact.CRM.Periods))
	copy(periods, contact.CRM.Periods)
	sort.SliceStable(periods, func(i, j int) bool {
		return suggestionKindRank(periods[i].Kind) < suggestionKindRank(periods[j].Kind)
	})

	var out []models.LifeEventSuggestion
	for _, p := range periods {
		if p.EntryID == "" {
			continue
		}
		if rule, ok := lifeEventSuggestionRules[p.Kind]; ok && p.Range.Start != nil {
			out = offerSuggestion(out, resolved, events, models.LifeEventSuggestion{
				EntityID:      contact.VCardUID,
				Type:          rule.Type,
				Category:      rule.Category,
				Date:          p.Range.Start,
				EndDate:       p.Range.End,
				SourceKind:    p.Kind,
				SourceEntryID: p.EntryID,
			})
		}
		if p.Kind == contactmodel.PeriodKindAddress && p.Range.End != nil &&
			!hasAddressSuccessor(periods, p) {
			out = offerSuggestion(out, resolved, events, models.LifeEventSuggestion{
				EntityID:      contact.VCardUID,
				Type:          models.LifeEventTypeMovedOut,
				Category:      models.LifeEventCategoryHomeLiving,
				Date:          p.Range.End,
				SourceKind:    p.Kind,
				SourceEntryID: p.EntryID,
			})
		}
	}
	return out, nil
}

// offerSuggestion appends candidate unless it has already been resolved for
// this (kind, entry, type), an event of the same type already anchors on the
// same date, or an equivalent candidate was already offered in this pass. The
// last guard keeps an organization and a title sharing a start date from
// producing two identical job_change candidates.
func offerSuggestion(
	out []models.LifeEventSuggestion,
	resolved map[[2]string]bool,
	events []models.LifeEvent,
	candidate models.LifeEventSuggestion,
) []models.LifeEventSuggestion {
	if resolved[[2]string{candidate.SourceKind + "\x00" + candidate.SourceEntryID, candidate.Type}] {
		return out
	}
	if lifeEventAlreadyCovered(events, candidate.Type, candidate.Date) {
		return out
	}
	for i := range out {
		if out[i].Type == candidate.Type && equalPartialDate(out[i].Date, candidate.Date) {
			return out
		}
	}
	return append(out, candidate)
}

// hasAddressSuccessor reports whether another address period starts at or after
// this period's end — the "they moved somewhere else" signal that makes an
// address end a move rather than a departure. A period whose end cannot be
// compared to any other start (missing years) has no demonstrable successor, so
// the departure is offered. The same entry is never its own successor, and an
// overlapping period that begins before this one ends does not count.
func hasAddressSuccessor(periods []contactmodel.EntryPeriod, p contactmodel.EntryPeriod) bool {
	for i := range periods {
		q := periods[i]
		if q.Kind != contactmodel.PeriodKindAddress || q.EntryID == p.EntryID || q.Range.Start == nil {
			continue
		}
		if cmp, ok := contactmodel.ComparePartialDates(q.Range.Start, p.Range.End); ok && cmp >= 0 {
			return true
		}
	}
	return false
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
