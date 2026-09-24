package models

// UpcomingOccasion is one row of the /occasions/upcoming aggregate (ADR 0024,
// issue #387, ticket #1224) — composing Contact.Birthday/Anniversary,
// LifeEvent rows with Remind=true, and active OccasionObligation rows into
// one sorted list.
type UpcomingOccasion struct {
	ContactID   uint   `json:"contact_id"`
	ContactName string `json:"contact_name"`
	// Source ∈ birthday|anniversary|life_event|obligation.
	Source string `json:"source"`
	Label  string `json:"label"`
	// Date is the resolved next-occurrence date, YYYY-MM-DD, in the reminder
	// zone (ADR 0015 Rule 4) — never year-aware, only month/day.
	Date      string `json:"date"`
	DaysUntil int    `json:"days_until"`
	// Kind carries OccasionObligation.Kind for source=obligation rows only
	// (card/gift/invite), letting a client filter the "card list" or "gift
	// shopping list" view out of this one aggregate.
	Kind string `json:"kind,omitempty"`
}
