package models

import "time"

// ContactBriefing is the read-only composition behind GET /contacts/:id/
// briefing (N2 — N2): everything the
// user wants to know about a person in the five minutes before seeing them,
// in one response. It is a pure aggregation of existing data — never
// persisted, never cached; every field degrades to its zero value when the
// source feature is empty or not yet built.
//
// Design rules (N2's traps):
//   - Only status:confirmed relationship edges are included; a suggested edge
//     is not fact (RelationshipEdge.Status's own doc comment).
//   - **The six slice blocks below must NOT carry `omitempty`, and the
//     controller must never leave them nil.** They serialize as `[]`, always.
//     They previously had `omitempty`, so a contact with no history returned
//     `{"contact_id":1,"uid":...,"name":...}` with every block *absent* rather
//     than empty — and PrepViewPage's `briefing.open_agenda_items.length`
//     crashed the whole page into the ErrorBoundary. That is the state every
//     freshly-created contact is in, so it broke the prep view on first use.
//     The frontend type (frontend/src/api/briefings.ts) declares these fields
//     required, which is the contract this struct now actually honours.
//   - Sensitivity (91.13): `secret` relationships are excluded in the query —
//     a secret relationship has no business on a screen likely to be open in
//     front of the person it concerns. `private` relationships stay: the
//     briefing is the user's own screen, and private gates sharing/exposure
//     (exports, sync, shared views), which this endpoint is not.
type ContactBriefing struct {
	ContactID uint   `json:"contact_id"`
	UID       string `json:"uid"`
	Name      string `json:"name"`
	// PhotoThumbnail is the avatar's data-URL/thumbnail as stored on the
	// Contact (same shape ContactsPage renders), empty when none.
	PhotoThumbnail string `json:"photo_thumbnail,omitempty"`
	// Kind is CRMEnvelope.Kind (human|animal), used for the header badge.
	Kind string `json:"kind,omitempty"`

	// LastActivity is the single most recent activity involving this contact
	// (the "what happened between us" anchor). Optional: a contact with no
	// activities has none, and the briefing must render fine without it.
	LastActivity *BriefingActivity `json:"last_activity,omitempty"`

	// RecentNotes are the contact's most recent notes, newest first, capped
	// (see BriefingNotesLimit). Notes are user-authored free text — part of
	// the "what was discussed" block alongside LastActivity.
	RecentNotes []Note `json:"recent_notes"`

	// CadenceHealth is T19's DERIVED relationship health (never stored), or
	// nil when the contact has no CadencePolicy. The briefing's "how overdue
	// is this relationship" block.
	Cadence *BriefingCadence `json:"cadence,omitempty"`

	// OpenAgendaItems are T21's not-yet-discussed conversation-agenda items
	// for this contact ("things to bring up"), newest first.
	OpenAgendaItems []ConversationAgenda `json:"open_agenda_items"`

	// Relationships are the confirmed edges involving this contact, each
	// resolved with the other party's display name + the label token as read
	// from this contact's perspective (the frontend reuses the i18n label for
	// the token). Sensitive edges (sensitivity above normal) excluded.
	Relationships []BriefingRelationship `json:"relationships"`

	// LifeEvents are this contact's life events, most recent first, capped.
	LifeEvents []LifeEvent `json:"life_events"`

	// UpcomingReminders are this contact's incomplete reminders due within the
	// briefing window, soonest first.
	UpcomingReminders []Reminder `json:"upcoming_reminders"`

	// UpcomingDates are the contact's upcoming calendar-ish dates (birthday,
	// anniversary), each with days-until. Derived from the flat
	// Birthday/Anniversary columns via services.DaysUntilBirthday.
	UpcomingDates []BriefingUpcomingDate `json:"upcoming_dates"`
}

// BriefingActivity is the slim projection of an Activity shown as the
// briefing's "last interaction" block — deliberately not the full Activity
// (no join-table contacts preload), just what the scan needs.
type BriefingActivity struct {
	ID          uint      `json:"id"`
	UUID        string    `json:"uuid,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Type        string    `json:"type,omitempty"`
	Location    string    `json:"location,omitempty"`
	Date        time.Time `json:"date"`
}

// BriefingCadence bundles a contact's CadencePolicy with its derived health.
// Health is a wire mirror of services.CadenceHealth (which this package cannot
// import — services imports models, so models must not import services). The
// controller copies the derived health fields across; the JSON shape is what
// the frontend's CadenceHealth mirrors.
type BriefingCadence struct {
	Policy CadencePolicy         `json:"policy"`
	Health BriefingCadenceHealth `json:"health"`
}

// BriefingCadenceHealth is the DERIVED relationship-health read-model (§91.10)
// as carried on the briefing — the same fields services.CadenceHealth
// computes, mirrored here to keep the models package import-clean. See
// services/cadence_service.go for the semantics: HasQualifyingInteraction is
// false until the contact has at least one qualifying interaction; OverdueBy
// is whole calendar days past due (0 when due today or later).
type BriefingCadenceHealth struct {
	HasQualifyingInteraction bool       `json:"has_qualifying_interaction"`
	LastInteraction          *time.Time `json:"last_interaction,omitempty"`
	NextDue                  *time.Time `json:"next_due,omitempty"`
	OverdueBy                int        `json:"overdue_by"`
}

// BriefingRelationship is one confirmed edge resolved for display: the raw
// edge plus the other party's name and the display label token as read from
// the viewed contact's perspective (see relationship_type_registry.go's
// InverseRelationType; the frontend already keys labels off these tokens).
type BriefingRelationship struct {
	Edge RelationshipEdge `json:"edge"`
	// OtherPartyContactID is the other contact's numeric ID, so the frontend
	// can link to /contacts/<id> (the detail route is numeric-ID-addressed,
	// like every other link in the app).
	OtherPartyContactID uint `json:"other_party_contact_id,omitempty"`
	// OtherPartyName is the other contact's display name (trimmed first +
	// last), or "" when the other endpoint is a thin/ghost contact with no
	// name on record.
	OtherPartyName string `json:"other_party_name,omitempty"`
	// OtherPartyUID is the other endpoint's VCardUID, the stable graph
	// identity (kept alongside the numeric ID for callers that need it).
	OtherPartyUID string `json:"other_party_uid,omitempty"`
	// DisplayToken is the relationship_type_registry token describing the
	// OTHER party from this contact's perspective (e.g. "parent_of" when the
	// stored edge is parent_of with this contact as target). The frontend
	// translates it via its existing relationships.types.* i18n labels.
	DisplayToken string `json:"display_token,omitempty"`
}

// BriefingUpcomingDate is one upcoming birthday/anniversary for the briefing.
type BriefingUpcomingDate struct {
	// Label is a stable token the frontend translates: "birthday" or
	// "anniversary".
	Label string `json:"label"`
	// Date is the stored YYYY-MM-DD or --MM-DD value.
	Date string `json:"date"`
	// DaysUntil is whole days from today; 0 when the date is today.
	DaysUntil int `json:"days_until"`
}

// Briefing collection limits — the briefing is a scannable-in-a-minute
// screen, not a list endpoint, so every slice is capped.
const (
	BriefingNotesLimit      = 5
	BriefingLifeEventsLimit = 5
	// BriefingReminderWindowDays is how far ahead "upcoming" reminders are
	// shown.
	BriefingReminderWindowDays = 30
)
