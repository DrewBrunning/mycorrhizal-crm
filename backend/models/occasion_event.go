package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RSVP status values stored on OccasionEventAttendee.RSVP. These are a
// manually-recorded fact the user enters after asking a contact in whatever
// channel they actually used — never evidence of a delivered invitation
// (docs/adrs/0026-occasions-events.md part 2). Validated as a closed set, since
// (unlike OccasionObligation.Kind) a UI status control has no open-ended need.
const (
	OccasionEventRSVPPending  = "pending"
	OccasionEventRSVPAccepted = "accepted"
	OccasionEventRSVPDeclined = "declined"
	OccasionEventRSVPMaybe    = "maybe"
)

// OccasionEvent is a one-off event the user is hosting: "the summer BBQ",
// "Mum's 70th". It is a first-class, UUID-PK, soft-deleting entity following
// the OccasionObligation template (occasion_obligation.go) — user-authored
// content (ADR 0004), not a machine-owned calendar import mapping
// (CalendarEventLink) and not a standing annual rule (OccasionObligation);
// see docs/adrs/0026-occasions-events.md part 1 for the boundary.
//
// StartsAt/EndsAt are RFC 3339 instants (ADR 0015 category 1) — an event
// happens at a moment in time, not on a date-only occurrence. No
// Revision/ETag: no sync surface and no conditional-write requirement, so
// this stays outside ADR 0006/0008 (conditional-write-exempt) exactly like
// OccasionObligation.
type OccasionEvent struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"not null;index" json:"-"`

	Title    string     `gorm:"not null" json:"title" validate:"required,max=200"`
	StartsAt time.Time  `gorm:"column:starts_at;not null;index" json:"starts_at" validate:"required"`
	EndsAt   *time.Time `gorm:"column:ends_at" json:"ends_at,omitempty"`

	Location string `json:"location,omitempty" validate:"omitempty,max=500"`

	// Sensitivity reuses the cross-cutting normal/private/secret set. Nothing
	// in v1 exports or syncs an event, so this is a stored marker only
	// (docs/adrs/0026-occasions-events.md part 4) — it gates nothing today, and
	// would gate a future export rather than needing a new field then.
	Sensitivity string `gorm:"not null;default:normal;index" json:"sensitivity" validate:"required,oneof=normal private secret"`

	// Notes is free-text context, encrypted at rest — mirrors
	// OccasionObligation.Notes.
	Notes string `gorm:"serializer:encrypted" json:"notes,omitempty" validate:"omitempty,max=2000"`

	// Deleted is the T17 change-feed tombstone marker, set by the list handler
	// when it reads a row with Unscoped() that has a non-null deleted_at.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// TableName pins the plural table name explicitly (CLAUDE.md backend trap 1 —
// never trust GORM's pluralizer against the hand-written migration).
func (OccasionEvent) TableName() string {
	return "occasion_events"
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see the
// tombstone (see Note.AfterDelete's doc comment for the full rationale). Hard
// deletes and bulk deletes are skipped via the DeletedAt guard.
func (e *OccasionEvent) AfterDelete(tx *gorm.DB) error {
	if !e.DeletedAt.Valid {
		return nil
	}
	return tx.Model(&OccasionEvent{}).Unscoped().Where("id = ?", e.ID).UpdateColumn("updated_at", time.Now()).Error
}

// BeforeCreate generates a UUID for new OccasionEvents, mirroring
// OccasionObligation's own BeforeCreate.
func (e *OccasionEvent) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

// OccasionEventAttendee is the invitee/RSVP join row for one OccasionEvent —
// an edge-shaped row keyed by (event_id, entity_id), hard-deleted (ADR 0004;
// a soft-deleted row would block re-inviting the same contact). EntityID is a
// Contact.VCardUID, the graph invariant every subject-scoped row follows.
//
// RSVP is the user's manually-recorded status (OccasionEventRSVP* above).
type OccasionEventAttendee struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID  uint   `gorm:"not null;index" json:"-"`
	EventID string `gorm:"column:event_id;not null;index;uniqueIndex:idx_occasion_event_attendees_event_entity,priority:1" json:"event_id"`

	// Explicit column name and uuid validation, matching CircleMember's own
	// MemberVCardUID fix — GORM's namer would split a different case.
	EntityID string `gorm:"column:entity_id;not null;index;uniqueIndex:idx_occasion_event_attendees_event_entity,priority:2" json:"entity_id" validate:"required,uuid4"`

	RSVP string `gorm:"not null;default:pending" json:"rsvp" validate:"required,oneof=pending accepted declined maybe"`
}

// TableName pins the plural table name explicitly.
func (OccasionEventAttendee) TableName() string {
	return "occasion_event_attendees"
}

// BeforeCreate generates a UUID for new attendees, mirroring
// OccasionObligation's own BeforeCreate.
func (a *OccasionEventAttendee) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// OccasionEventAttendeeView is one attendee enriched with its contact's
// display data, for the event-detail response. ContactID is the numeric
// Contact.ID a client links to; EntityID is the Contact.VCardUID.
type OccasionEventAttendeeView struct {
	ID          string `json:"id"`
	EventID     string `json:"event_id"`
	EntityID    string `json:"entity_id"`
	ContactID   uint   `json:"contact_id"`
	ContactName string `json:"contact_name"`
	RSVP        string `json:"rsvp"`
}

// InviteeSuggestion is one candidate from GET /occasion-events/invitee-suggestions
// — a contact in one of the requested circles (docs/adrs/0026-occasions-events.md
// part 3). It is computed, never stored.
type InviteeSuggestion struct {
	ContactID   uint   `json:"contact_id"`
	ContactName string `json:"contact_name"`
	EntityID    string `json:"entity_id"`
}
