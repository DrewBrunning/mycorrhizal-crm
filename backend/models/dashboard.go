package models

// DashboardReminder is one upcoming reminder enriched with its contact's
// display name (M3, M3
// design decision 2) so the dashboard composite's consumer never has to
// issue a second per-reminder contact fetch. Display name is
// nickname-preferred, falling back to firstname+lastname — the same rule
// DashboardPage.tsx's getContactName used client-side.
type DashboardReminder struct {
	Reminder
	ContactName string `json:"contact_name"`
}

// DashboardOverdueCadence mirrors services.OverdueCadence field-for-field
// (same JSON shape, so the frontend's existing OverdueCadence type needs no
// change) — kept as a models-local type rather than importing
// "mycorrhizal/services" here, the same import-cleanliness reasoning
// BriefingCadenceHealth documents in briefing.go. Health reuses
// BriefingCadenceHealth, itself already an exact mirror of
// services.CadenceHealth.
type DashboardOverdueCadence struct {
	Policy         CadencePolicy         `json:"policy"`
	Health         BriefingCadenceHealth `json:"health"`
	ContactID      uint                  `json:"contact_id"`
	ContactName    string                `json:"contact_name"`
	PhotoThumbnail string                `json:"photo_thumbnail,omitempty"`
}

// DashboardResponse is the M3 read-only composite of the four data blocks
// the dashboard ("today/overview") screen needs: upcoming birthdays, a
// handful of random contacts (the "stay in touch" nudge), upcoming
// reminders, and overdue cadences. Pure aggregation of existing per-user
// queries, no writes, no cache — the same shape of endpoint as the N2
// briefing composite (briefing_controller.go), just scoped to "what's due
// today" rather than one contact.
//
// No omitempty on any block: an empty dashboard must serialize every field
// as `[]`, never `null`/absent (CLAUDE.md frontend trap 8 — this exact bug
// broke the prep view once already).
type DashboardResponse struct {
	Birthdays         []Birthday                `json:"birthdays"`
	RandomContacts    []ContactResponse         `json:"random_contacts"`
	UpcomingReminders []DashboardReminder       `json:"upcoming_reminders"`
	Overdue           []DashboardOverdueCadence `json:"overdue"`
	// Favorites (issue #173): the user's favorite, non-archived contacts,
	// sorted by name — a quick-access block alongside the "stay in touch"
	// nudge. Same no-omitempty discipline as every other block.
	Favorites []ContactResponse `json:"favorites"`
	// ReachOutSuggestions (issue #177): pending event-driven reach-out
	// suggestions (org/title/address change detected). Same no-omitempty
	// discipline as every other block.
	ReachOutSuggestions []ReachOutSuggestionResponse `json:"reach_out_suggestions"`
}
