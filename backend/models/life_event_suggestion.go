package models

import (
	"time"

	"mycorrhizal/contactmodel"
)

// Resolution outcomes for an inferred life-event suggestion
// (docs/adrs/0025-temporal-periods.md). Both are terminal for that candidate:
// a resolved (kind, entry, type) pair is never suggested again.
const (
	LifeEventSuggestionAccepted  = "accepted"
	LifeEventSuggestionDismissed = "dismissed"
)

// LifeEventSuggestion is one *candidate* event inferred from a contact's dated
// field period — not a stored entity. It is computed on read from the card's
// periods and the resolution memory; the user accepts (materializing a normal,
// independent LifeEvent) or dismisses it. Nothing is written when a suggestion
// is merely offered (docs/adrs/0025-temporal-periods.md).
type LifeEventSuggestion struct {
	// EntityID is the subject Contact.VCardUID.
	EntityID string `json:"entity_id"`
	// Type and Category are the inferred LifeEvent classification.
	Type     string `json:"type"`
	Category string `json:"category"`
	// Date is the anchor (the period's start), EndDate the period's end when it
	// has one.
	Date    *contactmodel.PartialDate `json:"date"`
	EndDate *contactmodel.PartialDate `json:"end_date,omitempty"`

	// SourceKind/SourceEntryID identify the Card entry the inference came from
	// (address|organization + the neutral element ID). They are the resolution
	// key, not a live link: accepting does not tie the event back to the entry.
	SourceKind    string `json:"source_kind"`
	SourceEntryID string `json:"source_entry_id"`
}

// LifeEventSuggestionResolutionInput is the request body for resolving a
// suggestion (accept or dismiss). It names the candidate by its inference key.
type LifeEventSuggestionResolutionInput struct {
	EntityID      string `json:"entity_id" validate:"required,uuid4"`
	SourceKind    string `json:"source_kind" validate:"required,oneof=address organization"`
	SourceEntryID string `json:"source_entry_id" validate:"required,max=255"`
	EventType     string `json:"event_type" validate:"required,max=100"`
	Resolution    string `json:"resolution" validate:"required,oneof=accepted dismissed"`
}

// LifeEventSuggestionResolution is the permanent memory of a user's decision
// about one inferred candidate — "dismiss once, never offer again" (and
// likewise for an accepted one whose event the user later edits away from the
// inferred type). Keyed by the natural (entity, source kind, source entry,
// event type) tuple.
//
// uint primary key, hard delete per T26: this is a join-shaped row whose
// identity IS its unique composite index, like DismissedHouseholdSuggestion —
// a decision is a permanent, deterministic fact, not user-authored content
// with an undo button.
type LifeEventSuggestionResolution struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`

	UserID uint `gorm:"not null;index;uniqueIndex:idx_life_event_suggestion_resolution,priority:1" json:"-"`

	// EntityID is the subject Contact.VCardUID (the same graph invariant every
	// EntityID-scoped table follows), so DeleteContact/DeleteUser can cascade
	// this table too.
	EntityID string `gorm:"column:entity_id;not null;index;uniqueIndex:idx_life_event_suggestion_resolution,priority:2" json:"entity_id"`

	// SourceKind is the Card collection the candidate came from
	// (address|organization).
	SourceKind string `gorm:"column:source_kind;not null;uniqueIndex:idx_life_event_suggestion_resolution,priority:3" json:"source_kind"`

	// SourceEntryID is the referenced Card element's neutral ID.
	SourceEntryID string `gorm:"column:source_entry_id;not null;uniqueIndex:idx_life_event_suggestion_resolution,priority:4" json:"source_entry_id"`

	// EventType is the inferred LifeEvent type token (e.g. moved, job_change).
	EventType string `gorm:"column:event_type;not null;uniqueIndex:idx_life_event_suggestion_resolution,priority:5" json:"event_type"`

	// Resolution is accepted|dismissed.
	Resolution string `gorm:"not null" json:"resolution"`
}
