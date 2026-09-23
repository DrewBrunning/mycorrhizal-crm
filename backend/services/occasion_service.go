package services

import (
	"fmt"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"slices"
	"time"

	"gorm.io/gorm"
)

// lifeEventHasMonthDay mirrors controllers.eventHasMonthDay
// (life_event_controller.go) exactly — duplicated rather than imported
// because services cannot depend on controllers (layering). A partial date
// needs both month and day, in valid calendar range, for an annual
// recurrence to exist (ADR 0015 Rule 3).
func lifeEventHasMonthDay(d *contactmodel.PartialDate) bool {
	if d == nil || d.Month == nil || d.Day == nil {
		return false
	}
	m := *d.Month
	day := *d.Day
	if m < 1 || m > 12 || day < 1 || day > 31 {
		return false
	}
	check := time.Date(2000, time.Month(m), day, 0, 0, 0, 0, time.UTC)
	return check.Month() == time.Month(m)
}

// contactDisplayName mirrors GetUpcomingBirthdays' own name-building logic
// (birthday_service.go) — nickname preferred over firstname when present.
func contactDisplayName(contact *models.Contact) string {
	name := contact.Firstname
	if contact.Nickname != "" {
		name = contact.Nickname
	}
	if contact.Lastname != "" {
		name += " " + contact.Lastname
	}
	return name
}

// resolveAnnualOccurrence returns the days-until and the resolved calendar
// date (YYYY-MM-DD, in now's zone) of the next annual month/day occurrence —
// reusing DaysUntilBirthday's proven, DST-safe, 29-Feb-advancing arithmetic
// (ADR 0015 Rules 3/6) via a synthetic year-less `--MM-DD` string, rather than
// reimplementing it. Returns ok=false for a malformed/out-of-range month/day
// (DaysUntilBirthday's 999 sentinel).
func resolveAnnualOccurrence(month, day int, now time.Time) (daysUntil int, date string, ok bool) {
	partial := fmt.Sprintf("--%02d-%02d", month, day)
	daysUntil = DaysUntilBirthday(partial, now)
	if daysUntil >= 999 {
		return 0, "", false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	occurrence := today.AddDate(0, 0, daysUntil)
	return daysUntil, occurrence.Format("2006-01-02"), true
}

// occasionDateMonthDay splits a Contact.Birthday/Anniversary flat scalar
// (YYYY-MM-DD or year-less --MM-DD, ADR 0015) into month/day ints. Mirrors
// DaysUntilBirthday's own end-of-string extraction (birthday_service.go) so
// this and that function agree on every legacy/malformed shape.
func occasionDateMonthDay(value string) (month, day int, ok bool) {
	if len(value) < 7 {
		return 0, 0, false
	}
	length := len(value)
	m, errM := time.Parse("01", value[length-5:length-3])
	d, errD := time.Parse("02", value[length-2:])
	if errM != nil || errD != nil {
		return 0, 0, false
	}
	return int(m.Month()), d.Day(), true
}

// GetUpcomingOccasions composes Contact.Birthday/Anniversary, LifeEvent rows
// with Remind=true and a resolvable annual date, and active
// OccasionObligation rows with an anchor date into one list of every
// occasion whose next occurrence falls within `days` of now — sorted
// ascending by days-until (ADR 0024, issue #387, ticket #1224).
//
// Unlike GetUpcomingBirthdays (a capped, top-5-within-dashboard-widget
// query), this returns every match in the window: the two serve different
// surfaces and are deliberately not unified.
//
// Sensitivity: OccasionObligation rows above `normal` sensitivity are
// excluded unless includeSensitive is true (the standard query-level filter
// every sensitivity-bearing entity uses). LifeEvent carries no Sensitivity
// field today, so life-event rows are not filtered here — a real gap this
// endpoint cannot close on its own.
func GetUpcomingOccasions(db *gorm.DB, userID uint, now time.Time, days int, includeSensitive bool) ([]models.UpcomingOccasion, error) {
	var out []models.UpcomingOccasion

	var contacts []models.Contact
	if err := db.Where("user_id = ? AND archived = ? AND (birthday IS NOT NULL AND birthday != '' OR anniversary IS NOT NULL AND anniversary != '')", userID, false).
		Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve contacts for upcoming occasions: %w", err)
	}
	for _, contact := range contacts {
		name := contactDisplayName(&contact)
		if contact.Birthday != "" {
			if month, day, ok := occasionDateMonthDay(contact.Birthday); ok {
				if daysUntil, date, ok := resolveAnnualOccurrence(month, day, now); ok && daysUntil <= days {
					out = append(out, models.UpcomingOccasion{
						ContactID: contact.ID, ContactName: name,
						Source: "birthday", Label: name,
						Date: date, DaysUntil: daysUntil,
					})
				}
			}
		}
		if contact.Anniversary != "" {
			if month, day, ok := occasionDateMonthDay(contact.Anniversary); ok {
				if daysUntil, date, ok := resolveAnnualOccurrence(month, day, now); ok && daysUntil <= days {
					out = append(out, models.UpcomingOccasion{
						ContactID: contact.ID, ContactName: name,
						Source: "anniversary", Label: name,
						Date: date, DaysUntil: daysUntil,
					})
				}
			}
		}
	}

	var events []models.LifeEvent
	if err := db.Where("user_id = ? AND remind = ?", userID, true).Find(&events).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve life events for upcoming occasions: %w", err)
	}
	if len(events) > 0 {
		contactByUID, err := contactsByVCardUID(db, userID, events, func(e models.LifeEvent) string { return e.EntityID })
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			if !lifeEventHasMonthDay(event.Date) {
				continue
			}
			contact, found := contactByUID[event.EntityID]
			if !found {
				continue
			}
			daysUntil, date, ok := resolveAnnualOccurrence(*event.Date.Month, *event.Date.Day, now)
			if !ok || daysUntil > days {
				continue
			}
			name := contactDisplayName(&contact)
			out = append(out, models.UpcomingOccasion{
				ContactID: contact.ID, ContactName: name,
				Source: "life_event", Label: event.Type,
				Date: date, DaysUntil: daysUntil,
			})
		}
	}

	var obligations []models.OccasionObligation
	obligationQuery := db.Where("user_id = ? AND active = ? AND anchor_month IS NOT NULL AND anchor_day IS NOT NULL", userID, true)
	if !includeSensitive {
		obligationQuery = obligationQuery.Where("sensitivity = ?", models.RelationshipSensitivityNormal)
	}
	if err := obligationQuery.Find(&obligations).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve occasion obligations for upcoming occasions: %w", err)
	}
	if len(obligations) > 0 {
		contactByUID, err := contactsByVCardUID(db, userID, obligations, func(o models.OccasionObligation) string { return o.EntityID })
		if err != nil {
			return nil, err
		}
		for _, obligation := range obligations {
			contact, found := contactByUID[obligation.EntityID]
			if !found {
				continue
			}
			daysUntil, date, ok := resolveAnnualOccurrence(*obligation.AnchorMonth, *obligation.AnchorDay, now)
			if !ok || daysUntil > days {
				continue
			}
			name := contactDisplayName(&contact)
			out = append(out, models.UpcomingOccasion{
				ContactID: contact.ID, ContactName: name,
				Source: "obligation", Label: obligation.Label,
				Date: date, DaysUntil: daysUntil, Kind: obligation.Kind,
			})
		}
	}

	slices.SortFunc(out, func(a, b models.UpcomingOccasion) int {
		if a.DaysUntil != b.DaysUntil {
			return a.DaysUntil - b.DaysUntil
		}
		return int(a.ContactID) - int(b.ContactID)
	})

	return out, nil
}

// contactsByVCardUID batch-loads every distinct owned contact referenced by
// entityIDFn across rows, avoiding an N+1 per-row lookup.
func contactsByVCardUID[T any](db *gorm.DB, userID uint, rows []T, entityIDFn func(T) string) (map[string]models.Contact, error) {
	seen := map[string]bool{}
	var uids []string
	for _, row := range rows {
		uid := entityIDFn(row)
		if uid != "" && !seen[uid] {
			seen[uid] = true
			uids = append(uids, uid)
		}
	}
	if len(uids) == 0 {
		return map[string]models.Contact{}, nil
	}
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("failed to batch-load contacts: %w", err)
	}
	out := make(map[string]models.Contact, len(contacts))
	for _, c := range contacts {
		out[c.VCardUID] = c
	}
	return out, nil
}

// giftMatchWindowDays bounds how far back a Gift record can be from "now"
// and still count as "this cycle's gift" for GetGiftShoppingList's status
// heuristic — roughly 11 months, so a gift from the last time this occasion
// came around (just under a year ago) doesn't count as "already handled" for
// the upcoming one. Documented as a heuristic, not exact: Gift carries no
// OccasionObligationID link (a future decision, out of scope here), so this
// is a best-effort date-proximity match, not a hard reference.
const giftMatchWindowDays = 330

// GetGiftShoppingList returns every active Kind="gift" OccasionObligation
// due within `days`, joined against Gift to report whether this cycle's
// gift already has a linked idea/purchase (ADR 0024, issue #387, ticket
// #1226). Status resolution: a Gift row for the same contact is treated as
// "this cycle's gift" when (a) the obligation names a LinkedLifeEventID and
// the Gift's own LifeEventID matches it, or (b) failing that, the Gift's
// Date (or CreatedAt for an undated idea) falls within the last
// giftMatchWindowDays of now. No match -> Status "needed".
func GetGiftShoppingList(db *gorm.DB, userID uint, now time.Time, days int, includeSensitive bool) ([]models.GiftShoppingItem, error) {
	query := db.Where("user_id = ? AND active = ? AND kind = ? AND anchor_month IS NOT NULL AND anchor_day IS NOT NULL",
		userID, true, models.OccasionObligationKindGift)
	if !includeSensitive {
		query = query.Where("sensitivity = ?", models.RelationshipSensitivityNormal)
	}
	var obligations []models.OccasionObligation
	if err := query.Find(&obligations).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve gift obligations: %w", err)
	}
	if len(obligations) == 0 {
		return []models.GiftShoppingItem{}, nil
	}

	contactByUID, err := contactsByVCardUID(db, userID, obligations, func(o models.OccasionObligation) string { return o.EntityID })
	if err != nil {
		return nil, err
	}

	var gifts []models.Gift
	if err := db.Where("user_id = ?", userID).Find(&gifts).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve gifts for shopping list: %w", err)
	}
	giftsByEntity := make(map[string][]models.Gift, len(gifts))
	for _, g := range gifts {
		giftsByEntity[g.EntityID] = append(giftsByEntity[g.EntityID], g)
	}

	windowStart := now.AddDate(0, 0, -giftMatchWindowDays)

	var out []models.GiftShoppingItem
	for _, obligation := range obligations {
		contact, found := contactByUID[obligation.EntityID]
		if !found {
			continue
		}
		daysUntil, date, ok := resolveAnnualOccurrence(*obligation.AnchorMonth, *obligation.AnchorDay, now)
		if !ok || daysUntil > days {
			continue
		}

		status := "needed"
		linkedGiftID := ""
		if matched, found := matchGiftForObligation(giftsByEntity[obligation.EntityID], obligation, windowStart, now); found {
			status = matched.Status
			linkedGiftID = matched.ID
		}

		out = append(out, models.GiftShoppingItem{
			ContactID: contact.ID, ContactName: contactDisplayName(&contact),
			ObligationID: obligation.ID, Label: obligation.Label,
			Date: date, DaysUntil: daysUntil,
			Status: status, LinkedGiftID: linkedGiftID,
		})
	}

	slices.SortFunc(out, func(a, b models.GiftShoppingItem) int {
		if a.DaysUntil != b.DaysUntil {
			return a.DaysUntil - b.DaysUntil
		}
		return int(a.ContactID) - int(b.ContactID)
	})

	return out, nil
}

// matchGiftForObligation applies GetGiftShoppingList's status heuristic
// (doc comment above) over one contact's gifts, preferring a
// LinkedLifeEventID match and falling back to the most recent gift inside
// the date window.
func matchGiftForObligation(candidates []models.Gift, obligation models.OccasionObligation, windowStart, now time.Time) (models.Gift, bool) {
	if obligation.LinkedLifeEventID != "" {
		for _, g := range candidates {
			if g.LifeEventID == obligation.LinkedLifeEventID {
				return g, true
			}
		}
	}

	var best models.Gift
	var bestTime time.Time
	found := false
	for _, g := range candidates {
		ref := g.CreatedAt
		if g.Date != nil {
			ref = *g.Date
		}
		if ref.Before(windowStart) || ref.After(now) {
			continue
		}
		if !found || ref.After(bestTime) {
			best, bestTime, found = g, ref, true
		}
	}
	return best, found
}
