package models

import "time"

// ContactDetailUser is the "user" block of the M4 contact-detail composite:
// only what the detail screen needs from the current user, not the whole
// admin user object (docs/fork-plan/tickets/
// 83-M4-contact-detail-composite.md design decision 4).
type ContactDetailUser struct {
	EnabledContactFields []string `json:"enabled_contact_fields"`
}

// ContactDetailLifeEvent is a LifeEvent enriched with the display names of
// its RelatedEntityIDs (batch-resolved once per contact, never per event —
// design decision 3's "one batch query for related-contact names").
// RelatedEntityNames is keyed by Contact.VCardUID; an id with no resolvable
// contact (never happens today, but is not assumed) is simply absent from
// the map rather than erroring.
type ContactDetailLifeEvent struct {
	LifeEvent
	RelatedEntityNames map[string]string `json:"related_entity_names"`
}

// ImmichPersonSummary mirrors services.ImmichPersonSummary field-for-field
// (same JSON shape) — a models-local copy rather than an import of
// "mycorrhizal/services" here, the same import-cleanliness reasoning
// BriefingCadenceHealth documents in briefing.go.
type ImmichPersonSummary struct {
	Identity      ExternalIdentity `json:"identity"`
	PersonName    string           `json:"person_name"`
	PhotoCount    int              `json:"photo_count"`
	LatestAssetID string           `json:"latest_asset_id,omitempty"`
	LatestAt      *time.Time       `json:"latest_at,omitempty"`
}

// ContactDetailImmich is the composite's one legitimately-absent block
// (design decision 5): present only when the user has an Immich config at
// all (cheap one-row lookup), regardless of whether this particular contact
// is linked to an Immich person yet. Summary is null when no link exists.
type ContactDetailImmich struct {
	Summary *ImmichPersonSummary `json:"summary"`
}

// ContactDetailResponse is the M4 read-only composite of everything
// ContactDetailPage.tsx renders for one contact — the ~21-endpoint fan-out
// collapsed into one call, following the N2 briefing composite's pattern
// (briefing_controller.go) at a larger scale. No writes, no cache, no new
// tables; the profile picture stays a separate request by nature (a blob
// cannot sanely inline into this envelope).
//
// No omitempty on any collection block: every one of them must serialize as
// `[]` when empty, never `null`/absent (CLAUDE.md frontend trap 8 — the
// exact bug that broke the N2 prep view once already, called out again by
// this ticket's own traps section as "the single most likely regression").
// Immich is the sole exception — a genuinely-absent-not-empty block.
type ContactDetailResponse struct {
	Contact            ContactRecordResponse    `json:"contact"`
	User               ContactDetailUser        `json:"user"`
	Notes              []Note                   `json:"notes"`
	Activities         []Activity               `json:"activities"`
	Completions        []ReminderCompletion     `json:"completions"`
	Reminders          []Reminder               `json:"reminders"`
	RelationshipEdges  []BriefingRelationship   `json:"relationship_edges"`
	LifeEvents         []ContactDetailLifeEvent `json:"life_events"`
	Agenda             []ConversationAgenda     `json:"agenda"`
	Gifts              []Gift                   `json:"gifts"`
	FieldValues        []FieldValue             `json:"field_values"`
	ExternalIdentities []ExternalIdentity       `json:"external_identities"`
	ExternalActivities []ExternalActivity       `json:"external_activities"`
	// Circles/Tags are THIS contact's memberships, not the global per-user
	// list GET /circles and GET /tags return.
	Circles []Circle             `json:"circles"`
	Tags    []Tag                `json:"tags"`
	Immich  *ContactDetailImmich `json:"immich,omitempty"`
}
